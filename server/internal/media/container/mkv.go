package container

import (
	"io"
	"math"
	"math/bits"
	"time"
)

// Matroska element IDs, with their length markers, as the spec writes them.
const (
	idEBML           = 0x1a45dfa3
	idSegment        = 0x18538067
	idSeekHead       = 0x114d9b74
	idSeek           = 0x4dbb
	idSeekID         = 0x53ab
	idSeekPosition   = 0x53ac
	idInfo           = 0x1549a966
	idTimestampScale = 0x2ad7b1
	idTracks         = 0x1654ae6b
	idTrackEntry     = 0xae
	idTrackNumber    = 0xd7
	idTrackType      = 0x83
	idCues           = 0x1c53bb6b
	idCuePoint       = 0xbb
	idCueTime        = 0xb3
	idCueTrackPos    = 0xb7
	idCueTrack       = 0xf7
	idCluster        = 0x1f43b675
	idAttachments    = 0x1941a469
	idAttachedFile   = 0x61a7
	idFileName       = 0x466e
	idFileMediaType  = 0x4660
	idFileData       = 0x465c
)

const (
	trackTypeVideo   = 1
	defaultTimescale = 1_000_000 // nanoseconds per timestamp tick
	unknownSize      = -1
	maxSeekHeads     = 4 // a SeekHead may point to another; real files use two at most
)

// maxElement is the largest size of each element the reader will load.
var maxElement = map[uint32]int64{
	idSeekHead: 1 << 20,
	idInfo:     1 << 20,
	idTracks:   16 << 20,
	idCues:     64 << 20,
}

// element is an EBML element located in the file.
type element struct {
	id        uint32
	data, end int64 // end is unknownSize for a live-streamed element
}

// mkv holds what the reader has learned about one Matroska segment.
type mkv struct {
	r         io.ReaderAt
	base, end int64 // the bounds of the segment's data
	timescale uint64
	video     uint64          // the first video track's number
	cues      []byte          // the Cues body, parsed once the video track is known
	want      map[uint32]bool // the level-1 elements to load besides SeekHeads
	loaded    map[uint32]bool
	seek      map[uint32][]int64 // absolute positions the SeekHeads point to
	seen      map[uint32]int64   // positions of level-1 elements read before the first Cluster
	visited   map[int64]bool
}

// openMKV reads a segment's level-1 elements up to the first Cluster,
// loading the SeekHeads and the elements in want. It then follows the
// SeekHeads to any wanted elements it hasn't seen. It never reads clusters.
func openMKV(r io.ReaderAt, size int64, want ...uint32) (*mkv, error) {
	head, err := readElement(r, 0, size)
	if err != nil {
		return nil, err
	}
	if head.id != idEBML || head.end == unknownSize {
		return nil, corrupt("mkv: no EBML header")
	}
	// A partly written file is still a segment, so read what's there.
	seg, err := readElement(r, head.end, math.MaxInt64)
	if err != nil {
		return nil, err
	}
	if seg.id != idSegment {
		return nil, corrupt("mkv: no segment")
	}
	m := &mkv{r: r, base: seg.data, end: size, timescale: defaultTimescale, want: map[uint32]bool{idSeekHead: true},
		loaded: map[uint32]bool{}, seek: map[uint32][]int64{}, seen: map[uint32]int64{}, visited: map[int64]bool{}}
	for _, id := range want {
		m.want[id] = true
	}
	if seg.end != unknownSize {
		m.end = min(seg.end, size)
	}

	for off := m.base; off < m.end; {
		e, err := readElement(r, off, m.end)
		if err != nil {
			return nil, err
		}
		if e.id == idCluster || e.end == unknownSize {
			break
		}
		if _, ok := m.seen[e.id]; !ok {
			m.seen[e.id] = off
		}
		if err := m.load(off, e); err != nil {
			return nil, err
		}
		off = e.end
	}
	for i := 0; i < len(m.seek[idSeekHead]) && i < maxSeekHeads; i++ {
		if err := m.loadAt(idSeekHead, m.seek[idSeekHead][i]); err != nil {
			return nil, err
		}
	}
	for _, id := range want {
		if !m.loaded[id] && len(m.seek[id]) > 0 {
			if err := m.loadAt(id, m.seek[id][0]); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

// at returns where a level-1 element is. That is where the walk to the
// first Cluster found it, or else where a SeekHead points.
func (m *mkv) at(id uint32) (int64, bool) {
	if off, ok := m.seen[id]; ok {
		return off, true
	}
	if len(m.seek[id]) > 0 {
		return m.seek[id][0], true
	}
	return 0, false
}

// mkvKeyframes reads the Cues of the first video track.
func mkvKeyframes(r io.ReaderAt, size int64) ([]time.Duration, bool, error) {
	m, err := openMKV(r, size, idInfo, idTracks, idCues)
	if err != nil {
		return nil, false, err
	}
	if m.video == 0 || m.cues == nil {
		return nil, false, nil
	}
	out, err := m.parseCues()
	return out, err == nil, err
}

// loadAt loads the element a SeekHead says is at off, if its ID is id.
func (m *mkv) loadAt(id uint32, off int64) error {
	if off < m.base || off >= m.end {
		return nil // a stale SeekHead entry means the index is simply missing
	}
	e, err := readElement(m.r, off, m.end)
	if err != nil {
		return err
	}
	if e.id != id || e.end == unknownSize {
		return nil
	}
	return m.load(off, e)
}

// load reads and parses a level-1 element the reader cares about.
func (m *mkv) load(off int64, e element) error {
	limit, ok := maxElement[e.id]
	if !ok || !m.want[e.id] || m.visited[off] {
		return nil
	}
	m.visited[off] = true
	if e.end-e.data > limit {
		return corrupt("mkv: element %x is %d bytes", e.id, e.end-e.data)
	}
	b, err := readAt(m.r, e.data, e.end-e.data)
	if err != nil {
		return err
	}
	m.loaded[e.id] = true
	switch e.id {
	case idSeekHead:
		return m.parseSeekHead(b)
	case idInfo:
		return m.parseInfo(b)
	case idTracks:
		return m.parseTracks(b)
	default:
		m.cues = b
		return nil
	}
}

func (m *mkv) parseSeekHead(b []byte) error {
	return children(b, func(id uint32, b []byte) error {
		if id != idSeek {
			return nil
		}
		var target uint32
		var pos uint64
		err := children(b, func(id uint32, b []byte) error {
			var err error
			switch id {
			case idSeekID:
				var v uint64
				v, err = ebmlUint(b)
				target = uint32(v)
			case idSeekPosition:
				pos, err = ebmlUint(b)
			}
			return err
		})
		if err == nil && pos < uint64(m.end-m.base) {
			m.seek[target] = append(m.seek[target], m.base+int64(pos))
		}
		return err
	})
}

func (m *mkv) parseInfo(b []byte) error {
	return children(b, func(id uint32, b []byte) error {
		if id != idTimestampScale {
			return nil
		}
		v, err := ebmlUint(b)
		if err == nil && v == 0 {
			return corrupt("mkv: zero timestamp scale")
		}
		m.timescale = v
		return err
	})
}

func (m *mkv) parseTracks(b []byte) error {
	return children(b, func(id uint32, b []byte) error {
		if id != idTrackEntry || m.video != 0 {
			return nil
		}
		var num, kind uint64
		err := children(b, func(id uint32, b []byte) error {
			var err error
			switch id {
			case idTrackNumber:
				num, err = ebmlUint(b)
			case idTrackType:
				kind, err = ebmlUint(b)
			}
			return err
		})
		if kind == trackTypeVideo {
			m.video = num
		}
		return err
	})
}

// parseCues returns the times of the cue points for the video track.
func (m *mkv) parseCues() ([]time.Duration, error) {
	var out []time.Duration
	err := children(m.cues, func(id uint32, b []byte) error {
		if id != idCuePoint {
			return nil
		}
		var t uint64
		var video bool
		err := children(b, func(id uint32, b []byte) error {
			var err error
			switch id {
			case idCueTime:
				t, err = ebmlUint(b)
			case idCueTrackPos:
				err = children(b, func(id uint32, b []byte) error {
					if id != idCueTrack {
						return nil
					}
					n, err := ebmlUint(b)
					video = video || n == m.video
					return err
				})
			}
			return err
		})
		switch {
		case err != nil:
			return err
		case !video:
			return nil
		case len(out) == maxKeyframes:
			return errTooMany
		case t > math.MaxInt64/m.timescale:
			return corrupt("mkv: cue time %d overflows", t)
		}
		out = append(out, time.Duration(t*m.timescale))
		return nil
	})
	return out, err
}

// children calls fn for each child element in b, which is a master
// element's body.
func children(b []byte, fn func(id uint32, body []byte) error) error {
	for len(b) > 0 {
		id, n, err := ebmlID(b)
		if err != nil {
			return err
		}
		size, k, err := ebmlSize(b[n:])
		if err != nil {
			return err
		}
		b = b[n+k:]
		if size == unknownSize || size > int64(len(b)) {
			return corrupt("mkv: element %x overruns its parent", id)
		}
		if err := fn(id, b[:size]); err != nil {
			return err
		}
		b = b[size:]
	}
	return nil
}

// readElement reads the header of the element at off. The element must end
// by limit.
func readElement(r io.ReaderAt, off, limit int64) (element, error) {
	n := min(int64(12), limit-off) // a 4-byte ID and an 8-byte size at most
	if n < 2 {
		return element{}, corrupt("mkv: truncated at %d", off)
	}
	b := make([]byte, n)
	k, err := r.ReadAt(b, off)
	if k < len(b) && err != io.EOF {
		return element{}, unreadable(err)
	}
	b = b[:k]
	id, i, err := ebmlID(b)
	if err != nil {
		return element{}, err
	}
	size, j, err := ebmlSize(b[i:])
	if err != nil {
		return element{}, err
	}
	e := element{id: id, data: off + int64(i+j), end: unknownSize}
	if size != unknownSize {
		if size > limit-e.data {
			return element{}, corrupt("mkv: element %x at %d overruns its parent", id, off)
		}
		e.end = e.data + size
	}
	return e, nil
}

// vint decodes an EBML variable-length integer, whose length is one more
// than the number of leading zero bits in its first byte.
func vint(b []byte) (v uint64, n int, err error) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, corrupt("mkv: bad variable-length integer")
	}
	n = bits.LeadingZeros8(b[0]) + 1
	if len(b) < n {
		return 0, 0, corrupt("mkv: truncated variable-length integer")
	}
	for _, c := range b[:n] {
		v = v<<8 | uint64(c)
	}
	return v, n, nil
}

// ebmlID reads an element ID, which keeps its length marker.
func ebmlID(b []byte) (uint32, int, error) {
	v, n, err := vint(b)
	if err == nil && n > 4 {
		err = corrupt("mkv: element ID longer than 4 bytes")
	}
	return uint32(v), n, err
}

// ebmlSize reads an element size, returning unknownSize when every value
// bit is set.
func ebmlSize(b []byte) (int64, int, error) {
	v, n, err := vint(b)
	if err != nil {
		return 0, 0, err
	}
	mask := uint64(1)<<(7*n) - 1
	v &= mask
	if v == mask {
		return unknownSize, n, nil
	}
	return int64(v), n, nil
}

// ebmlUint reads an unsigned integer element's body.
func ebmlUint(b []byte) (uint64, error) {
	if len(b) > 8 {
		return 0, corrupt("mkv: integer longer than 8 bytes")
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v, nil
}
