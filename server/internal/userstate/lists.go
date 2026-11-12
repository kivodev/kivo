package userstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate/dbq"
)

// AddToList puts a work the Viewer can see in one of the Viewer's user's
// lists, first in its custom order, unless it's already there.
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

// ErrListAnchor means the work to move others after isn't in the list, or
// is one of the works to move.
var ErrListAnchor = fault.New(fault.Invalid, "no such work to move after")

// MoveInList moves works, in the order given, to just after the work after
// in the custom order of one of the Viewer's user's lists, or to its top
// when after is zero, and sets the list to show in that order. A work that
// isn't in the list is left out, so a move repeated changes nothing.
func (s *Service) MoveInList(ctx context.Context, v users.Viewer, list id.ID, works []id.ID, after id.ID) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := ownsList(ctx, q, v, list); err != nil {
			return err
		}
		rows, err := q.ListPlaces(ctx, list)
		if err != nil {
			return err
		}
		order, err := moved(rows, works, after)
		if err != nil {
			return err
		}
		for i, r := range order {
			if r.Position == int64(i) {
				continue
			}
			if err := q.SetListPlace(ctx, dbq.SetListPlaceParams{ListID: list, WorkID: r.WorkID, Position: int64(i)}); err != nil {
				return err
			}
		}
		return users.SetListSort(ctx, tx, list, users.ListCustom)
	})
	if err != nil {
		return err
	}
	s.publishListsChanged(v)
	return nil
}

// moved is rows, in order, with works, those of them in rows, moved to
// follow after, or to lead when after is zero.
func moved(rows []dbq.ListPlacesRow, works []id.ID, after id.ID) ([]dbq.ListPlacesRow, error) {
	if slices.Contains(works, after) {
		return nil, ErrListAnchor
	}
	var picked, rest []dbq.ListPlacesRow
	for _, w := range works {
		i := slices.IndexFunc(rows, func(r dbq.ListPlacesRow) bool { return r.WorkID == w })
		if i >= 0 && !slices.ContainsFunc(picked, func(r dbq.ListPlacesRow) bool { return r.WorkID == w }) {
			picked = append(picked, rows[i])
		}
	}
	for _, r := range rows {
		if !slices.Contains(works, r.WorkID) {
			rest = append(rest, r)
		}
	}
	at := 0 // how many of rest come before the moved works
	if !after.IsZero() {
		at = slices.IndexFunc(rest, func(r dbq.ListPlacesRow) bool { return r.WorkID == after }) + 1
		if at == 0 {
			return nil, ErrListAnchor
		}
	}
	return slices.Concat(rest[:at], picked, rest[at:]), nil
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
