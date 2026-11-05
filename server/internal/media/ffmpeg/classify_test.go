package ffmpeg

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/media"
)

// errFallback stands for whatever the caller falls back to.
var errFallback = errors.New("fallback")

// input is the source every stderr fixture was captured reading.
const input = "file:/m/a.mkv"

func TestClassifyCapturedStderr(t *testing.T) {
	files, err := filepath.Glob(fixture(t, "stderr/*.txt"))
	if err != nil || len(files) == 0 {
		t.Fatal("no stderr fixtures", err)
	}
	for _, f := range files {
		name := filepath.Base(f)
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := classify(string(b), input, errFallback), kindFromName(t, name); got != want {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
	if got := classify("Conversion failed!\n", input, errFallback); got != errFallback {
		t.Errorf("unrecognized output: got %v, want the fallback", got)
	}
}

// kindFromName reads the error a stderr fixture should map to from its name.
func kindFromName(t *testing.T, name string) error {
	kind, _, _ := strings.Cut(name, "-")
	switch kind {
	case "unreadable":
		return media.ErrSourceUnreadable
	case "corrupt":
		return media.ErrSourceCorrupt
	case "unsupported":
		return media.ErrUnsupported
	case "busy":
		return media.ErrEncoderBusy
	case "failed":
		return media.ErrEncoderFailed
	}
	t.Fatalf("stderr fixture %s names no error", name)
	return nil
}
