package catalog_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/id"
)

// Describe re-describes a root work in place. The work keeps its ID, the
// match's ID becomes its only external ID, and its episodes lose the old
// match's IDs.
func TestDescribe(t *testing.T) {
	b := catalogtest.New(t)
	tmdb := func(n string) catalog.ExternalID { return catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: n} }
	s := b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: "Gundam SEED", Provisional: true})
	season := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: s, Title: "Season 1", Number: 1})
	ep := b.Work(catalog.WorkInput{Kind: catalog.KindEpisode, Parent: season, Title: "x", ParentNumber: 1, Number: 1,
		ExternalIDs: []catalog.ExternalID{{Provider: "tmdb", Kind: "episode", ID: "100"}}})
	describe := func(x catalog.ExternalID, title string, lock bool) {
		t.Helper()
		b.Write(func(ctx context.Context, tx *store.Tx) error {
			return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error { return cb.Describe(ctx, s, x, title, 2002, lock) })
		})
	}
	describe(tmdb("1"), "Mobile Suit Gundam SEED", false)
	w, _ := b.Catalog.Work(ctx, s)
	if w.Provisional || w.Locked || w.Title != "Mobile Suit Gundam SEED" || w.Year != 2002 || !slices.Equal(w.ExternalIDs, []catalog.ExternalID{tmdb("1")}) {
		t.Fatalf("described %+v", w)
	}
	if e, _ := b.Catalog.Work(ctx, ep); len(e.ExternalIDs) != 1 {
		t.Fatalf("the same match cleared the episode's IDs: %+v", e)
	}
	describe(tmdb("2"), "Mobile Suit Gundam SEED Destiny", true)
	w, _ = b.Catalog.Work(ctx, s)
	if !w.Locked || w.Title != "Mobile Suit Gundam SEED Destiny" || !slices.Equal(w.ExternalIDs, []catalog.ExternalID{tmdb("2")}) {
		t.Fatalf("re-described %+v", w)
	}
	if e, _ := b.Catalog.Work(ctx, ep); len(e.ExternalIDs) != 0 {
		t.Fatalf("the old match's episode IDs stayed: %v", e.ExternalIDs)
	}
	err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error { return cb.Describe(ctx, ep, tmdb("3"), "x", 0, false) })
	})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("describing an episode: %v", err)
	}
}

func TestMatchedEpisodeNeverAdoptsProvisional(t *testing.T) {
	b := catalogtest.New(t)
	s := b.Series("Firefly")
	season := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: s, Title: "Season 1", Number: 1})
	in := catalog.WorkInput{Kind: catalog.KindEpisode, Parent: season, Title: "Episode 2", ParentNumber: 1, Number: 2, Provisional: true}
	prov := b.Work(in)
	if again := b.Work(in); again != prov {
		t.Fatal("a provisional episode isn't found again by number")
	}
	in.Provisional = false
	in.ExternalIDs = []catalog.ExternalID{{Provider: "tmdb", Kind: "episode", ID: "2"}}
	matched := b.Work(in)
	if matched == prov {
		t.Fatal("a matched episode adopted the provisional one")
	}
	in.Provisional, in.ExternalIDs = true, nil
	if found := b.Work(in); found != matched {
		t.Fatal("a new provisional version of the episode didn't join the matched one")
	}
	// Episode 2 in another ordering is a different episode, not this one.
	in.Provisional, in.ExternalIDs = false, []catalog.ExternalID{{Provider: "tmdb", Kind: "episode", ID: "3"}}
	if other := b.Work(in); other == matched {
		t.Fatal("an episode known by other IDs was taken for a renumbered one")
	}
	// An episode's ID is looked up within its own series only.
	s2 := b.Series("Firefly (copy)")
	season2 := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: s2, Title: "Season 1", Number: 1})
	in = catalog.WorkInput{Kind: catalog.KindEpisode, Parent: season2, Title: "Episode 2", ParentNumber: 1, Number: 2,
		ExternalIDs: []catalog.ExternalID{{Provider: "tmdb", Kind: "episode", ID: "2"}}}
	if other := b.Work(in); other == matched {
		t.Fatal("another series' episode was found by its ID")
	}
}

func TestSetOrdering(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Shows)
	s := b.Series("Firefly")
	ep := b.Episode(folder, s, 1, 1)
	before := matchRev(t, b, s)
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.SetOrdering(ctx, tx, s, "dvd") })
	var ordering string
	if err := b.DB.Reader().QueryRow(`SELECT ordering FROM work WHERE id = ?`, s).Scan(&ordering); err != nil || ordering != "dvd" {
		t.Fatalf("ordering %q, %v", ordering, err)
	}
	if matchRev(t, b, s) != before+1 {
		t.Fatal("setting the ordering didn't bump the match revision")
	}
	err := write(t, b, func(ctx context.Context, tx *store.Tx) error { return b.Catalog.SetOrdering(ctx, tx, ep.Work, "dvd") })
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("ordering an episode: %v", err)
	}
	err = write(t, b, func(ctx context.Context, tx *store.Tx) error { return b.Catalog.SetOrdering(ctx, tx, id.New(), "dvd") })
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("ordering a missing work: %v", err)
	}
}

func TestMoveEpisode(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Shows)
	s := b.Series("Firefly")
	ep := b.Episode(folder, s, 1, 11)
	before, _ := b.Catalog.Work(ctx, ep.Work)
	rev := matchRev(t, b, ep.Work)
	s2 := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: s, Title: "Season 2", Number: 2})
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error { return cb.MoveEpisode(ctx, ep.Work, s2, 3) })
	})
	w, _ := b.Catalog.Work(ctx, ep.Work)
	if w.Parent != s2 || w.ParentNumber != 2 || w.Number != 3 || matchRev(t, b, ep.Work) != rev+1 {
		t.Fatalf("moved episode %+v", w)
	}
	if len(libraries(t, b, s2)) != 1 || len(libraries(t, b, before.Parent)) != 0 {
		t.Fatal("work_library didn't follow the episode to its new season")
	}
	other := b.Series("Serenity")
	elsewhere := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: other, Title: "Season 1", Number: 1})
	err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error { return cb.MoveEpisode(ctx, ep.Work, elsewhere, 1) })
	})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("moving an episode to another series: %v", err)
	}
}
