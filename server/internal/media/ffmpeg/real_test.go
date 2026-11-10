package ffmpeg

import (
	"bytes"
	"errors"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/container"
)

// These tests run a real ffmpeg when KIVO_FFMPEG names one. ffprobe is
// KIVO_FFPROBE, or the ffprobe beside it. They generate their corpus with
// that ffmpeg, so nothing large is committed.

// realTools returns the ffmpeg and ffprobe to test, or skips.
func realTools(t *testing.T) (ffmpegBin, ffprobeBin string) {
	t.Helper()
	ffmpegBin = os.Getenv("KIVO_FFMPEG")
	if ffmpegBin == "" {
		t.Skip("KIVO_FFMPEG not set")
	}
	ffprobeBin = os.Getenv("KIVO_FFPROBE")
	if ffprobeBin == "" {
		ffprobeBin = filepath.Join(filepath.Dir(ffmpegBin), "ffprobe")
	}
	return ffmpegBin, ffprobeBin
}

func realMedia(t *testing.T) *Runner {
	return New(realTools(t))
}

var corpus struct {
	once sync.Once
	dir  string
	err  error
}

// corpusKeyframes are where the corpus forces keyframes. They are
// irregular, with gaps both shorter and longer than a segment.
const corpusKeyframes = "0,2,5,7.5,12,13,19,24,27"

// corpusFiles are the corpus's files. They are H.264 and HEVC in MKV and
// MP4, each with stereo AAC and chapters. The HEVC is open-GOP, as x265
// encodes by default.
var corpusFiles = []string{"h264.mkv", "hevc.mkv", "h264.mp4", "hevc.mp4"}

// corpusDir makes the corpus once per test binary, in a directory that
// TestMain removes.
func corpusDir(t *testing.T) string {
	t.Helper()
	ffmpegBin, _ := realTools(t)
	corpus.once.Do(func() {
		if corpus.dir, corpus.err = os.MkdirTemp("", "kivo-media-corpus-"); corpus.err != nil {
			return
		}
		meta := filepath.Join(corpus.dir, "chapters.txt")
		corpus.err = os.WriteFile(meta, []byte(";FFMETADATA1\n"+
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=5000\ntitle=Intro\n"+
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=5000\nEND=27000\ntitle=Main\n"+
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=27000\nEND=30000\ntitle=Credits\n"), 0o644)
		encoders := map[string][]string{
			"h264": {"-c:v", "libx264", "-preset", "ultrafast", "-bf", "3", "-x264-params", "scenecut=0:keyint=300"},
			"hevc": {"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:scenecut=0:keyint=300"},
		}
		for _, name := range corpusFiles {
			if corpus.err != nil {
				return
			}
			codec, ext, _ := strings.Cut(name, ".")
			args := []string{"-hide_banner", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=30",
				"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=30",
				"-i", meta, "-map", "0:v", "-map", "1:a", "-map_chapters", "2", "-ac", "2"}
			args = append(args, encoders[codec]...)
			args = append(args, "-force_key_frames", corpusKeyframes, "-c:a", "aac")
			if ext == "mp4" {
				args = append(args, "-movflags", "+faststart")
				if codec == "hevc" {
					args = append(args, "-tag:v", "hvc1")
				}
			}
			out, err := exec.Command(ffmpegBin, append(args, filepath.Join(corpus.dir, name))...).CombinedOutput()
			if err != nil {
				corpus.err = errors.New(string(out))
			}
		}
	})
	if corpus.err != nil {
		t.Fatal("making corpus:", corpus.err)
	}
	return corpus.dir
}

// seekPast is where the transcode package seeks a copied job that begins
// at keyframe k of the corpus, whose keyframes are at least a second apart.
func seekPast(k time.Duration) time.Duration {
	if k == 0 {
		return 0
	}
	return k + 500*time.Millisecond
}

// ffprobeKeyframes lists the video keyframes ffprobe sees in path.
func ffprobeKeyframes(t *testing.T, path string) []time.Duration {
	t.Helper()
	_, ffprobeBin := realTools(t)
	out, err := exec.Command(ffprobeBin, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var keys []time.Duration
	for line := range strings.Lines(string(out)) {
		pts, flags, _ := strings.Cut(strings.TrimSpace(line), ",")
		if strings.Contains(flags, "K") {
			keys = append(keys, parseSeconds(pts))
		}
	}
	return keys
}

func TestRealProbe(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	for _, name := range corpusFiles {
		res, err := m.Probe(t.Context(), filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		// MP4 adds a third track holding the chapters.
		codec, _, _ := strings.Cut(name, ".")
		if len(res.Tracks) < 2 || res.Tracks[0].Codec != media.Codec(codec) || res.Tracks[0].Width != 320 ||
			res.Tracks[1].Codec != media.AAC || res.Tracks[1].Channels != 2 {
			t.Errorf("%s: tracks %+v", name, res.Tracks)
		}
		if len(res.Chapters) != 3 || res.Chapters[2].Title != "Credits" || res.Chapters[2].Start != 27*time.Second {
			t.Errorf("%s: chapters %+v", name, res.Chapters)
		}
		if res.Duration < 29*time.Second || res.Duration > 31*time.Second {
			t.Errorf("%s: duration %v", name, res.Duration)
		}
	}
}

func TestRealKeyframes(t *testing.T) {
	dir := corpusDir(t)
	for _, name := range corpusFiles {
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		keys, complete, err := container.Keyframes(bytes.NewReader(b), int64(len(b)))
		if err != nil || !complete {
			t.Fatalf("%s: complete %v, %v", name, complete, err)
		}
		want := ffprobeKeyframes(t, path)
		if len(keys) != len(want) {
			t.Fatalf("%s: %d keyframes, ffprobe sees %d", name, len(keys), len(want))
		}
		for i := range keys {
			if d := keys[i].Time - want[i]; d < -time.Millisecond || d > time.Millisecond {
				t.Errorf("%s: keyframe %d at %v, ffprobe says %v", name, i, keys[i].Time, want[i])
			}
		}
	}
}

func TestRealRemux(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	for _, name := range corpusFiles {
		path := filepath.Join(dir, name)
		keys := ffprobeKeyframes(t, path)
		for _, k := range []int{0, 3, 5} {
			start := keys[k]
			t.Run(name+"@"+start.String(), func(t *testing.T) {
				req := media.TranscodeRequest{Path: path, Start: start, Seek: seekPast(start), SegmentDuration: 6 * time.Second,
					Video: media.VideoRendition{Codec: media.Copy},
					Audio: []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: media.Copy}}}}
				init, segs := transcode(t, m, req)
				// One segment per keyframe, from start.
				checkSegments(t, init, segs, keys[k:])
				if strings.HasPrefix(name, "hevc") && (!bytes.Contains(init, []byte("hvc1")) || bytes.Contains(init, []byte("hev1"))) {
					t.Error("HEVC init segment isn't tagged hvc1")
				}
			})
		}
	}
}

func TestRealAudioTranscode(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	path := filepath.Join(dir, "hevc.mkv")
	keys := ffprobeKeyframes(t, path)
	req := media.TranscodeRequest{Path: path, Start: keys[2], Seek: seekPast(keys[2]), SegmentDuration: 6 * time.Second,
		Video: media.VideoRendition{Codec: media.Copy},
		Audio: []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: media.AAC, Channels: 1, Bitrate: 96000}}}}
	init, segs := transcode(t, m, req)
	checkSegments(t, init, segs, keys[2:])
	f, err := mp4.DecodeFile(bytes.NewReader(init))
	if err != nil {
		t.Fatal(err)
	}
	if a := f.Init.Moov.Traks[1].Mdia.Minf.Stbl.Stsd.Mp4a; a == nil || a.ChannelCount != 1 {
		t.Errorf("audio sample entry %+v, want mono AAC", a)
	}
}

// TestRealSurround encodes a 5.1 source's audio. It mixes down to stereo
// while keeping dialogue clear, and applies a dialogue boost to stereo and
// to 5.1.
func TestRealSurround(t *testing.T) {
	m := realMedia(t)
	ffmpegBin, _ := realTools(t)
	path := filepath.Join(t.TempDir(), "surround.mkv")
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24:duration=6",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=6",
		"-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-preset", "ultrafast",
		"-af", "aformat=channel_layouts=5.1", "-c:a", "aac", path).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	res, err := m.Probe(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tracks[1].Channels != 6 {
		t.Fatalf("source has %d channels, want 6", res.Tracks[1].Channels)
	}
	for _, tt := range []struct {
		name     string
		channels int
		boost    bool
	}{
		{"stereo", 2, false},
		{"stereo dialogue boost", 2, true},
		{"5.1 dialogue boost", 6, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := media.TranscodeRequest{Path: path, SegmentDuration: 6 * time.Second,
				Video: media.VideoRendition{Codec: media.Copy},
				Audio: []media.AudioOut{{Track: 1, Source: res.Tracks[1].Codec, SourceChannels: 6,
					Out: media.AudioRendition{Codec: media.AAC, Channels: tt.channels, Bitrate: 64000 * tt.channels,
						SampleRate: 48000, DialogueBoost: tt.boost}}}}
			init, segs := transcode(t, m, req)
			checkSegments(t, init, segs, []time.Duration{0})
			f, err := mp4.DecodeFile(bytes.NewReader(init))
			if err != nil {
				t.Fatal(err)
			}
			if a := f.Init.Moov.Traks[1].Mdia.Minf.Stbl.Stsd.Mp4a; a == nil || int(a.ChannelCount) != tt.channels {
				t.Errorf("audio sample entry %+v, want %d-channel AAC", a, tt.channels)
			}
		})
	}
}

// transcode runs req to the end and returns its output.
func transcode(t *testing.T, m *Runner, req media.TranscodeRequest) ([]byte, []media.Segment) {
	t.Helper()
	out, err := m.Transcode(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	init, err := out.Init()
	if err != nil {
		t.Fatal(err)
	}
	var segs []media.Segment
	for seg, err := range out.Segments() {
		if err != nil {
			t.Fatal(err, out.Diagnostics().Stderr)
		}
		segs = append(segs, seg)
	}
	if d := out.Diagnostics(); d.ExitCode != 0 {
		t.Errorf("exit code %d: %s", d.ExitCode, d.Stderr)
	}
	return init, segs
}

// checkSegments checks that segments start at the wanted keyframes, each on
// a sync sample. It checks that every track's decode times continue from
// one segment to the next without a gap. It also checks that the whole
// output decodes cleanly.
func checkSegments(t *testing.T, init []byte, segs []media.Segment, want []time.Duration) {
	t.Helper()
	if len(segs) != len(want) {
		t.Fatalf("got %d segments, want %d starting at %v", len(segs), len(want), want)
	}
	f, err := mp4.DecodeFile(bytes.NewReader(init))
	if err != nil {
		t.Fatal(err)
	}
	trex := map[uint32]*mp4.TrexBox{}
	for _, x := range f.Init.Moov.Mvex.Trexs {
		trex[x.TrackID] = x
	}
	next := map[uint32]uint64{} // each track's decode time after the last segment
	all := bytes.Clone(init)
	for i, seg := range segs {
		if d := seg.Start - want[i]; d < -time.Millisecond || d > time.Millisecond {
			t.Errorf("segment %d starts at %v, want %v", i, seg.Start, want[i])
		}
		all = append(all, seg.Data...)
		sf, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init), seg.Data...)))
		if err != nil {
			t.Fatal(err)
		}
		for _, frag := range sf.Segments[0].Fragments {
			for _, traf := range frag.Moof.Trafs {
				id, dts := traf.Tfhd.TrackID, traf.Tfdt.BaseMediaDecodeTime()
				if prev, ok := next[id]; ok && prev != dts {
					t.Errorf("segment %d: track %d decodes from %d, the last ended at %d", i, id, dts, prev)
				}
				var dur uint64
				for j, trun := range traf.Truns {
					dur += trun.AddSampleDefaultValues(traf.Tfhd, trex[id])
					if id == 1 && j == 0 && !mp4.IsSyncSampleFlags(trun.Samples[0].Flags) {
						t.Errorf("segment %d doesn't start on a keyframe", i)
					}
				}
				next[id] = dts + dur
			}
		}
	}
	decodes(t, all)
}

// decodes checks that ffmpeg decodes b without complaint.
func decodes(t *testing.T, b []byte) {
	t.Helper()
	ffmpegBin, _ := realTools(t)
	path := filepath.Join(t.TempDir(), "out.mp4")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	// Keep the decoder's time base. MKV's millisecond timestamps, rounded to
	// 1/24 s, would look non-monotonic.
	out, err := exec.Command(ffmpegBin, "-v", "error", "-i", path, "-enc_time_base", "-1", "-f", "null", "-").CombinedOutput()
	if err != nil || len(out) > 0 {
		t.Errorf("output doesn't decode cleanly: %v: %s", err, out)
	}
}

func TestRealImage(t *testing.T) {
	m := realMedia(t)
	ffmpegBin, ffprobeBin := realTools(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "poster.png")
	if out, err := exec.Command(ffmpegBin, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=1000x1500",
		"-frames:v", "1", src).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	widths := map[int]int{240: 240, 480: 480, 2000: 1000} // requested width to actual width, never upscaled
	var outs []media.ImageOut
	for w := range widths {
		outs = append(outs, media.ImageOut{Width: w, Path: filepath.Join(dir, strconv.Itoa(w)+".webp")})
	}
	tiny, err := m.Image(t.Context(), src, outs)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range outs {
		b, err := os.ReadFile(o.Path)
		if err != nil || len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
			t.Fatalf("%s isn't WebP: %v", o.Path, err)
		}
		out, err := exec.Command(ffprobeBin, "-v", "error", "-show_entries", "stream=width", "-of", "csv=p=0", o.Path).Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != strconv.Itoa(widths[o.Width]) {
			t.Errorf("asked for %d wide, got %s (%v)", o.Width, got, err)
		}
	}
	img, err := png.Decode(bytes.NewReader(tiny))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 48 {
		t.Errorf("tiny frame is %v", b)
	}
}

func TestRealErrors(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	garbage := filepath.Join(t.TempDir(), "garbage.mkv")
	if err := os.WriteFile(garbage, bytes.Repeat([]byte("not a video "), 300), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Probe(t.Context(), filepath.Join(dir, "missing.mkv")); !errors.Is(err, media.ErrSourceUnreadable) {
		t.Errorf("probe missing file: %v", err)
	}
	if _, err := m.Probe(t.Context(), garbage); !errors.Is(err, media.ErrSourceCorrupt) {
		t.Errorf("probe garbage: %v", err)
	}
	if _, err := m.Image(t.Context(), garbage, []media.ImageOut{{Width: 100, Path: garbage + ".webp"}}); !errors.Is(err, media.ErrSourceCorrupt) {
		t.Errorf("image of garbage: %v", err)
	}
	out, err := m.Transcode(t.Context(), media.TranscodeRequest{Path: filepath.Join(dir, "h264.mkv"), VideoTrack: 7,
		Video: media.VideoRendition{Codec: media.Copy}})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := out.Init(); !errors.Is(err, media.ErrUnsupported) {
		t.Errorf("remux of a missing track: %v", err)
	}
}
