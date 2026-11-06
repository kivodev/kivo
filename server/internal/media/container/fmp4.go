package container

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/mmcdole/kivo/internal/media"
)

// tolerance absorbs rounding between timescales when comparing times.
const tolerance = time.Millisecond

// Splitter cuts the fragmented MP4 a transcode writes into its
// initialization segment and media segments, one per fragment. ffmpeg
// starts a fragment at each video keyframe, so each segment begins on a
// keyframe.
//
// When seeking a copied stream, ffmpeg may start at a keyframe before the
// requested start. A fragment that begins before the start is held until
// the next fragment shows whether it contains the start. Otherwise each
// segment is returned as soon as its fragment is complete.
//
// A Splitter that cuts is given a start on a keyframe of the lead track,
// and its first segment begins there. A held fragment that contains the
// start, because ffmpeg ran the fragment it began with on past it, is cut
// at that keyframe rather than returned whole.
type Splitter struct {
	r     io.Reader
	start time.Duration
	cut   bool
	near  time.Duration // how close to start a fragment begins to count as beginning there

	init    []byte
	lead    *track            // the track whose fragments make segments, which is video if there is any
	tracks  map[uint32]*track // every track by its ID, read only by a Splitter that cuts
	carry   []byte            // boxes read since the last segment, such as styp or a fragment without video
	held    *media.Segment
	reached bool // a segment at or after start has been seen
	queue   []media.Segment
	last    time.Duration
	err     error
}

// NewSplitter reads ffmpeg's output from r. Segments begin with the one
// that contains start. With cut, start is a keyframe of the lead track, and
// segments begin there: a fragment that begins earlier but runs past it is
// cut at it, and output with no keyframe at start is an error.
func NewSplitter(r io.Reader, start time.Duration, cut bool) *Splitter {
	near := tolerance
	if cut {
		near = cutSlack
	}
	return &Splitter{r: r, start: start, cut: cut, near: near, last: -1 << 63}
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
		moov, err := s.readInit(body)
		if err != nil {
			return nil, s.fail(err)
		}
		s.init = append(raw, moov...)
		return s.init, nil
	}
}

// readInit finds the lead track and returns the moov, retagged if needed.
func (s *Splitter) readInit(b []byte) ([]byte, error) {
	bx, err := decode(b)
	if err != nil {
		return nil, err
	}
	moov, ok := bx.(*mp4.MoovBox)
	if !ok {
		return nil, corrupt("mp4: bad moov")
	}
	if s.lead, err = leadTrack(moov); err != nil {
		return nil, err
	}
	if s.lead == nil {
		return nil, corrupt("mp4: no tracks")
	}
	if s.tracks, err = s.trackTimings(moov); err != nil {
		return nil, err
	}
	retag := false
	for _, trak := range moov.Traks {
		if trak.Mdia == nil || trak.Mdia.Minf == nil || trak.Mdia.Minf.Stbl == nil || trak.Mdia.Minf.Stbl.Stsd == nil {
			continue
		}
		if e := trak.Mdia.Minf.Stbl.Stsd.HvcX; e != nil && e.Type() == "hev1" {
			e.SetType("hvc1")
			retag = true
		}
	}
	if !retag {
		return b, nil
	}
	var out bytes.Buffer
	if err := moov.Encode(&out); err != nil {
		return nil, corrupt("mp4: re-encode moov: %v", err)
	}
	return out.Bytes(), nil
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
	for len(s.queue) == 0 {
		start, lead, data, err := s.readFragment()
		if err == io.EOF {
			if s.held == nil {
				s.err = io.EOF
				return media.Segment{}, io.EOF
			}
			// The start lies in the last fragment.
			if err := s.release(); err != nil {
				return media.Segment{}, s.fail(err)
			}
			break
		}
		if err != nil {
			return media.Segment{}, s.fail(err)
		}
		if !lead {
			s.carry = append(s.carry, data...)
			continue
		}
		if len(s.carry) > 0 {
			data, s.carry = append(s.carry, data...), nil
		}
		if err := s.place(media.Segment{Start: start, Data: data}); err != nil {
			return media.Segment{}, s.fail(err)
		}
	}
	seg = s.queue[0]
	s.queue = s.queue[1:]
	return seg, nil
}

// place queues a complete fragment. A fragment that begins before the
// requested start is held or dropped instead.
func (s *Splitter) place(seg media.Segment) error {
	switch {
	case s.reached:
	case seg.Start >= s.start-tolerance:
		if s.held != nil && seg.Start > s.start+s.near {
			if err := s.release(); err != nil {
				return err
			}
		}
		s.held, s.reached = nil, true
	default:
		s.held = &seg // replaces a held fragment that ended before the start
		return nil
	}
	s.queue = append(s.queue, seg)
	return nil
}

// release queues the held fragment, which contains the start. A Splitter
// that cuts queues it from the start's keyframe on.
func (s *Splitter) release() error {
	seg := *s.held
	s.held = nil
	if s.cut {
		var err error
		if seg.Data, seg.Start, err = s.cutAt(seg.Data, s.start); err != nil {
			return err
		}
	}
	s.queue = append(s.queue, seg)
	return nil
}

// readFragment reads boxes through the next mdat. It returns the
// presentation time of the fragment's first lead-track sample, on the
// source's timeline, and whether there is one.
func (s *Splitter) readFragment() (time.Duration, bool, []byte, error) {
	var data []byte
	for {
		// At the end, boxes after the last fragment (mfra) are dropped.
		bx, body, err := s.readBox(maxOther)
		if err != nil {
			return 0, false, nil, err
		}
		data = append(data, body...)
		if bx.typ != "moof" {
			continue
		}
		start, lead, err := s.fragmentStart(body)
		if err != nil {
			return 0, false, nil, err
		}
		mdat, err := s.readHeader()
		if err != nil {
			return 0, false, nil, noEOF(err)
		}
		if mdat.typ != "mdat" {
			return 0, false, nil, corrupt("mp4: %s after moof", mdat.typ)
		}
		if mdat.size > maxMdat {
			return 0, false, nil, corrupt("mp4: mdat is %d bytes", mdat.size)
		}
		buf := bytes.NewBuffer(data)
		if err := s.copyBody(buf, mdat); err != nil {
			return 0, false, nil, err
		}
		return start, lead, buf.Bytes(), nil
	}
}

// fragmentStart reads the lead track's start from a moof.
func (s *Splitter) fragmentStart(b []byte) (time.Duration, bool, error) {
	bx, err := decode(b)
	if err != nil {
		return 0, false, err
	}
	moof, ok := bx.(*mp4.MoofBox)
	if !ok {
		return 0, false, corrupt("mp4: bad moof")
	}
	for smp := range s.lead.samples(moof) {
		start := s.lead.onTimeline(smp.dts, smp.CompositionTimeOffset)
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
