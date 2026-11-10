package parse

import (
	"strings"
	"unicode/utf8"
)

// tagWords are the release tags that scene and fansub names put after a
// title: resolution, source, codec, audio and flag tokens. They are lower
// case with their inner separators removed, so "WEB-DL", "web.dl" and
// "WEB DL" are all "webdl", and "H.264" is "h264". Words that are also
// common title words ("web", "cam", "3d", "complete") are left out. The
// first tag marks where a title ends.
var tagWords = map[string]bool{
	"uhd": true, "bluray": true, "bdrip": true, "brrip": true, "bdremux": true, "remux": true,
	"webdl": true, "webrip": true, "hdtv": true, "hdtvrip": true, "pdtv": true, "sdtv": true,
	"dvd": true, "dvdrip": true, "dvdscr": true, "dvdr": true, "dvd5": true, "dvd9": true,
	"hdrip": true, "hdcam": true, "camrip": true, "telesync": true, "telecine": true, "hdts": true,
	"hdtc": true, "r5": true, "vhsrip": true, "satrip": true, "ppv": true,

	"x264": true, "x265": true, "h264": true, "h265": true, "hevc": true, "avc": true, "xvid": true,
	"divx": true, "av1": true, "10bit": true, "8bit": true, "hi10": true, "hi10p": true,

	"ac3": true, "eac3": true, "aac": true, "aac20": true, "aac51": true, "dts": true, "dtshd": true,
	"truehd": true, "atmos": true, "flac": true, "dd": true, "dd20": true, "dd51": true, "dd71": true,
	"ddp": true, "ddp20": true, "ddp51": true, "ddp71": true, "mp3": true, "opus": true,

	"hdr": true, "hdr10": true, "dovi": true, "imax": true, "proper": true, "repack": true,
	"rerip": true, "internal": true, "limited": true, "extended": true, "unrated": true,
	"uncut": true, "remastered": true, "theatrical": true, "readnfo": true,

	"subbed": true, "dubbed": true, "sub": true, "subs": true, "esub": true, "esubs": true,
	"msubs": true, "multisub": true, "multisubs": true, "dualaudio": true, "vostfr": true,
	"vost": true, "truefrench": true, "subfrench": true, "subforced": true, "fastsub": true,
}

// capsTags are language tags. They count only in capitals, as scene names
// write them, so "The French Connection" keeps its title.
var capsTags = map[string]bool{
	"FRENCH": true, "GERMAN": true, "ITALIAN": true, "SPANISH": true, "ENGLISH": true,
	"HINDI": true, "TAMIL": true, "TELUGU": true, "RUSSIAN": true, "MULTI": true, "MULTi": true,
	"DUAL": true, "NORDIC": true, "SWESUB": true, "PLDUB": true,
}

// tagDelims are the characters a tag may follow.
const tagDelims = " ._[](){},+-"

// firstTag returns where the first release tag after some title text
// starts, or len(s) when there is none.
func firstTag(s string) int {
	seen := false
	for i := 0; i < len(s); {
		if seen && (i == 0 || strings.IndexByte(tagDelims, s[i-1]) >= 0) && tagAt(s, i) >= 0 {
			return i
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		seen = seen || isWord(r)
		i += size
	}
	return len(s)
}

// startsWithTag reports whether s starts with a release tag, such as
// "[720p]" or "[DVDRip-ITA]". It checks only the start because a group name
// such as "Darksoul-Subs" can end in one.
func startsWithTag(s string) bool { return tagAt(s, 0) >= 0 }

// tagAt returns the end of the release tag starting at s[i], or -1. A tag is
// one to three ASCII letter-and-digit runs joined by single separators, and
// the longest run that forms a tag wins. The character after a tag can't be
// a letter or digit. A regular expression would need lookahead for that
// check.
func tagAt(s string, i int) int {
	var buf [32]byte
	n, end := 0, -1
	for run := 0; run < 3; run++ {
		k := i
		for k < len(s) && isAlnum(s[k]) {
			k++
		}
		if k == i || n+k-i > len(buf) {
			break
		}
		if run == 0 && capsTags[s[i:k]] && !wordAt(s, k) {
			end = k
		}
		for p := i; p < k; p++ {
			buf[n] = lower(s[p])
			n++
		}
		if !wordAt(s, k) && (tagWords[string(buf[:n])] || numericTag(buf[:n])) {
			end = k
		}
		if k >= len(s) || strings.IndexByte("-. _", s[k]) < 0 {
			break
		}
		i = k + 1
	}
	return end
}

// numericTag reports whether b, in lower case, is a resolution (1080p, 4k,
// 1920x1080) or a size (700mb, or 23gb from "2.3GB").
func numericTag(b []byte) bool {
	d := 0
	for d < len(b) && b[d] >= '0' && b[d] <= '9' {
		d++
	}
	rest := string(b[d:])
	switch {
	case d == 1 && (b[0] == '4' || b[0] == '8') && rest == "k":
		return true
	case d >= 3 && d <= 4 && (rest == "p" || rest == "i"):
		return true
	case d > 0 && (rest == "mb" || rest == "gb"):
		return true
	case d >= 3 && d <= 4 && len(rest) >= 4 && len(rest) <= 5 && rest[0] == 'x':
		for j := 1; j < len(rest); j++ {
			if rest[j] < '0' || rest[j] > '9' {
				return false
			}
		}
		return true
	}
	return false
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
