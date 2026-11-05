package parse

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// rule is one episode pattern. extract reads match m of the scanned name
// into g. It returns false to reject the match, so the next match is tried,
// and then the next rule. The title is the text before the match.
type rule struct {
	name string
	re   *regexp.Regexp
	// before checks the character before a match, standing in for the
	// lookbehind RE2 lacks. A nil before accepts anything.
	before func(s string, i int) bool
	// titleSeason reads a season marker at the end of the title, as in
	// "Show Season 2 - 05" or "Show 2nd Season 05". It is set for rules
	// whose pattern has no season of its own.
	titleSeason bool
	extract     func(sc *scan, m []int, g *Guess) bool
}

// The left boundaries that rules use. Checking them outside the pattern
// also keeps the patterns fast, because an alternation with ^ makes Go's
// matcher work harder at every position.
func notAlnum(s string, i int) bool { return i == 0 || !isAlnum(s[i-1]) }
func notDigit(s string, i int) bool { return !digitAt(s, i-1) }
func notWord(s string, i int) bool {
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return i == 0 || !isWord(r)
}
func bareStart(s string, i int) bool { return i == 0 || strings.IndexByte(" _.[(#", s[i-1]) >= 0 }

// scan is the state one Parse call shares with the rules.
type scan struct {
	p   Parser
	s   string // the name in match form, with IDs, edition, checksum and group removed
	tag int    // where the first release tag starts
	// letter is where the first letter is, or -1 if there is none. A bare
	// number needs a title before it.
	letter int
	// inSeason is set when the file sits in a season folder, where a name
	// starting with a number is that episode.
	inSeason bool
}

// episodeRules is the list of patterns for shows, most specific first. The
// first rule with an accepted match wins. The order is the spec.
var episodeRules = []rule{
	// S01E02, s01e02e03, S01E01-E03, S01 - E03, S01xE03, S01E05a.
	{name: "sxxeyy", re: regexp.MustCompile(`(?i)s(\d{1,4})(?:[ ._-]{0,3}|x)e(\d{1,4})`), before: notAlnum, extract: sxxeyy},
	// S01.07, s02 09: a season marker followed by a bare two-digit episode.
	{name: "sxx-yy", re: regexp.MustCompile(`(?i)s(\d{1,2})[ ._](\d{2})`), before: notAlnum, extract: sxxYY},
	// 2024-03-07 and 07-03-2024, which are looked up by air date.
	{name: "date", re: regexp.MustCompile(`((?:19|20)\d\d)([-._ ])(\d{1,2})([-._ ])(\d{1,2})`), before: notDigit, extract: dateYMD},
	{name: "date-dmy", re: regexp.MustCompile(`(\d{1,2})([-._ ])(\d{1,2})([-._ ])((?:19|20)\d\d)`), before: notDigit, extract: dateDMY},
	// 1x02, 7x23-24.
	{name: "nxnn", re: regexp.MustCompile(`(\d{1,4})[xX](\d{1,3})`), before: notAlnum, extract: nxnn},
	// Episode 2, Ep.05, E10, #04.
	{name: "episode", titleSeason: true, before: notWord, extract: episodeWord,
		re: regexp.MustCompile(`(?i)(?:(?:episodes?|episodio|eps?|cap|capitulo)[ ._+-]{0,2}|e|#[ ]?)(\d{1,4})`)},
	// [Group] Title - 012v2 [ABCD1234]: anime's absolute numbering.
	{name: "absolute", titleSeason: true, re: regexp.MustCompile(`[ _.]-[ _.]{1,3}(\d{1,4})`), extract: absolute},
	// 03 - Title, only in a season folder.
	{name: "leading", re: regexp.MustCompile(`^(\d{1,3})(?:[ ._-]|$)`), extract: leading},
	// Title 12: a bare number, accepted only before any release tag.
	{name: "bare", titleSeason: true, re: regexp.MustCompile(`(\d{1,4})`), before: bareStart, extract: bare},
	// Season 2, S02, 2nd Season, with no episode.
	{name: "season", re: seasonMarker, before: notAlnum, extract: seasonOnly},
}

// seasonMarker finds "S02", "Season 2" or "2nd Season". The season rule
// checks the characters around the match.
var seasonMarker = regexp.MustCompile(`(?i)(?:s(\d{1,2})|season[ ._]?(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)[ ._]season)`)

// hasSeasonMarker reports whether s has a season marker standing alone.
func hasSeasonMarker(s string) bool {
	for _, m := range seasonMarker.FindAllStringIndex(s, -1) {
		if notAlnum(s, m[0]) && !wordAt(s, m[1]) {
			return true
		}
	}
	return false
}

// trailingSeason finds a season marker at the end of a title.
var trailingSeason = regexp.MustCompile(`(?i)(?:^|[ ._(\[-])(?:s(\d{1,2})|season[ ._]?(\d{1,2})|(\d{1,2})(?:st|nd|rd|th)[ ._]season)[ ._)\]-]*$`)

// maxRange caps a range's span. A file holding more episodes than this is a
// misread ("S01 E15 - 459"), and the cap stops a hostile name from
// allocating much.
const maxRange = 100

// validSeason rejects seasons 200 to 1927 and above 2500, so a resolution
// such as 1920x1080 isn't an episode. Year-numbered seasons pass.
func validSeason(n int) bool { return n < 200 || n >= 1928 && n <= 2500 }

func sxxeyy(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	season := atoi(s[m[2]:m[3]])
	if !validSeason(season) || digitAt(s, m[5]) {
		return false
	}
	x := readSuffix(s, m[5], true, false)
	eps, pos := episodes(s, x.end, atoi(s[m[4]:m[5]]), true)
	g.Season, g.Episodes = season, eps
	x.apply(g)
	splitPart(s, pos, g)
	return true
}

func sxxYY(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	if wordAt(s, m[5]) {
		return false
	}
	g.Season, g.Episodes = atoi(s[m[2]:m[3]]), []int{atoi(s[m[4]:m[5]])}
	return true
}

func dateYMD(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	if s[m[4]:m[5]] != s[m[8]:m[9]] || digitAt(s, m[11]) {
		return false
	}
	return sc.p.airDate(atoi(s[m[2]:m[3]]), atoi(s[m[6]:m[7]]), atoi(s[m[10]:m[11]]), g)
}

func dateDMY(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	if s[m[4]:m[5]] != s[m[8]:m[9]] || digitAt(s, m[11]) {
		return false
	}
	return sc.p.airDate(atoi(s[m[10]:m[11]]), atoi(s[m[6]:m[7]]), atoi(s[m[2]:m[3]]), g)
}

// airDate sets g.AirDate when y-mo-d is a real date from 1900 to 14 days
// after Today. A month above 12 swaps day and month.
func (p Parser) airDate(y, mo, d int, g *Guess) bool {
	if mo > 12 {
		mo, d = d, mo
	}
	if y < 1900 || mo < 1 || mo > 12 || d < 1 {
		return false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d { // February 30 normalizes to March
		return false
	}
	if !p.Today.IsZero() && t.After(p.Today.time().AddDate(0, 0, 14)) {
		return false
	}
	g.AirDate = DateOf(t)
	return true
}

func nxnn(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	season := atoi(s[m[2]:m[3]])
	if !validSeason(season) || !numberEnds(s, m[5]) {
		return false
	}
	g.Season = season
	g.Episodes, _ = episodes(s, m[5], atoi(s[m[4]:m[5]]), true)
	return true
}

func episodeWord(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	x := readSuffix(s, m[3], false, true)
	if wordAt(s, x.end) {
		return false
	}
	x.apply(g)
	g.Episodes, _ = episodes(s, x.end, atoi(s[m[2]:m[3]]), false)
	return true
}

func absolute(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	if sc.isYear(s[m[2]:m[3]]) {
		return false
	}
	x := readSuffix(s, m[3], true, true)
	if wordAt(s, x.end) {
		return false
	}
	eps, pos := episodes(s, x.end, atoi(s[m[2]:m[3]]), false)
	// The number must end the name or be followed by a bracket or another
	// dash. So "Fairy Tail - 100 Years Quest - 05" is episode 5.
	if rest := strings.TrimLeft(s[pos:], " ._"); rest != "" && strings.IndexByte("[(-~", rest[0]) < 0 {
		return false
	}
	x.apply(g)
	g.Absolute = eps
	return true
}

func leading(sc *scan, m []int, g *Guess) bool {
	if !sc.inSeason {
		return false
	}
	g.Episodes = []int{atoi(sc.s[m[2]:m[3]])}
	return true
}

func bare(sc *scan, m []int, g *Guess) bool {
	s := sc.s
	num := s[m[2]:m[3]]
	if m[2] >= sc.tag || sc.letter < 0 || sc.letter >= m[0] || sc.isYear(num) || seasonWordBefore(s, m[2]) {
		return false
	}
	// Decimals such as 3.5 or 5.1 aren't episodes.
	if byteAt(s, m[2]-1) == '.' && digitAt(s, m[2]-2) || byteAt(s, m[3]) == '.' && digitAt(s, m[3]+1) {
		return false
	}
	// A season marker after the number makes the number part of the title,
	// as in "a.documentary.in.2.pieces.S01".
	if hasSeasonMarker(s[m[3]:]) {
		return false
	}
	x := readSuffix(s, m[3], false, false)
	if wordAt(s, x.end) {
		return false
	}
	x.apply(g)
	g.Absolute, _ = episodes(s, x.end, atoi(num), false)
	return true
}

func seasonOnly(sc *scan, m []int, g *Guess) bool {
	if wordAt(sc.s, m[1]) {
		return false
	}
	for i := 2; i < len(m); i += 2 {
		if m[i] >= 0 {
			g.Season = atoi(sc.s[m[i]:m[i+1]])
		}
	}
	return true
}

// suffix is what may follow an episode number: a split letter (05a), a
// revision (v2), or a decimal special (12.5).
type suffix struct {
	split    string
	revision int
	special  bool
	end      int // the position after the suffix
}

// readSuffix reads the suffix at pos. Each rule says whether a split letter
// or a decimal can follow its numbers. After S01E01, a ".1" is the next
// token, but after an anime number it marks a special.
func readSuffix(s string, pos int, split, decimal bool) suffix {
	x := suffix{end: pos}
	if c := lower(byteAt(s, x.end)); split && c >= 'a' && c <= 'd' && !wordAt(s, x.end+1) {
		x.split = string(c)
		x.end++
	}
	at := x.end // v2, or _v2 after a separator
	if strings.IndexByte(" _.", byteAt(s, at)) >= 0 && lower(byteAt(s, at+1)) == 'v' {
		at++
	}
	if lower(byteAt(s, at)) == 'v' && digitAt(s, at+1) && !wordAt(s, at+2) {
		x.revision = atoi(s[at+1 : at+2])
		x.end = at + 2
	}
	if decimal && byteAt(s, x.end) == '.' && digitAt(s, x.end+1) && !wordAt(s, x.end+2) {
		x.special = true
		x.end += 2
	}
	return x
}

func (x suffix) apply(g *Guess) {
	g.SplitPart, g.Revision, g.Special = x.split, x.revision, x.special
}

// contRe reads one more episode after the first, such as "-E03", "E03",
// "-03", "+04" or " ~ 12". A dash or tilde makes a range. Any other
// separator lists episodes.
var contRe = regexp.MustCompile(`^([ ._]{0,2}[-~+&][ ._]{0,2})?((?i:ep|e|x))?(\d{1,4})`)

// episodes reads the episodes that continue from first at pos. It returns
// them in strictly increasing order, with the position after the last one.
// A range's end must be above its start and not followed by p, i or a
// digit, so S09E14-1080p isn't E14 to E108. marked accepts an "E03" or "x03"
// with no separator, because S01E01E02 and 1x01x02 list episodes.
func episodes(s string, pos, first int, marked bool) ([]int, int) {
	eps := []int{first}
	for {
		m := contRe.FindStringSubmatchIndex(s[pos:])
		if m == nil {
			return eps, pos
		}
		sep := ""
		if m[2] >= 0 {
			sep = strings.Trim(s[pos+m[2]:pos+m[3]], " ._")
		}
		if sep == "" && !(marked && m[4] >= 0) {
			return eps, pos
		}
		end := pos + m[7]
		n, last := atoi(s[pos+m[6]:end]), eps[len(eps)-1]
		isRange := sep == "-" || sep == "~"
		if n <= last || !numberEnds(s, end) || isRange && n-last > maxRange {
			return eps, pos
		}
		// " - 12" with separators around the dash is a range only at the
		// end or before a bracket. So "Naruto - 116 - 360 Degrees" is a
		// title.
		if strings.ContainsAny(s[pos:pos+m[6]], " ._") && !atTail(s, end) {
			return eps, pos
		}
		if isRange {
			for e := last + 1; e < n; e++ {
				eps = append(eps, e)
			}
		}
		eps = append(eps, n)
		pos = end
	}
}

// numberEnds reports whether the number ending at i isn't followed by a
// digit or by p or i, which would make it a resolution.
func numberEnds(s string, i int) bool {
	c := lower(byteAt(s, i))
	return !digitAt(s, i) && c != 'p' && c != 'i'
}

// atTail reports whether only separators lie between i and either the end
// of s or a bracket.
func atTail(s string, i int) bool {
	rest := strings.TrimLeft(s[i:], " ._")
	return rest == "" || strings.IndexByte("[(", rest[0]) >= 0
}

// splitPartRe reads " - part1" or ".pt2" after an episode as its split
// part.
var splitPartRe = regexp.MustCompile(`(?i)^[ ._-]{1,3}(?:part|pt)[ ._]?(\d{1,2})`)

func splitPart(s string, pos int, g *Guess) {
	if g.SplitPart != "" {
		return
	}
	if m := splitPartRe.FindStringSubmatchIndex(s[pos:]); m != nil && !wordAt(s, pos+m[3]) {
		g.SplitPart = s[pos+m[2] : pos+m[3]]
	}
}

// isYear reports whether a number is a plausible year. The number rules
// leave years to the title.
func (sc *scan) isYear(num string) bool {
	if len(num) != 4 {
		return false
	}
	y := atoi(num)
	return y >= 1900 && y <= sc.p.maxYear()
}

// seasonWordBefore reports whether the number at i follows "Season", in
// which case the season rule reads it instead. After "2nd Season", a number
// is an episode.
func seasonWordBefore(s string, i int) bool {
	t := strings.TrimRight(s[:i], " ._")
	if len(t) < 6 || !strings.EqualFold(t[len(t)-6:], "season") {
		return false
	}
	t = t[:len(t)-6]
	return !ordinalEnd.MatchString(t[max(0, len(t)-5):])
}

// ordinalEnd matches a title area that ends in an ordinal, such as "2nd ".
var ordinalEnd = regexp.MustCompile(`(?i)\d(?:st|nd|rd|th)[ ._]$`)

// hasLetter reports whether s has a letter, so a title made of digits and
// punctuation doesn't count.
func hasLetter(s string) bool { return strings.IndexFunc(s, isLetter) >= 0 }

// isLetter reports whether r is a Latin letter or any non-ASCII letter or
// digit. Non-ASCII digits count so that CJK numerals count as title text.
func isLetter(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= 128 && isWord(r)
}
