package ffmpeg

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// Sample reads the request's stretch in one run of ffmpeg. The audio's
// fingerprint comes on stdout. Its silences and the keyframes' statistics
// come in the log. A failure that ffmpeg doesn't name is reported as
// unreadable, so the caller tries again.
func (r *Runner) Sample(ctx context.Context, req media.SampleRequest) (media.Sample, error) {
	op := "sample " + req.Path
	if req.Length <= 0 {
		return media.Sample{}, refuse(op, unsupported("an empty stretch"))
	}
	var dev *device
	if v := req.Video; v != nil {
		if v.Source.Width <= 0 || v.Source.Height <= 0 {
			return media.Sample{}, refuse(op, unsupported("the source's size is unknown"))
		}
		var err error
		if dev, err = r.device(v.Accelerator); err != nil {
			return media.Sample{}, refuse(op, err)
		}
	}
	args, g := sampleArgs(req, dev)
	log := newSampleLog(req.Start, g)
	out, err := run(ctx, op, media.ErrSourceUnreadable, r.ffmpeg, args, spawn{background: true, log: log.line})
	if err != nil {
		return media.Sample{}, err
	}
	return media.Sample{Points: points(out), Silences: log.silences, Keyframes: log.frames()}, nil
}

// Chromaprint reports whether ffmpeg writes raw Chromaprint fingerprints,
// which are needed to find skip markers by their audio. It asks ffmpeg
// once.
func (r *Runner) Chromaprint(ctx context.Context) bool {
	r.chromaprintOnce.Do(func() {
		out, err := run(ctx, "check chromaprint", media.ErrUnsupported, r.ffmpeg, []string{"-hide_banner", "-h", "muxer=chromaprint"}, spawn{})
		r.chromaprint = err == nil && bytes.Contains(out, []byte("fp_format")) && bytes.Contains(out, []byte("raw"))
	})
	return r.chromaprint
}

// chromaprintState holds Chromaprint's answer, which is computed once.
type chromaprintState struct {
	chromaprintOnce sync.Once
	chromaprint     bool
}

// sampleGraph names the video chain's measuring filters by their places in
// the chain. ffmpeg names their log lines the same way, such as
// "Parsed_blackframe_3".
type sampleGraph struct {
	black    [len(media.BlackLevels)]string // each blackframe's name
	metadata string
}

// sampleArgs reads the stretch once. The audio goes through silencedetect
// into the Chromaprint muxer on stdout. With req.Video, every keyframe goes
// through blackframe at each black level and through signalstats, printed
// to the log. When dev isn't the CPU, the device decodes and scales the
// keyframes.
func sampleArgs(req media.SampleRequest, dev *device) ([]string, sampleGraph) {
	args := []string{"-hide_banner", "-nostats", "-loglevel", "info", "-nostdin", "-threads", "1"}
	var g sampleGraph
	var chain []string
	if v := req.Video; v != nil {
		args = append(args, "-skip_frame", "nokey")
		chain, g = sampleChain(v.Source, dev)
		if dev.b != software {
			args = append(append(args, dev.b.init(dev.node)...), dev.b.decode...)
		}
	}
	args = append(args, "-ss", seconds(req.Start), "-t", seconds(req.Length))
	args = appendInput(args, req.Path)
	args = append(args, "-map", "0:"+strconv.Itoa(req.AudioTrack), "-vn", "-sn", "-dn",
		"-af", "silencedetect=noise=-50dB:d=0.5", "-ac", "2", "-f", "chromaprint", "-fp_format", "raw", "-")
	if v := req.Video; v != nil {
		args = append(args, "-map", "0:"+strconv.Itoa(v.Track), "-an", "-sn", "-dn",
			"-vf", strings.Join(chain, ","), "-f", "null", "-")
	}
	return args, g
}

// sampleChain returns the keyframes' video chain. It sets their color
// properties, crops the center, scales to media.SampleWidth and converts to
// 8-bit 4:2:0. Those steps run on dev when it's a device. The measuring
// then runs on the CPU.
func sampleChain(src media.Track, dev *device) ([]string, sampleGraph) {
	crop := "crop=trunc(iw*0.9/2)*2:trunc(ih*0.8/2)*2"
	chain := []string{colorParams(src, false)}
	if dev.b == software {
		chain = append(chain, crop, fmt.Sprintf("scale=%d:-2:flags=area", media.SampleWidth), "format=yuv420p")
	} else {
		// The frame is scaled whole on the device, so the crop keeps about
		// SampleWidth pixels.
		w, h := fit(src.Width, src.Height, media.SampleWidth*10/9, 0)
		chain = append(chain, dev.b.scale(w, h, true), "hwdownload", "format=nv12", crop, "format=yuv420p")
	}
	var g sampleGraph
	for i, level := range media.BlackLevels {
		g.black[i] = "Parsed_blackframe_" + strconv.Itoa(len(chain))
		chain = append(chain, "blackframe=amount=0:threshold="+strconv.Itoa(level))
	}
	chain = append(chain, "signalstats")
	g.metadata = "Parsed_metadata_" + strconv.Itoa(len(chain))
	return append(chain, "metadata=mode=print"), g
}

// points reads the Chromaprint muxer's raw output, which is little-endian
// 32-bit points. A trailing partial point is dropped.
func points(raw []byte) []uint32 {
	out := make([]uint32, 0, len(raw)/4)
	for ; len(raw) >= 4; raw = raw[4:] {
		out = append(out, binary.LittleEndian.Uint32(raw))
	}
	return out
}

// sampleLog collects a sample's silences and keyframe statistics from
// ffmpeg's log, a line at a time. The log's times are relative to the
// stretch, so start is added to put them on the file's timeline.
type sampleLog struct {
	start    time.Duration
	g        sampleGraph
	silences []media.Span
	open     time.Duration // the start of a silence not yet ended, or -1 for none
	stats    map[int]*frameLine
	current  int // the frame the last metadata header named
	order    []int
}

// frameLine is one keyframe's statistics, filled in as its lines arrive.
// seen counts the values set. A complete frame has all of them.
type frameLine struct {
	stats media.KeyframeStats
	black int
	seen  int
}

func newSampleLog(start time.Duration, g sampleGraph) *sampleLog {
	return &sampleLog{start: start, g: g, open: -1, stats: map[int]*frameLine{}, current: -1}
}

// statKeys lists the signalstats values a frame needs, with the field each
// one fills.
var statKeys = map[string]func(*media.KeyframeStats) *float32{
	"lavfi.signalstats.YMIN":   func(f *media.KeyframeStats) *float32 { return &f.YMin },
	"lavfi.signalstats.YLOW":   func(f *media.KeyframeStats) *float32 { return &f.YLow },
	"lavfi.signalstats.YHIGH":  func(f *media.KeyframeStats) *float32 { return &f.YHigh },
	"lavfi.signalstats.YMAX":   func(f *media.KeyframeStats) *float32 { return &f.YMax },
	"lavfi.signalstats.SATLOW": func(f *media.KeyframeStats) *float32 { return &f.SatLow },
	"lavfi.signalstats.SATAVG": func(f *media.KeyframeStats) *float32 { return &f.SatAvg },
}

// line reads one line of ffmpeg's log.
func (l *sampleLog) line(s string) {
	switch {
	case strings.Contains(s, "silence_start:"):
		if t, ok := secondsAfter(s, "silence_start:"); ok {
			l.open = max(0, t)
		}
	case strings.Contains(s, "silence_end:"):
		if t, ok := secondsAfter(s, "silence_end:"); ok && l.open >= 0 {
			l.silences = append(l.silences, media.Span{Start: l.start + l.open, End: l.start + t})
			l.open = -1
		}
	case l.g.metadata != "" && strings.Contains(s, "["+l.g.metadata+" "):
		l.metadataLine(s)
	default:
		for i, name := range l.g.black {
			if name != "" && strings.Contains(s, "["+name+" ") {
				l.blackLine(i, s)
			}
		}
	}
}

// metadataLine reads a metadata=print line. The line is either a frame's
// header, with its number and time, or one of the frame's values.
func (l *sampleLog) metadataLine(s string) {
	if n, ok := intAfter(s, "frame:"); ok && strings.Contains(s, "pts_time:") {
		t, _ := secondsAfter(s, "pts_time:")
		l.frame(n).stats.At = l.start + t
		l.current = n
		return
	}
	f := l.stats[l.current]
	if f == nil {
		return
	}
	_, kv, ok := strings.Cut(s, "] ")
	if !ok {
		return
	}
	key, value, ok := strings.Cut(strings.TrimSpace(kv), "=")
	field := statKeys[key]
	if !ok || field == nil {
		return
	}
	if v, err := strconv.ParseFloat(value, 32); err == nil {
		*field(&f.stats) = float32(v)
		f.seen++
	}
}

// blackLine reads a blackframe line, which gives a frame's share of pixels
// below one black level.
func (l *sampleLog) blackLine(level int, s string) {
	n, ok := intAfter(s, "frame:")
	if !ok {
		return
	}
	if p, ok := intAfter(s, "pblack:"); ok {
		l.frame(n).stats.Below[level] = uint8(min(max(p, 0), 100))
		l.frame(n).black++
	}
}

func (l *sampleLog) frame(n int) *frameLine {
	f := l.stats[n]
	if f == nil {
		f = &frameLine{}
		l.stats[n] = f
		l.order = append(l.order, n)
	}
	return f
}

// frames returns the keyframes that have every statistic, in order. A frame
// cut short when ffmpeg stopped is left out.
func (l *sampleLog) frames() []media.KeyframeStats {
	var out []media.KeyframeStats
	for _, n := range l.order {
		if f := l.stats[n]; f.black == len(media.BlackLevels) && f.seen == len(statKeys) {
			out = append(out, f.stats)
		}
	}
	return out
}

// secondsAfter reads the time in seconds after key in s.
func secondsAfter(s, key string) (time.Duration, bool) {
	v, ok := fieldAfter(s, key)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(f * float64(time.Second)), true
}

// intAfter reads the integer after key in s.
func intAfter(s, key string) (int, bool) {
	v, ok := fieldAfter(s, key)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

// fieldAfter returns the word after key in s, up to a space or a bar.
func fieldAfter(s, key string) (string, bool) {
	_, rest, ok := strings.Cut(s, key)
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if i := strings.IndexAny(rest, " |\t"); i >= 0 {
		rest = rest[:i]
	}
	return rest, rest != ""
}
