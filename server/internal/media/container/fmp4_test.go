package container

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/mmcdole/kivo/internal/media"
)

func split(t *testing.T, data []byte) (media.Init, []media.Segment, error) {
	t.Helper()
	s := NewSplitter(bytes.NewReader(data))
	b, err := s.Init()
	if err != nil {
		return media.Init{}, nil, err
	}
	init := media.Init{Data: b, Audio: s.AudioInit()}
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
	// the keyframe at 1.6 s. remux-hevc.mp4 is from the start. Both end at
	// 8 s.
	tests := []struct {
		fixture string
		want    []time.Duration
	}{
		{"remux-h264.mp4", []time.Duration{ms(1600), ms(2400), ms(4000), ms(6200)}},
		{"remux-hevc.mp4", []time.Duration{0, ms(1600), ms(4000), ms(6200)}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			data := readFixture(t, tt.fixture)
			init, segs, err := split(t, data)
			if err != nil {
				t.Fatal(err)
			}
			var got []time.Duration
			total := len(init.Data) + len(init.Audio)
			for _, s := range segs {
				got = append(got, s.Start)
				total += len(s.Data) + len(s.Audio)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("segments start at %v, want %v", got, tt.want)
			}
			for i := range got {
				if d := got[i] - tt.want[i]; d < -12*time.Microsecond || d > 12*time.Microsecond {
					t.Errorf("segment %d starts at %v, want %v", i, got[i], tt.want[i])
				}
			}
			if total < len(data)-1024 {
				t.Errorf("segments hold %d of %d bytes", total, len(data))
			}
		})
	}
}

func TestSplitterSegmentsAreValidFragments(t *testing.T) {
	init, segs, err := split(t, readFixture(t, "remux-hevc.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	for i, seg := range segs {
		f, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init.Data), seg.Data...)))
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
	_, segs, err := split(t, data[:len(data)-400])
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("err = %v, want ErrSourceCorrupt", err)
	}
	if len(segs) != 3 {
		t.Errorf("got %d segments before the error, want 3", len(segs))
	}
}

func TestSplitterNoOutput(t *testing.T) {
	_, err := NewSplitter(bytes.NewReader(nil)).Init()
	if !errors.Is(err, media.ErrSourceCorrupt) {
		t.Fatalf("err = %v, want ErrSourceCorrupt", err)
	}
}

// samplesOf returns every sample of the fragments in segs, decoded against
// init, which must hold one track.
func TestSplitterGivesAudioToASegmentWithout(t *testing.T) {
	// ffmpeg writes fragments without audio before the audio starts and
	// after it ends. Their segments' audio has no samples but is still a
	// fragment, placed where the video is.
	data := withoutAudio(t, readFixture(t, "remux-h264.mp4"), 1)
	init, segs, err := split(t, data)
	if err != nil {
		t.Fatal(err)
	}
	f, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init.Audio), segs[1].Audio...)))
	if err != nil {
		t.Fatal(err)
	}
	audio := f.Init.Moov.Trak
	frag := f.Segments[0].Fragments[0]
	if len(frag.Moof.Trafs) != 1 || frag.Moof.Traf.Tfhd.TrackID != audio.Tkhd.TrackID {
		t.Fatal("the audio isn't a fragment of the audio track")
	}
	if n := len(frag.Moof.Traf.Truns); n != 0 {
		t.Fatalf("%d runs of audio", n)
	}
	got := time.Duration(frag.Moof.Traf.Tfdt.BaseMediaDecodeTime()) * time.Second / time.Duration(audio.Mdia.Mdhd.Timescale)
	if want := segs[1].Start + media.TimestampOffset; got < want-time.Millisecond || got > want+time.Millisecond {
		t.Fatalf("audio at %v, video at %v", got, want)
	}
	for i, s := range segs {
		if i != 1 && len(s.Audio) <= len(segs[1].Audio) {
			t.Fatalf("segment %d lost its audio", i)
		}
	}
}

func TestSplitterTrimsAudioBeforeTheVideo(t *testing.T) {
	// A job that starts partway in gets copied audio from the source
	// keyframe before its start. The first segment's audio starts with the
	// video, and later segments are unchanged.
	plain := readFixture(t, "remux-h264.mp4")
	_, want, err := split(t, plain)
	if err != nil {
		t.Fatal(err)
	}
	init, segs, err := split(t, delayVideo(t, plain, 500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	videoAt := firstDecodeTime(t, init.Data, segs[0].Data)
	audioAt := firstDecodeTime(t, init.Audio, segs[0].Audio)
	if audioAt < videoAt-time.Millisecond || audioAt > videoAt+30*time.Millisecond {
		t.Fatalf("audio starts at %v, video at %v", audioAt, videoAt)
	}
	if len(segs[0].Audio) >= len(want[0].Audio) {
		t.Fatal("the first segment's audio wasn't trimmed")
	}
	for i := 1; i < len(segs); i++ {
		if !bytes.Equal(segs[i].Audio, want[i].Audio) {
			t.Fatalf("segment %d's audio changed", i)
		}
	}
}

// firstDecodeTime returns when seg's first sample decodes, read with init.
func firstDecodeTime(t *testing.T, init, seg []byte) time.Duration {
	t.Helper()
	f, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init), seg...)))
	if err != nil {
		t.Fatal(err)
	}
	ts := f.Init.Moov.Trak.Mdia.Mdhd.Timescale
	at := f.Segments[0].Fragments[0].Moof.Traf.Tfdt.BaseMediaDecodeTime()
	return time.Duration(at) * time.Second / time.Duration(ts)
}

// delayVideo returns muxed fMP4 with its first fragment's video decoding d
// later, so that fragment's audio starts before its video.
func delayVideo(t *testing.T, data []byte, d time.Duration) []byte {
	t.Helper()
	video, scale := trackOf(t, data, "vide")
	return editMoof(t, data, 0, func(moof *mp4.MoofBox) {
		for _, traf := range moof.Trafs {
			if traf.Tfhd.TrackID == video {
				traf.Tfdt.SetBaseMediaDecodeTime(traf.Tfdt.BaseMediaDecodeTime() + uint64(d*time.Duration(scale)/time.Second))
			}
		}
	})
}

// withoutAudio returns muxed fMP4 with the audio taken out of fragment n,
// counting from 0. Its samples' bytes stay in the mdat, unreferenced.
func withoutAudio(t *testing.T, data []byte, n int) []byte {
	t.Helper()
	audio, _ := trackOf(t, data, "soun")
	return editMoof(t, data, n, func(moof *mp4.MoofBox) {
		kept := moof.Trafs[:0]
		for _, traf := range moof.Trafs {
			if traf.Tfhd.TrackID != audio {
				kept = append(kept, traf)
			}
		}
		moof.Trafs = kept
		moof.Children = []mp4.Box{moof.Mfhd}
		for _, traf := range kept {
			moof.Children = append(moof.Children, traf)
		}
	})
}

// trackOf returns the ID and timescale of the file's track with handler.
func trackOf(t *testing.T, data []byte, handler string) (uint32, uint32) {
	t.Helper()
	f, err := mp4.DecodeFile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, trak := range f.Init.Moov.Traks {
		if trak.Mdia.Hdlr.HandlerType == handler {
			return trak.Tkhd.TrackID, trak.Mdia.Mdhd.Timescale
		}
	}
	t.Fatalf("no %s track", handler)
	return 0, 0
}

// editMoof returns data with edit applied to moof n, counting from 0. The
// data offsets, which count from the moof, follow any change in its size.
func editMoof(t *testing.T, data []byte, n int, edit func(*mp4.MoofBox)) []byte {
	t.Helper()
	var out []byte
	for pos, k := 0, 0; pos < len(data); {
		size := int(binary.BigEndian.Uint32(data[pos:]))
		b := data[pos : pos+size]
		pos += size
		if string(b[4:8]) == "moof" {
			k++
		}
		if string(b[4:8]) != "moof" || k-1 != n {
			out = append(out, b...)
			continue
		}
		bx, err := mp4.DecodeBox(0, bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		moof := bx.(*mp4.MoofBox)
		was := moof.Size()
		edit(moof)
		grow := int32(moof.Size()) - int32(was)
		for _, traf := range moof.Trafs {
			for _, trun := range traf.Truns {
				trun.DataOffset += grow
			}
		}
		var w bytes.Buffer
		if err := moof.Encode(&w); err != nil {
			t.Fatal(err)
		}
		out = append(out, w.Bytes()...)
	}
	return out
}

func samplesOf(t *testing.T, init []byte, segs [][]byte) (uint32, []mp4.FullSample) {
	t.Helper()
	var id uint32
	var out []mp4.FullSample
	for i, seg := range segs {
		f, err := mp4.DecodeFile(bytes.NewReader(append(bytes.Clone(init), seg...)))
		if err != nil {
			t.Fatalf("segment %d: %v", i, err)
		}
		if len(f.Init.Moov.Traks) != 1 {
			t.Fatalf("init segment has %d tracks", len(f.Init.Moov.Traks))
		}
		id = f.Init.Moov.Trak.Tkhd.TrackID
		trex, _ := f.Init.Moov.Mvex.GetTrex(id)
		for _, frag := range f.Segments[0].Fragments {
			for _, traf := range frag.Moof.Trafs {
				if traf.Tfhd.TrackID != id {
					t.Fatalf("segment %d holds track %d beside %d", i, traf.Tfhd.TrackID, id)
				}
			}
			ss, err := frag.GetFullSamples(trex)
			if err != nil {
				t.Fatalf("segment %d: %v", i, err)
			}
			out = append(out, ss...)
		}
	}
	return id, out
}

// muxedSamples returns track id's samples in a muxed fragmented file, read
// from each fragment's own data offsets.
func muxedSamples(t *testing.T, data []byte, id uint32) []mp4.FullSample {
	t.Helper()
	f, err := mp4.DecodeFile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	trex, _ := f.Init.Moov.Mvex.GetTrex(id)
	var out []mp4.FullSample
	for _, seg := range f.Segments {
		for _, frag := range seg.Fragments {
			for _, traf := range frag.Moof.Trafs {
				if traf.Tfhd.TrackID != id {
					continue
				}
				one := &mp4.Fragment{Moof: &mp4.MoofBox{Mfhd: frag.Moof.Mfhd, Traf: traf, Trafs: []*mp4.TrafBox{traf}}, Mdat: frag.Mdat}
				one.Moof.StartPos = frag.Moof.StartPos
				ss, err := one.GetFullSamples(trex)
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, ss...)
			}
		}
	}
	return out
}

func TestSplitterSplitsAudio(t *testing.T) {
	for _, fixture := range []string{"remux-h264.mp4", "remux-hevc.mp4"} {
		t.Run(fixture, func(t *testing.T) {
			data := readFixture(t, fixture)
			init, segs, err := split(t, data)
			if err != nil {
				t.Fatal(err)
			}
			if init.Audio == nil {
				t.Fatal("no audio init segment")
			}
			var video, audio [][]byte
			for i, s := range segs {
				if len(s.Audio) == 0 {
					t.Fatalf("segment %d has no audio", i)
				}
				video, audio = append(video, s.Data), append(audio, s.Audio)
			}
			videoID, videoSamples := samplesOf(t, init.Data, video)
			audioID, audioSamples := samplesOf(t, init.Audio, audio)
			if videoID == audioID {
				t.Fatalf("both renditions hold track %d", videoID)
			}
			// Every sample keeps its bytes, timing and flags.
			for _, c := range []struct {
				name string
				id   uint32
				got  []mp4.FullSample
			}{{"video", videoID, videoSamples}, {"audio", audioID, audioSamples}} {
				want := muxedSamples(t, data, c.id)
				if len(c.got) != len(want) {
					t.Fatalf("%s: %d samples, want %d", c.name, len(c.got), len(want))
				}
				for i := range want {
					g, w := c.got[i], want[i]
					if g.DecodeTime != w.DecodeTime || g.Dur != w.Dur || g.Flags != w.Flags ||
						g.CompositionTimeOffset != w.CompositionTimeOffset || !bytes.Equal(g.Data, w.Data) {
						t.Fatalf("%s sample %d differs", c.name, i)
					}
				}
			}
		})
	}
}
