package parse

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// keyStopWords are dropped from a Key except as its first or last word.
var keyStopWords = map[string]bool{"a": true, "an": true, "the": true, "and": true, "or": true, "of": true}

// Key returns the form titles are compared in. The title is decomposed with
// marks stripped, case-folded, and stripped of punctuation. "&" reads as
// "and". The words a, an, the, and, or, and of are dropped unless they are
// first or last. Apostrophes and periods vanish ("Marvel's",
// "S.H.I.E.L.D."), and other punctuation separates words.
func Key(title string) string {
	s := norm.NFKD.String(cases.Fold().String(title))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r), r == '\'', r == '’', r == '.':
		case r == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	words := strings.Fields(b.String())
	out := words[:0]
	for i, w := range words {
		if i == 0 || i == len(words)-1 || !keyStopWords[w] {
			out = append(out, w)
		}
	}
	return strings.Join(out, " ")
}
