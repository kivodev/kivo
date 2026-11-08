package ffmpeg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/mmcdole/kivo/internal/media"
)

// Video transcodes with a real ffmpeg, in software. CI's system ffmpeg has
// no GPU and lacks the bundled build's filters, so tests that need them
// skip.

// hdr10Encode encodes 10-bit HEVC tagged as HDR10, with one keyframe. The
// frames are tagged too, because the encoder takes its colors from them,
// and a lavfi source's are unknown.
var hdr10Encode = []string{"-vf", "setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc", "-pix_fmt", "yuv420p10le", "-c:v", "libx265", "-preset", "ultrafast",
	"-x265-params", "log-level=error:keyint=1000:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
	"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc"}

// grid lists the grid points from start to before end.
func grid(start, step, end time.Duration) []time.Duration {
	var out []time.Duration
	for t := start; t < end; t += step {
		out = append(out, t)
	}
	return out
}

// videoTrack probes path and returns its first track, which is the video.
func videoTrack(t *testing.T, m *Runner, path string) media.Track {
	t.Helper()
	res, err := m.Probe(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	return res.Tracks[0]
}

// pictureSize reads the video sample entry's size from an init segment.
func pictureSize(t *testing.T, init []byte) (int, int) {
	t.Helper()
	f, err := mp4.DecodeFile(bytes.NewReader(init))
	if err != nil {
		t.Fatal(err)
	}
	stsd := f.Init.Moov.Traks[0].Mdia.Minf.Stbl.Stsd
	switch {
	case stsd.AvcX != nil:
		return int(stsd.AvcX.Width), int(stsd.AvcX.Height)
	case stsd.HvcX != nil:
		return int(stsd.HvcX.Width), int(stsd.HvcX.Height)
	}
	t.Fatal("no video sample entry")
	return 0, 0
}

// hasFilter reports whether ffmpeg has the named filter.
func hasFilter(t *testing.T, name string) bool {
	t.Helper()
	ffmpegBin, _ := realTools(t)
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-filters").Output()
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(out)) {
		if f := strings.Fields(line); len(f) > 1 && f[1] == name {
			return true
		}
	}
	return false
}

func TestRealVideoTranscode(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	for _, name := range []string{"h264.mkv", "hevc.mp4"} {
		path := filepath.Join(dir, name)
		src := videoTrack(t, m, path)
		for _, codec := range []media.Codec{media.H264, media.HEVC} {
			for _, start := range []time.Duration{0, 12 * time.Second} {
				t.Run(fmt.Sprintf("%s to %s@%v", name, codec, start), func(t *testing.T) {
					req := media.TranscodeRequest{Path: path, Start: start, SegmentDuration: 6 * time.Second, Source: src,
						Video: media.VideoRendition{Codec: codec, Width: 160, Height: 120, Bitrate: 300_000},
						Audio: []media.AudioOut{{Track: 1, Out: media.AudioRendition{Codec: media.AAC, Channels: 2}}}}
					init, segs := transcode(t, m, req)
					// The source's keyframes are irregular. The output's are
					// on the grid, so each segment is one grid step.
					checkSegments(t, init, segs, grid(start, 6*time.Second, 30*time.Second))
					if w, h := pictureSize(t, init); w != 160 || h != 90 {
						t.Errorf("picture is %d×%d, want 160×90", w, h)
					}
					if codec == media.HEVC && !bytes.Contains(init, []byte("hvc1")) {
						t.Error("HEVC init segment isn't tagged hvc1")
					}
				})
			}
		}
	}
}

func TestRealVideoFrameRateCap(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	path := filepath.Join(dir, "h264.mkv")
	req := media.TranscodeRequest{Path: path, Start: 6 * time.Second, SegmentDuration: 4 * time.Second,
		Source: videoTrack(t, m, path), Video: media.VideoRendition{Codec: media.H264, FrameRate: 10, Bitrate: 300_000}}
	init, segs := transcode(t, m, req)
	checkSegments(t, init, segs, grid(6*time.Second, 4*time.Second, 30*time.Second))
	f, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init), segs[0].Data...)))
	if err != nil {
		t.Fatal(err)
	}
	if n := f.Segments[0].Fragments[0].Moof.Traf.Trun.SampleCount(); n != 40 {
		t.Errorf("the first 4 seconds have %d frames, want 40", n)
	}
}

func TestRealToneMap(t *testing.T) {
	ffmpegBin, _ := realTools(t)
	if !hasFilter(t, "tonemapx") {
		t.Skip("this ffmpeg has no tonemapx: it isn't jellyfin-ffmpeg")
	}
	m := realMedia(t)
	path := filepath.Join(t.TempDir(), "hdr10.mkv")
	args := append([]string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=8"},
		append(hdr10Encode, path)...)
	if out, err := exec.Command(ffmpegBin, args...).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	src := videoTrack(t, m, path)
	if src.Range != media.HDR10 {
		t.Fatalf("source range %q, want HDR10", src.Range)
	}
	req := media.TranscodeRequest{Path: path, Start: 2 * time.Second, SegmentDuration: 2 * time.Second, Source: src,
		Video: media.VideoRendition{Codec: media.H264, Bitrate: 1_000_000, HDR: media.ToneMap}}
	init, segs := transcode(t, m, req)
	checkSegments(t, init, segs, grid(2*time.Second, 2*time.Second, 8*time.Second))
}

// TestRealYCgCoSeek transcodes 10-bit H.264 tagged with the YCgCo matrix,
// which swscale can't convert, from the start and from a later segment.
// colorParams retags its frames as BT.709 so a conversion after it works.
// A start past zero adds -ss, so ffmpeg puts a trim filter in front of the
// graph, and the conversion to 8 bits must not land before colorParams.
func TestRealYCgCoSeek(t *testing.T) {
	ffmpegBin, _ := realTools(t)
	m := realMedia(t)
	path := filepath.Join(t.TempDir(), "ycgco.mkv")
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=8",
		"-pix_fmt", "yuv420p10le", "-c:v", "libx264", "-preset", "ultrafast",
		"-x264-params", "keyint=48:colormatrix=YCgCo", path).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	src := videoTrack(t, m, path)
	for _, start := range []time.Duration{0, 2 * time.Second} {
		t.Run(fmt.Sprintf("from %v", start), func(t *testing.T) {
			req := media.TranscodeRequest{Path: path, Start: start, SegmentDuration: 2 * time.Second, Source: src,
				Video: media.VideoRendition{Codec: media.H264, Bitrate: 500_000}}
			init, segs := transcode(t, m, req)
			checkSegments(t, init, segs, grid(start, 2*time.Second, 8*time.Second))
		})
	}
}

func TestRealBurnInText(t *testing.T) {
	if !hasFilter(t, "subtitles") {
		t.Skip("this ffmpeg has no libass")
	}
	m, dir := realMedia(t), corpusDir(t)
	subs := filepath.Join(t.TempDir(), escapedName)
	if err := os.WriteFile(subs, []byte("1\n00:00:00,000 --> 00:00:30,000\nKivo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hevc.mkv")
	req := media.TranscodeRequest{Path: path, Start: 6 * time.Second, SegmentDuration: 6 * time.Second,
		Source: videoTrack(t, m, path), Video: media.VideoRendition{Codec: media.H264, Bitrate: 500_000},
		BurnIn: &media.SubtitleRef{Path: subs}}
	init, segs := transcode(t, m, req)
	checkSegments(t, init, segs, grid(6*time.Second, 6*time.Second, 30*time.Second))
}

// TestRealInterlaced transcodes interlaced H.264 from its start and from a
// later segment, and checks that the output is progressive.
func TestRealInterlaced(t *testing.T) {
	ffmpegBin, _ := realTools(t)
	m := realMedia(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "interlaced.mkv")
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=50:duration=8", "-vf", "tinterlace=4",
		"-c:v", "libx264", "-preset", "ultrafast", "-flags", "+ilme+ildct", "-x264-params", "keyint=50", path).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	src := videoTrack(t, m, path)
	if !src.Interlaced {
		t.Fatal("the source doesn't probe as interlaced")
	}
	for _, start := range []time.Duration{0, 2 * time.Second} {
		t.Run(fmt.Sprintf("from %v", start), func(t *testing.T) {
			req := media.TranscodeRequest{Path: path, Start: start, SegmentDuration: 2 * time.Second, Source: src,
				Video: media.VideoRendition{Codec: media.H264, Bitrate: 500_000}}
			init, segs := transcode(t, m, req)
			checkSegments(t, init, segs, grid(start, 2*time.Second, 8*time.Second))
			b := bytes.Clone(init)
			for _, s := range segs {
				b = append(b, s.Data...)
			}
			mp4Path := filepath.Join(dir, "out.mp4")
			if err := os.WriteFile(mp4Path, b, 0o644); err != nil {
				t.Fatal(err)
			}
			if inter, prog := combing(t, mp4Path); prog < 4*inter {
				t.Errorf("idet finds %d interlaced and %d progressive frames, want nearly all progressive", inter, prog)
			}
		})
	}
}

// combing counts path's frames that look interlaced and progressive to
// ffmpeg's idet filter, which judges the picture rather than its flags.
func combing(t *testing.T, path string) (interlaced, progressive int) {
	t.Helper()
	ffmpegBin, _ := realTools(t)
	out, err := exec.Command(ffmpegBin, "-hide_banner", "-i", path, "-vf", "idet", "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	var tff, bff int
	for line := range strings.Lines(string(out)) {
		if _, stats, ok := strings.Cut(line, "Multi frame detection:"); ok {
			if _, err := fmt.Sscanf(strings.Join(strings.Fields(stats), " "), "TFF: %d BFF: %d Progressive: %d", &tff, &bff, &progressive); err != nil {
				t.Fatal(err, line)
			}
		}
	}
	return tff + bff, progressive
}
