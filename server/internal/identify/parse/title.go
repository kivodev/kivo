package parse

import (
	"regexp"
	"strings"
	"unicode"
)

// hasWord reports whether s contains a letter or digit.
func hasWord(s string) bool { return strings.IndexFunc(s, isWord) >= 0 }

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// yearRe finds four-digit numbers that could be years. years checks the
// characters around each one, and the range.
var yearRe = regexp.MustCompile(`((?:19|20)\d\d)`)

type yearAt struct {
	start, end, year int
	bracketed        bool // (1999), [1999] or {1999}
}

// years returns the plausible years in s, 1900 to maxYear.
func years(s string, maxYear int) []yearAt {
	var out []yearAt
	for _, m := range yearRe.FindAllStringSubmatchIndex(s, -1) {
		y := atoi(s[m[2]:m[3]])
		if y > maxYear || !notAlnum(s, m[2]) || wordAt(s, m[3]) {
			continue
		}
		open, cl := byteAt(s, m[2]-1), byteAt(s, m[3])
		out = append(out, yearAt{m[2], m[3], y, strings.IndexByte("([{", open) >= 0 && strings.IndexByte(")]}", cl) >= 0})
	}
	return out
}

// titleYear reads a title and year from a name with no episode in it. The
// title ends at the year or the first release tag. With several years, the
// bracketed one wins, then the last one. A year with no title before it is
// the title, as in "1923" or "2012 (2009)", so numeric titles stay
// titles.
func titleYear(s string, maxYear int) (title string, year, end int) {
	end = firstTag(s)
	ys := years(s, maxYear)
	word := strings.IndexFunc(s, isWord) // a year at or before this index has no title before it
	if word < 0 {
		word = len(s)
	}
	var pick *yearAt
	for i := range ys {
		y := &ys[i]
		if y.start >= end || y.start <= word {
			continue
		}
		if pick == nil || y.bracketed || !pick.bracketed {
			pick = y
		}
	}
	if pick == nil {
		// A year after the tags still counts, as in "Title [1080p] MULTi
		// 2013". A bracketed year can also lead, as in "(2000) Title".
		for i := range ys {
			y := &ys[i]
			switch {
			case y.start >= end && word < end:
				return cleanTitle(s[:end]), y.year, end
			case y.bracketed && y.start <= word && hasWord(s[y.end:]):
				t, _, e := titleYear(s[y.end+1:], maxYear)
				return t, y.year, y.end + 1 + e
			}
		}
		return cleanTitle(s[:end]), 0, end
	}
	return cleanTitle(s[:pick.start]), pick.year, pick.start
}

// cleanTitle turns the title part of a name into a title. Separators
// become spaces. Unclosed brackets, trailing [tags] and the junk around the
// edges are removed. A title made only of bracketed segments, such as
// "[Group][标题][Title]", takes the last segment with Latin letters. Anime's
// "(TV)" marker is removed too, since providers don't use it.
func cleanTitle(t string) string {
	t = strings.Trim(cutUnclosed(strings.TrimSpace(t)), edgeJunk)
	if strings.HasPrefix(t, "[") {
		if seg, ok := bracketList(t); ok {
			t = seg
		}
	}
	for strings.HasSuffix(t, "]") {
		i := strings.LastIndexByte(t, '[')
		if i < 0 || !hasWord(t[:i]) {
			break
		}
		t = strings.TrimRight(t[:i], edgeJunk)
	}
	for strings.HasPrefix(t, "[") {
		i := strings.IndexByte(t, ']')
		if i < 0 || !hasWord(t[i+1:]) {
			break
		}
		t = strings.TrimLeft(t[i+1:], edgeJunk)
	}
	if n := len(t) - len("(TV)"); n > 0 && strings.EqualFold(t[n:], "(TV)") {
		t = strings.TrimRight(t[:n], edgeJunk)
	}
	if strings.ContainsRune(t, ' ') {
		t = strings.ReplaceAll(t, "_", " ")
	} else {
		t = dotsToSpaces(t)
	}
	t = strings.TrimFunc(t, func(r rune) bool {
		return !isWord(r) && !strings.ContainsRune("!?'&)]$@#%*", r)
	})
	t = strings.TrimLeft(t, ")]")
	return strings.Join(strings.Fields(t), " ")
}

// edgeJunk lists the separators trimmed from a title's edges between
// steps.
const edgeJunk = " ._-"

// dotsToSpaces turns dots and underscores into spaces in a name that uses
// them as separators. It keeps decimal points, as in "Evangelion.1.11".
func dotsToSpaces(t string) string {
	b := []byte(t)
	for i, c := range b {
		if c == '_' || c == '.' && !(digitAt(t, i-1) && digitAt(t, i+1)) {
			b[i] = ' '
		}
	}
	return string(b)
}

// cutUnclosed cuts t at its first bracket that is never closed, which is
// where a title ran into a tag or year inside brackets. An unclosed bracket
// at the start of t is dropped instead. For example, "[Taxi 1998]" cut at
// its year leaves "[Taxi", whose title is Taxi.
func cutUnclosed(t string) string {
	var open []int
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case '(', '[', '{':
			open = append(open, i)
		case ')', ']', '}':
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	// Unclosed openers never matched anything, so dropping the leading ones
	// leaves the rest unchanged.
	k := 0
	for k < len(open) && open[k] == k {
		k++
	}
	if k < len(open) {
		return t[k:open[k]]
	}
	return t[k:]
}

// bracketList splits a string made only of [segments] and returns the last
// segment with a Latin letter, or the last segment.
func bracketList(t string) (string, bool) {
	var segs []string
	for t != "" {
		t = strings.TrimSpace(t)
		if !strings.HasPrefix(t, "[") {
			return "", t == ""
		}
		j := strings.IndexByte(t, ']')
		if j < 0 {
			return "", false
		}
		segs = append(segs, t[1:j])
		t = t[j+1:]
	}
	if len(segs) == 0 {
		return "", false
	}
	for i := len(segs) - 1; i >= 0; i-- {
		if strings.IndexFunc(segs[i], func(r rune) bool { return r < 128 && unicode.IsLetter(r) }) >= 0 {
			return segs[i], true
		}
	}
	return segs[len(segs)-1], true
}
