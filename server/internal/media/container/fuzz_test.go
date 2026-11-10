package container

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// allocLimit is how much one parse of n bytes may allocate. It is a small
// multiple of the input, plus room for the fixed cap on keyframes an index
// may list. Growing a slice to that cap allocates a few times its size.
func allocLimit(n int) uint64 { return 64*uint64(n) + 4*8*maxKeyframes + 8<<20 }

func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func addSeeds(f *testing.F, names ...string) {
	for _, name := range names {
		b := readFixture(f, name)
		f.Add(b)
		f.Add(b[:len(b)/3])
	}
}

func FuzzKeyframes(f *testing.F) {
	addSeeds(f, "h264.mkv", "h264.mp4", "h264-frag.mp4", "hevc.mkv")
	f.Fuzz(func(t *testing.T, data []byte) {
		var (
			keys     []media.Keyframe
			complete bool
			err      error
		)
		n := allocated(func() { keys, complete, err = Keyframes(bytes.NewReader(data), int64(len(data))) })
		if n > allocLimit(len(data)) {
			t.Fatalf("allocated %d bytes for %d bytes of input", n, len(data))
		}
		if err != nil {
			if !errors.Is(err, media.ErrSourceCorrupt) {
				t.Fatalf("err = %v, want ErrSourceCorrupt", err)
			}
			return
		}
		if complete && len(keys) == 0 {
			t.Fatal("complete index with no keyframes")
		}
		for i := 1; i < len(keys); i++ {
			if keys[i].Time <= keys[i-1].Time {
				t.Fatalf("keyframe %d at %v follows %v", i, keys[i].Time, keys[i-1].Time)
			}
		}
	})
}

func FuzzAttachments(f *testing.F) {
	addSeeds(f, "attachments.mkv", "h264.mkv")
	f.Fuzz(func(t *testing.T, data []byte) {
		var (
			files []media.AttachedFile
			err   error
		)
		n := allocated(func() { files, err = Attachments(bytes.NewReader(data), int64(len(data))) })
		if n > allocLimit(len(data)) {
			t.Fatalf("allocated %d bytes for %d bytes of input", n, len(data))
		}
		if err != nil && !errors.Is(err, media.ErrSourceCorrupt) {
			t.Fatalf("err = %v, want ErrSourceCorrupt", err)
		}
		for _, a := range files {
			if a.Size < 0 || a.Size > int64(len(data)) || len(a.SHA256) != 64 {
				t.Fatalf("attachment %+v from %d bytes", a, len(data))
			}
		}
	})
}

func FuzzSplitter(f *testing.F) {
	addSeeds(f, "remux-h264.mp4", "remux-hevc.mp4", "h264-frag.mp4")
	f.Fuzz(func(t *testing.T, data []byte) {
		var (
			segs []media.Segment
			err  error
		)
		n := allocated(func() {
			s := NewSplitter(bytes.NewReader(data))
			for {
				var seg media.Segment
				if seg, err = s.Next(); err != nil {
					return
				}
				segs = append(segs, seg)
			}
		})
		if n > allocLimit(len(data)) {
			t.Fatalf("allocated %d bytes for %d bytes of input", n, len(data))
		}
		if err != io.EOF && !errors.Is(err, media.ErrSourceCorrupt) {
			t.Fatalf("err = %v, want io.EOF or ErrSourceCorrupt", err)
		}
		last := time.Duration(-1 << 63)
		for i, seg := range segs {
			if seg.Start <= last {
				t.Fatalf("segment %d at %v follows %v", i, seg.Start, last)
			}
			last = seg.Start
		}
	})
}
