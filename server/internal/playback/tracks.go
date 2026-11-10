package playback

import (
	"context"
	"log/slog"
	"slices"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// trackPrefs choose a play's tracks when the request doesn't. The series'
// last choice comes first, then the user's languages and subtitle mode,
// then the file's own default flags.
type trackPrefs struct {
	audio    string                 // the base of the preferred audio language; empty means the file's default track
	subtitle string                 // the base of the preferred subtitle language, or else of the metadata language
	mode     string                 // one of the users package's subtitle modes
	choice   *userstate.TrackChoice // the series' last choice, for an episode
	series   id.ID                  // zero unless the work is an episode and the user remembers choices
}

// prefsFor returns a user's track preferences. For an episode, when the
// user remembers choices, it includes the series' last choice. Anything it
// can't read falls back to the file's defaults.
func (s *planner) prefsFor(ctx context.Context, user users.User, work id.ID) trackPrefs {
	p := trackPrefs{subtitle: media.BaseLanguage(s.deps.Settings.Get().MetadataLanguage), mode: users.SubtitlesForced}
	pb := user.Playback
	p.audio, p.mode = media.BaseLanguage(pb.AudioLanguage), pb.SubtitleMode
	if pb.SubtitleLanguage != "" {
		p.subtitle = media.BaseLanguage(pb.SubtitleLanguage)
	}
	series, c, ok, err := s.deps.Userstate.SeriesChoice(ctx, user, work)
	p.series = series
	if err == nil && ok {
		p.choice = &c
	}
	return p
}

// showsFull reports whether the user's mode shows full subtitles in the
// subtitle language when the audio is in audioLang. The always mode does.
// The smart mode does when the audio is in a language other than the
// subtitle language.
func (p trackPrefs) showsFull(audioLang string) bool {
	switch p.mode {
	case users.SubtitlesAlways:
		return true
	case users.SubtitlesSmart:
		return audioLang != "" && audioLang != p.subtitle
	}
	return false
}

// chosen holds the tracks a client chose for a play. A field is nil when
// the client chose no track, and a zero subtitle means subtitles off.
type chosen struct {
	audio, subtitle *id.ID
}

// remember records an episode's chosen tracks as its series' last
// choice, so the next episode starts with the same languages.
func (s *Service) remember(ctx context.Context, pl *play, p trackPrefs, ch chosen) {
	if p.series.IsZero() || ch.audio == nil && ch.subtitle == nil {
		return
	}
	c := userstate.TrackChoice{SubtitleKind: "off"}
	if p.choice != nil {
		c = *p.choice
	}
	if ch.audio != nil {
		if i := slices.IndexFunc(pl.files[0].Tracks, func(t catalog.Track) bool { return t.ID == *ch.audio }); i >= 0 {
			c.AudioLanguage = media.BaseLanguage(pl.files[0].Tracks[i].Language)
		}
	}
	if ch.subtitle != nil {
		c.SubtitleLanguage, c.SubtitleKind = "", "off"
		if sub, ok := subtitleTrack(pl.files[0], *ch.subtitle); ok {
			c.SubtitleLanguage, c.SubtitleKind = media.BaseLanguage(sub.lang), sub.kind()
		}
	}
	if err := s.deps.Userstate.SetTrackChoice(ctx, pl.user, p.series, c); err != nil {
		slog.WarnContext(ctx, "playback: remember track choice", "work", pl.work, "err", err)
	}
}

// Tracks are the audio and subtitle tracks a play starts with. A zero
// Audio means no audio, and a zero Subtitle means subtitles off.
type Tracks struct {
	Audio, Subtitle id.ID
}

// Preferred is how a user's plays of a work start when their requests
// choose no tracks.
type Preferred struct {
	Versions map[id.ID]Tracks // keyed by version
	// Remembers reports whether choosing tracks for the work remembers
	// them for its series. That's true for an episode when the user
	// remembers choices.
	Remembers bool
}

// Preferred returns the tracks that the viewer's play of each of a work's
// versions starts with when the request chooses none. They come from the
// user's preferences and, for an episode, the series' last choice.
func (s *Service) Preferred(ctx context.Context, v users.Viewer, work id.ID) (Preferred, error) {
	u, err := s.deps.Users.User(ctx, v.User())
	if err != nil {
		return Preferred{}, err
	}
	p := s.prefsFor(ctx, u, work)
	versions, err := s.deps.Catalog.VersionsOf(ctx, work)
	if err != nil {
		return Preferred{}, err
	}
	out := Preferred{Versions: map[id.ID]Tracks{}, Remembers: !p.series.IsZero()}
	for _, ver := range versions {
		if len(ver.Parts) == 0 {
			continue
		}
		f, err := s.deps.Catalog.File(ctx, ver.Parts[0])
		if err != nil {
			return Preferred{}, err
		}
		out.Versions[ver.ID] = preferredTracks(f, p)
	}
	return out, nil
}

// preferredTracks is the tracks a play of a file starts with when its
// request chooses none.
func preferredTracks(f catalog.File, p trackPrefs) Tracks {
	var audio []media.Track
	var ids []id.ID
	for _, t := range f.Tracks {
		if media.TrackKind(t.Type) == media.Audio {
			audio, ids = append(audio, probed(t)), append(ids, t.ID)
		}
	}
	var out Tracks
	var a *media.Track
	if len(audio) > 0 {
		i := pickAudio(audio, p)
		out.Audio, a = ids[i], &audio[i]
	}
	out.Subtitle = pickSubtitle(f, a, p)
	return out
}

// pickAudio returns the index in audio of the track to play. It looks for
// the language of the series' last choice, then the preferred language.
// Within a language, a default-flagged track wins. If neither language is
// present, it returns the file's default track.
func pickAudio(audio []media.Track, p trackPrefs) int {
	var langs []string
	if p.choice != nil && p.choice.AudioLanguage != "" {
		langs = append(langs, p.choice.AudioLanguage)
	}
	if p.audio != "" {
		langs = append(langs, p.audio)
	}
	for _, l := range langs {
		best := -1
		for i, t := range audio {
			if media.BaseLanguage(t.Language) == l && (best < 0 || t.Default && !audio[best].Default) {
				best = i
			}
		}
		if best >= 0 {
			return best
		}
	}
	return max(slices.IndexFunc(audio, func(t media.Track) bool { return t.Default }), 0)
}

// subTrack is one subtitle track a file offers, either embedded or a
// subtitle file of its own.
type subTrack struct {
	id         id.ID
	lang       string
	forced, hi bool
}

func (s subTrack) kind() string {
	switch {
	case s.forced:
		return "forced"
	case s.hi:
		return "sdh"
	}
	return "full"
}

func subtitlesOf(f catalog.File) []subTrack {
	var out []subTrack
	for _, t := range f.Tracks {
		if media.TrackKind(t.Type) == media.Subtitle {
			out = append(out, subTrack{id: t.ID, lang: t.Language, forced: t.Forced, hi: t.HearingImpaired})
		}
	}
	for _, x := range f.SubtitleFiles {
		out = append(out, subTrack{id: x.ID, lang: x.Language, forced: x.Forced, hi: x.HearingImpaired})
	}
	return out
}

func subtitleTrack(f catalog.File, track id.ID) (subTrack, bool) {
	subs := subtitlesOf(f)
	i := slices.IndexFunc(subs, func(s subTrack) bool { return s.id == track })
	if i < 0 {
		return subTrack{}, false
	}
	return subs[i], true
}

// offersSubtitle reports whether a subtitle track is one the play's
// version offers, or the zero ID for off.
func (pl *play) offersSubtitle(track id.ID) bool {
	_, ok := subtitleTrack(pl.files[0], track)
	return ok || track.IsZero()
}

// offersAudio reports whether an audio track is one the play's version
// offers.
func (pl *play) offersAudio(track id.ID) bool {
	return slices.ContainsFunc(pl.files[0].Tracks, func(t catalog.Track) bool {
		return t.ID == track && media.TrackKind(t.Type) == media.Audio
	})
}

// pickSubtitle returns the subtitle track to show, or the zero ID for none.
// It follows the series' last choice, or the user's subtitle mode when
// the file has nothing in that choice's language.
// Every mode except none falls back to a forced track in the audio's
// language, for the parts spoken in another language. A file's default
// flag isn't trusted, because mkvmerge sets it on the first track of every
// kind.
func pickSubtitle(f catalog.File, audio *media.Track, p trackPrefs) id.ID {
	subs := subtitlesOf(f)
	audioLang := ""
	if audio != nil {
		audioLang = media.BaseLanguage(audio.Language)
	}
	// first returns the first track in lang that ok accepts.
	first := func(lang string, ok func(subTrack) bool) id.ID {
		if i := slices.IndexFunc(subs, func(s subTrack) bool { return media.BaseLanguage(s.lang) == lang && ok(s) }); i >= 0 {
			return subs[i].id
		}
		return id.ID{}
	}
	full := func(s subTrack) bool { return !s.forced }
	forced := func(s subTrack) bool { return s.forced }

	if c := p.choice; c != nil {
		switch c.SubtitleKind {
		case "off", "":
			return id.ID{}
		case "forced":
			if i := first(c.SubtitleLanguage, forced); !i.IsZero() {
				return i
			}
		case "sdh":
			if i := first(c.SubtitleLanguage, func(s subTrack) bool { return s.hi }); !i.IsZero() {
				return i
			}
			fallthrough
		default:
			if i := first(c.SubtitleLanguage, full); !i.IsZero() {
				return i
			}
		}
	}
	if p.mode == users.SubtitlesNone {
		return id.ID{}
	}
	if p.showsFull(audioLang) {
		if i := first(p.subtitle, full); !i.IsZero() {
			return i
		}
	}
	if audioLang == "" {
		// The audio's language is unknown, so use a forced track in the
		// subtitle language.
		return first(p.subtitle, forced)
	}
	return first(audioLang, forced)
}
