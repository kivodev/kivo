package identify_test

import (
	"context"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// books is an audiobooks library whose files are on disk as well as in the
// catalog.
type books struct {
	*env
	lib catalog.Library
}

func newBooks(t *testing.T) books {
	e := newEnv(t)
	return books{env: e, lib: e.library(catalog.Audiobooks)}
}

// put adds files on disk and to the catalog, and identifies them. Each file
// is tagged with asin when asin isn't empty.
func (b books) put(asin string, rels ...string) []id.ID {
	b.t.Helper()
	files := b.add(b.lib, rels...)
	for k, rel := range rels {
		b.disk[b.lib.Folders[0].Path][rel] = &fstest.MapFile{}
		if asin == "" {
			continue
		}
		f, err := b.b.Catalog.File(ctx, files[k])
		if err != nil {
			b.t.Fatal(err)
		}
		b.b.Write(func(ctx context.Context, tx *store.Tx) error {
			return b.b.Catalog.SetProbe(ctx, tx, f.ID, f.Generation, catalog.Probe{Summary: catalog.ProbeSummary{Tags: map[string]string{"asin": asin}}})
		})
	}
	b.run(files...)
	return files
}

// remove takes files off disk. With gone set, it also marks them gone, as
// a scan does at the end of its run.
func (b books) remove(gone bool, files ...id.ID) {
	b.t.Helper()
	for _, f := range files {
		file, err := b.b.Catalog.File(ctx, f)
		if err != nil {
			b.t.Fatal(err)
		}
		delete(b.disk[b.lib.Folders[0].Path], file.RelPath)
	}
	if gone {
		b.b.Write(func(ctx context.Context, tx *store.Tx) error { return b.b.MarkGone(ctx, tx, files...) })
	}
}

// edited makes a book that a user has partly listened to and an admin has
// edited, locking its title and ASIN.
func (b books) edited(file id.ID, asin string) (work, user id.ID) {
	b.t.Helper()
	work = b.workOf(file)[0].ID
	user = b.newUser("anna")
	b.play(user, b.versionOf(file), file, 30*time.Minute)
	m := catalog.WorkMetadata{Title: "Project Hail Mary (edited)", Book: &catalog.BookMetadata{ASIN: asin}}
	b.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.b.Catalog.EditMetadata(ctx, tx, work, m, catalog.FieldTitle, catalog.FieldASIN)
	})
	return work, user
}

// kept checks that file is a version of work, and that work kept its
// edits, its ASIN and the user's position.
func (b books) kept(file, work, user id.ID, asin string) {
	b.t.Helper()
	got := b.workOf(file)
	if len(got) != 1 || got[0].ID != work {
		b.t.Fatalf("the new files are on %v, want the book they replace, %v\n%s", got, work, b.dump())
	}
	w := got[0]
	if w.Title != "Project Hail Mary (edited)" || !slices.Contains(w.LockedFields, catalog.FieldTitle) {
		b.t.Errorf("title %q, locked %v: the edit is lost", w.Title, w.LockedFields)
	}
	if !slices.Contains(w.ExternalIDs, catalog.ExternalID{Provider: catalog.ASIN.Provider, Kind: catalog.ASIN.Kind, ID: asin}) {
		b.t.Errorf("external IDs %v, want the ASIN %s", w.ExternalIDs, asin)
	}
	if s, ok := b.stateOf(user, work); !ok || s.position == 0 {
		b.t.Errorf("state %+v: the position is lost", s)
	}
}

func (b books) newBook(file, old id.ID) {
	b.t.Helper()
	if w := b.workOf(file)[0]; w.ID == old {
		b.t.Errorf("the new files took over %v, want a new book\n%s", old, b.dump())
	}
}

const phm = "B08FHBV4ZX"

// markOrders are the two possible orders within a scan run. The new files
// are identified either before or after the scan marks the old ones gone.
var markOrders = map[string]bool{"before marking gone": false, "after marking gone": true}

// MP3s replaced by one M4B in the same folder keep the book, whatever ASIN
// the new files give, because the folder is the root work.
func TestBookFilesReplacedKeepTheBook(t *testing.T) {
	for name, markedGone := range markOrders {
		t.Run(name, func(t *testing.T) {
			b := newBooks(t)
			mp3s := b.put("", "Andy Weir/Project Hail Mary/01.mp3", "Andy Weir/Project Hail Mary/02.mp3", "Andy Weir/Project Hail Mary/03.mp3")
			work, user := b.edited(mp3s[0], phm)
			b.remove(markedGone, mp3s...)
			m4b := b.put("B0CTKSG1VB", "Andy Weir/Project Hail Mary/Project Hail Mary.m4b")[0]
			if !markedGone {
				b.remove(true, mp3s...)
			}
			b.kept(m4b, work, user, phm)
		})
	}
}

// A book whose folder is renamed, with its files moved along, keeps the
// book.
func TestBookFolderRenamedKeepsTheBook(t *testing.T) {
	b := newBooks(t)
	mp3 := b.put("", "Andy Weir/Project Hail Mary/01.mp3")[0]
	work, user := b.edited(mp3, phm)
	b.move(b.lib, mp3, "Weir, Andy/2021 - Project Hail Mary/01.mp3")
	b.run(mp3)
	b.kept(mp3, work, user, phm)
}

// New files in another folder are another book, even with the ASIN of an
// existing book, whether that book's files are gone or still there. A book
// is never found by its match.
func TestBookIsNeverFoundByItsASIN(t *testing.T) {
	b := newBooks(t)
	mp3s := b.put("", "Andy Weir/Project Hail Mary/01.mp3")
	work, _ := b.edited(mp3s[0], phm)
	b.newBook(b.put(phm, "Copies/Project Hail Mary/Project Hail Mary.m4b")[0], work)
	b.remove(true, mp3s...)
	b.newBook(b.put(phm, "Weir, Andy/2021 - Project Hail Mary/Project Hail Mary.m4b")[0], work)
}

// Replacing a movie's file in its folder, as Radarr does when it upgrades
// one, keeps the movie, its match and its watch state.
func TestMovieFileReplacedKeepsTheMovie(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Movies)
	old := e.add(lib, "Heat (1995)/Heat (1995) 1080p.mkv")[0]
	e.run(old)
	heat := e.workOf(old)[0]
	anna := e.newUser("anna")
	e.play(anna, e.versionOf(old), old, 95*time.Minute)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.b.MarkGone(ctx, tx, old) })
	upgrade := e.add(lib, "Heat (1995)/Heat (1995) 2160p.mkv")[0]
	e.run(upgrade)
	if w := e.workOf(upgrade)[0]; w.ID != heat.ID || w.Provisional {
		t.Fatalf("the upgrade is on %+v, want Heat %v", w, heat.ID)
	}
	if s, ok := e.stateOf(anna, heat.ID); !ok || !s.played {
		t.Fatalf("watch state %+v, %v", s, ok)
	}
}
