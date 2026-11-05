package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// Batch groups one transaction's catalog writes that change which works are
// available. Each work they touch has its work_library row rebuilt once,
// when the batch ends, rather than after every write. Otherwise a write
// that moves every episode of a series would rebuild the series once per
// episode. The ancestors a work had when it was touched are rebuilt too,
// so an episode merged away before the batch ends still has its old season
// and series rebuilt.
type Batch struct {
	c     *Catalog
	tx    *store.Tx
	seen  map[id.ID]bool
	works []id.ID
}

// Batch runs fn with a batch on tx, then flushes it.
func (c *Catalog) Batch(ctx context.Context, tx *store.Tx, fn func(*Batch) error) error {
	b := &Batch{c: c, tx: tx, seen: map[id.ID]bool{}}
	if err := fn(b); err != nil {
		return err
	}
	if err := b.flush(ctx); err != nil {
		return fmt.Errorf("rebuild work libraries: %w", err)
	}
	return nil
}

func (b *Batch) q() *dbq.Queries { return dbq.New(b.tx) }

// flush rebuilds the work_library rows of the works the batch touched.
func (b *Batch) flush(ctx context.Context) error {
	for _, w := range b.works {
		if err := b.c.rebuildWork(ctx, b.tx, w); err != nil {
			return err
		}
	}
	return nil
}

// add collects works and their ancestors for the flush.
func (b *Batch) add(ctx context.Context, q *dbq.Queries, works []id.ID) error {
	for _, w := range works {
		up, err := q.Ancestors(ctx, w)
		if err != nil {
			return err
		}
		for _, a := range up {
			if !b.seen[a] {
				b.seen[a] = true
				b.works = append(b.works, a)
			}
		}
	}
	return nil
}

// refreshLibraries rebuilds work_library for works and their ancestors
// right away. It is for a write outside a batch.
func (c *Catalog) refreshLibraries(ctx context.Context, tx *store.Tx, works []id.ID) error {
	b := &Batch{c: c, tx: tx, seen: map[id.ID]bool{}}
	if err := b.add(ctx, b.q(), works); err != nil {
		return err
	}
	return b.flush(ctx)
}

// rebuildWork rebuilds one work's work_library row from its descendants.
// When the row changed and the work was a root before or is one after,
// it publishes a change for those libraries.
func (c *Catalog) rebuildWork(ctx context.Context, tx *store.Tx, w id.ID) error {
	q := dbq.New(tx)
	was, err := q.ClearWorkLibrary(ctx, w)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	is, err := q.FillWorkLibrary(ctx, w)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if was == dbq.ClearWorkLibraryRow(is) {
		return nil
	}
	var libs []id.ID
	if was.IsRoot {
		libs = append(libs, was.LibraryID)
	}
	if is.IsRoot {
		libs = append(libs, is.LibraryID)
	}
	c.tell(tx, libs)
	return nil
}

// tell publishes libraryChanged for libs once the write commits.
func (c *Catalog) tell(tx *store.Tx, libs []id.ID) {
	if len(libs) == 0 {
		return
	}
	tx.AfterCommit(func() {
		c.bus.Publish(events.Event{Kind: events.LibraryChanged, Scope: events.Scope{Libraries: libs}})
	})
}

// refreshFiles rebuilds work_library for the works whose versions use files.
func (c *Catalog) refreshFiles(ctx context.Context, tx *store.Tx, files []id.ID) error {
	if len(files) == 0 {
		return nil
	}
	_, works, err := versionsOfFiles(ctx, dbq.New(tx), files)
	if err != nil {
		return err
	}
	return c.refreshLibraries(ctx, tx, works)
}

// versionsOfFiles returns the versions that use files, and the works those
// versions cover.
func versionsOfFiles(ctx context.Context, q *dbq.Queries, files []id.ID) (versions, works []id.ID, err error) {
	if versions, err = q.VersionsOfFiles(ctx, id.Set(files...)); err != nil {
		return nil, nil, err
	}
	works, err = q.WorksOfVersions(ctx, id.Set(versions...))
	return versions, works, err
}
