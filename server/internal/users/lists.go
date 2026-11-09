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
	// ErrListKind means a smart list's rules were set on a list that isn't
	// one; a list's kind is fixed when it's made.
	ErrListKind = fault.New(fault.Conflict, "list kind is fixed")
	// ErrListLimit means the Viewer has MaxLists lists already.
	ErrListLimit = fault.New(fault.Conflict, "too many lists")
)

// myListName is the name My List has, on the server and in the clients.
const myListName = "Watchlist"

// insertMyList gives a new user My List, in the transaction that creates
// the user.
func insertMyList(ctx context.Context, q *dbq.Queries, user id.ID, at time.Time) error {
	return q.InsertList(ctx, dbq.InsertListParams{ID: id.New(), UserID: user, Name: myListName, BuiltIn: true, CreatedAt: at.UnixMilli()})
}

// CreateList creates a list for v's user and returns its ID: a manual
// list, or with query (a WorkQuery as JSON) a smart list.
func (s *Service) CreateList(ctx context.Context, v Viewer, name string, query *string) (id.ID, error) {
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
		err = q.InsertList(ctx, dbq.InsertListParams{ID: list, UserID: v.user, Name: name, Query: nullableString(query), CreatedAt: time.Now().UnixMilli()})
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

// UpdateList renames one of v's user's lists, when name is set, and
// replaces a smart list's rules with query, a WorkQuery as JSON, when query
// is set, in one transaction. My List can't be changed and a manual list
// has no rules.
func (s *Service) UpdateList(ctx context.Context, v Viewer, list id.ID, name, query *string) error {
	if name != nil {
		if err := checkListName(*name); err != nil {
			return err
		}
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		l, err := changeableList(ctx, q, v, list)
		if err != nil {
			return err
		}
		if query != nil && !l.Smart {
			return ErrListKind
		}
		if name != nil {
			err := q.RenameList(ctx, dbq.RenameListParams{ID: list, Name: *name})
			if store.IsUnique(err) {
				return fmt.Errorf("%w: %q", ErrListNameTaken, *name)
			}
			if err != nil {
				return err
			}
		}
		if query != nil {
			return q.SetListQuery(ctx, dbq.SetListQueryParams{ID: list, Query: nullableString(query)})
		}
		return nil
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
		if _, err := changeableList(ctx, q, v, list); err != nil {
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

// MovePersonInQueries points every saved query naming the person from at
// into, in the caller's transaction, when one is merged into the other.
func MovePersonInQueries(ctx context.Context, tx *store.Tx, from, into id.ID) error {
	return dbq.New(tx).MovePersonInListQueries(ctx, dbq.MovePersonInListQueriesParams{From: from.String(), Into: into.String()})
}

// nullableString is s for a nullable column, NULL when s is nil.
func nullableString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// changeableList checks that list is the user's and isn't My List.
func changeableList(ctx context.Context, q *dbq.Queries, v Viewer, list id.ID) (dbq.GetListRow, error) {
	l, err := q.GetList(ctx, dbq.GetListParams{ID: list, UserID: v.user})
	if errors.Is(err, sql.ErrNoRows) {
		return l, fmt.Errorf("%w: %s", ErrListNotFound, list)
	}
	if err != nil {
		return l, err
	}
	if l.BuiltIn {
		return l, ErrListBuiltIn
	}
	return l, nil
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

// SavedQuery is a smart list's stored query.
type SavedQuery struct {
	List  id.ID
	Query string
}

// SavedListQueries returns every smart list's stored query, for the check
// at startup.
func (s *Service) SavedListQueries(ctx context.Context) ([]SavedQuery, error) {
	rows, err := s.q.SavedListQueries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SavedQuery, len(rows))
	for i, r := range rows {
		out[i] = SavedQuery{List: r.ID, Query: r.Query.String}
	}
	return out, nil
}
