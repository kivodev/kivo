package identify_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/metadata"
)

// Unmatch returns a matched movie to the state of a file nothing matched,
// and locks it. The folder's next run leaves it alone, its watch state
// stays, and Fix match can match it again.
func TestUnmatchMovie(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	f := e.add(lib, "Heat (1995)/Heat (1995).mkv")[0]
	e.run(f)
	matched := e.workOf(f)[0]
	if matched.Provisional || len(matched.ExternalIDs) == 0 {
		t.Fatalf("Heat should match: %+v", matched)
	}
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(f), f, 95*time.Minute)
	e.poster(matched.ID)

	got, err := e.id.Unmatch(ctx, e.viewer(), matched.ID)
	if err != nil {
		t.Fatal(err)
	}
	w := e.workOf(f)[0]
	if got != matched.ID || w.ID != matched.ID {
		t.Fatalf("unmatched %v, file now on %v; want the same work %v", got, w.ID, matched.ID)
	}
	if !w.Provisional || !w.Locked || len(w.ExternalIDs) != 0 || w.Title != "Heat" || w.Year != 1995 {
		t.Fatalf("unmatched work %+v", w)
	}
	if e.artwork(matched.ID) != 0 {
		t.Fatal("the artwork wasn't cleared")
	}
	if s, ok := e.stateOf(anna, w.ID); !ok || !s.played {
		t.Fatalf("watch state after unmatching: %+v, %v", s, ok)
	}

	searches := e.video.searchCount()
	e.again(f)
	if w := e.workOf(f)[0]; !w.Provisional || len(w.ExternalIDs) != 0 || w.ID != matched.ID {
		t.Fatalf("the folder's next run matched it again: %+v", w)
	}
	if e.video.searchCount() != searches {
		t.Fatal("the folder's next run searched")
	}

	heat := metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "949"}, Title: "Heat", Year: 1995}
	if err := e.fix(e.versionOf(f), heat); err != nil {
		t.Fatal(err)
	}
	fixed := e.workOf(f)[0]
	if fixed.ID != matched.ID || fixed.Provisional || !fixed.Locked || !slices.Equal(fixed.ExternalIDs, []catalog.ExternalID{heat.ID}) {
		t.Fatalf("matched again %+v", fixed)
	}
	if s, ok := e.stateOf(anna, fixed.ID); !ok || !s.played || s.plays != 1 {
		t.Fatalf("watch state after Fix match: %+v, %v", s, ok)
	}
}

// A series' episodes become provisional too, numbered from their names. An
// episode whose name places it elsewhere moves there, with its watch
// state.
func TestUnmatchSeries(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv", "Firefly/Season 1/Firefly - S01E02.mkv")
	e.run(files...)
	ep := e.workOf(files[0])[0]
	if ep.Title != "The Train Job" {
		t.Fatalf("S01E01 is %+v", ep)
	}
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(files[0]), files[0], 95*time.Minute)

	series, err := e.id.Unmatch(ctx, e.viewer(), ep.Root)
	if err != nil {
		t.Fatal(err)
	}
	want := `series "Firefly" provisional locked
  season "Season 1" S01
    episode "Episode 1" S01E01 provisional
      = Firefly/Season 1/Firefly - S01E01.mkv
    episode "Episode 2" S01E02 provisional
      = Firefly/Season 1/Firefly - S01E02.mkv
`
	if got := e.dump(); got != want {
		t.Fatalf("after unmatching:\n%s\nwant:\n%s", got, want)
	}
	if series != ep.Root || e.workOf(files[0])[0].ID != ep.ID {
		t.Fatal("an episode numbered alike should keep its work")
	}
	if s, ok := e.stateOf(anna, ep.ID); !ok || !s.played {
		t.Fatalf("watch state after unmatching: %+v, %v", s, ok)
	}
	e.again(files...)
	if got := e.dump(); got != want {
		t.Fatalf("the folder's next run changed it:\n%s", got)
	}

	firefly := metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: "1437"}, Title: "Firefly", Year: 2002}
	if err := e.fix(e.versionOf(files[0]), firefly); err != nil {
		t.Fatal(err)
	}
	w := e.workOf(files[1])[0]
	if w.Title != "Bushwhacked" || w.Provisional || w.Root != series {
		t.Fatalf("the other episode after Fix match: %+v", w)
	}
	if s, ok := e.stateOf(anna, e.workOf(files[0])[0].ID); !ok || !s.played {
		t.Fatalf("watch state after Fix match: %+v, %v", s, ok)
	}
}

func TestUnmatchRenumbersFromNames(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	f := e.add(lib, "The Daily Show/The Daily Show - 2024-02-12.mkv")[0]
	e.run(f)
	matched := e.workOf(f)[0]
	if matched.ParentNumber != 2024 || matched.Number != 1 || matched.Provisional {
		t.Fatalf("matched %+v", matched)
	}
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(f), f, 30*time.Minute)

	if _, err := e.id.Unmatch(ctx, e.viewer(), matched.Root); err != nil {
		t.Fatal(err)
	}
	w := e.workOf(f)[0]
	if w.ID == matched.ID || w.Title != "2024-02-12" || w.Number != 0 || !w.Provisional {
		t.Fatalf("a dated episode is numbered by its date: %+v", w)
	}
	if !e.gone(matched.ID) {
		t.Fatal("the emptied episode wasn't merged")
	}
	if s, ok := e.stateOf(anna, w.ID); !ok || s.position == 0 {
		t.Fatalf("the resume position didn't follow: %+v, %v", s, ok)
	}
}

// An episode moved to another library waits for identify to place it
// there. The invariants hold meanwhile, and its series unmatches without
// it.
func TestUnmatchLeavesAPendingMoveToItsIdentify(t *testing.T) {
	e := newEnv(t)
	shows, other := e.library(catalog.Shows), e.library(catalog.Shows)
	files := e.add(shows, "Firefly/Season 1/Firefly - S01E01.mkv", "Firefly/Season 1/Firefly - S01E02.mkv")
	e.run(files...)
	series := e.workOf(files[0])[0].Root

	e.move(other, files[1], "Firefly/Season 1/Firefly - S01E02.mkv")
	if err := e.b.Catalog.CheckInvariants(ctx); err != nil {
		t.Fatalf("with a move pending: %v", err)
	}
	if _, err := e.id.Unmatch(ctx, e.viewer(), series); err != nil {
		t.Fatalf("unmatching with a move pending: %v", err)
	}
	if w := e.workOf(files[0])[0]; w.Root != series || !w.Provisional {
		t.Fatalf("the episode left behind is %+v, want it unmatched under %v", w, series)
	}
	e.run(files[1])
	if w := e.workOf(files[1])[0]; w.Root == series || w.Provisional {
		t.Fatalf("the moved episode is %+v, want it matched under a root work of its own", w)
	}
}

func TestUnmatchRejects(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	f := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv")[0]
	e.run(f)
	if _, err := e.id.Unmatch(ctx, e.viewer(), e.workOf(f)[0].ID); !errors.Is(err, identify.ErrInvalid) {
		t.Fatalf("unmatching an episode: %v", err)
	}
}
