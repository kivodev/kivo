package display

import (
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

func ptr[T any](v T) *T { return &v }

// audio is an audio track; an empty language or layout is left out.
func audio(codec, lang string, channels int, layout string) gen.Track {
	t := gen.Track{Type: gen.TrackTypeAudio, Codec: codec, Audio: &gen.AudioTrackInfo{Channels: channels}}
	if lang != "" {
		t.Language = &lang
	}
	if layout != "" {
		t.Audio.Layout = &layout
	}
	return t
}

// subtitle is a subtitle track from source; an empty language is left out.
func subtitle(codec, lang string, source gen.SubtitleSource) gen.Track {
	t := gen.Track{Type: gen.TrackTypeSubtitle, Codec: codec, Subtitle: &gen.SubtitleTrackInfo{Source: source}}
	if lang != "" {
		t.Language = &lang
	}
	return t
}

func with(t gen.Track, f func(*gen.Track)) gen.Track {
	f(&t)
	return t
}

func titled(t gen.Track, title string) gen.Track {
	return with(t, func(t *gen.Track) { t.Title = &title })
}

func names(ts ...gen.Track) []string {
	NameTracks(ts)
	var out []string
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}

func TestNames(t *testing.T) {
	const embedded, external, downloaded = gen.SubtitleSourceEmbedded, gen.SubtitleSourceExternal, gen.SubtitleSourceDownloaded
	atmos := func(t *gen.Track) { t.Audio.Profile = ptr(gen.AudioProfileAtmos) }
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{
			"a lossless or premium format always, other codecs only to tell tracks apart",
			names(with(audio("truehd", "eng", 8, "7.1"), atmos), audio("eac3", "eng", 6, "5.1"), audio("ac3", "eng", 6, "5.1"),
				audio("flac", "eng", 2, "2.0")),
			[]string{"English · 7.1 · TrueHD Atmos", "English · 5.1 · E-AC-3", "English · 5.1 · AC-3", "English · 2.0 · FLAC"},
		},
		{
			"the original beside a dub",
			names(with(audio("aac", "jpn", 2, "2.0"), func(t *gen.Track) { t.Original = ptr(true) }), audio("aac", "eng", 2, "2.0")),
			[]string{"Japanese [Original] · 2.0", "English · 2.0"},
		},
		{
			"flags, never the title",
			names(titled(with(subtitle("subrip", "eng", embedded), func(t *gen.Track) { t.Forced = ptr(true) }), "English [Forced]"),
				titled(subtitle("subrip", "eng", embedded), "Forced"),
				with(audio("ac3", "eng", 2, "2.0"), func(t *gen.Track) { t.Commentary = ptr(true) }),
				audio("aac", "eng", 1, "1.0")),
			[]string{"English · Forced", "English", "English · Commentary · 2.0", "English · Mono"},
		},
		{
			"an embedded subtitle beside a file, then numbers",
			names(subtitle("subrip", "eng", embedded), subtitle("subrip", "eng", external),
				subtitle("subrip", "fre", embedded), subtitle("subrip", "fre", embedded), subtitle("ass", "fre", embedded)),
			[]string{"English", "English · External", "French · SRT · 1", "French · SRT · 2", "French · ASS"},
		},
		{
			"where a downloaded subtitle came from",
			names(subtitle("subrip", "eng", embedded), subtitle("subrip", "eng", downloaded), subtitle("subrip", "eng", downloaded),
				subtitle("ass", "eng", downloaded)),
			[]string{"English", "English · OpenSubtitles · SRT · 1", "English · OpenSubtitles · SRT · 2", "English · OpenSubtitles · ASS"},
		},
		{
			"apart only when the titles are the same too",
			names(titled(subtitle("subrip", "eng", embedded), "Signs"), titled(subtitle("ass", "eng", embedded), "Dialogue"),
				titled(subtitle("subrip", "spa", embedded), "Signs"), titled(subtitle("ass", "spa", embedded), "Signs")),
			[]string{"English", "English", "Spanish · SRT", "Spanish · ASS"},
		},
		{
			"without a language",
			names(subtitle("subrip", "", embedded), subtitle("subrip", "null", embedded), audio("aac", "und", 2, "2.0")),
			[]string{"Unknown language · 1", "Unknown language · 2", "Unknown language · 2.0"},
		},
		{
			"a video by its codec, resolution and HDR",
			names(gen.Track{Type: gen.TrackTypeVideo, Codec: "hevc", Video: &gen.VideoTrackInfo{
				Width: 3840, Profile: ptr("Main 10"), Resolution: ptr(gen.Resolution("4k")), Hdr: ptr(gen.HdrFormatDolbyVision),
			}}),
			[]string{"HEVC 4K Dolby Vision"},
		},
	}
	for _, tt := range tests {
		if !slices.Equal(tt.got, tt.want) {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestFullName(t *testing.T) {
	tests := []struct {
		track gen.Track
		want  string
	}{
		{titled(with(audio("eac3", "eng", 6, "5.1"), func(t *gen.Track) { t.Default = true }), "E-AC3 DD+5.1"),
			"English · E-AC-3 5.1 · Default · “E-AC3 DD+5.1”"},
		{with(audio("truehd", "eng", 8, "7.1"), func(t *gen.Track) { t.Audio.Profile = ptr(gen.AudioProfileAtmos) }),
			"English · TrueHD Atmos 7.1"},
		{audio("aac", "eng", 3, ""), "English · AAC 3 channels"},
		{titled(with(subtitle("subrip", "eng", gen.SubtitleSourceExternal), func(t *gen.Track) { t.HearingImpaired = ptr(true) }), "SDH .srt"),
			"English · SDH · SRT · External · “SDH .srt”"},
		{gen.Track{Type: gen.TrackTypeVideo, Codec: "hevc", Video: &gen.VideoTrackInfo{
			Width: 3840, Profile: ptr("Main 10"), Resolution: ptr(gen.Resolution("4k")), Hdr: ptr(gen.HdrFormatDolbyVision),
		}}, "HEVC Main 10 4K Dolby Vision"},
	}
	for _, tt := range tests {
		if got := fullName(tt.track); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}
