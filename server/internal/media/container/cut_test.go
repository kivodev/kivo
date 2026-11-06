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

func splitCut(t *testing.T, data []byte, start time.Duration) ([]byte, []media.Segment, error) {
	t.Helper()
	s := NewSplitter(bytes.NewReader(data), start, true)
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

func starts(segs []media.Segment) []time.Duration {
	var out []time.Duration
	for _, s := range segs {
		out = append(out, s.Start)
	}
	return out
}

// fragSample is one sample of a segment's fragment, with its bytes.
type fragSample struct {
	dts, end uint64 // its decode time, and the one after it
	cto      int32
	sync     bool
	data     []byte
}

// fragSamples reads each track's samples in a segment of one fragment, and
// each track's timescale.
func fragSamples(t *testing.T, init, seg []byte) (map[uint32][]fragSample, map[uint32]uint32) {
	t.Helper()
	f, err := mp4.DecodeFile(bytes.NewReader(init))
	if err != nil {
		t.Fatal(err)
	}
	scales := map[uint32]uint32{}
	for _, trak := range f.Init.Moov.Traks {
		scales[trak.Tkhd.TrackID] = trak.Mdia.Mdhd.Timescale
	}
	var moof *mp4.MoofBox
	var moofAt uint64
	for off := uint64(0); off < uint64(len(seg)); {
		bx, err := mp4.DecodeBox(off, bytes.NewReader(seg[off:]))
		if err != nil {
			t.Fatal(err)
		}
		if m, ok := bx.(*mp4.MoofBox); ok {
			if moof != nil {
				t.Fatal("more than one fragment")
			}
			moof, moofAt = m, off
		}
		off += bx.Size()
	}
	out := map[uint32][]fragSample{}
	for _, traf := range moof.Trafs {
		id := traf.Tfhd.TrackID
		trex, _ := f.Init.Moov.Mvex.GetTrex(id)
		dts := traf.Tfdt.BaseMediaDecodeTime()
		for _, trun := range traf.Truns {
			trun.AddSampleDefaultValues(traf.Tfhd, trex)
			off := moofAt + uint64(trun.DataOffset)
			for _, s := range trun.Samples {
				out[id] = append(out[id], fragSample{dts: dts, end: dts + uint64(s.Dur), cto: s.CompositionTimeOffset,
					sync: mp4.IsSyncSampleFlags(s.Flags), data: seg[off : off+uint64(s.Size)]})
				off += uint64(s.Size)
				dts += uint64(s.Dur)
			}
		}
	}
	return out, scales
}

func onTimeline(v int64, scale uint32) time.Duration {
	return ticks(v, scale) - media.TimestampOffset
}

// remux-merged.mp4 is ffmpeg's output for a seek to 2.4 s with E-AC-3
// audio. Its first fragment runs from the keyframe at 1.6 s on past the one
// at 2.4 s, to 4 s. Cut at 2.4 s, the first segment begins there, with each
// track's samples from then on, byte for byte.
func TestSplitterCutsAtTheStart(t *testing.T) {
	data := readFixture(t, "remux-merged.mp4")
	_, whole, err := split(t, data, ms(2400))
	if err != nil {
		t.Fatal(err)
	}
	init, segs, err := splitCut(t, data, ms(2400))
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != len(whole) || len(segs) == 0 || segs[0].Start != ms(2400) || whole[0].Start != ms(1600) {
		t.Fatalf("cut segments start at %v, whole ones at %v", starts(segs), starts(whole))
	}
	for i := 1; i < len(segs); i++ {
		if !bytes.Equal(segs[i].Data, whole[i].Data) {
			t.Errorf("segment %d changed", i)
		}
	}
	cut, scales := fragSamples(t, init, segs[0].Data)
	before, _ := fragSamples(t, init, whole[0].Data)
	if len(cut) != 2 || len(before) != 2 {
		t.Fatalf("fragments have %d and %d tracks, want video and audio in both", len(cut), len(before))
	}
	for id, samples := range before {
		got := cut[id]
		if len(got) == 0 || len(got) >= len(samples) {
			t.Fatalf("track %d keeps %d of %d samples", id, len(got), len(samples))
		}
		dropped := samples[:len(samples)-len(got)]
		for i, s := range got {
			want := samples[len(dropped)+i]
			if s.dts != want.dts || s.cto != want.cto || s.sync != want.sync || !bytes.Equal(s.data, want.data) {
				t.Fatalf("track %d sample %d differs from the original", id, i)
			}
		}
		first, last := got[0], dropped[len(dropped)-1]
		begin, end := onTimeline(int64(first.dts), scales[id]), onTimeline(int64(first.end), scales[id])
		if id == 1 {
			if pts := onTimeline(int64(first.dts)+int64(first.cto), scales[id]); !first.sync || pts != ms(2400) {
				t.Errorf("video begins with a sample at %v, keyframe %v", pts, first.sync)
			}
			continue
		}
		// The audio sample running across 2.4 s is kept, and the one
		// before it, which ends by then, isn't.
		if begin >= ms(2400) || end <= ms(2400) {
			t.Errorf("audio begins with a sample from %v to %v", begin, end)
		}
		if e := onTimeline(int64(last.end), scales[id]); e > ms(2400) {
			t.Errorf("audio dropped a sample ending at %v", e)
		}
	}
	if _, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init), segs[0].Data...))); err != nil {
		t.Fatal(err)
	}
}

// With no keyframe at the start in the fragment that contains it, there is
// nothing to cut at.
func TestSplitterCutNeedsAKeyframeAtTheStart(t *testing.T) {
	_, _, err := splitCut(t, readFixture(t, "remux-merged.mp4"), ms(2000))
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("err = %v, want ErrSourceCorrupt", err)
	}
}

// Output whose fragment begins at the start is left as it is.
func TestSplitterCutAtAFragmentsStart(t *testing.T) {
	data := readFixture(t, "remux-h264.mp4")
	_, want, err := split(t, data, ms(2400))
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := splitCut(t, data, ms(2400))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || !bytes.Equal(got[0].Data, want[0].Data) {
		t.Fatalf("segments start at %v, want %v", starts(got), starts(want))
	}
}
