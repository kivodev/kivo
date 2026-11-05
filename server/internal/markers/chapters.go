package markers

import (
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// MarkerKind is the part of a video a marker's "Skip" button skips.
type MarkerKind string

const (
	NoMarker      MarkerKind = ""
	IntroMarker   MarkerKind = "intro"
	RecapMarker   MarkerKind = "recap"
	CreditsMarker MarkerKind = "credits"
	// previewMarker is a next-episode preview. It gets no marker of its
	// own. It is tracked so that credits followed only by a preview are
	// still final.
	previewMarker MarkerKind = "preview"
)

// MarkerSource is where a marker comes from.
type MarkerSource string

// A marker comes from the file's own named chapters, from its season's
// audio and pictures, or from an admin.
const (
	ChapterSource     MarkerSource = "chapter"
	FingerprintSource MarkerSource = "fingerprint"
	ManualSource      MarkerSource = "manual"
)

// Marker is a part of a version's timeline a player offers to skip.
type Marker struct {
	Kind       MarkerKind
	Start, End time.Duration
	Source     MarkerSource
	// Final is set on final credits. Only a next-episode preview, or
	// nothing, follows them to the end of the version.
	Final bool
}

// The limits of the chapter-name rule.
const (
	// markEdge is how far into a version an intro or recap may start, and
	// how far from its end credits may start, when that is more than a
	// quarter of the version's length.
	markEdge          = 15 * time.Minute
	minIntro          = 5 * time.Second
	maxIntro          = 3 * time.Minute
	maxPreview        = 2 * time.Minute
	minCredits        = 10 * time.Second
	maxEpisodeCredits = 8 * time.Minute
	maxMovieCredits   = 15 * time.Minute
	maxEnding         = 3 * time.Minute // for "Ending" or "Closing" alone, which can name a film's last act
	// endSlack is how far before a version's end its last marker may stop
	// and still be final.
	endSlack = 5 * time.Second
)

// kindsFor returns the markers a kind of work gets. An episode gets its
// intro, recap and credits, plus a preview, which never becomes a marker. A
// movie gets its credits.
func kindsFor(work catalog.WorkKind) []MarkerKind {
	switch work {
	case catalog.KindEpisode:
		return []MarkerKind{IntroMarker, RecapMarker, CreditsMarker, previewMarker}
	case catalog.KindMovie:
		return []MarkerKind{CreditsMarker}
	}
	return nil
}

// workKind returns a work's kind. It returns "" when the work can't be
// read, which gives its plays no markers.
func (s *Service) workKind(ctx context.Context, work id.ID) catalog.WorkKind {
	w, err := s.deps.Catalog.Work(ctx, work)
	if err != nil {
		return ""
	}
	return w.Kind
}

// chapters returns a version's chapters on its timeline, and its markers,
// for a work of the given kind.
func chapters(work catalog.WorkKind, parts []catalog.File) ([]catalog.Chapter, []Marker) {
	cs := timeline(parts)
	kinds := markers(work, cs, duration(parts))
	var ms []Marker
	for i, c := range cs {
		k := kinds[i]
		if k == previewMarker {
			k = NoMarker
		}
		if k == NoMarker {
			continue
		}
		if n := len(ms); n > 0 && ms[n-1].Kind == k && i > 0 && kinds[i-1] == k && contiguous(cs[i-1], c) {
			ms[n-1].End = c.End // one span, so one marker
			continue
		}
		ms = append(ms, Marker{Kind: k, Start: c.Start, End: c.End, Source: ChapterSource})
	}
	total := duration(parts)
	for i := range ms {
		ms[i].Final = ms[i].Kind == CreditsMarker && isFinal(ms[i], cs, kinds, total)
	}
	return cs, ms
}

// isFinal reports whether credits are final credits in a version of
// length total: nothing follows them, or only a next-episode preview.
func isFinal(m Marker, cs []catalog.Chapter, kinds []MarkerKind, total time.Duration) bool {
	end := m.End
	for i, c := range cs {
		if c.Start < m.End {
			continue
		}
		if kinds[i] != previewMarker || c.Start > end+time.Second {
			break
		}
		end = c.End
	}
	return end >= total-endSlack
}

// duration returns a version's length: its parts' durations, summed.
func duration(parts []catalog.File) time.Duration {
	var total time.Duration
	for _, f := range parts {
		total += f.Probe.Duration
	}
	return total
}

// timeline returns a version's chapters on its timeline. Each part's own
// chapters are shifted by the length of the parts before it. In a version
// of several parts, a part without sane chapters becomes one chapter that
// spans it, named after its file, the way a book of per-chapter files
// plays. A part's chapters are sane when they are in order, don't overlap
// and fit in the file. Ordered chapters, which play pieces of other files,
// aren't sane.
func timeline(parts []catalog.File) []catalog.Chapter {
	var out []catalog.Chapter
	var offset time.Duration
	for _, f := range parts {
		own := f.Chapters
		if !sane(own, f.Probe.Duration) {
			own = nil
		}
		if len(own) == 0 && len(parts) > 1 {
			name := f.RelPath[strings.LastIndexByte(f.RelPath, '/')+1:]
			own = []catalog.Chapter{{End: f.Probe.Duration, Title: strings.TrimSuffix(name, filepath.Ext(name))}}
		}
		for _, c := range own {
			c.Index, c.Start, c.End = len(out), c.Start+offset, c.End+offset
			out = append(out, c)
		}
		offset += f.Probe.Duration
	}
	return out
}

// chapterSlack is how far past a file's duration its last chapter may end,
// because containers round the two differently.
const chapterSlack = time.Second

// sane reports whether a file's chapters are in order, don't overlap, and
// fit in its duration.
func sane(chapters []catalog.Chapter, duration time.Duration) bool {
	var end time.Duration
	for _, c := range chapters {
		if c.Start < end || c.End < c.Start || c.Start >= duration || c.End > duration+chapterSlack {
			return false
		}
		end = c.End
	}
	return true
}

// markers returns the marker each chapter is, in a version of length total
// of a work of the given kind. A chapter's marker depends on its name (see
// titled), the chapters beside it (see coldOpens), and where its span is
// and how long (see fits). Adjacent chapters of one marker form one span.
// Either the whole span is the marker, or, when it doesn't fit, none of it
// is.
func markers(work catalog.WorkKind, cs []catalog.Chapter, total time.Duration) []MarkerKind {
	allowed := kindsFor(work)
	titles := make([]title, len(cs))
	for i, c := range cs {
		titles[i] = titled(c.Title)
		if !slices.Contains(allowed, titles[i].kind) {
			titles[i] = title{}
		}
	}
	coldOpens(titles)
	out := make([]MarkerKind, len(cs))
	for i := 0; i < len(cs); {
		t, j := titles[i], i+1
		for j < len(cs) && titles[j].kind == t.kind && contiguous(cs[j-1], cs[j]) {
			t.loose = t.loose && titles[j].loose
			j++
		}
		if t.kind != NoMarker && fits(t, catalog.Chapter{Start: cs[i].Start, End: cs[j-1].End}, total, work) {
			for k := i; k < j; k++ {
				out[k] = t.kind
			}
		}
		i = j
	}
	return out
}

// contiguous reports whether chapter b starts where chapter a ends.
func contiguous(a, b catalog.Chapter) bool {
	return b.Start-a.End <= time.Second
}

// coldOpens drops a plain "Intro" beside an opening. The opening is the
// intro, and the "Intro" chapter is a cold open.
func coldOpens(ts []title) {
	opening := func(i int) bool {
		return i >= 0 && i < len(ts) && ts[i].kind == IntroMarker && !ts[i].plain
	}
	for i := range ts {
		if ts[i].kind == IntroMarker && ts[i].plain && (opening(i-1) || opening(i+1)) {
			ts[i] = title{}
		}
	}
}

// fits reports whether a span named as a marker has a marker's place and
// length, in a version of length total.
//
// An intro or recap starts before the middle, within the first quarter or
// 15 minutes, and is 5 seconds to 3 minutes long. Credits start after the
// middle, within the last quarter or 15 minutes. They are 10 seconds to 8
// minutes long in an episode, up to 15 minutes in a movie, and up to 3
// minutes for "Ending" or "Closing". A preview starts as late as credits
// and is up to 2 minutes long.
func fits(t title, c catalog.Chapter, total time.Duration, work catalog.WorkKind) bool {
	edge := max(total/4, markEdge)
	early := c.Start < total/2 && c.Start <= edge
	late := c.Start > total/2 && c.Start >= total-edge
	length := c.End - c.Start
	switch t.kind {
	case IntroMarker, RecapMarker:
		return early && length >= minIntro && length <= maxIntro
	case previewMarker:
		return late && length > 0 && length <= maxPreview
	case CreditsMarker:
		limit := maxEpisodeCredits
		if work == catalog.KindMovie {
			limit = maxMovieCredits
		}
		if t.loose {
			limit = maxEnding
		}
		return late && length >= minCredits && length <= limit
	}
	return false
}

// title is what a chapter's name says the chapter is.
type title struct {
	kind MarkerKind
	// plain marks a plain "Intro", which is a cold open when it's beside an
	// opening.
	plain bool
	// loose marks credits named only "Ending" or "Closing". Those words may
	// name a story's last part, so such credits must be short.
	loose bool
}

var (
	// numbering is a name's leading number: "2. ", "07 ", "Chapter 4 - ".
	numbering = regexp.MustCompile(`^(?i:chapter|ch\.?|part)?\s*\d+\s*(?:[.:)\-–—]+\s*|\s+)`)
	// separators split a compound name into its parts.
	separators = regexp.MustCompile(`(?i)\s*(?:[,;/&]|\band\b)\s*`)
	// abbreviated matches the anime openings and endings. It matches
	// capitals only, so a word such as "Ed" isn't one.
	abbreviated = regexp.MustCompile(`^(OP|ED) ?[1-9]?$`)
	// songEnding is an ending named with its song: "ED: Song Name".
	songEnding = regexp.MustCompile(`^ED ?[1-9]?\s*[:\-–—]\s*\S`)
	// noted is a name with a note in brackets at its end: "End Credits
	// [1:12]", "Song Name (End Credits)".
	noted = regexp.MustCompile(`^(.*?)\s*[(\[]([^()\[\]]*)[)\]]$`)
	// dashed is a name with a note after a dash or colon: "Opening – Song".
	dashed = regexp.MustCompile(`^(.+?)\s*(?::|\s[-–—]+|[–—])\s*(\S.*)$`)
)

// phrases are the names of each marker, matched whole and in any case.
var phrases = map[string]title{
	"intro": {kind: IntroMarker, plain: true}, "introduction": {kind: IntroMarker, plain: true},
	"opening": {kind: IntroMarker}, "opening credits": {kind: IntroMarker}, "opening titles": {kind: IntroMarker},
	"opening theme": {kind: IntroMarker}, "opening sequence": {kind: IntroMarker},
	"title sequence": {kind: IntroMarker}, "main titles": {kind: IntroMarker},

	"credits": {kind: CreditsMarker}, "credit": {kind: CreditsMarker}, "end credits": {kind: CreditsMarker},
	"ending credits": {kind: CreditsMarker}, "closing credits": {kind: CreditsMarker}, "end titles": {kind: CreditsMarker},
	"ending": {kind: CreditsMarker, loose: true}, "closing": {kind: CreditsMarker, loose: true}, "outro": {kind: CreditsMarker},

	"recap": {kind: RecapMarker}, "previously": {kind: RecapMarker}, "previously on": {kind: RecapMarker},
	"last time": {kind: RecapMarker}, "last time on": {kind: RecapMarker},

	"preview": {kind: previewMarker}, "next episode preview": {kind: previewMarker}, "next time": {kind: previewMarker},
	"next on": {kind: previewMarker}, "next episode": {kind: previewMarker},
}

// titled returns what a chapter's name says the chapter is. A name is a
// marker only in these cases:
//   - the whole name is a marker's name (see wholly);
//   - it is a marker's name with a note after it, in brackets or after a
//     dash or colon ("End Credits (Song)", "Opening – Song");
//   - it is a song with only a marker's name in brackets after it
//     ("Song (End Credits)").
func titled(name string) title {
	if t := wholly(name); t.kind != NoMarker {
		return t
	}
	name = bare(name)
	if m := noted.FindStringSubmatch(name); m != nil && strings.TrimSpace(m[1]) != "" {
		if t := wholly(m[1]); t.kind != NoMarker {
			return t
		}
		if t := phrase(m[2]); t.kind != NoMarker {
			return t
		}
	}
	if m := dashed.FindStringSubmatch(name); m != nil {
		return wholly(m[1])
	}
	return title{}
}

// wholly returns the marker a whole name is, ignoring its numbering and
// brackets. In a compound name, every part must name the same kind of
// marker. "Intro end", "Post Intro", "Cold Open" and "Post-Credits" are not
// markers.
func wholly(name string) title {
	name = bare(name)
	if songEnding.MatchString(name) {
		return title{kind: CreditsMarker}
	}
	var out title
	for i, part := range separators.Split(name, -1) {
		t := phrase(part)
		if t.kind == NoMarker || i > 0 && t.kind != out.kind {
			return title{}
		}
		if i == 0 {
			out = t
		} else {
			out.plain = out.plain && t.plain
			out.loose = out.loose && t.loose
		}
	}
	return out
}

// bare returns a name without its numbering and the brackets around it.
func bare(name string) string {
	name = unbracket(strings.TrimSpace(name))
	if rest := numbering.ReplaceAllString(name, ""); rest != "" {
		name = unbracket(rest)
	}
	return name
}

// phrase returns the marker one part of a name is. A trailing "start"
// doesn't change it, so "Intro start" is an intro and "Credits start" is
// credits.
func phrase(part string) title {
	part = strings.TrimSpace(part)
	if abbreviated.MatchString(part) {
		if strings.HasPrefix(part, "OP") {
			return title{kind: IntroMarker}
		}
		return title{kind: CreditsMarker}
	}
	words := strings.Fields(strings.ToLower(strings.NewReplacer("-", " ", "_", " ").Replace(part)))
	if n := len(words); n > 1 && words[n-1] == "start" {
		words = words[:n-1]
	}
	return phrases[strings.Join(words, " ")]
}

// unbracket strips the brackets around a whole name: "[Opening]".
func unbracket(s string) string {
	for len(s) >= 2 {
		open, end := s[0], s[len(s)-1]
		if !(open == '(' && end == ')' || open == '[' && end == ']' || open == '{' && end == '}') {
			break
		}
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}
