package scan

import (
	"path"
	"strings"

	"golang.org/x/text/language"

	"github.com/mmcdole/kivo/internal/catalog"
)

// subFile is a subtitle file found while listing.
type subFile struct {
	rel    string
	inSubs bool // inside a Subs or Subtitles folder
}

func isSubsDir(name string) bool {
	return strings.EqualFold(name, "subs") || strings.EqualFold(name, "subtitles")
}

// attach returns the subtitle files that belong to the video at rel. Those
// are the files named after it, such as Movie.en.srt beside Movie.mkv. In
// a Subs folder, they are the files named after it, or all of the files
// when the folder holds only bare names like 2_English.srt.
func attach(rel string, subs []subFile) []catalog.SubtitleFile {
	stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	var out []catalog.SubtitleFile
	for _, s := range subs {
		base := path.Base(s.rel)
		name := strings.TrimSuffix(base, path.Ext(base))
		var tags string
		switch {
		case strings.EqualFold(name, stem):
		case len(name) > len(stem) && strings.EqualFold(name[:len(stem)], stem) && name[len(stem)] == '.':
			tags = name[len(stem)+1:]
		case s.inSubs && !strings.Contains(name, "."):
			tags = strings.TrimLeft(name, "0123456789_ -") // 2_English
		default:
			continue
		}
		t := catalog.SubtitleFile{RelPath: s.rel, Format: strings.TrimPrefix(ext(base), ".")}
		for tag := range strings.SplitSeq(tags, ".") {
			readTag(&t, tag)
		}
		out = append(out, t)
	}
	return out
}

// readTag reads one dot-separated token of a subtitle name: a language, or
// a flag (default, forced, sdh).
func readTag(t *catalog.SubtitleFile, tag string) {
	switch strings.ToLower(tag) {
	case "":
	case "default":
		t.Default = true
	case "forced", "foreign":
		t.Forced = true
	case "sdh", "cc", "hi":
		t.HearingImpaired = true
	default:
		if lang := parseLanguage(tag); lang != "" && t.Language == "" {
			t.Language = lang
		} else if t.Title == "" {
			t.Title = tag
		}
	}
}

// languageNames maps the English names people use instead of codes.
var languageNames = map[string]string{
	"english": "en", "french": "fr", "german": "de", "spanish": "es", "italian": "it", "portuguese": "pt",
	"brazilian": "pt-BR", "dutch": "nl", "swedish": "sv", "norwegian": "no", "danish": "da", "finnish": "fi",
	"polish": "pl", "russian": "ru", "japanese": "ja", "chinese": "zh", "korean": "ko", "arabic": "ar",
	"hebrew": "he", "turkish": "tr", "greek": "el", "czech": "cs", "hungarian": "hu", "romanian": "ro",
	"thai": "th", "vietnamese": "vi", "indonesian": "id", "hindi": "hi", "ukrainian": "uk",
}

// parseLanguage reads an ISO 639 code (en, eng, pt-BR) or an English name.
// It returns "" for anything else, so release-group tags aren't languages.
func parseLanguage(s string) string {
	if l, ok := languageNames[strings.ToLower(s)]; ok {
		return l
	}
	if len(s) < 2 || len(s) > 6 {
		return ""
	}
	tag, err := language.Parse(s)
	if err != nil {
		return ""
	}
	if b, conf := tag.Base(); conf == language.No || b.String() == "und" {
		return ""
	}
	return tag.String()
}
