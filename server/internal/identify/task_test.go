package identify_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/store"
)

// The task lists one file for each folder that needs identifying, by path,
// and counts every file. Doing one file identifies its whole folder, so
// another file of the folder has nothing left to do and searches for
// nothing.
func TestTaskDoesEachFolderOnce(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/S01E01.mkv", "Firefly/S01E02.mkv", "The Expanse/S01E01.mkv")
	for _, f := range files {
		e.exec(`UPDATE media_file SET probed = generation WHERE id = ?`, f)
	}
	w := e.task()
	pending, err := w.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{files[0], files[2]}; !slices.Equal(pending, want) {
		t.Fatalf("pending %v, want the first file of each folder %v", pending, want)
	}
	if n, err := w.PendingCount(ctx); err != nil || n != 3 {
		t.Fatalf("pending count %d %v, want every file", n, err)
	}

	if err := w.Do(ctx, files[0]); err != nil {
		t.Fatal(err)
	}
	if !e.identified(files[0]) || !e.identified(files[1]) || e.identified(files[2]) {
		t.Fatalf("after one file of Firefly: pending %v", e.pending())
	}
	searches := e.video.searchCount()
	if err := w.Do(ctx, files[1]); err != nil {
		t.Fatal(err)
	}
	if n := e.video.searchCount(); n != searches {
		t.Fatalf("the folder's second file searched %d more times", n-searches)
	}
	if n, err := w.PendingCount(ctx); err != nil || n != 1 {
		t.Fatalf("pending count %d %v, want the other folder's file", n, err)
	}

	// A file sent back is found through any file of its folder.
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.id.Reidentify(ctx, tx, files[1:2]) })
	if err := w.Do(ctx, files[0]); err != nil {
		t.Fatal(err)
	}
	if !e.identified(files[1]) {
		t.Fatal("a file sent back wasn't identified with its folder")
	}
}

// A new ordering sends the series' folder back and asks for that folder to
// be identified, by one of its files. It asks for no scan of the library.
func TestSetOrderingAsksForTheFolder(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	files := e.add(lib, "Firefly/Season 1/Firefly - S01E01.mkv", "Firefly/Season 1/Firefly - S01E02.mkv")
	e.run(files...)
	e.takeAsked()
	dvd := string(metadata.DVD)
	if err := e.id.EditWork(ctx, e.viewer(), e.workOf(files[0])[0].Root, identify.WorkEdit{Ordering: &dvd}); err != nil {
		t.Fatal(err)
	}
	if got := e.takeAsked(); len(got) != 1 || !slices.Contains(files, got[0]) {
		t.Fatalf("asked to identify %v, want one file of the folder %v", got, files)
	}
	if !slices.Equal(e.pending(), files) {
		t.Fatalf("pending %v, want the folder's files %v", e.pending(), files)
	}
}

// The task counts the files left to identify. A folder that meets an
// outage stays counted until a later pass. A folder the provider has no
// match for is identified, as unmatched, and is no longer counted.
func TestPendingCountThroughAnOutage(t *testing.T) {
	e := newEnv(t)
	movies, shows := e.library(catalog.Movies), e.library(catalog.Shows)
	unlisted := e.add(movies, "Arrival (2016)/Arrival (2016).mkv")
	episodes := e.add(shows, "Firefly/S01E01.mkv", "Firefly/S01E02.mkv")
	for _, f := range append(unlisted, episodes...) {
		e.exec(`UPDATE media_file SET probed = generation WHERE id = ?`, f)
	}
	left := func() int {
		t.Helper()
		n, err := e.task().PendingCount(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := left(); n != 3 {
		t.Fatalf("%d left, want 1 movie and 2 episodes", n)
	}

	e.video.down = true
	if err := e.identify(ctx); err != nil {
		t.Fatal(err)
	}
	if n := left(); n != 3 {
		t.Fatalf("%d left after an outage, want every file", n)
	}

	e.video.down = false
	if err := e.identify(ctx); err != nil {
		t.Fatal(err)
	}
	if w := e.workOf(unlisted[0])[0]; !w.Provisional {
		t.Fatalf("a movie the provider doesn't list matched: %+v", w)
	}
	if n := left(); n != 0 {
		t.Fatalf("%d left, want none once matched or unmatched", n)
	}
}
