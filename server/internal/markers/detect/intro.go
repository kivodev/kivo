package detect

import (
	"strconv"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

// intro returns the intro the season's heads give a file. It is the
// stretch that at least two other episodes share with the file, moved onto
// the file's timeline and snapped to its start, its chapters and its
// silences.
func (f File) intro(intros map[id.ID]found) *guess {
	m, ok := intros[f.ID]
	if !ok || m.partners < 2 || f.Fingerprint == nil || f.Fingerprint.Head == nil {
		return nil
	}
	s := span{m.start + introStartLead, m.end + introEndLead}
	if s.start <= zeroSnap {
		s.start = 0
	}
	s.start = snap(s.start, f.Chapters, f.Fingerprint.Head.Silences, false)
	s.end = snap(s.end, f.Chapters, f.Fingerprint.Head.Silences, true)
	if s.length() < introLimits.min || s.length() > introLimits.max {
		return nil
	}
	return &guess{span: s, reason: "fingerprint, shared with " + strconv.Itoa(m.partners) + " other episodes"}
}

// snap moves a boundary to a chapter's start within snapWithin of it.
// Failing that, it moves the boundary to the edge of a silence within
// snapWithin. A start snaps to a silence's end, and an end snaps to a
// silence's start.
func snap(t time.Duration, chapters []time.Duration, silences []media.Span, end bool) time.Duration {
	if t == 0 {
		return t
	}
	best, gap := t, snapWithin+1
	for _, c := range chapters {
		if d := (c - t).Abs(); d <= snapWithin && d < gap {
			best, gap = c, d
		}
	}
	if gap <= snapWithin {
		return best
	}
	for _, s := range silences {
		edge := s.End
		if end {
			edge = s.Start
		}
		if d := (edge - t).Abs(); d <= snapWithin && d < gap {
			best, gap = edge, d
		}
	}
	return best
}
