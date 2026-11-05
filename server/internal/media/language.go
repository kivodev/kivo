package media

import "golang.org/x/text/language"

// BaseLanguage returns a track tag's language without its region, which is
// how tracks are compared. It returns "en" for en-US and for ISO 639-2's
// eng. It returns "" when the tag gives no language: a tag that doesn't
// parse, und, mul, zxx, or any tag whose language would only be guessed.
// So an untagged track is never taken for English.
func BaseLanguage(tag string) string {
	t, err := language.Parse(tag)
	if err != nil {
		return ""
	}
	b, conf := t.Base()
	switch s := b.String(); {
	case conf != language.Exact, s == "und", s == "mul", s == "zxx":
		return ""
	default:
		return s
	}
}

// OriginalAudio reports, for each of tracks, whether it is audio in the
// work's original language. That tells the original audio from a dub, so it
// is only true when tracks also have audio in another language. A
// commentary is never the original.
func OriginalAudio(tracks []Track, original string) []bool {
	out := make([]bool, len(tracks))
	lang := BaseLanguage(original)
	if lang == "" {
		return out
	}
	other := false
	for i, t := range tracks {
		if t.Kind != Audio || t.IsCommentary() {
			continue
		}
		if BaseLanguage(t.Language) == lang {
			out[i] = true
		} else {
			other = true
		}
	}
	if !other {
		clear(out)
	}
	return out
}
