package container

import (
	"bytes"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/mmcdole/kivo/internal/media"
)

// cutSlack is how far a keyframe's time in the output may be from the
// start a Splitter cuts at, and still be the keyframe it cuts at. The start
// is read from the source's index, in its own timescale.
const cutSlack = 50 * time.Millisecond

// trackTimings reads the timing of each of moov's tracks, by its ID, for a
// Splitter that cuts. Only a cut reads the tracks besides the lead.
func (s *Splitter) trackTimings(moov *mp4.MoovBox) (map[uint32]*track, error) {
	if !s.cut {
		return nil, nil
	}
	out := map[uint32]*track{}
	for _, trak := range tracks(moov) {
		tr, err := readTrack(moov, trak)
		if err != nil {
			return nil, err
		}
		out[tr.id] = tr
	}
	return out, nil
}

// cutAt cuts held output, which begins before start, so it begins at the
// lead track's keyframe at start. Each fragment in data drops every track's
// samples before that point, and a fragment left with none is dropped. An
// audio sample that runs across the start is kept, so the audio has no gap.
// Boxes other than fragments are kept as they are. It returns the output
// and the keyframe's time.
func (s *Splitter) cutAt(data []byte, start time.Duration) ([]byte, time.Duration, error) {
	var out bytes.Buffer
	at := time.Duration(-1)
	for off := int64(0); off < int64(len(data)); {
		bx, err := boxAt(data, off)
		if err != nil {
			return nil, 0, err
		}
		end := off + bx.size
		if bx.typ != "moof" {
			out.Write(data[off:end])
			off = end
			continue
		}
		mdat, err := boxAt(data, end)
		if err != nil {
			return nil, 0, err
		}
		if mdat.typ != "mdat" {
			return nil, 0, corrupt("mp4: %s after moof", mdat.typ)
		}
		frag, t, err := s.cutFragment(data[off:end+mdat.size], bx.size, start)
		if err != nil {
			return nil, 0, err
		}
		if t >= 0 {
			at = t
		}
		out.Write(frag)
		off = end + mdat.size
	}
	if at < 0 {
		return nil, 0, corrupt("mp4: no keyframe at %v to cut at", start)
	}
	return out.Bytes(), at, nil
}

// boxAt reads the header of the box at off in data, which must hold the
// whole box.
func boxAt(data []byte, off int64) (box, error) {
	if int64(len(data))-off < 8 {
		return box{}, corrupt("mp4: truncated box header")
	}
	bx, err := parseBoxHeader(data[off:min(int64(len(data)), off+16)])
	if err != nil {
		return box{}, err
	}
	if bx.size < bx.hdr || bx.size > int64(len(data))-off {
		return box{}, corrupt("mp4: %s box overruns its fragment", bx.typ)
	}
	return bx, nil
}

// cutFragment drops one fragment's samples before start. frag is the moof,
// moofSize bytes long, and the mdat after it. It returns the fragment
// encoded again, or nothing when no sample is left, and the time of the
// lead track's keyframe at start, or -1 when the fragment doesn't have it.
func (s *Splitter) cutFragment(frag []byte, moofSize int64, start time.Duration) ([]byte, time.Duration, error) {
	bx, err := decode(frag[:moofSize])
	if err != nil {
		return nil, 0, err
	}
	moof, ok := bx.(*mp4.MoofBox)
	if !ok {
		return nil, 0, corrupt("mp4: bad moof")
	}
	at := time.Duration(-1)
	mdat := &mp4.MdatBox{}
	children := make([]mp4.Box, 0, len(moof.Children))
	var trafs []*mp4.TrafBox
	for _, c := range moof.Children {
		traf, ok := c.(*mp4.TrafBox)
		if !ok {
			children = append(children, c)
			continue
		}
		k, err := s.cutTraf(traf, frag, start)
		if err != nil {
			return nil, 0, err
		}
		if k.at >= 0 {
			at = k.at
		}
		if len(k.samples) == 0 {
			continue
		}
		trun := mp4.CreateTrun(uint32(len(trafs) + 1))
		trun.AddSamples(k.samples)
		traf.Tfdt.SetBaseMediaDecodeTime(k.dts)
		traf.Children = append(withoutTruns(traf.Children), trun)
		traf.Trun, traf.Truns = trun, []*mp4.TrunBox{trun}
		for _, r := range k.spans {
			mdat.AddSampleDataPart(frag[r[0]:r[1]])
		}
		trafs = append(trafs, traf)
		children = append(children, traf)
	}
	if len(trafs) == 0 {
		return nil, at, nil
	}
	moof.Children, moof.Trafs, moof.Traf = children, trafs, trafs[0]
	f := mp4.NewFragment()
	f.AddChild(moof)
	f.AddChild(mdat)
	var out bytes.Buffer
	if err := f.Encode(&out); err != nil {
		return nil, 0, corrupt("mp4: re-encode a cut fragment: %v", err)
	}
	return out.Bytes(), at, nil
}

func withoutTruns(boxes []mp4.Box) []mp4.Box {
	out := boxes[:0:0]
	for _, b := range boxes {
		if _, ok := b.(*mp4.TrunBox); !ok {
			out = append(out, b)
		}
	}
	return out
}

// kept is what a cut keeps of one track in a fragment: its samples, the
// first one's decode time, and where their bytes are in the fragment.
type kept struct {
	tr      *track
	lead    bool
	start   time.Duration
	keeping bool // the cut has begun, and keeps every sample from here on
	samples []mp4.Sample
	dts     uint64
	spans   [][2]int64    // runs of bytes, each from its start to its end
	at      time.Duration // the lead track's keyframe at start, or -1
}

// cutTraf keeps a track's samples in a fragment from start on. The lead
// track keeps everything from its keyframe at start. A keyframe after the
// start, before one at it, means there is none to cut at. Another track
// keeps every sample that ends after the start.
func (s *Splitter) cutTraf(traf *mp4.TrafBox, frag []byte, start time.Duration) (kept, error) {
	k := kept{at: -1, start: start}
	if err := cuttable(traf); err != nil {
		return k, err
	}
	if k.tr = s.tracks[traf.Tfhd.TrackID]; k.tr == nil {
		return k, corrupt("mp4: fragment of unknown track %d", traf.Tfhd.TrackID)
	}
	k.lead = k.tr.id == s.lead.id
	dts := traf.Tfdt.BaseMediaDecodeTime()
	for _, trun := range traf.Truns {
		var err error
		if dts, err = k.run(trun, traf.Tfhd, frag, dts); err != nil {
			return k, err
		}
	}
	return k, nil
}

// cuttable returns why a cut can't rewrite traf, if it can't. A cut moves
// samples' data, so their offsets must count from the moof, and it keeps
// no per-sample side data.
func cuttable(traf *mp4.TrafBox) error {
	tfhd := traf.Tfhd
	if tfhd == nil || traf.Tfdt == nil || tfhd.HasBaseDataOffset() || !tfhd.DefaultBaseIfMoof() ||
		traf.Saiz != nil || traf.Senc != nil || traf.Sbgp != nil {
		return corrupt("mp4: can't cut a fragment laid out this way")
	}
	for _, trun := range traf.Truns {
		if !trun.HasDataOffset() {
			return corrupt("mp4: can't cut a run without a data offset")
		}
	}
	return nil
}

// run keeps a run's samples from where the cut begins. dts is the run's
// first decode time, and run returns the one after it.
func (k *kept) run(trun *mp4.TrunBox, tfhd *mp4.TfhdBox, frag []byte, dts uint64) (uint64, error) {
	trun.AddSampleDefaultValues(tfhd, k.tr.trex)
	off := int64(trun.DataOffset)
	for _, smp := range trun.Samples {
		end := off + int64(smp.Size)
		if off < 0 || end > int64(len(frag)) {
			return 0, corrupt("mp4: sample overruns its fragment")
		}
		if !k.keeping {
			var err error
			if k.keeping, err = k.begins(smp, dts); err != nil {
				return 0, err
			}
		}
		if k.keeping {
			k.add(smp, dts, off, end)
		}
		off, dts = end, dts+uint64(smp.Dur)
	}
	return dts, nil
}

// begins reports whether a cut keeps the track's samples from this one on.
func (k *kept) begins(smp mp4.Sample, dts uint64) (bool, error) {
	if !k.lead {
		return k.tr.onTimeline(dts+uint64(smp.Dur), smp.CompositionTimeOffset) > k.start, nil
	}
	if !mp4.IsSyncSampleFlags(smp.Flags) {
		return false, nil
	}
	pts := k.tr.onTimeline(dts, smp.CompositionTimeOffset)
	switch {
	case pts < k.start-cutSlack:
		return false, nil
	case pts > k.start+cutSlack:
		return false, corrupt("mp4: keyframe at %v, not at %v to cut at", pts, k.start)
	}
	k.at = pts
	return true, nil
}

// onTimeline is a sample's presentation time on the source's timeline,
// which segments' starts are on: the output's, less the offset ffmpeg adds.
func (tr *track) onTimeline(dts uint64, cto int32) time.Duration {
	return tr.pts(dts, cto) - media.TimestampOffset
}

// add keeps a sample, whose bytes run from off to end. Bytes that follow
// the last kept ones extend their run.
func (k *kept) add(smp mp4.Sample, dts uint64, off, end int64) {
	if len(k.samples) == 0 {
		k.dts = dts
	}
	k.samples = append(k.samples, smp)
	if n := len(k.spans); n > 0 && k.spans[n-1][1] == off {
		k.spans[n-1][1] = end
		return
	}
	k.spans = append(k.spans, [2]int64{off, end})
}
