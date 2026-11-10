package users

import (
	"context"
	"errors"
	"testing"

	"github.com/mmcdole/kivo/internal/id"
)

// change returns a change that sets every preference to p's value.
func change(p Playback) PlaybackChange {
	return PlaybackChange{
		AudioLanguage: &p.AudioLanguage, SubtitleLanguage: &p.SubtitleLanguage, SubtitleMode: &p.SubtitleMode,
		RememberTracks: &p.RememberTracks, AutoplayNext: &p.AutoplayNext, BookSpeed: &p.BookSpeed,
	}
}

func TestUser(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(NewUser{
		Username: "ana", Admin: true, Limits: Limits{StreamLimit: 2, RemoteQualityCap: 4_000_000},
		Parental: Parental{Ceiling: ptr(13)},
	})

	got, err := f.s.User(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID || got.Username != "ana" || !got.Admin || got.Limits != u.Limits || *got.Parental.Ceiling != 13 ||
		got.Parental.BlockUnrated || !got.Downloads || got.Disabled || !got.Created.Equal(u.Created) ||
		got.Playback != DefaultPlayback || u.Playback != DefaultPlayback {
		t.Errorf("User = %+v, want %+v", got, u)
	}
	if err := f.s.Update(ctx, id.ID{}, u.ID, Change{Limits: &Limits{}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.User(ctx, u.ID); got.Limits != (Limits{}) {
		t.Errorf("Limits = %+v, want none", got.Limits)
	}
	if _, err := f.s.User(ctx, id.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
}

func TestRename(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(NewUser{Username: "david@example.com"})
	f.user(NewUser{Username: "eva"})
	if err := f.s.Update(ctx, id.ID{}, u.ID, Change{Username: ptr("david")}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.User(ctx, u.ID); got.Username != "david" {
		t.Errorf("Username = %q, want david", got.Username)
	}
	if err := f.s.Update(ctx, id.ID{}, u.ID, Change{Username: ptr("Eva")}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("another user's name in other case: %v, want ErrNameTaken", err)
	}
	if err := f.s.Update(ctx, id.ID{}, u.ID, Change{Username: ptr(" david")}); !errors.Is(err, ErrInvalidName) {
		t.Errorf("a padded name: %v, want ErrInvalidName", err)
	}
	if err := f.s.Update(ctx, id.ID{}, u.ID, Change{Username: ptr("David")}); err != nil {
		t.Errorf("its own name in other case: %v", err)
	}
}

func TestPlaybackPreferences(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(NewUser{Username: "ana"})
	edit := func(user id.ID, c PlaybackChange) error {
		v, _ := f.s.Viewer(ctx, user) // an unknown user gets the zero Viewer
		return f.s.EditPreferences(ctx, v, PreferencesChange{Playback: &c})
	}

	want := Playback{
		AudioLanguage: "ja", SubtitleLanguage: "pt-BR", SubtitleMode: SubtitlesSmart, RememberTracks: false, AutoplayNext: false,
		BookSpeed: 1.25,
	}
	in := want
	in.SubtitleLanguage = "pt-br" // stored in its canonical form
	if err := edit(u.ID, change(in)); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.User(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Playback != want {
		t.Errorf("Playback = %+v, want %+v", got.Playback, want)
	}

	// Back to the original audio and the server's subtitle language.
	if err := edit(u.ID, change(DefaultPlayback)); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.User(ctx, u.ID); got.Playback != DefaultPlayback {
		t.Errorf("Playback = %+v, want the defaults", got.Playback)
	}

	for _, bad := range []Playback{
		{AudioLanguage: "not a language", SubtitleMode: SubtitlesNone, BookSpeed: 1},
		{SubtitleMode: "default", BookSpeed: 1},
		{SubtitleMode: SubtitlesNone, BookSpeed: 3.5},
		{SubtitleMode: SubtitlesNone},
	} {
		if err := edit(u.ID, change(bad)); !errors.Is(err, ErrInvalidPlayback) {
			t.Errorf("edit(%+v) = %v, want ErrInvalidPlayback", bad, err)
		}
	}
	if err := edit(id.New(), change(DefaultPlayback)); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
}
