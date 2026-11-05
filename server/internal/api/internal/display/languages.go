package display

import (
	"cmp"
	"strings"
	"unicode"
)

// languageNames are CLDR's names for languages, scripts and regions, as
// Intl.DisplayNames gives them, with the pattern and separator that join a
// language to its script and region.
type languageNames struct {
	Pattern   string            `json:"pattern"`
	Separator string            `json:"separator"`
	Languages map[string]string `json:"languages"`
	Scripts   map[string]string `json:"scripts"`
	Regions   map[string]string `json:"regions"`
}

// tag is a language tag's language, script and region in their canonical
// case, any of the last two empty.
type tag struct{ lang, script, region string }

// language is a language tag in words: a dialect by its own name
// ("American English"), else the language with its script and region
// ("Chinese (Traditional, Taiwan)"). An ISO 639-2 code is read as its
// language ("eng" is English). A tag that isn't one is shown as it is; no
// tag, und or "null" is the unknown language.
func language(raw *string) string {
	if raw == nil || strings.TrimSpace(*raw) == "" || strings.EqualFold(strings.TrimSpace(*raw), "null") {
		return tracks.UnknownLanguage
	}
	t, ok := splitTag(*raw)
	if !ok {
		return *raw
	}
	if a, ok := splitTag(aliases[t.lang]); ok {
		t = tag{a.lang, cmp.Or(t.script, a.script), cmp.Or(t.region, a.region)}
	}
	if t.lang == "und" {
		return tracks.UnknownLanguage
	}
	if t.script == "" || t.region == "" {
		if dialect, found := languages.Languages[t.lang+"-"+t.script+t.region]; found {
			return dialect
		}
	}
	name := lookup(languages.Languages, t.lang, t.lang)
	if piece := t.piece(); piece != "" {
		return strings.NewReplacer("{0}", name, "{1}", piece).Replace(languages.Pattern)
	}
	return name
}

// piece is the tag's script and region in words, joined by the
// separator, brackets in place of parentheses; "" for neither.
func (t tag) piece() string {
	var pieces []string
	if t.script != "" {
		pieces = append(pieces, lookup(languages.Scripts, t.script, t.script))
	}
	if t.region != "" {
		pieces = append(pieces, lookup(languages.Regions, t.region, t.region))
	}
	joined := ""
	for i, p := range pieces {
		p = strings.NewReplacer("(", "[", ")", "]").Replace(p)
		if i == 0 {
			joined = p
		} else {
			joined = strings.NewReplacer("{0}", joined, "{1}", p).Replace(languages.Separator)
		}
	}
	return joined
}

// splitTag is a tag's language, script and region, or false for anything
// else, such as a variant.
func splitTag(raw string) (tag, bool) {
	parts := strings.Split(strings.ReplaceAll(raw, "_", "-"), "-")
	t := tag{lang: strings.ToLower(parts[0])}
	if len(t.lang) < 2 || len(t.lang) > 3 || !all(t.lang, func(r rune) bool { return r >= 'a' && r <= 'z' }) {
		return tag{}, false
	}
	for _, p := range parts[1:] {
		if !t.add(p) {
			return tag{}, false
		}
	}
	return t, true
}

// add takes subtag p as the script or region, or reports false for one
// that is neither.
func (t *tag) add(p string) bool {
	switch {
	case t.script == "" && t.region == "" && len(p) == 4 && all(p, unicode.IsLetter):
		t.script = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	case t.region == "" && len(p) == 2 && all(p, unicode.IsLetter):
		t.region = strings.ToUpper(p)
	case t.region == "" && len(p) == 3 && all(p, unicode.IsDigit):
		t.region = p
	default:
		return false
	}
	return true
}

func all(s string, f func(rune) bool) bool {
	for _, r := range s {
		if !f(r) {
			return false
		}
	}
	return true
}
