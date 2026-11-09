package store

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SearchWords returns the words of s as the search indexes' unicode61
// tokenizer splits them. A word is a run of letters and digits, without
// case or diacritics.
func SearchWords(s string) []string {
	return strings.Fields(fold(s))
}

// Fold is s with case and diacritics removed, for comparing text the way
// the fold SQL function does.
func Fold(s string) string { return fold(s) }

// SearchKey returns s in the form search uses to compare it whole. That is
// its words separated by single spaces, without a leading "the", "a" or
// "an" when more words follow.
func SearchKey(s string) string {
	k := fold(s)
	for _, a := range []string{"the ", "a ", "an "} {
		if rest, ok := strings.CutPrefix(k, a); ok {
			return rest
		}
	}
	return k
}

// fold returns the words of s separated by single spaces. Ranking folds
// every matching title, so ASCII text, which most titles are, takes a fast
// path.
func fold(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 0x80:
			return foldUnicode(s)
		case 'a' <= c && c <= 'z' || '0' <= c && c <= '9':
			b = append(b, c)
		case 'A' <= c && c <= 'Z':
			b = append(b, c+'a'-'A')
		case len(b) > 0 && b[len(b)-1] != ' ':
			b = append(b, ' ')
		}
	}
	return strings.TrimSuffix(string(b), " ")
}

func foldUnicode(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// MatchRank ranks a row of work_fts, person_fts or collection_fts that
// matches a query. Lower ranks are better.
//   - text is the row's first column, which is a work's title or a
//     person's or collection's name.
//   - key is the query's SearchKey, whose last word is a prefix.
//   - columns has bit i set when column i holds every word of the query.
//
// The ranks are:
//
//  1. text is the query
//  2. text starts with the query
//  3. text holds the query as a phrase
//  4. the first column holds every word otherwise
//  5. the second column (a work's alternative titles) holds every word
//  6. the words are spread over columns, or only in the third (the people
//     credited on the work)
//
// SQL calls it as the FTS5 auxiliary function match_rank(table, text, key).
func MatchRank(text, key string, columns uint64) int {
	if columns&1 == 0 {
		if columns&2 != 0 {
			return 5
		}
		return 6
	}
	k := SearchKey(text)
	switch {
	case k == key:
		return 1
	case strings.HasPrefix(k, key):
		return 2
	case strings.Contains(" "+k, " "+key):
		return 3
	}
	return 4
}
