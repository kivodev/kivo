package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

func TestRealSubtitles(t *testing.T) {
	m := realMedia(t)
	ffmpegBin, _ := realTools(t)
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	srt := write("in.srt", "1\n00:00:01,000 --> 00:00:02,500\nHello\n\n2\n00:00:03,000 --> 00:00:04,000\nWorld\n")
	ass := write("in.ass", "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\n"+
		"Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n"+
		"Style: Default,Kivo Sans,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,0,2,10,10,10,1\n\n"+
		"[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"+
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\i1}Styled{\\i0}\n")
	src := filepath.Join(dir, "anime.mkv")
	out, err := exec.Command(ffmpegBin, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10:duration=5",
		"-i", srt, "-i", ass, "-map", "0", "-map", "1", "-map", "2", "-c:v", "libx264", "-preset", "ultrafast", "-c:s", "copy", src).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}

	err = m.Subtitles(t.Context(), media.SubtitlesRequest{Path: src, Tracks: []media.SubtitleOut{
		{Track: 1, Format: media.SRT, Path: filepath.Join(dir, "1.srt")},
		{Track: 2, Format: media.ASS, Path: filepath.Join(dir, "2.ass")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "1.srt")); !strings.Contains(string(b), "00:00:01,000 --> 00:00:02,500\nHello") {
		t.Errorf("SubRip: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "2.ass")); !strings.Contains(string(b), "Kivo Sans") || !strings.Contains(string(b), `{\i1}Styled{\i0}`) {
		t.Errorf("ASS: %q", b)
	}
}

func TestRealPreviews(t *testing.T) {
	m, dir := realMedia(t), corpusDir(t)
	_, ffprobeBin := realTools(t)
	for _, name := range []string{"h264.mkv", "hevc.mp4"} {
		out := t.TempDir()
		// 30 seconds at a frame every 4 seconds is 8 frames, which fill two
		// 2×2 sheets.
		p, err := m.Previews(t.Context(), media.PreviewsRequest{
			Path: filepath.Join(dir, name), Source: media.Track{Kind: media.Video, Width: 320, Height: 180, BitDepth: 8},
			Interval: 4 * time.Second, Width: 160, Columns: 2, Rows: 2, Dir: out,
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := (media.Previews{Interval: 4 * time.Second, Width: 160, Height: 90, Columns: 2, Rows: 2, Sheets: 2}); p != want {
			t.Fatalf("%s: %+v, want %+v", name, p, want)
		}
		for _, sheet := range p.SheetNames() {
			got, err := exec.Command(ffprobeBin, "-v", "error", "-show_entries", "stream=codec_name,width,height", "-of", "csv=p=0",
				filepath.Join(out, sheet)).Output()
			if err != nil || strings.TrimSpace(string(got)) != "webp,320,180" {
				t.Errorf("%s %s: %q %v", name, sheet, got, err)
			}
		}
	}
}
