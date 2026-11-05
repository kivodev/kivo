package playback

import (
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
)

// dolbyTV plays Matroska directly, chooses a file's audio track itself, and
// decodes Dolby Digital and Dolby Digital Plus, but not TrueHD or DTS.
var dolbyTV = func() Capabilities {
	c := alike([]string{"mkv"}, true, []VideoEntry{{Codec: "h264", MaxBitDepth: 8}},
		[]AudioEntry{{Codec: "aac", MaxChannels: 6}, {Codec: "ac3", MaxChannels: 6}, {Codec: "eac3", MaxChannels: 6}}, vttFile)
	c.File.TrackSelection = true
	return c
}()

// TestStartPicksPlayableAudio checks that, when the track a play would
// choose can't be played from the file but another in the same language
// can, without losing surround, the play takes that one and plays the file
// directly: the one with the most channels, then E-AC-3 over AC-3, then the
// first in the file. A commentary, an audio description, another language,
// or a stereo track standing in for surround isn't taken, a client that
// can't choose a file's track keeps the file's default, and a track the
// client asks for is kept.
func TestStartPicksPlayableAudio(t *testing.T) {
	video := media.Track{Index: 0, Kind: media.Video, Codec: "h264", Profile: "High", Width: 1920, Height: 1080, BitDepth: 8}
	truehd := media.Track{Index: 1, Kind: media.Audio, Codec: "truehd", Channels: 8, Default: true, Language: "eng"}
	stereoDTS := media.Track{Index: 1, Kind: media.Audio, Codec: "dts", Channels: 2, Default: true, Language: "eng"}
	cases := []struct {
		name  string
		caps  Capabilities
		def   *media.Track // the file's default audio; TrueHD 7.1 when nil
		other []media.Track
		ask   int // the index of the audio track the request names, or zero
		want  int // the index of the audio track played
		file  bool
	}{
		{name: "same-language AC-3", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng"}}, want: 2, file: true},
		{name: "E-AC-3 over AC-3", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng"}, {Codec: "eac3", Channels: 6, Language: "eng"}}, want: 3, file: true},
		{name: "most channels first", caps: dolbyTV, def: &stereoDTS, other: []media.Track{{Codec: "eac3", Channels: 2, Language: "eng"}, {Codec: "ac3", Channels: 6, Language: "eng"}}, want: 3, file: true},
		{name: "equal tracks in the file's order", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng"}, {Codec: "ac3", Channels: 6, Language: "eng"}}, want: 2, file: true},
		{name: "audio description", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng", VisualImpaired: true}}, want: 1},
		{name: "descriptive by title", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng", Title: "English Descriptive Audio"}}, want: 1},
		{name: "audio description by title", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng", Title: "Audio Description"}}, want: 1},
		{name: "commentary", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng", Title: "Director's Commentary"}}, want: 1},
		{name: "commentary flag", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng", Commentary: true}}, want: 1},
		{name: "another language", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "fra"}}, want: 1},
		{name: "stereo for surround", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 2, Language: "eng"}}, want: 1},
		{name: "no track selection", caps: assTVWithDolby(), other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng"}}, want: 1},
		{name: "asked for", caps: dolbyTV, other: []media.Track{{Codec: "ac3", Channels: 6, Language: "eng"}}, ask: 1, want: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, 0)
			m := f.b.Movie(f.folder, "Blade Runner 2049", 2017)
			def := truehd
			if c.def != nil {
				def = *c.def
			}
			tracks := []media.Track{video, def}
			for i, o := range c.other {
				o.Index, o.Kind = 2+i, media.Audio
				tracks = append(tracks, o)
			}
			f.probeWith(m.File, "mkv", 2*time.Hour, 20_000_000, tracks...)
			cur, err := f.b.Catalog.File(ctx, m.File)
			if err != nil {
				t.Fatal(err)
			}
			off := id.ID{}
			r := StartRequest{Work: m.Work, SubtitleTrack: &off, Caps: c.caps, ClientIP: lan}
			if c.ask != 0 {
				asked := trackID(cur, "audio", c.ask)
				r.AudioTrack = &asked
			}
			s, err := f.svc.Start(ctx, f.p, f.v, r)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := audioOf(s), trackID(cur, "audio", c.want); got != want {
				t.Errorf("played audio %v, want track %d (%v)", got, c.want, want)
			}
			if got := s.Decision.Delivery == DeliverFile; got != c.file {
				t.Errorf("delivery %v, want file %v", s.Decision.Delivery, c.file)
			}
		})
	}
}

// assTVWithDolby is a TV that decodes Dolby but plays only a file's default
// audio track.
func assTVWithDolby() Capabilities {
	c := dolbyTV
	file := *c.File
	file.TrackSelection = false
	c.File = &file
	return c
}
