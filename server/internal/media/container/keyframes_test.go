package container

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// ffprobeKeyframes reads the keyframe times that ffprobe reported for a
// fixture when testdata/gen.sh made it.
func ffprobeKeyframes(t *testing.T, name string) []time.Duration {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", name+".keyframes"))
	if err != nil {
		t.Fatal(err)
	}
	var out []time.Duration
	for line := range strings.FieldsSeq(string(b)) {
		f, err := strconv.ParseFloat(line, 64)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, time.Duration(f*float64(time.Second)).Round(time.Microsecond))
	}
	return out
}

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestKeyframesMatchFFprobe(t *testing.T) {
	for _, name := range []string{"h264.mkv", "h264.mp4", "h264-frag.mp4", "hevc.mkv"} {
		t.Run(name, func(t *testing.T) {
			b := readFixture(t, name)
			keys, complete, err := Keyframes(bytes.NewReader(b), int64(len(b)))
			if err != nil {
				t.Fatal(err)
			}
			if !complete {
				t.Error("complete = false, want true")
			}
			want := ffprobeKeyframes(t, name)
			if len(keys) != len(want) {
				t.Fatalf("got %d keyframes %v, want %v", len(keys), keys, want)
			}
			for i, k := range keys {
				if d := k.Time - want[i]; d < -time.Microsecond || d > time.Microsecond {
					t.Errorf("keyframe %d at %v, want %v", i, k.Time, want[i])
				}
			}
		})
	}
}

func TestKeyframesIncompleteOrAbsent(t *testing.T) {
	mkv := readFixture(t, "h264.mkv")
	frag := readFixture(t, "h264-frag.mp4")
	tests := []struct {
		name         string
		data         []byte
		wantKeys     bool
		wantComplete bool
	}{
		// ffmpeg writes Cues at the end, so a cut-off MKV has none.
		{"truncated mkv", mkv[:len(mkv)/2], false, false},
		{"truncated fragmented mp4", frag[:len(frag)*3/4], true, false},
		{"not a container", []byte("#EXTM3U\n#EXT-X-VERSION:7\n"), false, false},
		{"empty", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, complete, err := Keyframes(bytes.NewReader(tt.data), int64(len(tt.data)))
			if err != nil {
				t.Fatal(err)
			}
			if (len(keys) > 0) != tt.wantKeys || complete != tt.wantComplete {
				t.Errorf("got %d keyframes, complete %v; want keyframes %v, complete %v", len(keys), complete, tt.wantKeys, tt.wantComplete)
			}
		})
	}
}

func TestKeyframesTooMany(t *testing.T) {
	// With no sync-sample table, every sample is a keyframe. Claim four
	// billion of them in a few bytes. The index must be treated as absent,
	// without allocating for it.
	b := bytes.Clone(readFixture(t, "h264.mp4"))
	stss := bytes.Index(b, []byte("stss"))
	copy(b[stss:], "free")
	stts := bytes.Index(b, []byte("stts"))
	copy(b[stts+12:], []byte{0xff, 0xff, 0xff, 0xff}) // the first entry's sample count
	var (
		keys []media.Keyframe
		err  error
	)
	n := allocated(func() { keys, _, err = Keyframes(bytes.NewReader(b), int64(len(b))) })
	if err != nil || keys != nil {
		t.Fatalf("got %d keyframes, %v; want none", len(keys), err)
	}
	if n > 1<<20 {
		t.Errorf("allocated %d bytes", n)
	}
}

func TestKeyframesCorrupt(t *testing.T) {
	mkv := readFixture(t, "h264.mkv")
	bad := bytes.Clone(mkv)
	// The segment's size field starts after the 4-byte ID. A zero byte is
	// not a valid EBML length.
	seg := bytes.Index(bad, []byte{0x18, 0x53, 0x80, 0x67})
	bad[seg+4] = 0
	_, _, err := Keyframes(bytes.NewReader(bad), int64(len(bad)))
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("err = %v, want ErrSourceCorrupt", err)
	}
}

func TestKeyframesUnreadable(t *testing.T) {
	_, _, err := Keyframes(failingReader{}, 1000)
	if !errors.Is(err, media.ErrSourceUnreadable) {
		t.Fatalf("err = %v, want ErrSourceUnreadable", err)
	}
}

type failingReader struct{}

func (failingReader) ReadAt([]byte, int64) (int, error) { return 0, os.ErrPermission }
