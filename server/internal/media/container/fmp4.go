package container

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/mmcdole/kivo/internal/media"
)

// maxMdatGrow caps the upfront buffer pre-allocation for an mdat box. It
// avoids repeated doublings on normal segments while keeping a truncated
// stream from allocating maxMdat bytes up front.
const maxMdatGrow = 16 << 20

// Splitter cuts the fragmented MP4 a transcode writes into its
// initialization segment and media segments, one per fragment. ffmpeg
// starts a fragment at each video keyframe, so each segment begins on a
// keyframe. Each segment is returned as soon as its fragment is complete.
//
// When the output has video and audio, the audio is split from the video:
// it gets an initialization segment of its own, and each segment's audio
// is a fragment of its own, cut from the same fragment as the video. So
// the audio's segments cover the same spans as the video's.
type Splitter struct {
	r io.Reader

	init  []byte
	audio []byte   // the audio's initialization segment, when it's split from the video
	lead  *track   // the track whose fragments make segments, which is video if there is any
	rest  []*track // the tracks beside the lead, when the audio is split from the video
	trexs map[uint32]*mp4.TrexBox
	carry []byte // boxes read since the last segment, such as styp or a fragment without video
	// carryAudio is the audio of fragments read since the last segment.
	carryAudio []byte
	last       time.Duration
	err        error
}

// NewSplitter reads ffmpeg's output from r.
func NewSplitter(r io.Reader) *Splitter {
	return &Splitter{r: r, last: -1 << 63}
}

// Init returns the initialization segment, reading it first if needed. An
// HEVC track tagged hev1 is retagged hvc1, as Apple's players require. With
// stream copy, ffmpeg keeps whichever tag the source used.
func (s *Splitter) Init() (init []byte, err error) {
	if s.init != nil {
		return s.init, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	defer s.recoverPanic(&err)
	var raw []byte
	for {
		bx, body, err := s.readBox(maxInit)
		if err == io.EOF {
			err = corrupt("mp4: output ends before the initialization segment")
		}
		if err != nil {
			return nil, s.fail(err)
		}
		if bx.typ == "moof" || bx.typ == "mdat" {
			return nil, s.fail(corrupt("mp4: %s before moov", bx.typ))
		}
		if bx.typ != "moov" {
			raw = append(raw, body...)
			continue
		}
		moov, audio, err := s.readInit(body)
		if err != nil {
			return nil, s.fail(err)
		}
		if audio != nil {
			s.audio = append(slices.Clone(raw), audio...)
		}
		s.init = append(raw, moov...)
		return s.init, nil
	}
}

// AudioInit returns the audio's initialization segment, when the audio is
// split from the video, or nil. It's known once Init has returned.
func (s *Splitter) AudioInit() []byte { return s.audio }

// readInit finds the lead track and returns the moov, retagged if needed.
// When the audio is split from the video, it returns the video's moov and
// the audio's.
func (s *Splitter) readInit(b []byte) (moovB, audio []byte, err error) {
	bx, err := decode(b)
	if err != nil {
		return nil, nil, err
	}
	moov, ok := bx.(*mp4.MoovBox)
	if !ok {
		return nil, nil, corrupt("mp4: bad moov")
	}
	if s.lead, err = leadTrack(moov); err != nil {
		return nil, nil, err
	}
	if s.lead == nil {
		return nil, nil, corrupt("mp4: no tracks")
	}
	retagged := retagHEV1(moov)
	video, _, err := videoTrack(moov)
	switch {
	case err != nil:
		return nil, nil, err
	case video != nil && len(tracks(moov)) > 1:
		return s.splitInit(moov)
	case !retagged:
		return b, nil, nil
	}
	var out bytes.Buffer
	if err := moov.Encode(&out); err != nil {
		return nil, nil, corrupt("mp4: re-encode moov: %v", err)
	}
	return out.Bytes(), nil, nil
}

// retagHEV1 retags each HEVC track tagged hev1 as hvc1, and reports
// whether there was one.
func retagHEV1(moov *mp4.MoovBox) bool {
	retagged := false
	for _, trak := range moov.Traks {
		if trak.Mdia == nil || trak.Mdia.Minf == nil || trak.Mdia.Minf.Stbl == nil || trak.Mdia.Minf.Stbl.Stsd == nil {
			continue
		}
		if e := trak.Mdia.Minf.Stbl.Stsd.HvcX; e != nil && e.Type() == "hev1" {
			e.SetType("hvc1")
			retagged = true
		}
	}
	return retagged
}

// splitInit returns the video's moov and the audio's, the lead track and
// the rest, and keeps every track's defaults for splitting fragments.
func (s *Splitter) splitInit(moov *mp4.MoovBox) (video, audio []byte, err error) {
	s.trexs = map[uint32]*mp4.TrexBox{}
	if moov.Mvex != nil {
		for _, x := range moov.Mvex.Trexs {
			s.trexs[x.TrackID] = x
		}
	}
	isLead := func(id uint32) bool { return id == s.lead.id }
	for _, trak := range tracks(moov) {
		if isLead(trak.Tkhd.TrackID) {
			continue
		}
		tr, err := readTrack(moov, trak)
		if err != nil {
			return nil, nil, err
		}
		s.rest = append(s.rest, tr)
	}
	if video, err = moovOf(moov, isLead); err != nil {
		return nil, nil, err
	}
	if audio, err = moovOf(moov, func(id uint32) bool { return !isLead(id) }); err != nil {
		return nil, nil, err
	}
	return video, audio, nil
}

// Next returns the next segment, or io.EOF after the last.
func (s *Splitter) Next() (seg media.Segment, err error) {
	if s.err != nil {
		return media.Segment{}, s.err
	}
	if _, err := s.Init(); err != nil {
		return media.Segment{}, err
	}
	defer s.recoverPanic(&err)
	for {
		f, err := s.readFragment()
		if err == io.EOF {
			s.err = io.EOF
			return media.Segment{}, io.EOF
		}
		if err != nil {
			return media.Segment{}, s.fail(err)
		}
		if !f.lead {
			s.carry = append(s.carry, f.data...)
			s.carryAudio = append(s.carryAudio, f.audio...)
			continue
		}
		if len(s.carry) > 0 {
			f.data, s.carry = append(s.carry, f.data...), nil
		}
		if len(s.carryAudio) > 0 {
			f.audio, s.carryAudio = append(s.carryAudio, f.audio...), nil
		}
		return media.Segment{Start: f.start, Data: f.data, Audio: f.audio}, nil
	}
}

// fragment is one fragment as read: the boxes before it, its moof and its
// mdat.
type fragment struct {
	start time.Duration // the presentation time of its first lead-track sample, on the source's timeline
	lead  bool          // whether it has a lead-track sample
	data  []byte        // the fragment, or its video when the audio is split
	audio []byte        // its audio when the audio is split, which has no samples when the fragment has video but no audio
}

// readFragment reads boxes through the next mdat.
func (s *Splitter) readFragment() (fragment, error) {
	var data []byte
	for {
		// At the end, boxes after the last fragment (mfra) are dropped.
		bx, body, err := s.readBox(maxOther)
		if err != nil {
			return fragment{}, err
		}
		data = append(data, body...)
		if bx.typ != "moof" {
			continue
		}
		moof, err := decodeMoof(body)
		if err != nil {
			return fragment{}, err
		}
		f := fragment{}
		if f.start, f.lead, err = s.fragmentStart(moof); err != nil {
			return fragment{}, err
		}
		if f.data, err = s.readMdat(data); err != nil {
			return fragment{}, err
		}
		if s.audio != nil {
			before := len(data) - len(body) // the boxes before the moof
			video, audio, err := splitFragment(moof, f.data[before:], s.lead.id, s.trexs)
			if err == nil && audio == nil && f.lead {
				audio, err = emptyFragment(moof.Mfhd, f.start+media.TimestampOffset, s.rest)
			}
			if err != nil {
				return fragment{}, err
			}
			f.data, f.audio = append(f.data[:before:before], video...), audio
		}
		return f, nil
	}
}

func decodeMoof(b []byte) (*mp4.MoofBox, error) {
	bx, err := decode(b)
	if err != nil {
		return nil, err
	}
	moof, ok := bx.(*mp4.MoofBox)
	if !ok {
		return nil, corrupt("mp4: bad moof")
	}
	return moof, nil
}

// readMdat reads the mdat that follows a moof, and returns it after data.
func (s *Splitter) readMdat(data []byte) ([]byte, error) {
	mdat, err := s.readHeader()
	if err != nil {
		return nil, noEOF(err)
	}
	if mdat.typ != "mdat" {
		return nil, corrupt("mp4: %s after moof", mdat.typ)
	}
	if mdat.size > maxMdat {
		return nil, corrupt("mp4: mdat is %d bytes", mdat.size)
	}
	buf := bytes.NewBuffer(data)
	buf.Grow(int(min(mdat.size, maxMdatGrow)))
	if err := s.copyBody(buf, mdat); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fragmentStart reads the lead track's start from a moof.
func (s *Splitter) fragmentStart(moof *mp4.MoofBox) (time.Duration, bool, error) {
	for smp := range s.lead.samples(moof) {
		start := s.lead.pts(smp.dts, smp.CompositionTimeOffset) - media.TimestampOffset
		if start <= s.last {
			return 0, false, corrupt("mp4: fragment at %v follows %v", start, s.last)
		}
		s.last = start
		return start, true, nil
	}
	return 0, false, nil
}

// readBox reads a whole box of at most limit bytes, header included.
func (s *Splitter) readBox(limit int64) (box, []byte, error) {
	bx, err := s.readHeader()
	if err != nil {
		return box{}, nil, err
	}
	if bx.typ == "moof" {
		limit = maxMoof
	}
	if bx.size > limit {
		return box{}, nil, corrupt("mp4: %s is %d bytes", bx.typ, bx.size)
	}
	var b bytes.Buffer
	if err := s.copyBody(&b, bx); err != nil {
		return box{}, nil, err
	}
	return bx, b.Bytes(), nil
}

// readHeader reads a box header. It returns io.EOF only between boxes.
func (s *Splitter) readHeader() (box, error) {
	var h [16]byte
	if _, err := io.ReadFull(s.r, h[:8]); err != nil {
		return box{}, err
	}
	n := 8
	if binary.BigEndian.Uint32(h[:]) == 1 {
		if _, err := io.ReadFull(s.r, h[8:]); err != nil {
			return box{}, noEOF(err)
		}
		n = 16
	}
	bx, err := parseBoxHeader(h[:n])
	if err != nil {
		return box{}, err
	}
	if bx.size == -1 {
		return box{}, corrupt("mp4: %s box runs to the end of the stream", bx.typ)
	}
	return bx, nil
}

// copyBody writes the box's header to w, followed by its body read from the
// stream. The body is copied as it arrives, so a short stream costs only
// what it holds.
func (s *Splitter) copyBody(w io.Writer, bx box) error {
	h := make([]byte, bx.hdr)
	binary.BigEndian.PutUint32(h, uint32(bx.size))
	copy(h[4:], bx.typ)
	if bx.hdr == 16 {
		binary.BigEndian.PutUint32(h, 1)
		binary.BigEndian.PutUint64(h[8:], uint64(bx.size))
	}
	if _, err := w.Write(h); err != nil {
		return err
	}
	if _, err := io.CopyN(w, s.r, bx.size-bx.hdr); err != nil {
		return noEOF(err)
	}
	return nil
}

// fail records err. A Splitter that has failed keeps failing.
func (s *Splitter) fail(err error) error {
	s.err = noEOF(err)
	return s.err
}

// recoverPanic turns a panic in mp4ff into media.ErrSourceCorrupt.
func (s *Splitter) recoverPanic(err *error) {
	if p := recover(); p != nil {
		*err = s.fail(corrupt("mp4: %v", p))
	}
}

// noEOF reports a stream that ends inside a box as truncated, not as io.EOF.
func noEOF(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return corrupt("mp4: output ends inside a box")
	}
	return err
}
