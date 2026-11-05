package container

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/mmcdole/kivo/internal/media"
)

func split(t *testing.T, data []byte, start time.Duration) ([]byte, []media.Segment, error) {
	t.Helper()
	s := NewSplitter(bytes.NewReader(data), start)
	init, err := s.Init()
	if err != nil {
		return nil, nil, err
	}
	var segs []media.Segment
	for {
		seg, err := s.Next()
		if err == io.EOF {
			return init, segs, nil
		}
		if err != nil {
			return init, segs, err
		}
		segs = append(segs, seg)
	}
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestSplitterSegments(t *testing.T) {
	// remux-h264.mp4 is ffmpeg's output for a seek to 2.4 s, which began at
	// the keyframe at 1.6 s. Its keyframes are at 1.6, 2.4, 4 and 6.2 s.
	// remux-hevc.mp4 has keyframes at 0, 1.6, 4 and 6.2 s. Both end at 8 s.
	tests := []struct {
		name    string
		fixture string
		start   time.Duration
		want    []time.Duration
	}{
		{"seek to a keyframe drops the one before", "remux-h264.mp4", ms(2400), []time.Duration{ms(2400), ms(4000), ms(6200)}},
		{"seek inside a fragment keeps it", "remux-h264.mp4", ms(2000), []time.Duration{ms(1600), ms(2400), ms(4000), ms(6200)}},
		{"seek within rounding of a keyframe", "remux-h264.mp4", ms(2400) + 500*time.Microsecond, []time.Duration{ms(2400), ms(4000), ms(6200)}},
		{"from zero", "remux-hevc.mp4", 0, []time.Duration{0, ms(1600), ms(4000), ms(6200)}},
		{"seek into the last fragment", "remux-hevc.mp4", ms(9000), []time.Duration{ms(6200)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := readFixture(t, tt.fixture)
			init, segs, err := split(t, data, tt.start)
			if err != nil {
				t.Fatal(err)
			}
			var got []time.Duration
			total := len(init)
			for _, s := range segs {
				got = append(got, s.Start)
				total += len(s.Data)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("segments start at %v, want %v", got, tt.want)
			}
			for i := range got {
				if d := got[i] - tt.want[i]; d < -12*time.Microsecond || d > 12*time.Microsecond {
					t.Errorf("segment %d starts at %v, want %v", i, got[i], tt.want[i])
				}
			}
			if tt.start == 0 && total < len(data)-1024 {
				t.Errorf("segments hold %d of %d bytes", total, len(data))
			}
		})
	}
}

func TestSplitterSegmentsAreValidFragments(t *testing.T) {
	init, segs, err := split(t, readFixture(t, "remux-hevc.mp4"), 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, seg := range segs {
		f, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init), seg.Data...)))
		if err != nil {
			t.Fatal(err)
		}
		stsd := f.Init.Moov.Trak.Mdia.Minf.Stbl.Stsd
		if stsd.HvcX == nil || stsd.HvcX.Type() != "hvc1" || stsd.HvcX.HvcC == nil {
			t.Fatalf("sample entry %v, want hvc1 with hvcC", stsd.HvcX)
		}
		frag := f.Segments[0].Fragments[0]
		trun := frag.Moof.Traf.Trun
		trun.AddSampleDefaultValues(frag.Moof.Traf.Tfhd, f.Init.Moov.Mvex.Trex)
		if !mp4.IsSyncSampleFlags(trun.Samples[0].Flags) {
			t.Errorf("segment %d does not start on a keyframe", i)
		}
	}
}

func TestSplitterTruncated(t *testing.T) {
	data := readFixture(t, "remux-h264.mp4")
	// Cut inside the last fragment, before the trailing mfra box.
	_, segs, err := split(t, data[:len(data)-400], ms(2400))
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("err = %v, want ErrSourceCorrupt", err)
	}
	if len(segs) != 2 {
		t.Errorf("got %d segments before the error, want 2", len(segs))
	}
}

func TestSplitterNoOutput(t *testing.T) {
	_, err := NewSplitter(bytes.NewReader(nil), 0).Init()
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("err = %v, want ErrSourceCorrupt", err)
	}
}
