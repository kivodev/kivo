package users

import (
	"context"
	"fmt"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users/dbq"
)

// LibraryChoice is one library in a user's own order, and whether it's
// pinned. A pinned library is shown in the sidebar.
type LibraryChoice struct {
	Library id.ID
	Pinned  bool
}

// setLibraries replaces the order of the libraries v's user sees, and which
// of them are pinned. A library left out is pinned after the listed ones,
// A to Z by name. Every library is treated that way until the user
// first sets the order. A library v doesn't see, or one listed twice,
// returns ErrUnknownLibrary.
func setLibraries(ctx context.Context, q *dbq.Queries, v Viewer, libs []LibraryChoice) error {
	seen := map[id.ID]bool{}
	for _, l := range libs {
		if !v.CanSee(l.Library) || seen[l.Library] {
			return fmt.Errorf("%w: %v", ErrUnknownLibrary, l.Library)
		}
		seen[l.Library] = true
	}
	if err := q.DeleteLibraryOrder(ctx, v.user); err != nil {
		return err
	}
	for i, l := range libs {
		err := q.InsertLibraryOrder(ctx, dbq.InsertLibraryOrderParams{
			UserID: v.user, LibraryID: l.Library, Position: int64(i), Pinned: l.Pinned,
		})
		if store.IsForeignKey(err) {
			return fmt.Errorf("%w: %v", ErrUnknownLibrary, l.Library)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
