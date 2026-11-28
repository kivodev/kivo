package ffmpeg

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// DetectAccelerators finds the devices that might encode video. It
// test-encodes clips on each through the same pipeline and splitter that
// serve real plays. It keeps what passed, for Capabilities and Transcode,
// until it runs again. The clips are made once, in dir.
//
// All devices are tested at once, each on its own. First each encoder is
// tested with keyframes forced on the grid, and with a fixed GOP if the
// forced keyframes miss it. Then, with the first encoder that passed, the
// test tries decoding each source format on the device, and tone mapping
// and drawing subtitles there. The CPU is tested the same way, in software.
func (r *Runner) DetectAccelerators(ctx context.Context, dir string) media.Capabilities {
	began := time.Now()
	found, problems := r.findDevices()
	all := append(found, foundDevice{b: software, name: "CPU"})
	slices.SortStableFunc(all, func(a, b foundDevice) int { return rank[a.b.kind] - rank[b.b.kind] })

	have, err := r.makeClips(ctx, dir)
	if err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "media: accelerator test can't make its clips; video can't be transcoded", "err", err)
		}
		return media.Capabilities{}
	}
	results := make([]*device, len(all))
	errs := make([]error, len(all))
	var wg sync.WaitGroup
	for i, fd := range all {
		t := tester{r: r, dir: dir, have: have, fd: fd}
		wg.Go(func() { results[i], errs[i] = t.testDevice(ctx) })
	}
	wg.Wait()
	if ctx.Err() != nil {
		return media.Capabilities{}
	}

	hw := &accelerators{caps: media.Capabilities{Tested: true}, devices: map[media.Accelerator]*device{}}
	cpu := results[len(results)-1]
	for i, fd := range all {
		a := acceleratorOf(fd.b, fd.node)
		if results[i] == nil {
			if fd.b == software {
				slog.ErrorContext(ctx, "media: the CPU failed every test encode; video can't be transcoded", "err", errs[i])
				hw.caps.CPU = media.Caps{Backend: software.kind, Name: fd.name}
				continue
			}
			problems = append(problems, testProblem(fd, errs[i]))
			continue
		}
		hw.devices[a] = results[i]
		ac := passed(fd, results[i], cpu)
		if fd.b == software {
			hw.caps.CPU = ac
		} else {
			hw.caps.Accelerators = append(hw.caps.Accelerators, ac)
		}
		slog.InfoContext(ctx, "media: ready", describe(ac)...)
	}
	// When one accelerator works, another that can't be used needs no
	// action, so it is logged at Info, not Warn. It is often a
	// display-only card.
	level := slog.LevelWarn
	if len(hw.caps.Accelerators) > 0 {
		level = slog.LevelInfo
	}
	for _, p := range problems {
		slog.Log(ctx, level, "media: accelerator unusable", "accelerator", label(p.Accelerator), "name", p.Name, "reason", p.Reason, "err", p.Detail)
	}
	hw.caps.Problems = problems
	r.hw.Store(hw)
	slog.InfoContext(ctx, "media: accelerator test finished", "accelerators", len(hw.caps.Accelerators), "problems", len(problems),
		"duration", time.Since(began).Round(time.Millisecond))
	return hw.caps
}

// foundDevice is a device that was found, before its test.
type foundDevice struct {
	b    *backend
	node string
	name string
}

// rank orders devices best first. A discrete GPU comes first, then Quick
// Sync before plain VA-API on the same Intel GPU, and the CPU last.
var rank = map[media.Backend]int{media.NVENC: 0, media.QSV: 1, media.VAAPI: 2, media.VideoToolbox: 3, media.Software: 4}

// testClip is a clip the test makes once, in software.
type testClip struct {
	file  string
	track media.Track // the clip's track as Probe would describe it
	args  []string    // how to encode it
}

var (
	// decodeClips are the source formats a device may decode itself.
	// Encoders are tested on the first one.
	decodeClips = []testClip{
		{"h264.mkv", clipTrack(media.H264, 8, media.SDR), []string{"-pix_fmt", "yuv420p", "-c:v", "libx264", "-preset", "ultrafast", "-g", "1000"}},
		{"hevc.mkv", clipTrack(media.HEVC, 8, media.SDR), x265("yuv420p", "")},
		{"hevc10.mkv", clipTrack(media.HEVC, 10, media.SDR), x265("yuv420p10le", "")},
		{"av1-10.mkv", clipTrack("av1", 10, media.SDR), []string{"-pix_fmt", "yuv420p10le", "-c:v", "libsvtav1", "-preset", "12", "-g", "1000"}},
	}
	// hdrClip is an HDR10 clip for testing tone mapping.
	hdrClip = testClip{"hdr10.mkv", clipTrack(media.HEVC, 10, media.HDR10), append(
		x265("yuv420p10le", ":colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc"),
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")}
)

const (
	// testPattern is the picture every clip shows.
	testPattern = "testsrc2=size=1280x720:rate=24:duration=5"
	// subtitleClip is the text subtitle burned in.
	subtitleClip = "subtitle.srt"
)

func clipTrack(codec media.Codec, depth int, r media.DynamicRange) media.Track {
	return media.Track{Kind: media.Video, Codec: codec, BitDepth: depth, Width: 1280, Height: 720, FrameRate: 24, Range: r}
}

// x265 returns the arguments that encode HEVC at pixFmt with one keyframe,
// plus params.
func x265(pixFmt, params string) []string {
	return []string{"-pix_fmt", pixFmt, "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:keyint=1000" + params}
}

// makeClips makes the clips that dir lacks and reports which ones it has.
// A clip this ffmpeg can't make, such as AV1 without libsvtav1, is left out
// and its format goes untested. The clip the encoders are tested on is
// required.
func (r *Runner) makeClips(ctx context.Context, dir string) (map[string]bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	have := map[string]bool{}
	srt := filepath.Join(dir, subtitleClip)
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:01:00,000\nKivo\n"), 0o644); err == nil {
		have[subtitleClip] = true
	}
	for _, c := range append(slices.Clone(decodeClips), hdrClip) {
		path := filepath.Join(dir, c.file)
		if _, err := os.Stat(path); err == nil {
			have[c.file] = true
			continue
		}
		if err := r.makeClip(ctx, c, path); err != nil {
			if c.file == decodeClips[0].file {
				return nil, err
			}
			slog.DebugContext(ctx, "media: accelerator test clip not made", "file", c.file, "err", err)
			continue
		}
		have[c.file] = true
	}
	return have, nil
}

// makeClip encodes c to path. The file appears at path only once it's
// complete.
func (r *Runner) makeClip(ctx context.Context, c testClip, path string) error {
	tmp := path + ".tmp"
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "lavfi", "-i", testPattern}, c.args...)
	if _, err := run(ctx, "make test clip "+c.file, media.ErrUnsupported, r.ffmpeg, append(args, "-f", "matroska", tmp), spawn{}); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// tester runs one device's test encodes on the clips in dir.
type tester struct {
	r    *Runner
	dir  string
	have map[string]bool // the clips made
	fd   foundDevice
}

// testEncode is one test encode of clip to codec on dev. dev is set up as
// the device would be if this encode passes.
type testEncode struct {
	what    string // a description for the log
	dev     *device
	clip    testClip
	codec   media.Codec
	toneMap bool // tone-map the clip to SDR
	burnIn  bool // draw the subtitle clip over it
}

const (
	testStep    = time.Second // the test's segment grid
	testFrom    = time.Second // where each test encode starts, which is on the grid
	testLength  = 5 * time.Second
	testTimeout = 30 * time.Second // per encode, so a hung driver fails its step
	// testSlack is how far from its grid point a segment may start. It is
	// under half a frame at the clips' 24 fps.
	testSlack = 20 * time.Millisecond
)

// testDevice tests the device. When no encoder passed, it returns nil and
// the reason.
func (t tester) testDevice(ctx context.Context) (*device, error) {
	dev := &device{b: t.fd.b, node: t.fd.node, kernel: t.r.kernel, encoders: map[media.Codec]media.GOPMode{}, decoders: map[media.VideoFormat]bool{}}
	base := decodeClips[0]
	var failure error
	for _, codec := range []media.Codec{media.H264, media.HEVC} {
		for _, mode := range []media.GOPMode{media.GridGOP, media.FixedGOP} {
			trial := dev.clone()
			trial.encoders = map[media.Codec]media.GOPMode{codec: mode}
			onGrid, err := t.run(ctx, testEncode{what: fmt.Sprintf("encode %s, %s GOP", codec, mode), dev: trial, clip: base, codec: codec})
			if err != nil {
				failure = cmp.Or(failure, err)
				break
			}
			if onGrid {
				dev.encoders[codec] = mode
				break
			}
			if mode == media.FixedGOP {
				failure = cmp.Or(failure, fmt.Errorf("%s keyframes miss the segment grid", codec))
			}
		}
	}
	if len(dev.encoders) == 0 {
		return nil, failure
	}

	codec := media.H264
	if _, ok := dev.encoders[codec]; !ok {
		codec = media.HEVC
	}
	cpu := t.fd.b == software
	if !cpu {
		for _, clip := range decodeClips {
			f := format(clip.track)
			trial := dev.clone()
			trial.decoders = map[media.VideoFormat]bool{f: true}
			if t.passes(ctx, testEncode{what: "decode " + f.String(), dev: trial, clip: clip, codec: codec}) {
				dev.decoders[f] = true
			}
		}
	}
	if cpu || dev.decoders[format(hdrClip.track)] {
		trial := dev.clone()
		trial.toneMap = true
		dev.toneMap = t.passes(ctx, testEncode{what: "tone-map", dev: trial, clip: hdrClip, codec: codec, toneMap: true})
	}
	if cpu || dev.decoders[format(base.track)] {
		trial := dev.clone()
		trial.burnIn = true
		dev.burnIn = t.passes(ctx, testEncode{what: "burn in subtitles", dev: trial, clip: base, codec: codec, burnIn: true})
	}
	return dev, nil
}

// passes reports whether e ran with its segments on the grid. An encode
// whose clips weren't made doesn't pass.
func (t tester) passes(ctx context.Context, e testEncode) bool {
	if !t.have[e.clip.file] || e.burnIn && !t.have[subtitleClip] {
		return false
	}
	onGrid, err := t.run(ctx, e)
	return err == nil && onGrid
}

// run runs e and reports whether its segments fell on the grid.
func (t tester) run(ctx context.Context, e testEncode) (bool, error) {
	req := media.TranscodeRequest{Path: filepath.Join(t.dir, e.clip.file), Start: testFrom, SegmentDuration: testStep,
		Source: e.clip.track, Accelerator: acceleratorOf(t.fd.b, t.fd.node),
		Video: media.VideoRendition{Codec: e.codec, Width: 640, Height: 360, Bitrate: 2_000_000}}
	if e.toneMap {
		req.Video.HDR = media.ToneMap
	}
	if e.burnIn {
		req.BurnIn = &media.SubtitleRef{Path: filepath.Join(t.dir, subtitleClip)}
	}
	began := time.Now()
	onGrid, err := t.r.onGrid(ctx, req, e.dev)
	attrs := []any{"accelerator", label(req.Accelerator), "step", e.what, "duration", time.Since(began).Round(time.Millisecond)}
	var me *media.Error
	switch {
	case err == nil:
		slog.DebugContext(ctx, "media: accelerator test step passed", append(attrs, "grid", onGrid)...)
	case errors.As(err, &me):
		slog.DebugContext(ctx, "media: accelerator test step failed", append(attrs, "err", err,
			"command", me.Diagnostics.Command, "stderr", me.Diagnostics.Stderr)...)
	default:
		slog.DebugContext(ctx, "media: accelerator test step failed", append(attrs, "err", err)...)
	}
	return onGrid, err
}

// onGrid runs req on dev and reports whether a segment started at each grid
// point and nowhere else.
func (r *Runner) onGrid(ctx context.Context, req media.TranscodeRequest, dev *device) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	out, err := r.transcode(ctx, req, dev)
	if err != nil {
		return false, err
	}
	defer out.Close()
	var starts []time.Duration
	for seg, err := range out.Segments() {
		if err != nil {
			return false, err
		}
		starts = append(starts, seg.Start)
	}
	if len(starts) != int((testLength-testFrom)/testStep) {
		return false, nil
	}
	for i, s := range starts {
		if d := s - testFrom - time.Duration(i)*testStep; d < -testSlack || d > testSlack {
			return false, nil
		}
	}
	return true, nil
}

func (d *device) clone() *device {
	c := *d
	c.encoders = maps.Clone(d.encoders)
	c.decoders = maps.Clone(d.decoders)
	return &c
}

// passed reports what dev passed. Tone mapping and subtitles that didn't
// pass on an accelerator run on the CPU instead, if they passed there.
func passed(fd foundDevice, dev, cpu *device) media.Caps {
	dc := media.Caps{Accelerator: acceleratorOf(fd.b, fd.node), Backend: fd.b.kind, Name: fd.name}
	for _, codec := range []media.Codec{media.H264, media.HEVC} {
		if mode, ok := dev.encoders[codec]; ok {
			dc.Encoders = append(dc.Encoders, media.EncoderCaps{Codec: codec, GOP: mode})
		}
	}
	for _, clip := range decodeClips {
		if f := format(clip.track); dev.decoders[f] {
			dc.Decoders = append(dc.Decoders, f)
		}
	}
	placement := func(passed, cpuPassed bool) media.Placement {
		switch {
		case passed && fd.b == software:
			return media.OnCPU
		case passed:
			return media.OnAccelerator
		case cpuPassed:
			return media.OnCPU
		}
		return ""
	}
	cpuToneMap, cpuBurnIn := cpu != nil && cpu.toneMap, cpu != nil && cpu.burnIn
	dc.ToneMap = placement(dev.toneMap, cpuToneMap)
	dc.BurnIn = placement(dev.burnIn, cpuBurnIn)
	return dc
}

// testProblem describes an accelerator on which no encoder passed.
func testProblem(fd foundDevice, err error) media.AcceleratorProblem {
	p := media.AcceleratorProblem{Accelerator: acceleratorOf(fd.b, fd.node), Backend: fd.b.kind, Name: fd.name, Reason: media.TestFailed}
	if err == nil {
		return p
	}
	p.Detail = err.Error()
	var me *media.Error
	if errors.As(err, &me) {
		p.Detail = cmp.Or(me.Detail, p.Detail)
		for _, s := range driverMissing {
			if strings.Contains(me.Diagnostics.Stderr, s) {
				p.Reason = media.NoDriver
			}
		}
	}
	return p
}

// driverMissing matches error output saying a device's driver isn't
// reachable.
var driverMissing = []string{"Cannot load libcuda", "Cannot load libnvidia-encode", "va_openDriver() returns",
	"Failed to initialise VAAPI connection", "No VA display found"}

// describe returns the log attributes of an accelerator or the CPU.
func describe(dc media.Caps) []any {
	var enc, dec []string
	for _, e := range dc.Encoders {
		enc = append(enc, fmt.Sprintf("%s (%s GOP)", e.Codec, e.GOP))
	}
	for _, f := range dc.Decoders {
		dec = append(dec, f.String())
	}
	attrs := []any{"name", dc.Name, "encoders", strings.Join(enc, ", ")}
	if dc.Backend != media.Software {
		attrs = append(attrs, "accelerator", dc.Accelerator, "decoders", cmp.Or(strings.Join(dec, ", "), "none"))
	}
	return append(attrs, "tonemap", cmp.Or(string(dc.ToneMap), "none"), "burnin", cmp.Or(string(dc.BurnIn), "none"))
}

// label returns the name of an accelerator, or the CPU, for logs.
func label(d media.Accelerator) string {
	return cmp.Or(string(d), "cpu")
}
