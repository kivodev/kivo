package media

import (
	"slices"
	"testing"
)

func TestBaseLanguage(t *testing.T) {
	for tag, want := range map[string]string{
		"en": "en", "eng": "en", "en-US": "en", "pt-BR": "pt", "jpn": "ja",
		"": "", "und": "", "und-JP": "", "mul": "", "zxx": "", "not a tag": "",
	} {
		if got := BaseLanguage(tag); got != want {
			t.Errorf("BaseLanguage(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestOriginalAudio(t *testing.T) {
	audio := func(lang string, commentary bool) Track {
		return Track{Kind: Audio, Language: lang, Commentary: commentary}
	}
	// With a dub beside the original, the original tracks are marked but its commentary isn't.
	if got := OriginalAudio([]Track{audio("jpn", false), audio("eng", false), audio("ja", true)}, "ja"); !slices.Equal(got, []bool{true, false, false}) {
		t.Errorf("anime with a dub: %v", got)
	}
	// A track whose title says commentary is a commentary too.
	titled := Track{Kind: Audio, Language: "jpn", Title: "Director's commentary"}
	if got := OriginalAudio([]Track{audio("jpn", false), audio("eng", false), titled}, "ja"); !slices.Equal(got, []bool{true, false, false}) {
		t.Errorf("a commentary by its title: %v", got)
	}
	// When every track is in one language, there's nothing to tell apart.
	if got := OriginalAudio([]Track{audio("eng", false), audio("eng", false), audio("eng", true)}, "en"); !slices.Equal(got, []bool{false, false, false}) {
		t.Errorf("one language: %v", got)
	}
	// A commentary in another language is not a dub.
	if got := OriginalAudio([]Track{audio("eng", false), audio("fre", true)}, "en"); !slices.Equal(got, []bool{false, false}) {
		t.Errorf("a commentary in another language: %v", got)
	}
	// Nothing is marked with no original language, or with no audio in it.
	if got := OriginalAudio([]Track{audio("eng", false), audio("fre", false)}, ""); !slices.Equal(got, []bool{false, false}) {
		t.Errorf("no original language: %v", got)
	}
	// Only audio is marked.
	if got := OriginalAudio([]Track{{Kind: Subtitle, Language: "jpn"}, audio("jpn", false), audio("eng", false)}, "ja"); !slices.Equal(got, []bool{false, true, false}) {
		t.Errorf("a subtitle in the original language: %v", got)
	}
}
