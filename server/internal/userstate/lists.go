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

// listGap is how far apart a list's places are spaced: far enough that a
// work is put between two others 32 times over before any other work has
// to move.
const listGap = 1 << 32

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
		places, err := placesAfter(rows, works, after)
		if err != nil {
			return err
		}
		for _, p := range places {
			if err := q.SetListPlace(ctx, dbq.SetListPlaceParams{ListID: list, WorkID: p.WorkID, Position: p.Position}); err != nil {
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

// placesAfter is the places to write so that works, those of them in rows,
// follow after in rows' order, or lead it when after is zero. They are the
// moved works' alone, between their new neighbours' places, or every
// work's, spaced out again, when no places are left between those two.
func placesAfter(rows []dbq.ListPlacesRow, works []id.ID, after id.ID) ([]dbq.ListPlacesRow, error) {
	if slices.Contains(works, after) {
		return nil, ErrListAnchor
	}
	var moved, rest []dbq.ListPlacesRow
	for _, w := range works {
		i := slices.IndexFunc(rows, func(r dbq.ListPlacesRow) bool { return r.WorkID == w })
		if i >= 0 && !slices.ContainsFunc(moved, func(r dbq.ListPlacesRow) bool { return r.WorkID == w }) {
			moved = append(moved, rows[i])
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
	n := int64(len(moved))
	first, step := int64(listGap), int64(listGap)
	switch {
	case len(rest) == 0:
	case at == 0:
		first = rest[0].Position - n*listGap
	case at == len(rest):
		first = rest[at-1].Position + listGap
	default:
		step = (rest[at].Position - rest[at-1].Position) / (n + 1)
		first = rest[at-1].Position + step
	}
	if step > 0 {
		for i := range moved {
			moved[i].Position = first + int64(i)*step
		}
		return moved, nil
	}
	all := slices.Concat(rest[:at], moved, rest[at:])
	for i := range all {
		all[i].Position = int64(i+1) * listGap
	}
	return all, nil
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
