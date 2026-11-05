package identify

import (
	"cmp"
	"context"
	"database/sql"
	"errors"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
)

// root returns the work that a unit's folder stands for, its root work,
// and links the folder to it. It picks the first of these that applies:
//
//   - The work the folder is already linked to.
//   - For a folder that was renamed or moved, the work its files are on, if
//     they are all of that work's present files. The work's link and match
//     row then follow the folder.
//   - A new work, named from the file names, and provisional until a match
//     describes it.
//
// Through its link, a root work survives its files being replaced. A root
// work is never found by what its match says. An unreachable folder's
// files count as present, so an outage never gives a root work away.
func (i *Identifier) root(ctx context.Context, tx *store.Tx, s *snapshot, ps placements) (id.ID, error) {
	q := dbq.New(tx)
	u := s.unit
	w, err := q.FolderWork(ctx, dbq.FolderWorkParams{LibraryID: s.lib.ID, Subject: u.subject})
	if !errors.Is(err, sql.ErrNoRows) {
		return w, err
	}
	if a, ok := ps.anchor(); ok {
		moved, err := i.follow(ctx, tx, s, a)
		if err != nil || moved {
			return a, err
		}
	}
	in := catalog.WorkInput{Kind: u.kind, Title: cmp.Or(u.title, stem(u.files()[0].Rel)), Year: u.year, Edition: u.edition}
	switch u.kind {
	case catalog.KindMovie, catalog.KindSeries:
		in.Provisional = true
	case catalog.KindVideo, catalog.KindBook:
		in.Year = 0
	}
	if w, err = i.deps.Catalog.EnsureWork(ctx, tx, in); err != nil {
		return id.ID{}, err
	}
	// TODO: notify webhooks of the item added.
	return w, q.LinkFolder(ctx, dbq.LinkFolderParams{LibraryID: s.lib.ID, Subject: u.subject, WorkID: w})
}

// follow moves a root work's folder link, with its match row, to the
// unit's folder when the unit holds every present file of the root. That
// means the root's folder was renamed or moved. The root's versions with no
// file present, left behind where the folder was, are dropped. A folder
// moved to another library takes the root there. It reports whether it
// followed.
func (i *Identifier) follow(ctx context.Context, tx *store.Tx, s *snapshot, root id.ID) (bool, error) {
	q := dbq.New(tx)
	st, err := q.WorkState(ctx, root)
	if err != nil || catalog.WorkKind(st.Kind) != s.unit.kind {
		return false, err
	}
	elsewhere, err := q.PresentElsewhere(ctx, dbq.PresentElsewhereParams{RootID: root, Files: id.Set(s.unit.fileIDs()...)})
	if err != nil || elsewhere != 0 {
		return false, err
	}
	gone, err := q.GoneRootFiles(ctx, root)
	if err != nil {
		return false, err
	}
	if err := i.deps.Catalog.DeleteFiles(ctx, tx, gone); err != nil {
		return false, err
	}
	if st.LibraryID != s.lib.ID {
		if err := i.deps.Catalog.MoveRoot(ctx, tx, root, s.lib.ID); err != nil {
			return false, err
		}
	}
	old, err := q.FolderOf(ctx, root)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return false, err
	default:
		err := q.MoveMatch(ctx, dbq.MoveMatchParams{OldLibraryID: old.LibraryID, OldSubject: old.Subject, LibraryID: s.lib.ID, Subject: s.unit.subject})
		if err != nil {
			return false, err
		}
		if err := q.UnlinkWork(ctx, root); err != nil {
			return false, err
		}
	}
	return true, q.LinkFolder(ctx, dbq.LinkFolderParams{LibraryID: s.lib.ID, Subject: s.unit.subject, WorkID: root})
}
