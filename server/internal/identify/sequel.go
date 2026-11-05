package identify

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mmcdole/kivo/internal/identify/parse"
)

// halves matches a number and a half, as in "The Lion King 1½". A key would
// otherwise split "1½" into the numbers 1 and 2.
var halves = regexp.MustCompile(`([0-9])½`)

// numbering are the words after which a sequel number can also be a word or
// the numeral I, as in "Part One" or "Chapter I".
var numbering = map[string]bool{"part": true, "chapter": true, "vol": true, "volume": true}

var numberWords = map[string]string{
	"one": "1", "two": "2", "three": "3", "four": "4", "five": "5",
	"six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10",
}

var romans = map[string]string{
	"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7",
	"viii": "8", "ix": "9", "x": "10", "xi": "11", "xii": "12", "xiii": "13",
	"xiv": "14", "xv": "15", "xvi": "16", "xvii": "17", "xviii": "18", "xix": "19", "xx": "20",
}

// titleKey returns the key a title is compared by. It is the title's
// parse.Key with its sequel numbers in digits, so "Rocky II" compares as
// "rocky 2" and "Dune Part One" as "dune part 1".
func titleKey(title string) string {
	words := strings.Fields(parse.Key(halves.ReplaceAllString(title, "${1}half")))
	for i := range words {
		if n := sequelNumber(words, i); n != "" {
			words[i] = n
		}
	}
	return strings.Join(words, " ")
}

// sequelNumber returns the sequel number words[i] is, in digits, or empty
// when it is none. Any word that is a whole number from 1 to 99 without a
// leading zero is one, and so is a number and a half ("1half"). So is a
// Roman numeral from II to XX as the last word, and after "Part",
// "Chapter" or "Volume" also I or a number word from one to ten. A title of
// one word has none. So "21 Jump Street", "Toy Story 3", "Rocky II" and
// "Dune Part One" have sequel numbers, while "1917", "101 Dalmatians",
// "Blade Runner 2049" and "Ocean's Eleven" don't.
func sequelNumber(words []string, i int) string {
	w := words[i]
	if len(words) < 2 {
		return ""
	}
	if whole, half := strings.CutSuffix(w, "half"); small(w) || half && small(whole) {
		return w
	}
	after := i > 0 && numbering[words[i-1]]
	switch {
	case after && numberWords[w] != "":
		return numberWords[w]
	case romans[w] != "" && (after || i == len(words)-1 && w != "i"):
		return romans[w]
	}
	return ""
}

// small reports whether s is a whole number from 1 to 99, written without a
// leading zero.
func small(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && s[0] != '0' && n > 0 && n < 100
}

// differentSequel reports whether two keys, as titleKey returns them, name
// different entries in a series. They do when both have sequel numbers and
// the numbers differ, as "How to Train Your Dragon 2" and "3" do, or "21
// Jump Street" and "22". They also do when one is the other plus sequel
// numbers other than a lone 1, as "Stalingrad 2" is to "Stalingrad".
func differentSequel(a, b string) bool {
	baseA, numsA := sequelNumbers(a)
	baseB, numsB := sequelNumbers(b)
	switch {
	case len(numsA) > 0 && len(numsB) > 0:
		return !slices.Equal(numsA, numsB)
	case len(numsA) > 0:
		return baseA == b && !slices.Equal(numsA, []string{"1"})
	case len(numsB) > 0:
		return baseB == a && !slices.Equal(numsB, []string{"1"})
	}
	return false
}

// sequelNumbers splits a key from titleKey into the key without its sequel
// numbers and those numbers, sorted.
func sequelNumbers(k string) (string, []string) {
	words := strings.Fields(k)
	var base, nums []string
	for i, w := range words {
		if sequelNumber(words, i) != "" {
			nums = append(nums, w)
		} else {
			base = append(base, w)
		}
	}
	slices.Sort(nums)
	return strings.Join(base, " "), nums
}
