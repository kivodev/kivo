package container

import (
	"bytes"
	"encoding/binary"
	"math/bits"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
)

// moovOf returns moov with only the tracks keep accepts, encoded. The
// other boxes beside the movie header and the tracks' defaults are left
// out, because a player needs only those to play the fragments.
func moovOf(moov *mp4.MoovBox, keep func(trackID uint32) bool) ([]byte, error) {
	out := mp4.NewMoovBox()
	out.AddChild(moov.Mvhd)
	for _, trak := range moov.Traks {
		if trak.Tkhd != nil && keep(trak.Tkhd.TrackID) {
			out.AddChild(trak)
		}
	}
	if moov.Mvex != nil {
		mvex := mp4.NewMvexBox()
		if moov.Mvex.Mehd != nil {
			mvex.AddChild(moov.Mvex.Mehd)
		}
		for _, x := range moov.Mvex.Trexs {
			if keep(x.TrackID) {
				mvex.AddChild(x)
			}
		}
		out.AddChild(mvex)
	}
	var b bytes.Buffer
	if err := out.Encode(&b); err != nil {
		return nil, corrupt("mp4: encode moov: %v", err)
	}
	return b.Bytes(), nil
}

// splitFragment cuts one fragment into a fragment of the lead track and a
// fragment of the other tracks. frag is the moof followed by its mdat, and
// moof is that moof decoded. Each new fragment keeps its samples' bytes,
// timing and flags, and only its data offsets change, to point into its
// own mdat. Either is nil when the fragment has no samples of its tracks.
// The data offsets must count from the moof, as ffmpeg's default_base_moof
// writes them.
func splitFragment(moof *mp4.MoofBox, frag []byte, lead uint32, trexs map[uint32]*mp4.TrexBox) (leadFrag, rest []byte, err error) {
	var leads, others []*mp4.TrafBox
	for _, traf := range moof.Trafs {
		if traf.Tfhd == nil {
			return nil, nil, corrupt("mp4: traf without tfhd")
		}
		if traf.Tfhd.TrackID == lead {
			leads = append(leads, traf)
		} else {
			others = append(others, traf)
		}
	}
	if leadFrag, err = fragmentOf(moof.Mfhd, leads, frag, trexs); err != nil {
		return nil, nil, err
	}
	if rest, err = fragmentOf(moof.Mfhd, others, frag, trexs); err != nil {
		return nil, nil, err
	}
	return leadFrag, rest, nil
}

// fragmentOf builds a fragment of trafs, with their samples' bytes read
// from frag. It's nil when trafs is empty.
func fragmentOf(mfhd *mp4.MfhdBox, trafs []*mp4.TrafBox, frag []byte, trexs map[uint32]*mp4.TrexBox) ([]byte, error) {
	if len(trafs) == 0 {
		return nil, nil
	}
	moof := &mp4.MoofBox{}
	if mfhd != nil {
		_ = moof.AddChild(mfhd)
	}
	var payload []byte
	var at []int // where each trun's samples begin in payload, in order
	for _, traf := range trafs {
		var err error
		if payload, at, err = trafData(traf, frag, trexs[traf.Tfhd.TrackID], payload, at); err != nil {
			return nil, err
		}
		_ = moof.AddChild(traf)
	}
	// The truns' sizes don't depend on their offsets, so the moof's size
	// is known before the offsets are set.
	const mdatHeader = 8
	base := int(moof.Size()) + mdatHeader
	n := 0
	for _, traf := range trafs {
		for _, trun := range traf.Truns {
			trun.DataOffset = int32(base + at[n])
			n++
		}
	}
	var b bytes.Buffer
	if err := moof.Encode(&b); err != nil {
		return nil, corrupt("mp4: encode moof: %v", err)
	}
	var h [mdatHeader]byte
	binary.BigEndian.PutUint32(h[:], uint32(mdatHeader+len(payload)))
	copy(h[4:], "mdat")
	b.Write(h[:])
	b.Write(payload)
	return b.Bytes(), nil
}

// trafData appends the bytes of each of traf's truns, read from frag, to
// payload, and where each begins in payload to at.
func trafData(traf *mp4.TrafBox, frag []byte, trex *mp4.TrexBox, payload []byte, at []int) ([]byte, []int, error) {
	if traf.Tfhd.HasBaseDataOffset() {
		return nil, nil, corrupt("mp4: traf with an explicit base data offset")
	}
	for _, trun := range traf.Truns {
		if !trun.HasDataOffset() {
			return nil, nil, corrupt("mp4: trun without a data offset")
		}
		trun.AddSampleDefaultValues(traf.Tfhd, trex)
		size := 0
		for _, s := range trun.Samples {
			size += int(s.Size)
		}
		off := int(trun.DataOffset)
		if off < 0 || off > len(frag) || size > len(frag)-off {
			return nil, nil, corrupt("mp4: trun data runs outside its fragment")
		}
		at = append(at, len(payload))
		payload = append(payload, frag[off:off+size]...)
	}
	return payload, at, nil
}

// emptyFragment returns a fragment of tracks that has no samples, for a
// fragment of the lead track that has none of theirs, such as before the
// audio starts or after it ends. Each track's decode time begins at start,
// where the lead track's first sample shows. So a segment without audio is
// still one a player can read, placed where the video's is.
func emptyFragment(mfhd *mp4.MfhdBox, start time.Duration, tracks []*track) ([]byte, error) {
	out := &mp4.MoofBox{}
	if mfhd != nil {
		_ = out.AddChild(mfhd)
	}
	for _, tr := range tracks {
		traf := &mp4.TrafBox{}
		_ = traf.AddChild(mp4.CreateTfhd(tr.id))
		hi, lo := bits.Mul64(uint64(max(start, 0)), uint64(tr.timescale))
		at, _ := bits.Div64(hi, lo, uint64(time.Second))
		_ = traf.AddChild(mp4.CreateTfdt(at))
		_ = out.AddChild(traf)
	}
	var b bytes.Buffer
	if err := out.Encode(&b); err != nil {
		return nil, corrupt("mp4: encode moof: %v", err)
	}
	b.Write([]byte{0, 0, 0, 8, 'm', 'd', 'a', 't'})
	return b.Bytes(), nil
}
