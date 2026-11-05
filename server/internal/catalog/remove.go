package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// TaskLibraryChanges removes folders from their libraries and deletes
// libraries. Its items are the marked libraries and folders.
//
// Each library or folder is marked first. That takes it off the library
// list or its library's folder list, and stops scans of it. The task then
// deletes its files a batch at a time, so it never holds the writer for
// long and resumes after a restart. Items leave browse, search and Home as
// their files are deleted.
const TaskLibraryChanges = "library-changes"

// Tasks returns the catalog's task for cmd/kivo to add. Its work always
// runs to the end, because a half-deleted library is worse than a deleted
// one.
func (c *Catalog) Tasks() []tasks.Task {
	return []tasks.Task{{Name: TaskLibraryChanges, Finish: true, Pending: c.removals, Do: c.remove}}
}

// removals lists the libraries being deleted, then the folders being
// removed from libraries that aren't being deleted.
func (c *Catalog) removals(ctx context.Context) ([]id.ID, error) {
	libs, err := c.q.DeletingLibraries(ctx)
	if err != nil {
		return nil, err
	}
	folders, err := c.q.RemovingFolders(ctx)
	return append(libs, folders...), err
}

// remove deletes a library marked as being deleted, or removes a folder
// marked as being removed.
func (c *Catalog) remove(ctx context.Context, item id.ID) error {
	lib, err := c.q.GetLibrary(ctx, item)
	switch {
	case err == nil:
		return c.deleteLibrary(ctx, lib)
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	f, err := c.q.GetFolder(ctx, item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // removed already
	}
	if err != nil {
		return err
	}
	return c.removeMarkedFolder(ctx, f)
}

// MarkDeleting marks a library as being deleted and each of its folders as
// being removed. That takes the library off the library list at once. Once
// the transaction commits, the caller asks TaskLibraryChanges to delete it.
// A library already being deleted is ErrNotFound. scan.DeleteLibrary calls
// it, and then stops watching the folders.
func (c *Catalog) MarkDeleting(ctx context.Context, tx *store.Tx, lib id.ID) error {
	q := dbq.New(tx)
	now := sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true}
	n, err := q.MarkLibraryDeleting(ctx, dbq.MarkLibraryDeletingParams{ID: lib, Now: now})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: library %s", ErrNotFound, lib)
	}
	c.tell(tx, []id.ID{lib}) // the library's root works leave with it
	return q.MarkLibraryFoldersRemoving(ctx, dbq.MarkLibraryFoldersRemovingParams{LibraryID: lib, Now: now})
}

// removeMarkedFolder removes one folder that is marked as being removed.
func (c *Catalog) removeMarkedFolder(ctx context.Context, f dbq.GetFolderRow) error {
	if !f.RemovingAt.Valid {
		return nil // not being removed, so there's nothing to do
	}
	total, err := c.q.CountFolderFiles(ctx, f.ID)
	if err != nil {
		return err
	}
	done := 0
	progress := func(n int) {
		done += n
		tasks.Progress(ctx, "", done, int(total))
	}
	if err := c.removeFolder(ctx, f.ID, progress); err != nil {
		return err
	}
	tasks.Summarize(ctx, fmt.Sprintf("Removed %s and its %d files", f.Path, done))
	return nil
}

// deleteLibrary removes every folder of a library being deleted, then the
// library.
func (c *Catalog) deleteLibrary(ctx context.Context, lib dbq.Library) error {
	if !lib.DeletingAt.Valid {
		return nil // not being deleted, so there's nothing to do
	}
	folders, err := c.q.AllFolders(ctx, lib.ID)
	if err != nil {
		return err
	}
	var total int64
	for _, r := range folders {
		n, err := c.q.CountFolderFiles(ctx, r.ID)
		if err != nil {
			return err
		}
		total += n
	}
	done := 0
	progress := func(n int) {
		done += n
		tasks.Progress(ctx, "", done, int(total))
	}
	for _, r := range folders {
		if err := c.removeFolder(ctx, r.ID, progress); err != nil {
			return err
		}
	}
	err = c.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		n, err := dbq.New(tx).DeleteDeletedLibrary(ctx, lib.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("a folder was added meanwhile")
		}
		return nil
	})
	if err != nil {
		return err
	}
	tasks.Summarize(ctx, fmt.Sprintf("Deleted %s and its %d files", lib.Name, done))
	return nil
}

// removeFolder deletes a folder's files a batch at a time, then deletes the
// folder once none is left. Each batch commits on its own and is safe to
// repeat, so an interrupted removal resumes where it stopped. progress is
// told how many files each batch deleted.
func (c *Catalog) removeFolder(ctx context.Context, folder id.ID, progress func(n int)) error {
	for {
		err := c.db.Drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) {
			q := dbq.New(tx)
			files, err := q.RemovingFolderFiles(ctx, dbq.RemovingFolderFilesParams{FolderID: folder, Limit: store.BatchSize})
			if err != nil {
				return 0, err
			}
			if err := c.DeleteFiles(ctx, tx, files); err != nil {
				return 0, err
			}
			tx.AfterCommit(func() { progress(len(files)) })
			return len(files), nil
		})
		if err != nil {
			return err
		}
		// Delete the folder only if no file is left. A scan still running
		// may have added one meanwhile.
		var gone bool
		err = c.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			q := dbq.New(tx)
			f, err := q.GetFolder(ctx, folder)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				gone = true // removed meanwhile
				return nil
			case err != nil:
				return err
			case !f.RemovingAt.Valid:
				return fmt.Errorf("folder %s isn't being removed", f.Path)
			}
			n, err := q.DeleteRemovedFolder(ctx, folder)
			gone = n > 0
			return err
		})
		if err != nil || gone {
			return err
		}
	}
}
