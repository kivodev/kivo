package detect

import (
	"cmp"
	"maps"
	"math/bits"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

// These constants control how two fingerprints are matched.
//
// A point matches another point at most maxBitErrors bits away. Every
// shiftSample-th point of one fingerprint votes for the offsets at which
// the other fingerprint has a matching point. The maxShifts offsets with
// the most votes are each walked. A run is the points that match within
// drift points of the offset, with no gap in it longer than maxGap.
const (
	maxBitErrors = 6
	shiftSample  = 8
	maxShifts    = 8
	minVotes     = 3
	drift        = 2
	maxGap       = 3500 * time.Millisecond
	minDensity   = 0.5
)

// These constants control how a season's fingerprints are compared.
//
// Each episode is compared with the next neighbors episodes. An episode
// that none of those matched is compared with up to fallback more, nearest
// first.
//
// A file's match is the median of its partners' matches that agree with
// the match most partners agree with. Two matches agree when they overlap
// by at least agreeOverlap of their union. Each partner episode gets one
// vote, however many versions it has.
const (
	neighbors    = 8
	fallback     = 48
	agreeOverlap = 0.5
)

// matchLimits bound the length of a run two files share.
type matchLimits struct {
	min, max time.Duration
}

// span is a stretch of time. While matching, it is measured from the start
// of a fingerprint. Afterwards, it is on the file's timeline.
type span struct {
	start, end time.Duration
}

func (s span) length() time.Duration { return s.end - s.start }

// overlap returns how much of two spans' union they share, from 0 to 1.
func overlap(a, b span) float64 {
	shared := min(a.end, b.end) - max(a.start, b.start)
	if shared <= 0 {
		return 0
	}
	return float64(shared) / float64(max(a.end, b.end)-min(a.start, b.start))
}

// near reports whether two fingerprint points match.
func near(a, b uint32) bool { return bits.OnesCount32(a^b) <= maxBitErrors }

// steady is how many points back a point is compared with to detect steady
// sound. Silence, hum and noise have points that barely change, and those
// points match any other file's.
const steady = 4

// telling reports whether the i-th point of a fingerprint differs from the
// point steady points before it, which means it tells one sound from
// another. Steady points neither vote nor match, but a run carries on over
// them.
func telling(points []uint32, i int) bool {
	return i >= steady && !near(points[i], points[i-steady])
}

// pair finds the longest run of audio two fingerprints share within lim. It
// returns the run as a span of each fingerprint.
func pair(a, b []uint32, lim matchLimits) (sa, sb span, ok bool) {
	var best int
	for _, shift := range shifts(a, b) {
		first, last, firstB, lastB := run(a, b, shift)
		if n := last - first; first >= 0 && n > best {
			length := time.Duration(n+1) * media.PointStep
			if length < lim.min || length > lim.max {
				continue
			}
			best = n
			sa = span{time.Duration(first) * media.PointStep, time.Duration(last+1) * media.PointStep}
			sb = span{time.Duration(firstB) * media.PointStep, time.Duration(lastB+1) * media.PointStep}
			ok = true
		}
	}
	return sa, sb, ok
}

// shifts returns the offsets of b against a that the most points vote for,
// most votes first. An offset's votes include the votes for offsets within
// drift of it, and it needs at least minVotes. No two returned offsets are
// within drift of each other.
func shifts(a, b []uint32) []int {
	votes := map[int]int{}
	for i := 0; i < len(a); i += shiftSample {
		if !telling(a, i) {
			continue
		}
		for j, p := range b {
			if near(a[i], p) && telling(b, j) {
				votes[j-i]++
			}
		}
	}
	type vote struct{ shift, n int }
	var all []vote
	for s := range votes {
		n := 0
		for d := -drift; d <= drift; d++ {
			n += votes[s+d]
		}
		if n >= minVotes {
			all = append(all, vote{s, n})
		}
	}
	slices.SortFunc(all, func(x, y vote) int {
		return cmp.Or(y.n-x.n, abs(x.shift)-abs(y.shift), x.shift-y.shift)
	})
	var out []int
	for _, v := range all {
		if len(out) == maxShifts {
			break
		}
		if !slices.ContainsFunc(out, func(s int) bool { return abs(s-v.shift) <= drift }) {
			out = append(out, v.shift)
		}
	}
	return out
}

// run walks a against b at shift and returns the longest run in which at
// least minDensity of the telling points match. It returns the run's first
// and last points in a, and the points of b they matched. first is -1 when
// nothing matched. The density rule stops sparse chance matches from
// forming a run, which a gap allowance alone would chain together.
func run(a, b []uint32, shift int) (first, last, firstB, lastB int) {
	gap := int(maxGap / media.PointStep)
	first, last = -1, -1
	var cur struct{ start, startB, end, endB, matched, telling, pending int }
	cur.start = -1
	keep := func() {
		if cur.start >= 0 && cur.end-cur.start > last-first && float64(cur.matched) >= minDensity*float64(cur.telling) {
			first, last, firstB, lastB = cur.start, cur.end, cur.startB, cur.endB
		}
	}
	for i, p := range a {
		if !telling(a, i) {
			continue
		}
		j := match(p, b, i+shift)
		if j < 0 || !telling(b, j) {
			cur.pending++
			continue
		}
		if cur.start < 0 || i-cur.end > gap {
			keep()
			cur.start, cur.startB, cur.matched, cur.telling, cur.pending = i, j, 0, 0, 0
		}
		cur.end, cur.endB = i, j
		cur.matched++
		cur.telling += cur.pending + 1
		cur.pending = 0
	}
	keep()
	return first, last, firstB, lastB
}

// match returns the point of b within drift of at that matches p, trying
// the nearest first. It returns -1 if none matches.
func match(p uint32, b []uint32, at int) int {
	for d := 0; d <= drift; d++ {
		for _, j := range []int{at - d, at + d} {
			if j >= 0 && j < len(b) && near(p, b[j]) {
				return j
			}
		}
	}
	return -1
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// print is one file's fingerprint of a window, in a season.
type print struct {
	file id.ID
	// episode is the episode's place among the season's episodes that
	// have files, counting from 0. All versions of an episode share it.
	episode int
	points  []uint32
}

// found is a file's match across its season. Its span is measured from the
// start of the file's fingerprint, and partners counts the partner
// episodes that agree on it.
type found struct {
	span
	partners int
}

// season matches a season's fingerprints in episode order, and returns
// each matched file's consensus. The constants beside neighbors describe
// which episodes are compared.
func season(prints []print, lim matchLimits) map[id.ID]found {
	prints = slices.Clone(prints)
	slices.SortStableFunc(prints, func(a, b print) int { return a.episode - b.episode })
	results := map[id.ID]map[int][]span{} // by file, then partner episode
	done := map[[2]int]bool{}
	compare := func(i, j int) {
		i, j = min(i, j), max(i, j)
		if done[[2]int{i, j}] || prints[i].episode == prints[j].episode {
			return
		}
		done[[2]int{i, j}] = true
		sa, sb, ok := pair(prints[i].points, prints[j].points, lim)
		if !ok {
			return
		}
		record(results, prints[i].file, prints[j].episode, sa)
		record(results, prints[j].file, prints[i].episode, sb)
	}
	for i, p := range prints {
		for j := i + 1; j < len(prints) && prints[j].episode-p.episode <= neighbors; j++ {
			compare(i, j)
		}
	}
	matched := map[id.ID]bool{}
	for f := range results {
		matched[f] = true
	}
	for i, p := range prints {
		if matched[p.file] {
			continue
		}
		for _, j := range nearest(prints, i) {
			compare(i, j)
		}
	}
	out := map[id.ID]found{}
	for f, byPartner := range results {
		if c, ok := consensus(byPartner); ok {
			out[f] = c
		}
	}
	return out
}

func record(results map[id.ID]map[int][]span, file id.ID, partner int, s span) {
	if results[file] == nil {
		results[file] = map[int][]span{}
	}
	results[file][partner] = append(results[file][partner], s)
}

// nearest returns the files of the episodes within reach of the i-th
// file's episode, nearest first. The reach is neighbors plus fallback
// episodes.
func nearest(prints []print, i int) []int {
	ep := prints[i].episode
	var out []int
	for j := range prints {
		if d := abs(prints[j].episode - ep); d > 0 && d <= neighbors+fallback {
			out = append(out, j)
		}
	}
	slices.SortStableFunc(out, func(a, b int) int { return abs(prints[a].episode-ep) - abs(prints[b].episode-ep) })
	return out
}

// consensus reduces a file's matches, keyed by partner episode, to the span
// most partners agree on. The anchor is the match that the most partners
// have an overlapping match for, with the longer one winning a tie. The
// result is the median of each agreeing partner's closest match to the
// anchor.
func consensus(byPartner map[int][]span) (found, bool) {
	partners := slices.Sorted(maps.Keys(byPartner))
	agree := func(s span) int {
		n := 0
		for _, p := range partners {
			if slices.ContainsFunc(byPartner[p], func(o span) bool { return overlap(s, o) >= agreeOverlap }) {
				n++
			}
		}
		return n
	}
	var anchor span
	votes := 0
	for _, p := range partners {
		for _, s := range byPartner[p] {
			if n := agree(s); n > votes || n == votes && s.length() > anchor.length() {
				anchor, votes = s, n
			}
		}
	}
	if votes == 0 {
		return found{}, false
	}
	var starts, ends []time.Duration
	for _, p := range partners {
		if best, ok := closest(byPartner[p], anchor); ok {
			starts, ends = append(starts, best.start), append(ends, best.end)
		}
	}
	return found{span: span{median(starts), median(ends)}, partners: len(starts)}, true
}

// closest returns the span that overlaps anchor most, if it overlaps by at
// least agreeOverlap.
func closest(spans []span, anchor span) (span, bool) {
	best, most := span{}, 0.0
	for _, s := range spans {
		if o := overlap(anchor, s); o > most {
			best, most = s, o
		}
	}
	return best, most >= agreeOverlap
}

// median returns the middle value, or the mean of the middle two.
func median(v []time.Duration) time.Duration {
	v = slices.Clone(v)
	slices.Sort(v)
	if n := len(v); n%2 == 0 {
		return (v[n/2-1] + v[n/2]) / 2
	}
	return v[len(v)/2]
}
