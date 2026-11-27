package container

import (
	"bytes"
	"math/bits"

	"github.com/Eyevinn/mp4ff/mp4"
)

// A segment's audio has to cover the segment. Players that follow each
// sample's timestamp don't mind when it doesn't. But a player that lays a
// rendition's segments end to end (Media Source's sequence mode, which
// Shaka and so the Cast receiver use for HLS) puts a segment's first audio
// sample where the segment begins, whatever its timestamp says. So audio
// that starts late in its segment, as when a file's audio track begins a
// second after its video, plays early from there on, and a segment with no
// audio at all moves everything after it. Where the audio is AAC-LC, the
// missing stretch is filled with silent frames, which need nothing from the
// track but its channel layout.

// silence is what fills a track's missing audio: one silent frame, and how
// long it lasts in the track's timescale.
type silence struct {
	frame    []byte
	duration uint64
}

// aacSilence returns the silence for trak, or nil if it isn't AAC-LC with
// 1024-sample frames and one of the standard channel layouts.
func aacSilence(trak *mp4.TrakBox) *silence {
	if trak.Mdia == nil || trak.Mdia.Minf == nil || trak.Mdia.Minf.Stbl == nil || trak.Mdia.Minf.Stbl.Stsd == nil {
		return nil
	}
	entry := trak.Mdia.Minf.Stbl.Stsd.Mp4a
	if entry == nil || entry.Esds == nil || entry.Esds.DecConfigDescriptor == nil || entry.Esds.DecConfigDescriptor.DecSpecificInfo == nil {
		return nil
	}
	const mpeg4Audio = 0x40
	if entry.Esds.DecConfigDescriptor.ObjectType != mpeg4Audio {
		return nil
	}
	// AudioSpecificConfig: object type (5 bits), sampling frequency index
	// (4, or 15 and the rate in 24), channel configuration (4), then
	// GASpecificConfig: frame length flag, depends on core coder, extension.
	r := bitReader{b: entry.Esds.DecConfigDescriptor.DecSpecificInfo.DecConfig}
	const aacLC = 2
	if r.read(5) != aacLC {
		return nil
	}
	rates := []uint64{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	var rate uint64
	switch i := r.read(4); {
	case i == 15:
		rate = r.read(24)
	case int(i) < len(rates):
		rate = rates[i]
	}
	channels := r.read(4)
	shortFrames, dependsOnCore := r.read(1), r.read(1)
	scale := uint64(trak.Mdia.Mdhd.Timescale)
	if r.short || rate == 0 || shortFrames != 0 || dependsOnCore != 0 || 1024*scale%rate != 0 {
		return nil
	}
	frame := silentAAC(int(channels))
	if frame == nil {
		return nil
	}
	return &silence{frame: frame, duration: 1024 * scale / rate}
}

// silentAAC returns one AAC-LC frame of silence for a channel
// configuration (ISO/IEC 14496-3, table 1.19), or nil for one that names
// no layout. Each channel is a long window with no scale factor bands, so
// it carries no spectrum.
func silentAAC(channelConfig int) []byte {
	const (
		single = 0 // a single channel element
		pair   = 1 // a channel pair element
		lfe    = 3 // a low-frequency effects element
		end    = 7
	)
	layouts := map[int][]int{
		1: {single},
		2: {pair},
		3: {single, pair},
		4: {single, pair, single},
		5: {single, pair, pair},
		6: {single, pair, pair, lfe},
		7: {single, pair, pair, pair, lfe},
	}
	layout, ok := layouts[channelConfig]
	if !ok {
		return nil
	}
	var w bitWriter
	info := func() { // ics_info: reserved, window sequence, window shape, max_sfb, predictor
		w.write(0, 1+2+1+6+1)
	}
	channel := func(withInfo bool) { // individual_channel_stream
		w.write(0, 8) // global gain
		if withInfo {
			info()
		}
		w.write(0, 3) // no pulse data, no TNS, no gain control
	}
	tags := map[int]uint64{}
	for _, e := range layout {
		w.write(uint64(e), 3)
		w.write(tags[e], 4)
		tags[e]++
		if e == pair {
			w.write(1, 1) // both channels share one ics_info
			info()
			w.write(0, 2) // no mid/side
			channel(false)
			channel(false)
		} else {
			channel(true)
		}
	}
	w.write(end, 3)
	return w.bytes()
}

// silentFragment returns a fragment of n silent frames of tr that begins
// decoding at start, in tr's timescale.
func silentFragment(mfhd *mp4.MfhdBox, tr *track, start uint64, n int) ([]byte, error) {
	moof := &mp4.MoofBox{}
	if mfhd != nil {
		_ = moof.AddChild(mfhd)
	}
	traf := &mp4.TrafBox{}
	_ = traf.AddChild(mp4.CreateTfhd(tr.id))
	_ = traf.AddChild(mp4.CreateTfdt(start))
	trun := mp4.CreateTrun(0)
	trun.Flags = mp4.TrunDataOffsetPresentFlag | mp4.TrunSampleDurationPresentFlag | mp4.TrunSampleSizePresentFlag | mp4.TrunSampleFlagsPresentFlag
	for range n {
		trun.AddSample(mp4.Sample{Flags: mp4.SyncSampleFlags, Dur: uint32(tr.silence.duration), Size: uint32(len(tr.silence.frame))})
	}
	_ = traf.AddChild(trun)
	_ = moof.AddChild(traf)
	const mdatHeader = 8
	trun.DataOffset = int32(moof.Size()) + mdatHeader
	var b bytes.Buffer
	if err := moof.Encode(&b); err != nil {
		return nil, corrupt("mp4: encode moof: %v", err)
	}
	size := mdatHeader + n*len(tr.silence.frame)
	b.Write([]byte{byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size), 'm', 'd', 'a', 't'})
	b.Write(bytes.Repeat(tr.silence.frame, n))
	return b.Bytes(), nil
}

// fillAudio returns a lead fragment's audio with any missing stretch at its
// start filled with silence: all of it when the fragment has no audio. moof
// is the fragment as read, and rest the one audio track. The fill is a
// fragment of its own ahead of the audio, so the audio's own samples keep
// their bytes and their times.
func fillAudio(moof *mp4.MoofBox, audio []byte, lead, rest *track) ([]byte, error) {
	from, to, ok := decodeSpan(moof, lead)
	if !ok {
		return audio, nil
	}
	start := rescale(from, lead.timescale, rest.timescale)
	end := rescale(to, lead.timescale, rest.timescale)
	d := rest.silence.duration
	// With audio, fill up to its first sample, in whole frames that end
	// there. Without, cover the fragment. Audio whose samples were all
	// trimmed counts as none.
	first, has := firstDecodeTimeOf(moof, rest.id)
	var n uint64
	switch {
	case !has:
		audio = nil
		n = (end - start + d - 1) / d
		end = start + n*d
	case first < start+2*d:
		return audio, nil // it starts with the video, to within a frame or so
	default:
		n = (first - start) / d
		end = first
	}
	fill, err := silentFragment(moof.Mfhd, rest, end-n*d, int(n))
	if err != nil {
		return nil, err
	}
	return append(fill, audio...), nil
}

// decodeSpan returns when tr's samples in moof decode, from the first to
// the end of the last, in tr's timescale.
func decodeSpan(moof *mp4.MoofBox, tr *track) (from, to uint64, ok bool) {
	if from, ok = leadDecodeTime(moof, tr.id); !ok {
		return 0, 0, false
	}
	to = from
	for _, traf := range moof.Trafs {
		if traf.Tfhd == nil || traf.Tfhd.TrackID != tr.id {
			continue
		}
		for _, trun := range traf.Truns {
			to += trun.AddSampleDefaultValues(traf.Tfhd, tr.trex)
		}
	}
	return from, to, true
}

// firstDecodeTimeOf returns when track id's first sample in moof decodes,
// and false if it has no samples there.
func firstDecodeTimeOf(moof *mp4.MoofBox, id uint32) (uint64, bool) {
	for _, traf := range moof.Trafs {
		if traf.Tfhd == nil || traf.Tfdt == nil || traf.Tfhd.TrackID != id {
			continue
		}
		for _, trun := range traf.Truns {
			if len(trun.Samples) > 0 {
				return traf.Tfdt.BaseMediaDecodeTime(), true
			}
		}
	}
	return 0, false
}

// rescale converts t from units of 1/from seconds to units of 1/to.
func rescale(t uint64, from, to uint32) uint64 {
	hi, lo := bits.Mul64(t, uint64(to))
	out, _ := bits.Div64(hi, lo, uint64(from))
	return out
}

// bitReader reads an MPEG bit string, most significant bit first.
type bitReader struct {
	b     []byte
	at    int
	short bool // a read ran past the end
}

func (r *bitReader) read(n int) uint64 {
	var v uint64
	for range n {
		if r.at >= 8*len(r.b) {
			r.short = true
			return 0
		}
		v = v<<1 | uint64(r.b[r.at/8]>>(7-r.at%8)&1)
		r.at++
	}
	return v
}

// bitWriter writes an MPEG bit string, most significant bit first.
type bitWriter struct {
	b []byte
	n int
}

func (w *bitWriter) write(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		w.b[w.n/8] |= byte(v>>i&1) << (7 - w.n%8)
		w.n++
	}
}

// bytes returns what was written, padded with zeros to a whole byte.
func (w *bitWriter) bytes() []byte { return w.b }
