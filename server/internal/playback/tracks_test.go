package playback

import (
	"encoding/json"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// An undetermined tag isn't a language, so it's never English and never a
// match.
func TestUndeterminedLanguagesAreUnknown(t *testing.T) {
	for _, tag := range []string{"und", "mul", "zxx"} {
		audio := []media.Track{{Language: tag, Default: true}, {Language: "eng"}}
		if got := pickAudio(audio, trackPrefs{audio: "en"}); got != 1 {
			t.Errorf("%s: English audio picks track %d, want the eng one", tag, got)
		}

		// Smart mode with French subtitles. Audio in an unknown language
		// isn't known to be foreign, so only French forced subtitles show.
		full, forced := id.New(), id.New()
		f := catalog.File{Tracks: []catalog.Track{
			{ID: id.New(), Type: "subtitle", Language: "eng", Forced: true},
			{ID: full, Type: "subtitle", Language: "fra"},
			{ID: forced, Type: "subtitle", Language: "fra", Forced: true},
		}}
		p := trackPrefs{subtitle: "fr", mode: users.SubtitlesSmart}
		if got := pickSubtitle(f, &audio[0], p); got != forced {
			t.Errorf("%s audio: subtitles %v, want the French forced track %v", tag, got, forced)
		}
	}
}

// dualAudio is an anime release with an English dub and the Japanese
// original, English signs as a forced track, and full English subtitles.
// The audio track named by def is flagged default.
func dualAudio(def string) (f catalog.File, eng, jpn, signs, full id.ID) {
	eng, jpn, signs, full = id.New(), id.New(), id.New(), id.New()
	track := func(i int, tid id.ID, kind media.TrackKind, codec, lang string, def, forced bool) catalog.Track {
		info, _ := json.Marshal(media.Track{Index: i, Kind: kind, Codec: media.Codec(codec), Language: lang, Default: def})
		return catalog.Track{ID: tid, Index: i, Type: string(kind), Codec: codec, Language: lang, Default: def, Forced: forced, Info: info}
	}
	f.Tracks = []catalog.Track{
		track(0, id.New(), media.Video, "h264", "", true, false),
		track(1, eng, media.Audio, "aac", "eng", def == "eng", false),
		track(2, jpn, media.Audio, "aac", "jpn", def == "jpn", false),
		track(3, signs, media.Subtitle, "ass", "eng", false, true),
		track(4, full, media.Subtitle, "subrip", "eng", false, false),
	}
	return f, eng, jpn, signs, full
}

// A user with no audio language hears the file's default track, whatever
// the work's original language. A preferred language that the file has
// wins.
func TestDualAudioWithNoPreferencePlaysTheDefaultTrack(t *testing.T) {
	dub, eng, jpn, signs, full := dualAudio("eng")
	orig, _, origJpn, _, origFull := dualAudio("jpn")
	tests := []struct {
		name                string
		f                   catalog.File
		p                   trackPrefs
		wantAudio, wantSubs id.ID
	}{
		{"dub default, forced mode", dub, trackPrefs{subtitle: "en", mode: users.SubtitlesForced}, eng, signs},
		{"dub default, smart mode", dub, trackPrefs{subtitle: "en", mode: users.SubtitlesSmart}, eng, signs},
		{"dub default, always", dub, trackPrefs{subtitle: "en", mode: users.SubtitlesAlways}, eng, full},
		{"dub default, never subtitles", dub, trackPrefs{subtitle: "en", mode: users.SubtitlesNone}, eng, id.ID{}},
		{"dub default, Japanese preferred", dub, trackPrefs{audio: "ja", subtitle: "en", mode: users.SubtitlesSmart}, jpn, full},
		{"original default, smart mode", orig, trackPrefs{subtitle: "en", mode: users.SubtitlesSmart}, origJpn, origFull},
	}
	for _, tt := range tests {
		got := preferredTracks(tt.f, tt.p)
		if got.Audio != tt.wantAudio || got.Subtitle != tt.wantSubs {
			t.Errorf("%s: audio %v subtitle %v, want %v and %v", tt.name, got.Audio, got.Subtitle, tt.wantAudio, tt.wantSubs)
		}
	}
}

// A series' last subtitle choice in a language the episode lacks falls
// back to the user's subtitle mode, as the audio falls back to the
// preferred language. A choice of off stays off.
func TestASeriesChoiceTheEpisodeLacksFallsBackToTheMode(t *testing.T) {
	f, eng, _, _, full := dualAudio("eng")
	spanish := &userstate.TrackChoice{AudioLanguage: "es", SubtitleLanguage: "es", SubtitleKind: "full"}
	got := preferredTracks(f, trackPrefs{audio: "en", subtitle: "en", mode: users.SubtitlesAlways, choice: spanish})
	if got.Audio != eng || got.Subtitle != full {
		t.Errorf("audio %v subtitle %v, want the English %v and %v", got.Audio, got.Subtitle, eng, full)
	}
	off := &userstate.TrackChoice{SubtitleKind: "off"}
	if got := preferredTracks(f, trackPrefs{subtitle: "en", mode: users.SubtitlesAlways, choice: off}); !got.Subtitle.IsZero() {
		t.Errorf("a choice of off shows %v", got.Subtitle)
	}
}
