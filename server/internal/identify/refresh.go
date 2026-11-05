package identify

import (
	"context"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// Refresh asks for a metadata refresh of a matched work, including the
// seasons and episodes under it. For an unmatched work, it instead has the
// work's folders identified again, which searches for it again. The work
// must be one the Viewer can see.
func (i *Identifier) Refresh(ctx context.Context, v users.Viewer, work id.ID) error {
	w, err := i.visibleWork(ctx, v, work)
	if err != nil {
		return err
	}
	libs, err := i.libraries(ctx)
	if err != nil {
		return err
	}
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if w.Provisional {
			return i.reidentifyWork(ctx, tx, libs, work)
		}
		return i.deps.Metadata.Refresh(ctx, tx, work)
	})
}

// RefreshLibrary does what Refresh does for every item in a library, and
// returns how many items it asked for. Seasons and episodes are asked for
// as works of their own, so each transaction handles at most one batch.
func (i *Identifier) RefreshLibrary(ctx context.Context, lib id.ID) (int, error) {
	if _, err := i.deps.Catalog.Library(ctx, lib); err != nil {
		return 0, err
	}
	works, err := i.q.LibraryRefreshWorks(ctx, lib)
	if err != nil {
		return 0, err
	}
	libs, err := i.libraries(ctx)
	if err != nil {
		return 0, err
	}
	err = store.Batches(ctx, i.db, works, func(ctx context.Context, tx *store.Tx, batch []dbq.LibraryRefreshWorksRow) error {
		var matched []id.ID
		for _, w := range batch {
			if !w.Provisional {
				matched = append(matched, w.ID)
			} else if err := i.reidentifyWork(ctx, tx, libs, w.ID); err != nil {
				return err
			}
		}
		return i.deps.Metadata.RefreshWorks(ctx, tx, matched...)
	})
	if err != nil {
		return 0, err
	}
	return len(works), nil
}

// libraries returns the libraries by ID, except those being deleted. A
// write that sends folders back reads them beforehand, rather than through
// the reader pool inside the write.
func (i *Identifier) libraries(ctx context.Context) (map[id.ID]catalog.Library, error) {
	all, err := i.deps.Catalog.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	libs := make(map[id.ID]catalog.Library, len(all))
	for _, l := range all {
		libs[l.ID] = l
	}
	return libs, nil
}

// reidentifyWork has each folder holding the work's files identified again.
// It skips folders of a library missing from libs, which is one being
// deleted.
func (i *Identifier) reidentifyWork(ctx context.Context, tx *store.Tx, libs map[id.ID]catalog.Library, work id.ID) error {
	files, err := dbq.New(tx).FilesOfWork(ctx, work)
	if err != nil {
		return err
	}
	byLib := map[id.ID][]id.ID{}
	for _, f := range files {
		if _, ok := libs[f.LibraryID]; ok {
			byLib[f.LibraryID] = append(byLib[f.LibraryID], f.ID)
		}
	}
	for lib, ids := range byLib {
		if err := i.reidentifyAndScan(ctx, tx, lib, ids...); err != nil {
			return err
		}
	}
	return nil
}
