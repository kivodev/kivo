package identify

import (
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/metadata"
)

const (
	// threshold is the score a match needs to be accepted.
	threshold = 80
	// nearTie is how close to the best score a match must be to count as a
	// tie. A tie is typically a remake with the same title.
	nearTie = 5
	// yearSlack is the most years a match's release may be from the wanted
	// year for the match to be accepted.
	yearSlack = 2
)

// want is what a search looks for.
type want struct {
	kind    catalog.WorkKind // KindMovie or KindSeries
	title   string
	year    int    // zero for any year
	country string // a country code the name gives ("US"); empty for any
}

// accept picks the match to accept, or none. Each match scores 0 to 100,
// and only a match at or above the threshold can win. Among those, a match
// within a year of the wanted year beats one further off, whatever their
// titles score. Then the best score wins. Near-ties go to the country the
// name gives ("The Office (US)"), then to the closest year, then to the
// provider's own ranking, which is the order of matches. A match of the
// wrong kind never wins, so a series search doesn't accept a movie, and the
// reverse.
func accept(w want, matches []metadata.Match) (metadata.Match, int, bool) {
	scores := make([]int, len(matches))
	for i, c := range matches {
		scores[i] = -1
		if c.ID.Kind == string(w.kind) {
			scores[i] = score(w, c)
		}
	}
	bestScore := slices.Max(append(scores, -1))
	if bestScore < threshold {
		return metadata.Match{}, bestScore, false
	}
	bestScore = keepNear(w, matches, scores)
	best := -1
	for i, c := range matches {
		if scores[i] < threshold || scores[i] < bestScore-nearTie {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		if a, b := fromCountry(w, c), fromCountry(w, matches[best]); a != b {
			if a {
				best = i
			}
			continue
		}
		if yearDistance(w.year, c) < yearDistance(w.year, matches[best]) {
			best = i
		}
	}
	return matches[best], scores[best], true
}

// keepNear drops the passing matches more than a year from the wanted year
// when another passing match is within a year, by setting their scores to
// -1. It returns the best score left.
func keepNear(w want, matches []metadata.Match, scores []int) int {
	near := false
	for i, c := range matches {
		near = near || scores[i] >= threshold && yearDistance(w.year, c) <= 1
	}
	best := -1
	for i, c := range matches {
		if near && yearDistance(w.year, c) > 1 {
			scores[i] = -1
		}
		best = max(best, scores[i])
	}
	return best
}

// score returns the likeness of the titles minus 3 points per year off. A
// match more than yearSlack years off scores zero, and so does a match with
// no release year when w has one, since it can't be told from a namesake.
// Any of the match's release years counts. With no year in w there is no
// penalty.
func score(w want, c metadata.Match) int {
	if w.year != 0 && !dated(c) {
		return 0
	}
	d := yearDistance(w.year, c)
	if d > yearSlack {
		return 0
	}
	return max(0, likeness(w.title, c)-3*d)
}

// likeness returns how alike title is to the match's title, original title
// and alternative titles, from 0 to 100, taking the best of them. Each
// comparison is based on edit distance between comparison keys, either as
// written or with their words sorted. A title with a different sequel number
// counts for nothing.
func likeness(title string, c metadata.Match) int {
	q := titleKey(title)
	sim := 0
	for _, t := range append([]string{c.Title, c.OriginalTitle}, c.AltTitles...) {
		if t == "" {
			continue
		}
		k := titleKey(t)
		if !differentSequel(q, k) {
			sim = max(sim, similarity(q, k), similarity(sortWords(q), sortWords(k)))
		}
	}
	return sim
}

// dated reports whether the match has any release year.
func dated(c metadata.Match) bool {
	return c.Year != 0 || slices.ContainsFunc(c.OtherYears, func(y int) bool { return y != 0 })
}

// yearDistance returns how many years separate year from the nearest of the
// match's years. It returns zero when either side has no year.
func yearDistance(year int, c metadata.Match) int {
	if year == 0 {
		return 0
	}
	best := -1
	for _, y := range append([]int{c.Year}, c.OtherYears...) {
		if y == 0 {
			continue
		}
		d := max(y-year, year-y)
		if best < 0 || d < best {
			best = d
		}
	}
	return max(best, 0)
}

// sortWords sorts a key's words, so a title with its words reordered
// compares as the same words. An example is "Undead Unwanted Adventurer" for
// "The Unwanted Undead Adventurer".
func sortWords(k string) string {
	w := strings.Fields(k)
	slices.Sort(w)
	return strings.Join(w, " ")
}

// similarity returns 100 for equal strings. It falls as the edit distance
// grows relative to the longer string.
func similarity(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := max(len(ra), len(rb))
	if n == 0 {
		return 0
	}
	return 100 * (n - levenshtein(ra, rb)) / n
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// fromCountry reports whether c was made in the country w names. Names say
// UK where providers use the ISO code GB.
func fromCountry(w want, c metadata.Match) bool {
	country := w.country
	if country == "UK" {
		country = "GB"
	}
	return country != "" && slices.Contains(c.Countries, country)
}
