package catalog_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// TestVisible checks each part of the rule: the library grant, the rating
// ceiling against a work's own or inherited rating, and unrated works by
// kind.
func TestVisible(t *testing.T) {
	b := catalogtest.New(t)
	moviesLib, movies := b.Library(catalog.Movies)
	showsLib, shows := b.Library(catalog.Shows)
	videosLib, videos := b.Library(catalog.Videos)
	booksLib, books := b.Library(catalog.Audiobooks)
	otherLib, _ := b.Library(catalog.Movies)

	rate := func(w id.ID, age int) {
		t.Helper()
		b.Write(func(ctx context.Context, tx *store.Tx) error {
			return b.Catalog.UpdateMetadata(ctx, tx, w, catalog.WorkMetadata{Title: "Rated", ContentRating: "R", ContentRatingAge: &age})
		})
	}
	extra := func(parent id.ID, rel string) id.ID {
		t.Helper()
		w := b.Work(catalog.WorkInput{Kind: catalog.KindExtra, Parent: parent, Title: "Trailer", ExtraType: "trailer"})
		b.Version([]id.ID{b.File(movies, rel)}, w)
		return w
	}

	rated := b.Movie(movies, "Rated", 2020).Work
	rate(rated, 13)
	ratedExtra := extra(rated, "Rated (2020)/Trailers/t.mkv")
	unrated := b.Movie(movies, "Unrated", 2020).Work
	unratedExtra := extra(unrated, "Unrated (2020)/Trailers/t.mkv")
	provisional := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Parsed", Provisional: true})
	b.Version([]id.ID{b.File(movies, "Parsed.mkv")}, provisional)
	nowhere := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Nowhere"})

	series := b.Series("Rated Show")
	rate(series, 13)
	episode := b.Episode(shows, series, 1, 1).Work
	season := mustWork(t, b, episode).Parent
	ownRating := b.Episode(shows, series, 1, 2).Work
	rate(ownRating, 18)
	unratedSeries := b.Series("Unrated Show")
	unratedEpisode := b.Episode(shows, unratedSeries, 1, 1).Work
	unratedSeason := mustWork(t, b, unratedEpisode).Parent

	video := b.Work(catalog.WorkInput{Kind: catalog.KindVideo, Title: "Clip"})
	b.Version([]id.ID{b.File(videos, "Clip.mp4")}, video)
	book := b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Book"})
	b.Version([]id.ID{b.File(books, "Book/book.m4b")}, book)

	svc := users.New(b.DB, events.NewBus())
	viewer := func(name string, u users.NewUser) users.Viewer {
		t.Helper()
		u.Username = name
		var user id.ID
		b.Write(func(ctx context.Context, tx *store.Tx) error {
			created, err := svc.CreateUser(ctx, tx, u)
			user = created.ID
			return err
		})
		v, err := svc.Viewer(context.Background(), user)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	ceiling := func(age int) users.Parental { return users.Parental{Ceiling: &age} }
	sorted := func(ids ...id.ID) []id.ID {
		slices.SortFunc(ids, func(a, b id.ID) int { return slices.Compare(a[:], b[:]) })
		return ids
	}
	all := users.Grants{All: true}
	var (
		everything = viewer("everything", users.NewUser{Grants: all})
		granted    = viewer("granted", users.NewUser{Grants: users.Grants{Libraries: sorted(moviesLib, showsLib, videosLib, booksLib)}})
		elsewhere  = viewer("elsewhere", users.NewUser{Grants: users.Grants{Libraries: []id.ID{otherLib}}})
		under13    = viewer("under13", users.NewUser{Grants: all, Parental: ceiling(7)})
		at13       = viewer("at13", users.NewUser{Grants: all, Parental: ceiling(13)})
		over13     = viewer("over13", users.NewUser{Grants: all, Parental: ceiling(18)})
		strict     = viewer("strict", users.NewUser{Grants: all, Parental: users.Parental{BlockUnrated: true}})
	)

	cases := []struct {
		name string
		v    users.Viewer
		work id.ID
		want bool
	}{
		{"every library", everything, rated, true},
		{"granted library", granted, rated, true},
		{"library not granted", elsewhere, rated, false},
		{"book in a library not granted", elsewhere, book, false},
		{"zero viewer", users.Viewer{}, rated, false},
		{"no library", everything, nowhere, false},
		{"no such work", everything, id.New(), false},
		{"provisional", everything, provisional, true},

		{"no ceiling", everything, rated, true},
		{"ceiling below", under13, rated, false},
		{"ceiling equal", at13, rated, true},
		{"ceiling above", over13, rated, true},

		{"rated series under ceiling", under13, series, false},
		{"season inherits below", under13, season, false},
		{"season inherits equal", at13, season, true},
		{"episode inherits below", under13, episode, false},
		{"episode inherits equal", at13, episode, true},
		{"extra inherits below", under13, ratedExtra, false},
		{"extra inherits equal", at13, ratedExtra, true},
		{"episode's own rating over ceiling", at13, ownRating, false},
		{"episode's own rating within ceiling", over13, ownRating, true},
		{"unrated under a ceiling", under13, unrated, true},

		{"shows unrated movie", everything, unrated, true},
		{"shows unrated series", everything, unratedSeries, true},
		{"shows unrated season", everything, unratedSeason, true},
		{"shows unrated episode", everything, unratedEpisode, true},
		{"shows unrated extra", everything, unratedExtra, true},
		{"shows unrated video", everything, video, true},
		{"shows unrated book", everything, book, true},
		{"blocks unrated movie", strict, unrated, false},
		{"blocks unrated series", strict, unratedSeries, false},
		{"blocks unrated season", strict, unratedSeason, false},
		{"blocks unrated episode", strict, unratedEpisode, false},
		{"blocks unrated extra", strict, unratedExtra, false},
		{"blocks unrated video", strict, video, false},
		{"blocks unrated but not a book", strict, book, true},
		{"blocks unrated but not a rated movie", strict, rated, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := b.Catalog.Visible(context.Background(), c.v, c.work)
			switch {
			case c.want && err != nil:
				t.Errorf("hidden: %v", err)
			case !c.want && !errors.Is(err, catalog.ErrNotFound):
				t.Errorf("got %v, want ErrNotFound", err)
			}
		})
	}
}

func mustWork(t *testing.T, b *catalogtest.Builder, w id.ID) catalog.Work {
	t.Helper()
	work, err := b.Catalog.Work(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	return work
}
