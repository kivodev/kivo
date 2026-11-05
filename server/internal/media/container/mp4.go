package container

import (
	"encoding/binary"
	"io"
	"iter"
	"math"
	"time"

	"github.com/Eyevinn/mp4ff/bits"
	"github.com/Eyevinn/mp4ff/mp4"
)

// Limits on the sizes of boxes the MP4 readers load.
const (
	maxMoov  = 128 << 20 // three hours of progressive MP4 needs a few tens of MB
	maxMoof  = 16 << 20
	maxInit  = 16 << 20 // what comes before ffmpeg's first moof
	maxMdat  = 1 << 30  // ten seconds of the highest-bitrate remux is a few hundred MB
	maxOther = 4 << 20  // mfra at the end of a long remux is a few hundred KB
)

// topLevel lists the box types an MP4 file can start with.
var topLevel = map[string]bool{"ftyp": true, "styp": true, "moov": true, "mdat": true, "free": true, "skip": true, "wide": true}

func isMP4(fourcc []byte) bool { return topLevel[string(fourcc)] }

// box is a box located in a file or stream.
type box struct {
	typ       string
	hdr, size int64 // the header length and the whole box size
}

// parseBoxHeader reads a box header from b. b holds at least 8 bytes, and
// at least 16 when the box uses a 64-bit size. A size of zero, which means
// the box runs to the end of the file, is returned as -1.
func parseBoxHeader(b []byte) (box, error) {
	size := int64(binary.BigEndian.Uint32(b))
	bx := box{typ: string(b[4:8]), hdr: 8, size: size}
	switch size {
	case 0:
		bx.size = -1
	case 1:
		if len(b) < 16 {
			return box{}, corrupt("mp4: truncated box header")
		}
		large := binary.BigEndian.Uint64(b[8:])
		if large > math.MaxInt64 {
			return box{}, corrupt("mp4: box size overflows")
		}
		bx.hdr, bx.size = 16, int64(large)
	}
	if bx.size != -1 && bx.size < bx.hdr {
		return box{}, corrupt("mp4: %q box is %d bytes", bx.typ, bx.size)
	}
	return bx, nil
}

// decode parses a whole box held in memory with mp4ff. Callers recover
// mp4ff's panics.
func decode(b []byte) (mp4.Box, error) {
	bx, err := mp4.DecodeBoxSR(0, bits.NewFixedSliceReader(b))
	if err != nil {
		return nil, corrupt("mp4: %v", err)
	}
	return bx, nil
}

// track is the video track of an MP4 file, with what the keyframe reader
// needs from it.
type track struct {
	id        uint32
	timescale uint32
	trex      *mp4.TrexBox
	shift     time.Duration // added to each composition time by the edit list
	mediaTime int64         // subtracted from each composition time by the edit list
	next      uint64        // decode time after the last fragment read
}

// mp4Keyframes walks the file's top-level boxes. It reads moov and each
// moof, and seeks past everything else. In a progressive file, the
// sync-sample table lists every keyframe. In a fragmented file, keyframes
// come from each fragment's sample flags, and the list is complete once
// every fragment has been read.
func mp4Keyframes(r io.ReaderAt, size int64) ([]time.Duration, bool, error) {
	var (
		tr   *track
		keys []time.Duration
	)
	for off := int64(0); off < size; {
		head := make([]byte, min(16, size-off))
		if len(head) < 8 {
			return keys, false, nil
		}
		if _, err := r.ReadAt(head, off); err != nil && err != io.EOF {
			return nil, false, unreadable(err)
		}
		bx, err := parseBoxHeader(head)
		if err != nil {
			return nil, false, err
		}
		if bx.size == -1 {
			bx.size = size - off
		}
		if bx.size > size-off {
			return keys, false, nil // a partly written file, so the keyframes here are incomplete
		}
		switch {
		case bx.typ == "moov" && tr == nil:
			if bx.size > maxMoov {
				return nil, false, corrupt("mp4: moov is %d bytes", bx.size)
			}
			b, err := readAt(r, off, bx.size)
			if err != nil {
				return nil, false, err
			}
			if tr, keys, err = readMoov(b); err != nil || tr == nil {
				return nil, false, err
			}
		case bx.typ == "moof" && tr != nil:
			if bx.size > maxMoof {
				return nil, false, corrupt("mp4: moof is %d bytes", bx.size)
			}
			b, err := readAt(r, off, bx.size)
			if err != nil {
				return nil, false, err
			}
			if keys, err = tr.readMoof(b, keys); err != nil {
				return nil, false, err
			}
		}
		off += bx.size
	}
	return keys, tr != nil, nil
}

// readMoov finds the first video track and the keyframes its sample table
// lists. It returns a nil track when the file has no video.
func readMoov(b []byte) (*track, []time.Duration, error) {
	bx, err := decode(b)
	if err != nil {
		return nil, nil, err
	}
	moov, ok := bx.(*mp4.MoovBox)
	if !ok {
		return nil, nil, corrupt("mp4: bad moov")
	}
	tr, trak, err := videoTrack(moov)
	if tr == nil || err != nil {
		return nil, nil, err
	}
	var keys []time.Duration
	if trak.Mdia.Minf != nil && trak.Mdia.Minf.Stbl != nil {
		if keys, err = tr.sampleTable(trak.Mdia.Minf.Stbl); err != nil {
			return nil, nil, err
		}
	}
	return tr, keys, nil
}

// videoTrack returns the first video track, or nil when there is none.
func videoTrack(moov *mp4.MoovBox) (*track, *mp4.TrakBox, error) {
	for _, trak := range tracks(moov) {
		if trak.Mdia.Hdlr.HandlerType == "vide" {
			tr, err := readTrack(moov, trak)
			return tr, trak, err
		}
	}
	return nil, nil, nil
}

// leadTrack returns the first video track, or the first track when there is
// no video, or nil when there is none.
func leadTrack(moov *mp4.MoovBox) (*track, error) {
	tr, _, err := videoTrack(moov)
	if tr != nil || err != nil {
		return tr, err
	}
	if ts := tracks(moov); len(ts) > 0 {
		return readTrack(moov, ts[0])
	}
	return nil, nil
}

// tracks returns moov's tracks that are complete enough to read.
func tracks(moov *mp4.MoovBox) []*mp4.TrakBox {
	var out []*mp4.TrakBox
	for _, trak := range moov.Traks {
		if trak.Tkhd != nil && trak.Mdia != nil && trak.Mdia.Hdlr != nil && trak.Mdia.Mdhd != nil {
			out = append(out, trak)
		}
	}
	return out
}

// readTrack reads trak's timing from moov.
func readTrack(moov *mp4.MoovBox, trak *mp4.TrakBox) (*track, error) {
	if moov.Mvhd == nil || moov.Mvhd.Timescale == 0 || trak.Mdia.Mdhd.Timescale == 0 {
		return nil, corrupt("mp4: zero timescale")
	}
	tr := &track{id: trak.Tkhd.TrackID, timescale: trak.Mdia.Mdhd.Timescale}
	if moov.Mvex != nil {
		for _, x := range moov.Mvex.Trexs {
			if x.TrackID == tr.id {
				tr.trex = x
			}
		}
	}
	// Apply the edit list as ffmpeg's demuxer does. Leading empty edits
	// delay the track, and the first real edit's media time is where it
	// starts.
	if trak.Edts != nil && len(trak.Edts.Elst) > 0 {
		for _, e := range trak.Edts.Elst[0].Entries {
			if e.MediaTime != -1 {
				tr.mediaTime = e.MediaTime
				break
			}
			tr.shift += ticks(int64(min(e.SegmentDuration, math.MaxInt64)), moov.Mvhd.Timescale)
		}
	}
	return tr, nil
}

// pts returns a sample's presentation time from its decode time and
// composition offset.
func (tr *track) pts(dts uint64, cto int32) time.Duration {
	return ticks(int64(dts)+int64(cto)-tr.mediaTime, tr.timescale) + tr.shift
}

// sampleTable returns the sync samples of a progressive file. With no sync
// sample table, every sample is a keyframe.
func (tr *track) sampleTable(stbl *mp4.StblBox) ([]time.Duration, error) {
	if stbl.Stts == nil {
		return nil, nil
	}
	var sync []uint32 // one-based sample numbers, ascending
	n := 0
	if stbl.Stss != nil {
		sync = stbl.Stss.SampleNumber
		n = len(sync)
	} else {
		for _, c := range stbl.Stts.SampleCount {
			if n += int(c); n > maxKeyframes {
				break
			}
		}
	}
	if n > maxKeyframes {
		return nil, errTooMany
	}
	keys := make([]time.Duration, 0, n)
	// Each stts entry is a run of samples with one duration.
	var first, dts uint64 = 1, 0 // the run's first sample number and decode time
	si := 0
	for i, count := range stbl.Stts.SampleCount {
		delta, end := uint64(stbl.Stts.SampleTimeDelta[i]), first+uint64(count)
		for n := first; stbl.Stss == nil && n < end; n++ {
			keys = append(keys, tr.pts(dts+delta*(n-first), tr.cto(stbl.Ctts, n)))
		}
		for ; si < len(sync) && uint64(sync[si]) < end; si++ {
			n := uint64(sync[si])
			if n < first {
				return nil, corrupt("mp4: sync samples out of order")
			}
			keys = append(keys, tr.pts(dts+delta*(n-first), tr.cto(stbl.Ctts, n)))
		}
		first, dts = end, dts+delta*uint64(count)
	}
	return keys, nil
}

// cto returns sample n's composition offset.
func (tr *track) cto(ctts *mp4.CttsBox, n uint64) int32 {
	if ctts == nil || n == 0 || n > math.MaxUint32 {
		return 0
	}
	return ctts.GetCompositionTimeOffset(uint32(n))
}

// readMoof appends the keyframes of the video track's fragment in moof.
func (tr *track) readMoof(b []byte, keys []time.Duration) ([]time.Duration, error) {
	bx, err := decode(b)
	if err != nil {
		return nil, err
	}
	moof, ok := bx.(*mp4.MoofBox)
	if !ok {
		return nil, corrupt("mp4: bad moof")
	}
	for s := range tr.samples(moof) {
		if mp4.IsSyncSampleFlags(s.Flags) {
			if len(keys) == maxKeyframes {
				return nil, errTooMany
			}
			keys = append(keys, tr.pts(s.dts, s.CompositionTimeOffset))
		}
	}
	return keys, nil
}

// sample is one sample of a fragment with its decode time.
type sample struct {
	mp4.Sample
	dts uint64
}

// samples yields the track's samples in moof, with defaults applied. It
// advances tr.next past them.
func (tr *track) samples(moof *mp4.MoofBox) iter.Seq[sample] {
	return func(yield func(sample) bool) {
		for _, traf := range moof.Trafs {
			if traf.Tfhd == nil || traf.Tfhd.TrackID != tr.id {
				continue
			}
			if traf.Tfdt != nil {
				tr.next = traf.Tfdt.BaseMediaDecodeTime()
			}
			for _, trun := range traf.Truns {
				trun.AddSampleDefaultValues(traf.Tfhd, tr.trex)
				for _, s := range trun.Samples {
					dts := tr.next
					tr.next += uint64(s.Dur)
					if !yield(sample{s, dts}) {
						return
					}
				}
			}
		}
	}
}
