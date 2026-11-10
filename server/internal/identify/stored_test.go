package identify_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/metadata"
)

// calls counts the provider's searches and episode-list fetches.
func (f *fakeVideo) calls() (searches, lists int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.searches), len(f.episodes)
}

// mark zeroes the match rows' times, so a row written again differs even
// within the same millisecond, and returns what identify then keeps.
func (e *env) mark() string {
	e.t.Helper()
	e.exec(`UPDATE match SET updated_at = 0`)
	return e.kept()
}

// kept prints what identify keeps: the works as dump prints them, the match
// rows, and the works' match revisions.
func (e *env) kept() string {
	e.t.Helper()
	out := e.dump()
	rows, err := e.b.DB.Reader().Query(`SELECT subject, source, query, status, coalesce(ext_id, ''), coalesce(ordering, ''), updated_at
		FROM match ORDER BY subject`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var subject, source, query, status, ext, ordering string
		var at int64
		if err := rows.Scan(&subject, &source, &query, &status, &ext, &ordering, &at); err != nil {
			e.t.Fatal(err)
		}
		out += fmt.Sprintf("match %q %s %q %s %s %q at %d\n", subject, source, query, status, ext, ordering, at)
	}
	var revs int64
	if err := e.b.DB.Reader().QueryRow(`SELECT coalesce(sum(match_rev), 0) FROM work`).Scan(&revs); err != nil {
		e.t.Fatal(err)
	}
	return out + fmt.Sprintf("match revisions %d\n", revs)
}

// A folder that is identified again with nothing changed calls no provider
// and writes nothing.
func TestUnchangedFolderIsLeftAlone(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  catalog.LibraryKind
		files []string
	}{
		{name: "movie", kind: catalog.Movies, files: []string{"Heat (1995)/Heat (1995).mkv"}},
		{name: "series", kind: catalog.Shows, files: []string{"Firefly/S01E01.mkv", "Firefly/S01E02.mkv", "Firefly/Featurettes/Bloopers.mkv"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			files := e.add(e.library(tc.kind), tc.files...)
			e.run(files...)
			if w := e.workOf(files[0])[0]; w.Provisional {
				t.Fatalf("unmatched: %+v", w)
			}
			kept := e.mark()
			searches, lists := e.video.calls()
			e.takeRefreshes()
			e.again(files...)
			if s, l := e.video.calls(); s != searches || l != lists {
				t.Errorf("the provider was called: %d searches and %d episode lists, had %d and %d", s, l, searches, lists)
			}
			if got := e.kept(); got != kept || len(e.takeRefreshes()) != 0 {
				t.Errorf("written again:\n%s\nwas:\n%s", got, kept)
			}
		})
	}
}

// A series placed in another ordering than its own fetches that ordering's
// episode list and places its versions by it. It isn't searched for again,
// and once placed it is left alone.
func TestAnotherOrderingFetchesItsEpisodes(t *testing.T) {
	e := newEnv(t)
	files := e.add(e.library(catalog.Shows), "Firefly/S01E01.mkv", "Firefly/S01E02.mkv")
	e.run(files...)
	series := e.workOf(files[0])[0].Root
	searches, lists := e.video.calls()
	dvd := string(metadata.DVD)
	if err := e.id.EditWork(ctx, e.viewer(), series, identify.WorkEdit{Ordering: &dvd}); err != nil {
		t.Fatal(err)
	}
	e.run(e.pending()...)
	if s, _ := e.video.calls(); s != searches {
		t.Errorf("searched again: %d searches, had %d", s, searches)
	}
	if got := e.video.episodes[lists:]; !slices.Equal(got, []string{"1437/dvd"}) {
		t.Errorf("episode lists fetched %v, want the DVD order once", got)
	}
	if w := e.workOf(files[0])[0]; w.Title != "Serenity" {
		t.Errorf("S01E01 in DVD order is %+v", w)
	}

	kept := e.mark()
	searches, lists = e.video.calls()
	e.again(files...)
	if s, l := e.video.calls(); s != searches || l != lists {
		t.Errorf("the provider was called: %d searches and %d episode lists, had %d and %d", s, l, searches, lists)
	}
	if got := e.kept(); got != kept {
		t.Errorf("written again:\n%s\nwas:\n%s", got, kept)
	}
}

// A version on a provisional episode is looked up again each time, without
// another search, while the versions already placed stay where they are.
func TestProvisionalEpisodeIsLookedUpAgain(t *testing.T) {
	e := newEnv(t)
	files := e.add(e.library(catalog.Shows), "Firefly/S01E01.mkv", "Firefly/S01E09.mkv")
	e.run(files...)
	placed, unlisted := e.workOf(files[0])[0], e.workOf(files[1])[0]
	if placed.Provisional || !unlisted.Provisional {
		t.Fatalf("S01E01 %+v, S01E09 %+v; want only the unlisted one provisional", placed, unlisted)
	}
	searches, lists := e.video.calls()
	e.again(files...)
	if s, l := e.video.calls(); s != searches || l == lists {
		t.Errorf("%d searches and %d episode lists, had %d and %d; want only the episode list fetched", s, l, searches, lists)
	}
	if w := e.workOf(files[0])[0]; w.ID != placed.ID {
		t.Errorf("the placed version moved to %+v", w)
	}
}

// A root work that no longer is what its folder's decision describes is
// described again from the stored decision, without a search.
func TestRootNotDescribedIsWritten(t *testing.T) {
	e := newEnv(t)
	files := e.add(e.library(catalog.Movies), "Heat (1995)/Heat (1995).mkv")
	e.run(files...)
	root := e.workOf(files[0])[0]
	e.exec(`DELETE FROM work_external_id WHERE work_id = ?`, root.ID)
	e.refreshedAt(root.ID)
	e.takeRefreshes()
	searches, _ := e.video.calls()
	e.again(files...)
	if s, _ := e.video.calls(); s != searches {
		t.Errorf("searched again: %d searches, had %d", s, searches)
	}
	w := e.workOf(files[0])[0]
	if w.ID != root.ID || len(w.ExternalIDs) != 1 || w.ExternalIDs[0].ID != "949" || !e.refreshed(root.ID) {
		t.Errorf("the root wasn't described again: %+v", w)
	}
}

// A settled folder that lost its match row is decided again, and the
// decision is recorded, so the next run doesn't search.
func TestLostMatchRowIsWrittenAgain(t *testing.T) {
	e := newEnv(t)
	files := e.add(e.library(catalog.Movies), "Heat (1995)/Heat (1995).mkv")
	e.run(files...)
	kept := e.mark()
	e.exec(`DELETE FROM match`)
	searches, _ := e.video.calls()
	e.again(files...)
	if s, _ := e.video.calls(); s != searches+1 {
		t.Errorf("%d searches, had %d; want one more", s, searches)
	}
	if got := e.mark(); got != kept {
		t.Errorf("after deciding again:\n%s\nwas:\n%s", got, kept)
	}
	e.again(files...)
	if s, _ := e.video.calls(); s != searches+1 {
		t.Errorf("searched again with the decision recorded: %d searches", s)
	}
}

// A folder whose stored state can't be read is left pending. The
// failure doesn't count as the folder having no match: no provider is
// called and nothing is written.
func TestUnreadableMatchLeavesTheFolder(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/S01E01.mkv", "Firefly/S01E02.mkv")
	e.run(files...)
	kept := e.mark()
	searches, lists := e.video.calls()
	e.takeRefreshes()

	e.exec(`ALTER TABLE match RENAME TO match_away`)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.id.Reidentify(ctx, tx, files) })
	if err := e.identify(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(e.pending(), files) {
		t.Errorf("pending %v, want the folder's files %v left pending", e.pending(), files)
	}
	e.exec(`ALTER TABLE match_away RENAME TO match`)
	if s, l := e.video.calls(); s != searches || l != lists {
		t.Errorf("the provider was called: %d searches and %d episode lists, had %d and %d", s, l, searches, lists)
	}
	if got := e.kept(); got != kept || len(e.takeRefreshes()) != 0 {
		t.Errorf("written:\n%s\nwas:\n%s", got, kept)
	}

	if err := e.identify(ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.pending()) != 0 {
		t.Errorf("pending %v once the match can be read", e.pending())
	}
}

// Fix match fails when a folder's match row can't be read. It doesn't take
// the series as placed in no ordering and fetch its episode list.
func TestFixMatchUnreadableMatch(t *testing.T) {
	e := newEnv(t)
	files := e.add(e.library(catalog.Shows), "Firefly/S01E01.mkv", "Firefly/S01E02.mkv")
	e.run(files...)
	series := e.workOf(files[0])[0].Root
	dvd := string(metadata.DVD)
	if err := e.id.EditWork(ctx, e.viewer(), series, identify.WorkEdit{Ordering: &dvd}); err != nil {
		t.Fatal(err)
	}
	e.run(e.pending()...)
	kept := e.mark()
	_, lists := e.video.calls()

	e.exec(`ALTER TABLE match RENAME TO match_away`)
	_, err := e.id.FixMatch(ctx, e.viewer(), series, catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: "1437"})
	e.exec(`ALTER TABLE match_away RENAME TO match`)
	if err == nil {
		t.Error("Fix match succeeded without reading the folder's match")
	}
	if _, l := e.video.calls(); l != lists {
		t.Errorf("episode lists fetched: %v", e.video.episodes[lists:])
	}
	if got := e.kept(); got != kept {
		t.Errorf("written:\n%s\nwas:\n%s", got, kept)
	}
}
