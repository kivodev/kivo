package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/mmcdole/kivo/internal/store"

	"golang.org/x/text/language"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users/dbq"
)

// Subtitle modes decide when playback shows subtitles that nobody chose.
const (
	SubtitlesNone   = "none"   // never
	SubtitlesForced = "forced" // a forced track in the audio's language
	SubtitlesSmart  = "smart"  // full subtitles when the audio isn't in the subtitle language, otherwise forced
	SubtitlesAlways = "always" // full subtitles in the subtitle language
)

// Playback is a user's playback preferences. Every client signed in as the
// user uses them. How text subtitles look is up to each client.
type Playback struct {
	// AudioLanguage is the preferred audio language, as a BCP 47 tag. Empty
	// plays each file's original default track.
	AudioLanguage string
	// SubtitleLanguage is the preferred subtitle language, as a BCP 47 tag.
	// Empty means the server's metadata language.
	SubtitleLanguage string
	SubtitleMode     string // none, forced, smart or always
	// RememberTracks starts each episode with the audio and subtitle
	// languages last chosen for its series.
	RememberTracks bool
	// AutoplayNext makes up next play the next episode by itself. When it
	// is off, up next waits for the user to play it.
	AutoplayNext bool
	// BookSpeed is the speed books play at, from MinBookSpeed to
	// MaxBookSpeed. 1 is normal speed.
	BookSpeed float64
}

// The speeds a book can play at.
const (
	MinBookSpeed = 0.5
	MaxBookSpeed = 3
)

// DefaultPlayback is a new user's preferences, matching the schema's
// defaults: the original audio, forced subtitles, remembered choices, the
// next episode played by itself, and books at normal speed.
var DefaultPlayback = Playback{SubtitleMode: SubtitlesForced, RememberTracks: true, AutoplayNext: true, BookSpeed: 1}

// PlaybackChange is a change to a user's playback preferences. Each field
// that is set replaces its preference.
type PlaybackChange struct {
	AudioLanguage, SubtitleLanguage, SubtitleMode *string
	RememberTracks, AutoplayNext                  *bool
	BookSpeed                                     *float64
}

// apply returns p with the change made.
func (c PlaybackChange) apply(p Playback) Playback {
	for _, f := range []struct {
		dst *string
		v   *string
	}{{&p.AudioLanguage, c.AudioLanguage}, {&p.SubtitleLanguage, c.SubtitleLanguage}, {&p.SubtitleMode, c.SubtitleMode}} {
		if f.v != nil {
			*f.dst = *f.v
		}
	}
	if c.RememberTracks != nil {
		p.RememberTracks = *c.RememberTracks
	}
	if c.AutoplayNext != nil {
		p.AutoplayNext = *c.AutoplayNext
	}
	if c.BookSpeed != nil {
		p.BookSpeed = *c.BookSpeed
	}
	return p
}

// PreferencesChange is a user's change to their own preferences. Each field
// that is set replaces those preferences.
type PreferencesChange struct {
	Libraries *[]LibraryChoice // the user's library order, and which libraries are pinned
	Playback  *PlaybackChange
}

// EditPreferences changes v's user's preferences in one transaction, so if
// any part is refused nothing changes. Languages are stored in their
// canonical form, such as pt-BR.
func (s *Service) EditPreferences(ctx context.Context, v Viewer, c PreferencesChange) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if c.Libraries != nil {
			if err := setLibraries(ctx, q, v, *c.Libraries); err != nil {
				return err
			}
		}
		if c.Playback != nil {
			return editPlayback(ctx, q, v.user, *c.Playback)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// editPlayback changes a user's playback preferences.
func editPlayback(ctx context.Context, q *dbq.Queries, user id.ID, c PlaybackChange) error {
	u, err := q.GetUser(ctx, user)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, user)
	}
	if err != nil {
		return err
	}
	params, err := playbackParams(c.apply(fromRow(u, nil, 0).Playback))
	if err != nil {
		return err
	}
	params.ID = user
	if _, err := q.SetPlayback(ctx, params); err != nil {
		return err
	}
	return nil
}

// playbackParams checks preferences and returns them as stored.
func playbackParams(p Playback) (dbq.SetPlaybackParams, error) {
	audio, err := languages(p.AudioLanguage)
	if err != nil {
		return dbq.SetPlaybackParams{}, err
	}
	subtitles, err := languages(p.SubtitleLanguage)
	if err != nil {
		return dbq.SetPlaybackParams{}, err
	}
	if !slices.Contains(subtitleModes, p.SubtitleMode) {
		return dbq.SetPlaybackParams{}, fmt.Errorf("%w: subtitle mode %q", ErrInvalidPlayback, p.SubtitleMode)
	}
	if !(p.BookSpeed >= MinBookSpeed && p.BookSpeed <= MaxBookSpeed) {
		return dbq.SetPlaybackParams{}, fmt.Errorf("%w: book speed %v", ErrInvalidPlayback, p.BookSpeed)
	}
	return dbq.SetPlaybackParams{
		AudioLanguages: audio, SubtitleLanguages: subtitles, SubtitleMode: p.SubtitleMode,
		RememberTracks: p.RememberTracks, AutoplayNext: p.AutoplayNext, BookSpeed: p.BookSpeed,
	}, nil
}

var subtitleModes = []string{SubtitlesNone, SubtitlesForced, SubtitlesSmart, SubtitlesAlways}

// firstLanguage returns a language column's first tag. The columns hold
// JSON arrays, most preferred first, though a user sets only one.
func firstLanguage(column string) string {
	var tags []string
	_ = json.Unmarshal([]byte(column), &tags) // written by languages, or the schema's default '[]'
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

// languages returns a language column's value for tag. An empty tag gives
// an empty array. Otherwise the array holds the tag in its canonical form.
func languages(tag string) (string, error) {
	if tag == "" {
		return "[]", nil
	}
	t, err := language.Parse(tag)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrInvalidPlayback, tag)
	}
	b, _ := json.Marshal([]string{t.String()})
	return string(b), nil
}
