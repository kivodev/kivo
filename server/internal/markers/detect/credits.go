package detect

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

// A keyframe of a file's tail is credits-like when it shows text on true
// black, true black alone, or text on a flat card. The thresholds are on
// 8-bit luma, measured against the tail's own black level. The black level
// is the 10th percentile of the tail's darkest keyframes, capped at
// maxBlackLevel, so a tail with no black doesn't count a dim scene as
// black.
const (
	maxBlackLevel = 30
	// A black background has these properties:
	//   - its 10th percentile of luma is within blackMargin of the black
	//     level;
	//   - it is near-grey: its 10th percentile of saturation is under
	//     blackSaturation, which keeps a blue night scene out;
	//   - at least blackCover percent of it is below luma 32, and
	//     strictCover percent is below the level about four above black.
	// Dark scenes pass the loose share but not the strict one.
	blackMargin     = 2
	blackSaturation = 10
	blackCover      = 85
	strictCover     = 75
	// mostlyCover is the share below luma 32 for dense credits or logos on
	// black, which pass every other test. Such keyframes join a run of
	// text, but never start or end one, and don't count toward its text.
	mostlyCover = 70
	// textContrast is how much brighter than the background text is.
	textContrast = 60
	// A card has a flat background, with its 10th and 90th percentiles
	// within cardFlat. The background is lighter than black by at least
	// cardLift. Its text is textContrast brighter than the background, and
	// its saturation is modest.
	cardFlat       = 8
	cardLift       = 24
	cardSaturation = 96
)

// A run of credits starts at a text keyframe. It carries on over
// credits-like keyframes no more than runGap apart, until one shows story,
// and ends at its last text keyframe. A run counts when it lasts at least
// minCredits, holds at least minRunText text keyframes, and text is most of
// its keyframes, not counting mostly black ones.
const (
	runGap     = 20 * time.Second
	minRunText = 3
	minCredits = 15 * time.Second
	maxCredits = 450 * time.Second
)

// look is what a keyframe of a tail shows.
type look int

const (
	story    look = iota
	black         // true black, no text
	lettered      // text on true black
	card          // text on a flat, lighter background
	mostly        // black with too much on it to call black
)

func (l look) text() bool        { return l == lettered || l == card }
func (l look) creditsLike() bool { return l != story }

// keyframe is a classified tail keyframe, with whether other episodes
// repeat it (see repeats).
type keyframe struct {
	at       time.Duration
	look     look
	repeated bool
}

// classify classifies a tail's keyframes against its black level.
func classify(frames []media.KeyframeStats) []keyframe {
	if len(frames) == 0 {
		return nil
	}
	lows := make([]float32, len(frames))
	for i, f := range frames {
		lows[i] = f.YLow
	}
	slices.Sort(lows)
	level := min(lows[max(0, len(lows)/100-1)], maxBlackLevel)
	out := make([]keyframe, len(frames))
	for i, f := range frames {
		out[i] = keyframe{at: f.At, look: lookOf(f, level)}
	}
	return out
}

// lookOf returns what one keyframe shows, judged against a black level.
func lookOf(f media.KeyframeStats, level float32) look {
	background := onBlack(f, level)
	switch {
	case background && f.Below[2] >= blackCover && f.YMax-f.YLow >= textContrast:
		return lettered
	case background && f.Below[2] >= blackCover:
		return black
	case onCard(f, level):
		return card
	case background && f.Below[2] >= mostlyCover:
		return mostly
	}
	return story
}

// onBlack reports whether a keyframe's background is true black for a
// black level. The caller checks how much of the keyframe it covers.
func onBlack(f media.KeyframeStats, level float32) bool {
	strict := f.Below[2]
	switch {
	case level <= 16:
		strict = f.Below[0]
	case level <= 22:
		strict = f.Below[1]
	}
	return f.YLow <= level+blackMargin && f.SatLow < blackSaturation && strict >= strictCover
}

// onCard reports whether a keyframe is text on a flat card.
func onCard(f media.KeyframeStats, level float32) bool {
	return f.YHigh-f.YLow <= cardFlat && max(f.YMax-f.YHigh, f.YLow-f.YMin) >= textContrast &&
		f.SatAvg < cardSaturation && f.YLow >= level+cardLift
}

// A tail keyframe is repeated when two other episodes have a keyframe that
// looks alike at the same point of their audio match, within repeatSlack.
// Two keyframes look alike when their luma's 10th and 90th percentiles and
// their saturation are each within repeatClose. A title sequence shows the
// same pictures in each episode under the same music. A story's last scene
// under that music doesn't.
const (
	repeatSlack = 400 * time.Millisecond
	repeatClose = 10
)

// repeats returns, for each file with an audio match in its tail, which of
// its tail's keyframes are repeated, in order.
func repeats(files []File, matches map[id.ID]found) map[id.ID][]bool {
	var pics []pic
	for _, f := range files {
		if m, ok := matches[f.ID]; ok && f.Fingerprint != nil && f.Fingerprint.Tail != nil {
			for _, k := range f.Fingerprint.Tail.Keyframes {
				pics = append(pics, pic{k.At - f.Fingerprint.Tail.Start - m.start, f.Episode, k})
			}
		}
	}
	slices.SortFunc(pics, func(a, b pic) int { return cmp.Compare(a.at, b.at) })
	out := map[id.ID][]bool{}
	for _, f := range files {
		m, ok := matches[f.ID]
		if !ok || f.Fingerprint == nil || f.Fingerprint.Tail == nil {
			continue
		}
		flags := make([]bool, len(f.Fingerprint.Tail.Keyframes))
		for i, k := range f.Fingerprint.Tail.Keyframes {
			flags[i] = repeatedBy(pics, pic{k.At - f.Fingerprint.Tail.Start - m.start, f.Episode, k}) >= 2
		}
		out[f.ID] = flags
	}
	return out
}

// pic is a keyframe of a tail with an audio match. It records how far the
// keyframe is from the start of its file's match, and its episode.
type pic struct {
	at      time.Duration
	episode int
	k       media.KeyframeStats
}

// repeatedBy counts how many other episodes repeat p among pics, which are
// in order.
func repeatedBy(pics []pic, p pic) int {
	var episodes []int
	i, _ := slices.BinarySearchFunc(pics, p.at-repeatSlack, func(q pic, t time.Duration) int { return cmp.Compare(q.at, t) })
	for ; i < len(pics) && pics[i].at <= p.at+repeatSlack; i++ {
		if q := pics[i]; q.episode != p.episode && alike(q.k, p.k) && !slices.Contains(episodes, q.episode) {
			episodes = append(episodes, q.episode)
		}
	}
	return len(episodes)
}

// alike reports whether two keyframes look alike.
func alike(a, b media.KeyframeStats) bool {
	return abs32(a.YLow-b.YLow) <= repeatClose && abs32(a.YHigh-b.YHigh) <= repeatClose && abs32(a.SatAvg-b.SatAvg) <= repeatClose
}

func abs32(f float32) float32 { return max(f, -f) }

// textRun is a run of credits keyframes. Its span runs from its first text
// keyframe to its last, and first is the index of its first keyframe.
type textRun struct {
	span
	first int
}

// textRuns returns the runs of credits among a tail's keyframes, joining
// keyframes no more than runGap apart.
func textRuns(ks []keyframe) []textRun {
	var runs []textRun
	for i := 0; i < len(ks); {
		if !ks[i].look.text() {
			i++
			continue
		}
		r, ok, next := runFrom(ks, i)
		switch n := len(runs); {
		case !ok:
		case n > 0 && r.start-runs[n-1].end <= runGap:
			runs[n-1].end = r.end
		default:
			runs = append(runs, r)
		}
		i = next
	}
	return runs
}

// runFrom returns the run starting at the text keyframe ks[i], whether it
// counts, and the index where the next run may start.
func runFrom(ks []keyframe, i int) (textRun, bool, int) {
	last, lastText := i, i
	for j := i + 1; j < len(ks) && ks[j].look.creditsLike() && ks[j].at-ks[last].at <= runGap; j++ {
		last = j
		if ks[j].look.text() {
			lastText = j
		}
	}
	text, counted := 0, 0
	for _, k := range ks[i : lastText+1] {
		if k.look.text() {
			text++
		}
		if k.look != mostly {
			counted++
		}
	}
	r := textRun{span: span{ks[i].at, ks[lastText].at}, first: i}
	return r, r.length() >= minCredits && text >= minRunText && 2*text > counted, lastText + 1
}

// These constants control how a tail's evidence places credits.
//
// An audio match is moved to start where the first text run near it
// starts, because the credits' music often starts over the story's last
// scene, dark or not. The match grows over runs within runGap of it.
//
// Credits that end within endSnap of the end of the file, with no story
// after them, run to the end. Credits that reach within finalSlack of the
// end are final.
//
// Credits found by their pictures alone must end within videoEnd of the
// end of the file. An earlier run was a dark scene or a title card inside
// the story. Their start moves back over the black just before the first
// text, up to blackLead.
const (
	endSnap    = 15 * time.Second
	finalSlack = 5 * time.Second
	videoEnd   = 120 * time.Second
	blackLead  = 10 * time.Second
	// storyShare is the share of story keyframes above which an audio
	// match with no text to back it is the story's own scene with the
	// credits' music under it, not credits.
	storyShare = 0.5
	// maxPreview is the longest next-episode preview that may follow
	// shared audio that is credits. previewLead is how far before the end
	// of the audio match the silence between the two may start.
	maxPreview  = 45 * time.Second
	previewLead = 6 * time.Second
)

// tail is what's known about one file's end.
type tail struct {
	duration time.Duration // the file's length
	window   time.Duration // where the tail window starts
	audio    *found        // the season audio match in the tail, on the file's timeline; nil if none
	frames   []keyframe
	silences []media.Span
}

// placed is credits as a tail places them: their span, how they were
// found, and whether a next-episode preview follows them.
type placed struct {
	span
	how     string
	preview bool
}

// credits places a file's credits from its tail.
func (t tail) credits() (placed, bool) {
	runs := textRuns(t.frames)
	p, ok := t.fromAudio(runs)
	if !ok {
		p, ok = t.fromPictures(runs)
	}
	if !ok {
		return placed{}, false
	}
	p.span = t.ended(p.span)
	if p.start < t.window || p.length() < minCredits || p.length() > maxCredits || p.end > t.duration {
		return placed{}, false
	}
	return p, true
}

// fromAudio places credits from a season audio match. The match is moved
// to and grown over the text runs near it. With no runs near, the match
// stands alone (see alone). A run that reaches the end of the file well
// after a match that stops early wins, because the match was a recurring
// cue.
func (t tail) fromAudio(runs []textRun) (placed, bool) {
	a := t.audio
	if a == nil || a.partners < 2 {
		return placed{}, false
	}
	s := a.span
	near := nearRuns(runs, s)
	if t.cue(runs, s) {
		return placed{}, false
	}
	if len(near) == 0 {
		return t.alone(s)
	}
	first := near[0]
	s.start = t.blackBefore(first)
	s.end = max(s.end, near[len(near)-1].end)
	return placed{span: s, how: "audio and pictures"}, true
}

// alone places credits from an audio match that no text run is near.
//
// The match must reach the end of the file, or leave no more than a
// next-episode preview after it. In the second case it ends at the silence
// before the preview.
//
// Over pictures that aren't mostly story, the credits start at the first
// text, if any. Over story pictures, such as an ending theme sung over
// animation or footage, the match needs the pictures of a title sequence,
// and the credits start where that sequence does.
func (t tail) alone(s span) (placed, bool) {
	if t.duration-s.end > maxPreview {
		return placed{}, false
	}
	if !t.reachesEnd(s.end) {
		s.end = t.silenceBefore(s.end)
	}
	if t.storyShare(s) > storyShare {
		start, ok := t.sequence(s)
		if !ok {
			return placed{}, false
		}
		s.start = start
		return placed{span: s, how: "audio and repeated pictures", preview: !t.reachesEnd(s.end)}, true
	}
	if i := slices.IndexFunc(t.frames, func(k keyframe) bool { return k.at >= s.start && k.look.text() }); i >= 0 {
		s.start = t.blackBefore(textRun{span: span{t.frames[i].at, s.end}, first: i})
	}
	return placed{span: s, how: "audio", preview: !t.reachesEnd(s.end)}, true
}

// silenceBefore returns where shared audio before a next-episode preview
// ends. That is the start of the last silence starting within previewLead
// before end, or else end itself.
func (t tail) silenceBefore(end time.Duration) time.Duration {
	at := end
	for _, s := range t.silences {
		if s.Start <= end && end-s.Start <= previewLead {
			at = s.Start
		}
	}
	return at
}

// sequence returns where a title sequence under an audio match starts. It
// starts at the first repeated keyframe in the match from which at least
// half the keyframes to the match's end are repeated. When that is the
// match's first keyframe, it starts at the start of the match. Keyframes
// before it show the story's last scene with the theme under it.
func (t tail) sequence(s span) (time.Duration, bool) {
	var in []keyframe
	for _, k := range t.frames {
		if k.at >= s.start && k.at <= s.end {
			in = append(in, k)
		}
	}
	repeated, from := 0, -1
	for i := len(in) - 1; i >= 0; i-- {
		if in[i].repeated {
			repeated++
			if 2*repeated >= len(in)-i {
				from = i
			}
		}
	}
	switch {
	case from < 0:
		return 0, false
	case from == 0:
		return s.start, true
	}
	return in[from].at, true
}

// nearRuns returns the runs that overlap s or lie within runGap of it.
func nearRuns(runs []textRun, s span) []textRun {
	var out []textRun
	for _, r := range runs {
		if r.end >= s.start-runGap && r.start <= s.end+runGap {
			out = append(out, r)
		}
	}
	return out
}

// cue reports whether an audio match is a recurring cue. That is when the
// match stops short of the end of the file, and a run that reaches the end
// starts well after it.
func (t tail) cue(runs []textRun, s span) bool {
	last := len(runs) - 1
	return last >= 0 && runs[last].start > s.end+runGap && t.reachesEnd(runs[last].end) && !t.reachesEnd(s.end)
}

// fromPictures places credits from text runs alone. It takes the last run,
// which must end near the end of the file, and moves its start back over
// the black before it.
func (t tail) fromPictures(runs []textRun) (placed, bool) {
	if len(runs) == 0 {
		return placed{}, false
	}
	r := runs[len(runs)-1]
	if t.duration-r.end > videoEnd {
		return placed{}, false
	}
	return placed{span: span{t.blackBefore(r), r.end}, how: "pictures"}, true
}

// blackBefore returns where a run's credits start. That is the run's first
// text, moved back over the true black keyframes just before it, by up to
// blackLead.
func (t tail) blackBefore(r textRun) time.Duration {
	start := r.start
	for i := r.first - 1; i >= 0 && t.frames[i].look == black && r.start-t.frames[i].at <= blackLead; i-- {
		start = t.frames[i].at
	}
	return start
}

// ended returns s with its end moved to the end of the file when s
// reaches near it with no story after.
func (t tail) ended(s span) span {
	if t.duration-s.end <= endSnap && !t.storyAfter(s.end) {
		s.end = t.duration
	}
	return s
}

// final reports whether credits are final. Nothing may follow them except,
// perhaps, a next-episode preview.
func (t tail) final(p placed) bool { return p.preview || p.end >= t.duration-finalSlack }

func (t tail) reachesEnd(end time.Duration) bool { return t.duration-end <= endSnap }

// storyShare returns the share of the keyframes in s that show story. It
// is 0 when s has no keyframes.
func (t tail) storyShare(s span) float64 {
	n, storied := 0, 0
	for _, k := range t.frames {
		if k.at >= s.start && k.at <= s.end {
			n++
			if k.look == story {
				storied++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return float64(storied) / float64(n)
}

// storyAfter reports whether a keyframe after at shows story.
func (t tail) storyAfter(at time.Duration) bool {
	return slices.ContainsFunc(t.frames, func(k keyframe) bool { return k.at > at && k.look == story })
}

// ending is what a season's tails share. It holds each file's audio match,
// measured from the start of its tail, and which of its tail's keyframes
// other episodes repeat.
type ending struct {
	matches  map[id.ID]found
	repeated map[id.ID][]bool
}

// endingOf matches the season's tails.
func endingOf(files []File) ending {
	e := ending{matches: season(prints(files, func(fp *Fingerprint) *Window { return fp.Tail }), creditsLimits)}
	e.repeated = repeats(files, e.matches)
	return e
}

// tail returns what's known about a file's end.
func (f File) tail(e ending) tail {
	tl := tail{duration: f.Duration}
	if f.Fingerprint != nil && f.Fingerprint.Tail != nil {
		w := f.Fingerprint.Tail
		tl.window, tl.frames, tl.silences = w.Start, classify(w.Keyframes), w.Silences
		for i, r := range e.repeated[f.ID] {
			tl.frames[i].repeated = r
		}
		if m, ok := e.matches[f.ID]; ok {
			m.span = span{w.Start + m.start + creditsStartLead, w.Start + m.end + creditsEndLead}
			tl.audio = &m
		}
	}
	return tl
}

// credits returns the credits a file's tail gives it, if the file is its
// version's last part.
func (f File) credits(e ending) *guess {
	if f.Part != f.Parts || f.Fingerprint == nil || f.Fingerprint.Tail == nil {
		return nil
	}
	tl := f.tail(e)
	p, ok := tl.credits()
	if !ok {
		return nil
	}
	reason := "fingerprint, by " + p.how
	if tl.audio != nil && p.how != "pictures" {
		reason += fmt.Sprintf(", shared with %d other episodes", tl.audio.partners)
	}
	return &guess{span: p.span, final: tl.final(p), reason: reason}
}
