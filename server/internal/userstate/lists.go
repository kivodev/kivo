package userstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate/dbq"
)

// AddToList puts a work the Viewer can see in one of the Viewer's user's
// lists, unless it's already there.
func (s *Service) AddToList(ctx context.Context, v users.Viewer, list, work id.ID) error {
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return err
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := ownsList(ctx, q, v, list); err != nil {
			return err
		}
		return q.AddToList(ctx, dbq.AddToListParams{ListID: list, WorkID: work, At: time.Now().UnixMilli()})
	})
	if err != nil {
		return err
	}
	s.publishListsChanged(v)
	return nil
}

// RemoveFromList takes a work out of one of the Viewer's user's lists.
func (s *Service) RemoveFromList(ctx context.Context, v users.Viewer, list, work id.ID) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := ownsList(ctx, q, v, list); err != nil {
			return err
		}
		return q.RemoveFromList(ctx, dbq.RemoveFromListParams{ListID: list, WorkID: work})
	})
	if err != nil {
		return err
	}
	s.publishListsChanged(v)
	return nil
}

func ownsList(ctx context.Context, q *dbq.Queries, v users.Viewer, list id.ID) error {
	l, err := q.ListOwner(ctx, list)
	if errors.Is(err, sql.ErrNoRows) || err == nil && l.UserID != v.User() {
		return fmt.Errorf("%w: %s", users.ErrListNotFound, list)
	}
	if err == nil && l.Smart {
		return users.ErrListKind
	}
	return err
}

// publishListsChanged tells the user's clients their lists changed.
func (s *Service) publishListsChanged(v users.Viewer) {
	s.deps.Events.Publish(events.Event{Kind: events.ListsChanged, Scope: events.Scope{User: v.User()}})
}
