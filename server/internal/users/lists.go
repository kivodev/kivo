package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users/dbq"
)

// MaxLists is how many lists a user can have, My List included.
const MaxLists = 100

var (
	// ErrListNotFound means there is no such list of the Viewer's.
	ErrListNotFound = fault.New(fault.NotFound, "list not found")
	// ErrListNameTaken means the Viewer already has a list of that name.
	ErrListNameTaken = fault.Coded(fault.Conflict, "nameTaken", "list name taken")
	// ErrListBuiltIn means the change would remove or rename My List.
	ErrListBuiltIn = fault.New(fault.Conflict, "my list can't be changed")
	// ErrListLimit means the Viewer has MaxLists lists already.
	ErrListLimit = fault.New(fault.Conflict, "too many lists")
)

// myListName is the name My List has, on the server and in the clients.
const myListName = "My List"

// insertMyList gives a new user My List, in the transaction that creates
// the user.
func insertMyList(ctx context.Context, q *dbq.Queries, user id.ID, at time.Time) error {
	return q.InsertList(ctx, dbq.InsertListParams{ID: id.New(), UserID: user, Name: myListName, BuiltIn: true, CreatedAt: at.UnixMilli()})
}

// CreateList creates a list for v's user and returns its ID.
func (s *Service) CreateList(ctx context.Context, v Viewer, name string) (id.ID, error) {
	if err := checkListName(name); err != nil {
		return id.ID{}, err
	}
	list := id.New()
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		n, err := q.CountLists(ctx, v.user)
		if err != nil {
			return err
		}
		if n >= MaxLists {
			return ErrListLimit
		}
		err = q.InsertList(ctx, dbq.InsertListParams{ID: list, UserID: v.user, Name: name, CreatedAt: time.Now().UnixMilli()})
		if store.IsUnique(err) {
			return fmt.Errorf("%w: %q", ErrListNameTaken, name)
		}
		return err
	})
	if err != nil {
		return id.ID{}, err
	}
	s.publishListsChanged(v)
	return list, nil
}

// RenameList renames one of v's user's lists. My List keeps its name.
func (s *Service) RenameList(ctx context.Context, v Viewer, list id.ID, name string) error {
	if err := checkListName(name); err != nil {
		return err
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := changeableList(ctx, q, v, list); err != nil {
			return err
		}
		err := q.RenameList(ctx, dbq.RenameListParams{ID: list, Name: name})
		if store.IsUnique(err) {
			return fmt.Errorf("%w: %q", ErrListNameTaken, name)
		}
		return err
	})
	if err != nil {
		return err
	}
	s.publishListsChanged(v)
	return nil
}

// DeleteList deletes one of v's user's lists, except My List.
func (s *Service) DeleteList(ctx context.Context, v Viewer, list id.ID) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := changeableList(ctx, q, v, list); err != nil {
			return err
		}
		return q.DeleteList(ctx, list)
	})
	if err != nil {
		return err
	}
	s.publishListsChanged(v)
	return nil
}

// changeableList checks that list is the user's and isn't My List.
func changeableList(ctx context.Context, q *dbq.Queries, v Viewer, list id.ID) error {
	l, err := q.GetList(ctx, dbq.GetListParams{ID: list, UserID: v.user})
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrListNotFound, list)
	}
	if err != nil {
		return err
	}
	if l.BuiltIn {
		return ErrListBuiltIn
	}
	return nil
}

// checkListName refuses a list name that is empty, too long, padded with
// spaces or contains control characters.
func checkListName(name string) error {
	if err := checkName(name); err != nil {
		return fmt.Errorf("list name: %w", err)
	}
	return nil
}

// publishListsChanged tells the user's clients their lists changed.
func (s *Service) publishListsChanged(v Viewer) {
	s.bus.Publish(events.Event{Kind: events.ListsChanged, Scope: events.Scope{User: v.user}})
}
