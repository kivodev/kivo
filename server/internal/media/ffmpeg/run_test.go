package ffmpeg

import (
	"strings"
	"testing"
)

func TestTailKeepsTheEnd(t *testing.T) {
	var tl tail
	line := strings.Repeat("x", 99) + "\n"
	for range 1000 {
		tl.Write([]byte(line))
	}
	tl.Write([]byte("the error\n"))
	got := tl.String()
	if len(got) > stderrTail || !strings.HasSuffix(got, "the error\n") || !strings.HasPrefix(got, "xxx") {
		t.Errorf("kept %d bytes: %q…", len(got), got[:20])
	}
	if lastLine(got) != "the error" {
		t.Errorf("lastLine = %q", lastLine(got))
	}
}

func TestQuote(t *testing.T) {
	got := quote([]string{"ffmpeg", "-i", "file:/a b/it's.mkv", "-filter_complex", "[0:v]split=2[a][b]", "-b:a:0", "128000"})
	want := `ffmpeg -i 'file:/a b/it'\''s.mkv' -filter_complex '[0:v]split=2[a][b]' -b:a:0 128000`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
