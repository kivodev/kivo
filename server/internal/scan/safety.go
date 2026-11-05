package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/scan/dbq"
)

// reachable is the safety check. A folder that can't be read or is empty is
// treated as unmounted, and nothing under it changes. A folder whose mtime
// matches what the last run recorded is the same directory, so it isn't
// listed. An unmounted mount point shows a different, empty directory with
// its own mtime.
func (r *run) reachable() error {
	info, err := fs.Stat(r.fsys, ".")
	if err != nil {
		return err
	}
	if k, ok := r.known["."]; ok && !r.full && k.Mtime == info.ModTime().UnixMilli() {
		return nil
	}
	entries, err := fs.ReadDir(r.fsys, ".")
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errors.New("the folder is empty")
	}
	return nil
}

// unreachable records that folder can't be read, and asks for its
// library's scan again in an hour. Nothing in the folder changes.
func (s *Scanner) unreachable(ctx context.Context, folder catalog.Folder, cause error) error {
	slog.WarnContext(ctx, "scan: folder unreachable; nothing in it changes", "folder", folder.Path, "err", cause)
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return s.setFolderReachable(ctx, tx, folder.ID, false)
	})
	s.setProblem("unreachable:"+folder.ID.String(), health.Problem{
		Component: "scan", Code: "folderUnreachable", Severity: health.Warning, RaisedAt: time.Now(),
		Message: fmt.Sprintf("Can't reach %s (%v). No items were removed.", folder.Path, cause),
	})
	s.askLater(folder.Library, time.Hour)
	return err
}

// setFolderReachable records whether a folder can be read. It returns
// catalog.ErrNotFound for a folder removed from its library.
func (s *Scanner) setFolderReachable(ctx context.Context, tx *store.Tx, folder id.ID, reachable bool) error {
	n, err := dbq.New(tx).SetFolderReachable(ctx, dbq.SetFolderReachableParams{ID: folder, Reachable: reachable})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: folder %s", catalog.ErrNotFound, folder)
	}
	return nil
}
