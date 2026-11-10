package identify_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/userstate"
)

var thing1982 = metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "1091"}, Title: "The Thing", Year: 1982}

type state struct {
	played   bool
	plays    int
	position int64
	rating   *int
}

func (e *env) stateOf(user, work id.ID) (state, bool) {
	e.t.Helper()
	var s state
	var rating sql.NullInt64
	err := e.b.DB.Reader().QueryRow(`SELECT played, play_count, position_ms, rating FROM user_state WHERE user_id = ? AND work_id = ?`,
		user, work).Scan(&s.played, &s.plays, &s.position, &rating)
	if errors.Is(err, sql.ErrNoRows) {
		return state{}, false
	}
	if err != nil {
		e.t.Fatal(err)
	}
	if rating.Valid {
		r := int(rating.Int64)
		s.rating = &r
	}
	return s, true
}

// play reports a position on a version and stops, as a play would.
func (e *env) play(user, version id.ID, file id.ID, pos time.Duration) {
	e.t.Helper()
	f, err := e.b.Catalog.File(ctx, file)
	if err != nil {
		e.t.Fatal(err)
	}
	if f.Probe.Duration == 0 {
		e.b.Write(func(ctx context.Context, tx *store.Tx) error {
			return e.b.Catalog.SetProbe(ctx, tx, file, f.Generation, catalog.Probe{Summary: catalog.ProbeSummary{Duration: 100 * time.Minute}})
		})
	}
	err = e.state.Report(ctx, userstate.Progress{Play: id.New(), User: user, Version: version, Position: pos, At: time.Now(), Stopped: true})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) gone(work id.ID) bool {
	_, err := e.b.Catalog.Work(ctx, work)
	return errors.Is(err, catalog.ErrNotFound)
}

// This is the acceptance test for wrong-match recovery. A wrong automatic
// match is watched and rated. Fix match re-describes the root
// work in place, so the root keeps its ID along with that state.
func TestFixMatchRedescribesInPlace(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	f := e.add(lib, "The Thing/The Thing.mkv")[0]
	e.run(f)
	wrong := e.workOf(f)[0]
	if wrong.ExternalIDs[0].ID != "60935" {
		t.Fatalf("with no year the popular remake should win: %+v", wrong)
	}
	anna := e.newUser("anna")
	v := e.versionOf(f)
	e.play(anna, v, f, 95*time.Minute)
	five := 5
	if err := e.state.SetRating(ctx, anna, wrong.ID, &five); err != nil {
		t.Fatal(err)
	}
	e.refreshedAt(wrong.ID)
	e.poster(wrong.ID)
	e.takeRefreshes()

	if err := e.fix(v, thing1982); err != nil {
		t.Fatal(err)
	}
	right := e.workOf(f)[0]
	if right.ID != wrong.ID || !right.Locked || right.Provisional || right.Year != 1982 {
		t.Fatalf("fixed work %+v, want %v re-described", right, wrong.ID)
	}
	if !slices.Equal(right.ExternalIDs, []catalog.ExternalID{thing1982.ID}) {
		t.Fatalf("external IDs %v, want the fix's alone", right.ExternalIDs)
	}
	s, ok := e.stateOf(anna, right.ID)
	if !ok || !s.played || s.plays != 1 || s.rating == nil || *s.rating != 5 {
		t.Fatalf("state on the root: %+v, %v", s, ok)
	}
	if !e.refreshed(right.ID) || e.artwork(right.ID) != 0 {
		t.Fatal("the root wasn't refreshed from the new match, its old artwork cleared")
	}
	// The folder's next run keeps the manual decision.
	e.again(f)
	if got := e.workOf(f)[0]; got.ID != right.ID || !slices.Equal(got.ExternalIDs, []catalog.ExternalID{thing1982.ID}) {
		t.Fatalf("re-identifying undid the fix: %+v", got)
	}
}

// A locked match is never changed automatically. Neither the folder's own
// search, an NFO, nor a path ID changes it, even when the file moves into a
// folder that names another movie.
func TestLockedMatchWinsOverPathIDs(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	f := e.add(lib, "The Thing/The Thing.mkv")[0]
	e.run(f)
	if err := e.fix(e.versionOf(f), thing1982); err != nil {
		t.Fatal(err)
	}
	fixed := e.workOf(f)[0]

	e.nfo(lib, "The Thing/movie.nfo", `<movie><uniqueid type="tmdb">60935</uniqueid></movie>`)
	e.again(f)
	if got := e.workOf(f)[0]; got.ID != fixed.ID {
		t.Fatalf("an NFO overrode the lock: %+v", got)
	}

	// Move it into a folder whose name carries the other ID.
	e.move(lib, f, "The Thing {tmdb-60935}/The Thing.mkv")
	e.run(f)
	if got := e.workOf(f)[0]; got.ID != fixed.ID || !got.Locked {
		t.Fatalf("a path ID overrode the lock: %+v", got)
	}
}

// A result for a file or a match that changed while identify searched is
// dropped, and the file is left pending.
func TestStaleResultsAreDropped(t *testing.T) {
	t.Run("file changed while searching", func(t *testing.T) {
		e := newEnv(t)
		lib := e.library(catalog.Movies)
		f := e.add(lib, "Heat (1995)/Heat (1995).mkv")[0]
		e.video.hook = func() {
			e.b.Write(func(ctx context.Context, tx *store.Tx) error {
				_, err := e.b.Catalog.UpsertFiles(ctx, tx, lib.Folders[0].ID, []catalog.FileObservation{{RelPath: "Heat (1995)/Heat (1995).mkv", Size: 5, MTime: time.Now()}})
				return err
			})
		}
		e.run(f)
		if w := e.workOf(f)[0]; !w.Provisional {
			t.Fatalf("a result for the old file was written: %+v", w)
		}
		if e.identified(f) {
			t.Fatal("the changed file was recorded identified")
		}
		e.video.hook = nil
		e.run(f) // probed at its new generation
		if w := e.workOf(f)[0]; w.Provisional || w.ExternalIDs[0].ID != "949" || !e.identified(f) {
			t.Fatalf("the next scan didn't match: %+v", w)
		}
	})
	t.Run("match changed while searching", func(t *testing.T) {
		e := newEnv(t)
		lib := e.library(catalog.Movies)
		f := e.add(lib, "Heat (1995)/Heat (1995).mkv")[0]
		e.video.hook = func() {
			v := e.versionOf(f)
			e.b.Write(func(ctx context.Context, tx *store.Tx) error {
				ver, err := e.b.Catalog.Version(ctx, v)
				if err != nil {
					return err
				}
				return e.b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
					_, err := cb.ReassignVersion(ctx, v, ver.Works) // bumps match_rev
					return err
				})
			})
		}
		e.run(f)
		if w := e.workOf(f)[0]; !w.Provisional {
			t.Fatalf("a result for the old match was written: %+v", w)
		}
		if !slices.Equal(e.pending(), []id.ID{f}) {
			t.Fatalf("pending %v, want the file left pending", e.pending())
		}
		e.video.hook = nil
		if err := e.identify(ctx); err != nil {
			t.Fatal(err)
		}
		if w := e.workOf(f)[0]; w.Provisional || w.ExternalIDs[0].ID != "949" {
			t.Fatalf("the next scan didn't match: %+v", w)
		}
	})
}

// identified reports whether a file is recorded as identified at its
// current generation.
func (e *env) identified(file id.ID) bool {
	e.t.Helper()
	var ok bool
	if err := e.b.DB.Reader().QueryRow(`SELECT identified IS generation FROM media_file WHERE id = ?`, file).Scan(&ok); err != nil {
		e.t.Fatal(err)
	}
	return ok
}

// A file appears as a provisional work before any provider call. A folder
// that meets an outage is left unidentified.
func TestProvisionalFirstAndOutage(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "The Expanse (2015)/Season 01/S01E01.mkv", "The Expanse (2015)/Season 01/S01E02.mkv")
	e.video.down = true
	e.run(files[0])
	if e.identified(files[0]) {
		t.Fatal("a folder was recorded identified during an outage")
	}
	for _, f := range files {
		ws := e.workOf(f)
		if len(ws) != 1 || !ws[0].Provisional || ws[0].Number == 0 {
			t.Fatalf("provisional episode %+v", ws)
		}
	}
	var unmatched int
	err := e.b.DB.Reader().QueryRow(`SELECT count(*) FROM work_library wl JOIN work w ON w.id = wl.work_id
		WHERE wl.library_id = ? AND wl.is_root AND w.provisional`, lib.ID).Scan(&unmatched)
	if err != nil || unmatched != 1 {
		t.Fatalf("the unmatched banner would count %d, %v; want the one series", unmatched, err)
	}

	e.video.down = false
	e.run(files...)
	e.again(files...)
	if n := e.video.searchCount(); n != 1 {
		t.Fatalf("searched %d times for one series folder", n)
	}
	if ws := e.workOf(files[1]); ws[0].Provisional || ws[0].Title != "The Big Empty" {
		t.Fatalf("matched episode %+v", ws)
	}
}

// Two identifications of one series folder run at once. The folder is
// locked, so the second finds the first one's decision and doesn't
// search.
func TestOneSearchPerFolderUnderConcurrency(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/S01E01.mkv", "Firefly/S01E02.mkv", "Firefly/S01E03.mkv", "Firefly/S01E04.mkv")
	for _, f := range files {
		e.exec(`UPDATE media_file SET probed = generation WHERE id = ?`, f)
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(files))
	for range files {
		wg.Go(func() { errs <- e.identify(ctx) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := e.video.searchCount(); n != 1 {
		t.Fatalf("searched %d times", n)
	}
	for _, f := range files {
		if w := e.workOf(f)[0]; w.Provisional {
			t.Fatalf("unmatched episode %+v", w)
		}
	}
}

// An unmatched search is looked up again on the next run, so a movie the
// provider lists later is found. A match is reused. Refresh metadata on an
// unmatched work sends its folder back, and asks for the folder alone to be
// identified, not for a scan of its library.
func TestUnmatchedIsSearchedAgain(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	files := e.add(lib, "Arrival (2016)/Arrival (2016).mkv")
	e.run(files...)
	w := e.workOf(files[0])[0]
	if !w.Provisional {
		t.Fatalf("a movie the provider doesn't list matched: %+v", w)
	}
	defer func(saved []listing) { world = saved }(world)
	world = append(slices.Clone(world), listing{kind: "movie", id: "329865", name: "Arrival", year: 2016})

	if err := e.id.Refresh(ctx, e.viewer(), w.ID); err != nil {
		t.Fatal(err)
	}
	if p := e.pending(); !slices.Equal(p, files) {
		t.Fatalf("pending %v, want the folder's file %v", p, files)
	}
	if got := e.takeAsked(); !slices.Equal(got, files) {
		t.Fatalf("asked to identify %v, want the folder's file %v", got, files)
	}
	n := e.video.searchCount()
	e.run(files...)
	if e.video.searchCount() == n {
		t.Fatal("the unmatched decision was reused")
	}
	if w := e.workOf(files[0])[0]; w.Provisional || w.Title != "Arrival" {
		t.Fatalf("after the provider listed it: %+v", w)
	}
	n = e.video.searchCount()
	e.again(files...)
	if e.video.searchCount() != n {
		t.Fatal("a match was searched for again")
	}
}

// Refresh all metadata asks for a refresh of each matched work in the
// library, including seasons and episodes. It sends each unmatched work
// back to identify, as each item's Refresh metadata does.
func TestRefreshLibrary(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv", "Nothing Like It/Nothing Like It - S01E01.mkv")
	e.run(files...)
	ep, unmatched := e.workOf(files[0])[0], e.workOf(files[1])[0]
	if ep.Provisional || !unmatched.Provisional {
		t.Fatalf("set-up: %+v, %+v", ep, unmatched)
	}
	e.refreshedAt(ep.Root, ep.Parent, ep.ID, unmatched.ID)
	e.takeRefreshes()

	n, err := e.id.RefreshLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 { // the series, season and episode, and the unmatched series
		t.Errorf("asked for %d items, want 4", n)
	}
	want := []id.ID{ep.Root, ep.Parent, ep.ID}
	got := e.takeRefreshes()
	slices.SortFunc(got, func(a, b id.ID) int { return bytes.Compare(a[:], b[:]) })
	slices.SortFunc(want, func(a, b id.ID) int { return bytes.Compare(a[:], b[:]) })
	if !slices.Equal(got, want) {
		t.Errorf("refreshed %v, want the series, season and episode %v", got, want)
	}
	for _, w := range want {
		if !e.due(w) {
			t.Errorf("%v isn't due a refresh", w)
		}
	}
	if e.due(unmatched.ID) {
		t.Error("the unmatched series is due a refresh")
	}
	if p := e.pending(); !slices.Equal(p, files[1:]) {
		t.Errorf("sent back %v, want the unmatched folder's file %v", p, files[1:])
	}
	if got := e.takeAsked(); !slices.Equal(got, files[1:]) {
		t.Errorf("asked to identify %v, want the unmatched folder's file %v", got, files[1:])
	}
	if _, err := e.id.RefreshLibrary(ctx, id.New()); !errors.Is(err, catalog.ErrNotFound) {
		t.Errorf("a missing library: %v", err)
	}
}

func TestSetOrderingRenumbersEpisodes(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv", "Firefly/Season 1/Firefly - S01E02.mkv")
	e.run(files...)
	trainJob := e.workOf(files[0])[0]
	if trainJob.Title != "The Train Job" {
		t.Fatalf("aired S01E01 is %+v", trainJob)
	}
	dvd := string(metadata.DVD)
	if err := e.id.EditWork(ctx, e.viewer(), trainJob.Root, identify.WorkEdit{Ordering: &dvd}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(e.pending(), files) {
		t.Fatalf("pending %v, want the series folder's files %v", e.pending(), files)
	}
	e.run(e.pending()...)
	if got := e.workOf(files[0])[0]; got.Title == "The Train Job" {
		t.Fatalf("in DVD order S01E01 is the pilot, got %+v", got)
	}
	moved, err := e.b.Catalog.Work(ctx, trainJob.ID)
	if err != nil {
		t.Fatalf("The Train Job's work: %v", err)
	}
	if moved.Number != 2 {
		t.Fatalf("The Train Job is E%02d in DVD order, want E02 (same work, renumbered)", moved.Number)
	}
	if got := e.workOf(files[1])[0]; got.ID != trainJob.ID {
		t.Fatalf("the DVD-named S01E02 file should be The Train Job, got %+v", got)
	}
}

// A watched file stays watched under the number a new ordering gives it.
// Its plays move to the episode it now covers, and the episode it leaves
// keeps its work.
func TestSetOrderingCarriesWatchState(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E03.mkv")
	e.run(files...)
	reynolds := e.workOf(files[0])[0]
	if reynolds.Title != "Our Mrs. Reynolds" {
		t.Fatalf("aired S01E03 is %+v", reynolds)
	}
	anna, bob := e.newUser("anna"), e.newUser("bob")
	e.play(anna, e.versionOf(files[0]), files[0], 95*time.Minute)
	dvd := string(metadata.DVD)
	if err := e.id.EditWork(ctx, e.viewer(), reynolds.Root, identify.WorkEdit{Ordering: &dvd}); err != nil {
		t.Fatal(err)
	}
	e.run(e.pending()...)
	now := e.workOf(files[0])[0]
	if now.Title != "Bushwhacked" || now.Number != 3 {
		t.Fatalf("in DVD order the file is %+v, want Bushwhacked, S01E03", now)
	}
	if s, ok := e.stateOf(anna, now.ID); !ok || !s.played || s.plays != 1 {
		t.Fatalf("anna's state on DVD S01E03: %+v, %v", s, ok)
	}
	if _, ok := e.stateOf(bob, now.ID); ok {
		t.Fatal("bob never watched it")
	}
	if e.gone(reynolds.ID) {
		t.Fatal("the matched episode left empty was deleted")
	}
	if s, _ := e.stateOf(anna, reynolds.ID); s.played || s.plays != 0 {
		t.Fatalf("anna's state on the episode the file left: %+v", s)
	}
}

// Whichever order a pass moves files in, each one's plays follow it. In
// DVD order the S01E01 file leaves The Train Job for Serenity before the
// S01E04 file leaves Serenity, so Serenity is never left empty.
func TestSetOrderingStateFollowsEachFile(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv", "Firefly/Season 1/Firefly - S01E04.mkv")
	e.run(files...)
	anna, bob := e.newUser("anna"), e.newUser("bob")
	e.play(anna, e.versionOf(files[0]), files[0], 95*time.Minute)
	e.play(bob, e.versionOf(files[1]), files[1], 95*time.Minute)

	root := e.workOf(files[0])[0].Root
	dvd := string(metadata.DVD)
	if err := e.id.EditWork(ctx, e.viewer(), root, identify.WorkEdit{Ordering: &dvd}); err != nil {
		t.Fatal(err)
	}
	e.run(e.pending()...)
	first, fourth := e.workOf(files[0])[0], e.workOf(files[1])[0]
	if first.Title != "Serenity" || fourth.Title != "Our Mrs. Reynolds" {
		t.Fatalf("in DVD order the files are %+v and %+v", first, fourth)
	}
	for _, c := range []struct {
		name            string
		user, has, lack id.ID
	}{{"anna", anna, first.ID, fourth.ID}, {"bob", bob, fourth.ID, first.ID}} {
		if s, ok := e.stateOf(c.user, c.has); !ok || !s.played || s.plays != 1 {
			t.Errorf("%s's state on the file she watched: %+v, %v", c.name, s, ok)
		}
		if s, _ := e.stateOf(c.user, c.lack); s.played || s.plays != 0 {
			t.Errorf("%s's state on the other file: %+v", c.name, s)
		}
	}
}

func TestFixMatchRejects(t *testing.T) {
	e := newEnv(t)
	movies := e.library(catalog.Movies)
	f := e.add(movies, "Heat (1995)/Heat (1995).mkv", "Heat (1995)/Trailers/Trailer.mkv")
	e.run(f...)
	series := metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: "1437"}, Title: "Firefly"}
	if err := e.fix(e.versionOf(f[0]), series); !errors.Is(err, identify.ErrInvalid) {
		t.Fatalf("a series for a movie: %v", err)
	}
	if err := e.fix(e.versionOf(f[1]), thing1982); !errors.Is(err, identify.ErrInvalid) {
		t.Fatalf("fixing an extra: %v", err)
	}
	videos := e.library(catalog.Videos)
	v := e.add(videos, "Cake.mp4")
	e.run(v...)
	if err := e.fix(e.versionOf(v[0]), thing1982); !errors.Is(err, identify.ErrInvalid) {
		t.Fatalf("fixing another video: %v", err)
	}
	if err := e.fix(id.New(), thing1982); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("a missing version: %v", err)
	}
}

// Fix match on one episode re-describes its series in place and places
// every episode of its folder under the new match at once.
func TestFixMatchSeriesFolder(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Show/S01E01.mkv", "Show/S01E02.mkv")
	e.run(files...)
	before := e.workOf(files[0])[0]
	if !before.Provisional {
		t.Fatal("a show named Show shouldn't match anything")
	}
	firefly := metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: "1437"}, Title: "Firefly", Year: 2002}
	if err := e.fix(e.versionOf(files[0]), firefly); err != nil {
		t.Fatal(err)
	}
	for f, want := range map[id.ID]string{files[0]: "The Train Job", files[1]: "Bushwhacked"} {
		if w := e.workOf(f)[0]; w.Title != want || w.Root != before.Root || w.Provisional {
			t.Fatalf("fixed episode %+v, want %s under the same series %v", w, want, before.Root)
		}
	}
	if n := len(e.placed); n == 0 || len(e.placed[n-1]) != 2 {
		t.Fatalf("placed %v after the fix", e.placed)
	}
	series, _ := e.b.Catalog.Work(ctx, before.Root)
	if series.Title != "Firefly" || !series.Locked {
		t.Fatalf("series %+v", series)
	}
	var n int
	if err := e.b.DB.Reader().QueryRow(`SELECT count(*) FROM work WHERE kind IN ('series', 'episode')`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("%d series and episodes, want the series and its two episodes", n)
	}
}

// Two folders that the same match describes are two root works. Neither
// joins the other, and Fix match on one leaves the other as it is.
func TestFoldersOfOneMatchAreTwoRoots(t *testing.T) {
	e := newEnv(t)
	a := e.add(e.library(catalog.Shows), "Firefly/S01E01.mkv")
	b := e.add(e.library(catalog.Shows), "Firefly/S01E02.mkv")
	e.run(a[0], b[0])
	wa, wb := e.workOf(a[0])[0], e.workOf(b[0])[0]
	if wa.Root == wb.Root {
		t.Fatal("two folders of one match were joined")
	}
	sa, _ := e.b.Catalog.Work(ctx, wa.Root)
	sb, _ := e.b.Catalog.Work(ctx, wb.Root)
	if !slices.Equal(sa.ExternalIDs, sb.ExternalIDs) || len(sa.ExternalIDs) != 1 {
		t.Fatalf("external IDs %v and %v, want Firefly's on both", sa.ExternalIDs, sb.ExternalIDs)
	}
	expanse := metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: "63639"}, Title: "The Expanse"}
	if _, err := e.id.FixMatch(ctx, e.viewer(), wa.Root, expanse.ID); err != nil {
		t.Fatal(err)
	}
	if after, _ := e.b.Catalog.Work(ctx, wb.Root); !slices.Equal(after.ExternalIDs, sb.ExternalIDs) || after.Locked {
		t.Fatalf("the other folder's root changed: %+v", after)
	}
}

// Identify identifies the folders of a library's files that were probed but
// not yet identified, with one search per folder. It records each folder's
// files as identified at their generation, so the next scan leaves them
// alone. A file not yet probed waits for its probe.
func TestIdentify(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	files := e.add(lib, "Heat (1995)/Heat (1995) - 1080p.mkv", "Heat (1995)/Heat (1995) - 4K.mkv", "Dune (2021)/Dune.mkv", "Blade Runner (1982)/Blade Runner.mkv")
	for _, f := range files[:3] {
		e.exec(`UPDATE media_file SET probed = generation WHERE id = ?`, f)
	}
	if err := e.identify(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.video.searchCount(); n != 2 {
		t.Fatalf("searched %d times, want once per folder", n)
	}
	for _, f := range files[:3] {
		if !e.identified(f) || e.workOf(f)[0].Provisional {
			t.Fatalf("%v: identified %v, works %+v", f, e.identified(f), e.workOf(f))
		}
	}
	if e.workOf(files[0])[0].ID != e.workOf(files[1])[0].ID {
		t.Fatal("a folder's two versions weren't one work")
	}
	if e.identified(files[3]) || len(e.pending()) != 0 {
		t.Fatalf("the unprobed file: identified %v, pending %v", e.identified(files[3]), e.pending())
	}

	if err := e.identify(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.video.searchCount(); n != 2 {
		t.Fatalf("a second scan searched %d more times", n-2)
	}
}

// A folder that can't be read is left pending, which identifies
// it once it is back.
func TestIdentifyUnreachableFolder(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	f := e.add(lib, "Heat (1995)/Heat (1995).mkv")[0]
	e.offline.Store(true)
	e.run(f)
	if e.identified(f) || !slices.Equal(e.pending(), []id.ID{f}) {
		t.Fatalf("an unreachable folder: identified %v, pending %v", e.identified(f), e.pending())
	}
	e.offline.Store(false)
	if err := e.identify(ctx); err != nil {
		t.Fatal(err)
	}
	if w := e.workOf(f)[0]; w.Provisional || !e.identified(f) {
		t.Fatalf("once it's back: %+v, identified %v", w, e.identified(f))
	}
}

// Sending a folder back asks for it to be identified only once the write
// commits. It doesn't ask before the commit, or at all if the write rolls
// back.
func TestReidentifyAsksAfterCommit(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv", "Nothing Like It/Nothing Like It - S01E01.mkv")
	e.run(files...)

	// Refresh all metadata sends the unmatched folder back, then fails while
	// asking for the matched works' refreshes.
	e.exec(`CREATE TRIGGER refresh_fails BEFORE UPDATE OF refreshed_at ON work BEGIN SELECT RAISE(ABORT, 'refresh failed'); END`)
	if _, err := e.id.RefreshLibrary(ctx, lib.ID); err == nil || !strings.Contains(err.Error(), "refresh failed") {
		t.Fatalf("got %v, want the refresh's error", err)
	}
	if got := e.takeAsked(); len(got) != 0 || len(e.pending()) != 0 {
		t.Fatalf("a rolled back write asked for %v, sent back %v", got, e.pending())
	}

	// The task sees the folder sent back, because the write committed.
	e.exec(`DROP TRIGGER refresh_fails`)
	e.onAsk = func(id.ID) {
		if !slices.Equal(e.pending(), files[1:]) {
			t.Errorf("asked before the commit: pending %v", e.pending())
		}
	}
	if err := e.id.Refresh(ctx, e.viewer(), e.workOf(files[1])[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := e.takeAsked(); !slices.Equal(got, files[1:]) || !slices.Equal(e.pending(), files[1:]) {
		t.Fatalf("asked to identify %v, pending %v", got, e.pending())
	}
}
