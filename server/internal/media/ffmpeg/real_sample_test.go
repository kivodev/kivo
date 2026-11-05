package ffmpeg

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// TestRealSample reads a stretch of a file whose picture is a test pattern
// followed by white boxes on black. Its audio is a tone, two seconds of
// silence, and the tone again.
func TestRealSample(t *testing.T) {
	m := realMedia(t)
	ffmpegBin, _ := realTools(t)
	if !New(ffmpegBin, "").Chromaprint(t.Context()) {
		t.Skip("this ffmpeg writes no raw Chromaprint fingerprints")
	}
	src := filepath.Join(t.TempDir(), "sample.mkv")
	out, err := exec.Command(ffmpegBin, "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10:duration=20",
		"-f", "lavfi", "-i", "color=black:size=320x180:rate=10:duration=20,drawbox=x=100:y=80:w=120:h=12:color=white:t=fill",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=15",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono:d=2",
		"-f", "lavfi", "-i", "sine=frequency=660:duration=23",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v];[2:a][3:a][4:a]concat=n=3:v=0:a=1[a]",
		"-map", "[v]", "-map", "[a]", "-c:v", "libx264", "-preset", "ultrafast", "-g", "20", "-c:a", "aac", src).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}

	s, err := m.Sample(t.Context(), media.SampleRequest{
		Path: src, AudioTrack: 1, Start: 10 * time.Second, Length: 30 * time.Second,
		Video: &media.SampleVideo{Track: 0, Source: media.Track{Kind: media.Video, Width: 320, Height: 180, BitDepth: 8}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := int(30 * time.Second / media.PointStep); len(s.Points) < want-30 || len(s.Points) > want {
		t.Errorf("%d points, want about %d", len(s.Points), want)
	}
	if len(s.Silences) != 1 || s.Silences[0].Start < 14800*time.Millisecond || s.Silences[0].End > 17200*time.Millisecond {
		t.Errorf("silences %v, want one from 15s to 17s", s.Silences)
	}
	if len(s.Keyframes) < 14 || len(s.Keyframes) > 16 {
		t.Fatalf("%d keyframes, want 15 (one every 2s over 30s)", len(s.Keyframes))
	}
	if f := s.Keyframes[0]; f.At < 10*time.Second || f.At > 11*time.Second || f.Below[2] > 50 {
		t.Errorf("first keyframe %+v, want the test pattern at 10s", f)
	}
	if f := s.Keyframes[len(s.Keyframes)-1]; f.At < 38*time.Second || f.Below[2] < 85 || f.YMax-f.YLow < 60 || f.SatLow > 10 {
		t.Errorf("last keyframe %+v, want a white box on black near 38s", f)
	}
}
