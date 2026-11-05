package identify_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/users"
)

// TestSearchMatches searches by the work's own title, by a folder-style
// query, and by a provider ID. A folder-style query is retried without its
// year when it finds nothing. The search refuses a provider that isn't the
// work's, a work no provider matches, and a work the Viewer can't see. A
// book matches only an Audible book.
func TestSearchMatches(t *testing.T) {
	e := newEnv(t)
	movies, shows := e.library(catalog.Movies), e.library(catalog.Shows)
	heat := e.add(movies, "Heat (1995)/Heat.mkv")
	firefly := e.add(shows, "Firefly/Season 1/Firefly - S01E01.mkv")
	e.run(heat[0], firefly[0])
	v := e.viewer()
	str := func(s string) *string { return &s }
	titles := func(ms []metadata.Match) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Title)
		}
		return out
	}

	movie, episode := e.workOf(heat[0])[0].ID, e.workOf(firefly[0])[0].ID
	for name, tc := range map[string]struct {
		search identify.MatchSearch
		on     string
		want   []string
	}{
		"its own title":       {identify.MatchSearch{}, "movie", []string{"Heat"}},
		"its series' title":   {identify.MatchSearch{}, "episode", []string{"Firefly"}},
		"a folder name":       {identify.MatchSearch{Query: str("Dune (1984)")}, "movie", []string{"Dune"}},
		"a year that's off":   {identify.MatchSearch{Query: str("Heat (1996)")}, "movie", []string{"Heat"}},
		"a provider ID":       {identify.MatchSearch{Query: str("tmdb-603")}, "movie", []string{"The Matrix"}},
		"an unknown ID":       {identify.MatchSearch{Query: str("tmdb-1")}, "movie", nil},
		"the work's provider": {identify.MatchSearch{Provider: str("tmdb")}, "movie", []string{"Heat"}},
	} {
		work := movie
		if tc.on == "episode" {
			work = episode
		}
		got, err := e.id.SearchMatches(ctx, v, work, tc.search)
		if err != nil || !slices.Equal(titles(got), tc.want) {
			t.Errorf("%s: %v %v, want %v", name, titles(got), err, tc.want)
		}
	}

	if _, err := e.id.SearchMatches(ctx, v, movie, identify.MatchSearch{Provider: str("audible")}); err == nil {
		t.Error("a movie searched on audible")
	}
	if _, err := e.id.SearchMatches(ctx, users.Viewer{}, movie, identify.MatchSearch{}); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("a work the Viewer doesn't see: %v", err)
	}
	extra := e.b.Work(catalog.WorkInput{Kind: catalog.KindExtra, Parent: movie, Title: "Trailer", ExtraType: "trailer"})
	e.b.Version([]id.ID{e.b.File(movies.Folders[0].ID, "Heat (1995)/Trailers/Trailer.mkv")}, extra)
	if _, err := e.id.SearchMatches(ctx, v, extra, identify.MatchSearch{}); !errors.Is(err, identify.ErrNotMatchable) {
		t.Errorf("an extra: %v", err)
	}

	// A book searches by its title and first author, or looks up an ASIN.
	books := e.library(catalog.Audiobooks)
	book := e.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Leviathan Wakes"})
	e.b.Version([]id.ID{e.b.File(books.Folders[0].ID, "James S. A. Corey/Leviathan Wakes/a.m4b")}, book)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.UpdateMetadata(ctx, tx, book, catalog.WorkMetadata{Title: "Leviathan Wakes",
			Book: &catalog.BookMetadata{Authors: []string{"James S. A. Corey"}}})
	})
	got, err := e.id.SearchMatches(ctx, v, book, identify.MatchSearch{})
	if err != nil || !slices.Equal(titles(got), []string{"Leviathan Wakes"}) || !slices.Contains(e.video.searches, "book:Leviathan Wakes/James S. A. Corey") {
		t.Errorf("a book's own: %v %v, searches %v", titles(got), err, e.video.searches)
	}
	if got, err := e.id.SearchMatches(ctx, v, book, identify.MatchSearch{Query: str("b005vr5grq")}); err != nil || !slices.Equal(titles(got), []string{"Caliban's War"}) {
		t.Errorf("an ASIN: %v %v", titles(got), err)
	}
	if _, err := e.id.SearchMatches(ctx, v, book, identify.MatchSearch{Provider: str("tmdb")}); err == nil {
		t.Error("a book searched on tmdb")
	}

	// A book's match must be an Audible book, named by its ASIN.
	if _, err := e.id.FixMatch(ctx, v, book, catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "949"}); !errors.Is(err, identify.ErrBookMatch) {
		t.Errorf("a book matched to a movie: %v", err)
	}
	leviathan := catalog.ExternalID{Provider: catalog.ASIN.Provider, Kind: catalog.ASIN.Kind, ID: "B0058ZZZ5A"}
	if got, err := e.id.FixMatch(ctx, v, book, leviathan); err != nil || got != book {
		t.Errorf("a book matched: %v %v", got, err)
	}
	if w, err := e.b.Catalog.Work(ctx, book); err != nil || !slices.Equal(w.ExternalIDs, []catalog.ExternalID{leviathan}) || !e.refreshed(book) {
		t.Errorf("the matched book: %+v %v, want its ASIN set and a refresh asked for", w, err)
	}
}
