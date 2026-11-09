package users

import (
	"context"
	"database/sql"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users/dbq"
)

// HomeRow is a row of a user's Home layout and whether it shows.
type HomeRow struct {
	ID    string
	Shown bool
}

// SetHomeRows replaces v's user's Home layout with rows, in order. It
// arranges the Home: every row gets a position.
func (s *Service) SetHomeRows(ctx context.Context, v Viewer, rows []HomeRow) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := q.DeleteHomeRows(ctx, v.user); err != nil {
			return err
		}
		for i, r := range rows {
			err := q.InsertHomeRow(ctx, dbq.InsertHomeRowParams{
				UserID: v.user, RowID: r.ID, Position: sql.NullInt64{Int64: int64(i), Valid: true}, Shown: r.Shown,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.publishHomeChanged(v)
	return nil
}

// ResetHomeRows forgets v's user's Home layout.
func (s *Service) ResetHomeRows(ctx context.Context, v Viewer) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).DeleteHomeRows(ctx, v.user)
	})
	if err != nil {
		return err
	}
	s.publishHomeChanged(v)
	return nil
}

// publishHomeChanged tells the user's clients their Home layout may have
// changed.
func (s *Service) publishHomeChanged(v Viewer) {
	s.bus.Publish(events.Event{Kind: events.HomeChanged, Scope: events.Scope{User: v.user}})
}
