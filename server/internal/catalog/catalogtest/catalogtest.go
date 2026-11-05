// Package catalogtest builds catalog fixtures for other packages' tests, and
// checks the catalog's invariants when each test ends.
package catalogtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
)

// Builder makes libraries, files, works and versions in a fresh database.
type Builder struct {
	t       testing.TB
	DB      *store.DB
	Bus     *events.Bus // the bus the catalog publishes on
	Catalog *catalog.Catalog
	n       int
}

// Item is a playable work with its one single-file version.
type Item struct{ Work, Version, File id.ID }

// New returns a builder over a new database. When the test ends it fails the
// test if any catalog invariant is broken.
func New(t testing.TB) *Builder {
	t.Helper()
	db := storetest.New(t)
	bus := events.NewBus()
	b := &Builder{t: t, DB: db, Bus: bus, Catalog: catalog.New(db, bus)}
	t.Cleanup(func() {
		if err := b.Catalog.CheckInvariants(context.Background()); err != nil {
			t.Errorf("catalog invariants: %v", err)
		}
	})
	return b
}

// Write runs fn in a write transaction and fails the test on error.
func (b *Builder) Write(fn func(ctx context.Context, tx *store.Tx) error) {
	b.t.Helper()
	if err := b.DB.Write(context.Background(), fn); err != nil {
		b.t.Fatal(err)
	}
}

// Library makes a library of kind (catalog.Movies, Shows or Videos) with one
// folder, and returns the library and the folder.
func (b *Builder) Library(kind catalog.LibraryKind) (lib, folder id.ID) {
	b.t.Helper()
	b.n++
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{
			Name: fmt.Sprintf("%s %d", kind, b.n), Kind: kind, Folders: []string{fmt.Sprintf("/media/%s%d", kind, b.n)},
		})
		lib, folder = l.ID, l.Folders[0].ID
		return err
	})
	return lib, folder
}

// File adds a present file at rel under folder.
func (b *Builder) File(folder id.ID, rel string) id.ID {
	b.t.Helper()
	var f id.ID
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		st, err := b.Catalog.UpsertFiles(ctx, tx, folder, []catalog.FileObservation{{RelPath: rel, Size: 1 << 30, MTime: time.UnixMilli(1)}})
		if err == nil {
			f = st[0].ID
		}
		return err
	})
	return f
}

// BulkFiles adds n present files, dir/0.mkv to dir/<n-1>.mkv, under folder
// in one statement. It builds a large library quickly for tests of large
// writes. The files are unprobed and in no version.
func (b *Builder) BulkFiles(folder id.ID, dir string, n int) {
	b.t.Helper()
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `
			WITH RECURSIVE i (n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM i WHERE n + 1 < ?)
			INSERT INTO media_file (id, folder_id, rel_path, size, mtime, status)
			SELECT randomblob(16), ?, ? || '/' || n || '.mkv', 1 << 30, 1, 'present' FROM i`, n, folder, dir)
		return err
	})
}

// MarkGone marks files gone in tx, as a scan does when it finds them
// missing from their paths.
func (b *Builder) MarkGone(ctx context.Context, tx *store.Tx, files ...id.ID) error {
	for _, f := range files {
		file, err := b.Catalog.File(ctx, f)
		if err != nil {
			return err
		}
		if err := b.Catalog.MarkGone(ctx, tx, file.Folder, []catalog.FileRef{{ID: f, RelPath: file.RelPath}}); err != nil {
			return err
		}
	}
	return nil
}

// Work finds or makes a work.
func (b *Builder) Work(in catalog.WorkInput) id.ID {
	b.t.Helper()
	var w id.ID
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		w, err = b.Catalog.EnsureWork(ctx, tx, in)
		return err
	})
	return w
}

// Version makes files into a version covering works.
func (b *Builder) Version(parts []id.ID, works ...id.ID) id.ID {
	b.t.Helper()
	var v id.ID
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
			var err error
			v, err = cb.PutVersion(ctx, catalog.VersionInput{Parts: parts, Works: works})
			return err
		})
	})
	return v
}

// Movie adds a movie with one file under folder.
func (b *Builder) Movie(folder id.ID, title string, year int) Item {
	b.t.Helper()
	w := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: title, Year: year})
	f := b.File(folder, fmt.Sprintf("%s (%d)/%s (%d) %d.mkv", title, year, title, year, b.next()))
	return Item{Work: w, Version: b.Version([]id.ID{f}, w), File: f}
}

// Series adds a series with no episodes.
func (b *Builder) Series(title string) id.ID {
	return b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: title})
}

// Episode adds an episode of series, with its season and one file under folder.
func (b *Builder) Episode(folder, series id.ID, season, episode int) Item {
	b.t.Helper()
	s := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: fmt.Sprintf("Season %d", season), Number: season})
	w := b.Work(catalog.WorkInput{
		Kind: catalog.KindEpisode, Parent: s, Title: fmt.Sprintf("Episode %d", episode), ParentNumber: season, Number: episode,
	})
	f := b.File(folder, fmt.Sprintf("Show %v/Season %02d/S%02dE%02d %d.mkv", series, season, season, episode, b.next()))
	return Item{Work: w, Version: b.Version([]id.ID{f}, w), File: f}
}

func (b *Builder) next() int {
	b.n++
	return b.n
}
