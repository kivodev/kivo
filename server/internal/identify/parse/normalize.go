package parse

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// brackets maps the CJK lenticular brackets that anime releases use to
// ASCII ones. NFKC leaves them alone, unlike full-width brackets and
// digits.
var brackets = strings.NewReplacer("【", "[", "】", "]")

// matchForm returns the form every rule matches against. It applies NFKC,
// so full-width digits, brackets and spaces read as ASCII, and it makes
// lenticular brackets square. Callers pass NFC text, which is already NFKC
// for most names.
func matchForm(s string) string {
	return brackets.Replace(norm.NFKC.String(s))
}

// extensions are the file extensions stripped before parsing. Names without
// one of these keep their last dot-separated token, since scene names such
// as "Show.S01E03" have no extension to strip.
var extensions = map[string]bool{
	"3gp": true, "asf": true, "avi": true, "divx": true, "flv": true, "iso": true, "m2ts": true,
	"m4v": true, "mk3d": true, "mkv": true, "mov": true, "mp4": true, "mpeg": true, "mpg": true,
	"mts": true, "ogm": true, "ogv": true, "rm": true, "rmvb": true, "strm": true, "ts": true,
	"vob": true, "webm": true, "wmv": true,
	"ass": true, "idx": true, "nfo": true, "smi": true, "srt": true, "ssa": true, "sub": true,
	"sup": true, "vtt": true,
}

// stripExt removes a known extension from a file name.
func stripExt(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || len(name)-i > 6 || !extensions[strings.ToLower(name[i+1:])] {
		return name
	}
	return name[:i]
}

// wordAt reports whether the rune starting at s[i] is a letter or digit. It
// returns false at the end of s. Rules call it to check the character after
// a match, which is how they restate a negative lookahead.
func wordAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// digitAt reports whether s[i] is an ASCII digit. It returns false outside
// s.
func digitAt(s string, i int) bool { c := byteAt(s, i); return c >= '0' && c <= '9' }

// byteAt returns s[i], or 0 past the end.
func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

// lower returns the ASCII lower case of b.
func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}

// atoi parses a run of ASCII digits, saturating instead of overflowing. The
// rules cap their digit counts, so saturation never shows.
func atoi(s string) int {
	n := 0
	for i := 0; i < len(s) && n < 1e6; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// blank replaces s[i:j] with a space, so the words on each side stay apart.
func blank(s string, i, j int) string { return s[:i] + " " + s[j:] }
