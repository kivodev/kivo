package userstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate/dbq"
)

// MaxLists is how many lists a user can have, My List included.
const MaxLists = 100

// maxListNameLen is the most runes a list name may have.
const maxListNameLen = 64

var (
	// ErrListNotFound means there is no such list of the Viewer's.
	ErrListNotFound = fault.New(fault.NotFound, "list not found")
	// ErrListNameTaken means the Viewer already has a list of that name.
	ErrListNameTaken = fault.Coded(fault.Conflict, "nameTaken", "list name taken")
	// ErrListBuiltIn means the change would remove or rename My List.
	ErrListBuiltIn = fault.New(fault.Conflict, "my list can't be changed")
	// ErrListKind means a smart list's rules were set on a list that isn't
	// one, or a manual-list operation targeted a smart list; a list's kind
	// is fixed when it's made.
	ErrListKind = fault.New(fault.Conflict, "list kind is fixed")
	// ErrListLimit means the Viewer has MaxLists lists already.
	ErrListLimit = fault.New(fault.Conflict, "too many lists")
	// ErrListSort means a sort isn't one a list can be shown in.
	ErrListSort = fault.New(fault.Invalid, "unknown list sort")
	// ErrListAnchor means the work to move others after isn't in the list, or
	// is one of the works to move.
	ErrListAnchor = fault.New(fault.Invalid, "no such work to move after")
)

// ListSort is the order a manual list is shown in.
type ListSort string

// The sorts of a manual list. ListCustom is the user's own order, in which
// a work added goes first; the rest order the works by one of their fields.
const (
	ListCustom      ListSort = "custom"
	ListTitle       ListSort = "title"
	ListReleaseDate ListSort = "releaseDate"
	ListRating      ListSort = "rating"
	ListDateAdded   ListSort = "dateAdded"
)

// Valid reports whether s is one of the sorts.
func (s ListSort) Valid() bool {
	switch s {
	case ListCustom, ListTitle, ListReleaseDate, ListRating, ListDateAdded:
		return true
	}
	return false
}

// myListName is the name My List has, on the server and in the clients.
const myListName = "Watchlist"

// CreateMyList gives a new user My List, in the transaction that creates
// the user.
func CreateMyList(ctx context.Context, tx *store.Tx, user id.ID, at time.Time) error {
	return dbq.New(tx).InsertList(ctx, dbq.InsertListParams{
		ID: id.New(), UserID: user, Name: myListName, BuiltIn: true, CreatedAt: at.UnixMilli(),
	})
}

// CreateList creates a list for v's user and returns its ID: a manual
// list, or with query (a WorkQuery as JSON) a smart list.
func (s *Service) CreateList(ctx context.Context, v users.Viewer, name string, query *string) (id.ID, error) {
	if err := checkListName(name); err != nil {
		return id.ID{}, err
	}
	list := id.New()
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		n, err := q.CountLists(ctx, v.User())
		if err != nil {
			return err
		}
		if n >= MaxLists {
			return ErrListLimit
		}
		err = q.InsertList(ctx, dbq.InsertListParams{
			ID: list, UserID: v.User(), Name: name, Query: nullableString(query), CreatedAt: time.Now().UnixMilli(),
		})
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

// UpdateList renames one of v's user's lists, when name is set, replaces a
// smart list's rules with query, a WorkQuery as JSON, when query is set,
// and sets the order a manual list is shown in, when sort is set, in one
// transaction. My List keeps its name, a manual list has no rules, and a
// smart list has no sort.
func (s *Service) UpdateList(ctx context.Context, v users.Viewer, list id.ID, name, query *string, sort *ListSort) error {
	if name != nil {
		if err := checkListName(*name); err != nil {
			return err
		}
	}
	if sort != nil && !sort.Valid() {
		return fmt.Errorf("%w: %q", ErrListSort, *sort)
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		l, err := ownList(ctx, q, v, list)
		switch {
		case err != nil:
			return err
		case l.BuiltIn && (name != nil || query != nil):
			return ErrListBuiltIn
		case query != nil && !l.Smart, sort != nil && l.Smart:
			return ErrListKind
		}
		return writeList(ctx, q, list, name, query, sort)
	})
	if err != nil {
		return err
	}
	s.publishListsChanged(v)
	return nil
}

// writeList writes those of a list's name, rules and sort that are set.
func writeList(ctx context.Context, q *dbq.Queries, list id.ID, name, query *string, sort *ListSort) error {
	if name != nil {
		err := q.RenameList(ctx, dbq.RenameListParams{ID: list, Name: *name})
		if store.IsUnique(err) {
			return fmt.Errorf("%w: %q", ErrListNameTaken, *name)
		}
		if err != nil {
			return err
		}
	}
	if sort != nil {
		if err := q.SetListSort(ctx, dbq.SetListSortParams{ID: list, Sort: string(*sort)}); err != nil {
			return err
		}
	}
	if query != nil {
		return q.SetListQuery(ctx, dbq.SetListQueryParams{ID: list, Query: nullableString(query)})
	}
	return nil
}

// DeleteList deletes one of v's user's lists, except My List.
func (s *Service) DeleteList(ctx context.Context, v users.Viewer, list id.ID) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		l, err := ownList(ctx, q, v, list)
		if err != nil {
			return err
		}
		if l.BuiltIn {
			return ErrListBuiltIn
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

// AddToList puts a work the Viewer can see in one of the Viewer's user's
// lists, first in its custom order, unless it's already there.
func (s *Service) AddToList(ctx context.Context, v users.Viewer, list, work id.ID) error {
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return err
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := ownManualList(ctx, q, v, list); err != nil {
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
		if err := ownManualList(ctx, q, v, list); err != nil {
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

// MoveInList moves works, in the order given, to just after the work after
// in the custom order of one of the Viewer's user's lists, or to its top
// when after is zero, and sets the list to show in that order. A work that
// isn't in the list is left out, so a move repeated changes nothing.
func (s *Service) MoveInList(ctx context.Context, v users.Viewer, list id.ID, works []id.ID, after id.ID) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := ownManualList(ctx, q, v, list); err != nil {
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
		return q.SetListSort(ctx, dbq.SetListSortParams{ID: list, Sort: string(ListCustom)})
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

// ownList reads list, which must be the viewer's user's.
func ownList(ctx context.Context, q *dbq.Queries, v users.Viewer, list id.ID) (dbq.GetListRow, error) {
	l, err := q.GetList(ctx, dbq.GetListParams{ID: list, UserID: v.User()})
	if errors.Is(err, sql.ErrNoRows) {
		return l, fmt.Errorf("%w: %s", ErrListNotFound, list)
	}
	return l, err
}

// ownManualList checks that list belongs to the viewer's user and is a
// manual list.
func ownManualList(ctx context.Context, q *dbq.Queries, v users.Viewer, list id.ID) error {
	l, err := ownList(ctx, q, v, list)
	if err != nil {
		return err
	}
	if l.Smart {
		return ErrListKind
	}
	return nil
}

// nullableString is s for a nullable column, NULL when s is nil.
func nullableString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// checkListName refuses a list name that is empty, too long, padded with
// spaces or contains control characters.
func checkListName(name string) error {
	n := utf8.RuneCountInString(name)
	if n == 0 || n > maxListNameLen || !utf8.ValidString(name) || strings.TrimSpace(name) != name ||
		strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("list name: %w: %q", users.ErrInvalidName, name)
	}
	return nil
}

// publishListsChanged tells the user's clients their lists changed.
func (s *Service) publishListsChanged(v users.Viewer) {
	s.deps.Events.Publish(events.Event{Kind: events.ListsChanged, Scope: events.Scope{User: v.User()}})
}
