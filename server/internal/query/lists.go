package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// List is a list the user keeps. Works counts the works the Viewer sees;
// a smart list's is set only by CountedLists.
type List struct {
	ID      id.ID
	Name    string
	BuiltIn bool
	Works   int
	// Sort is the order a manual list is shown in.
	Sort users.ListSort
	// Smart is a smart list's rules, nil for a manual list.
	Smart *SmartList
	// Items are the list's first works, up to a Home row's, in its order.
	// Only CountedLists sets them.
	Items []WorkSummary
}

// SmartList is a smart list's rules, and why they can't run if they can't.
type SmartList struct {
	Query  WorkQuery
	Errors []QueryProblem
}

// listOrders is the sort key of each order a manual list is shown in.
var listOrders = map[users.ListSort]string{
	users.ListCustom:      sortListPosition,
	users.ListTitle:       "title",
	users.ListReleaseDate: "releaseDate",
	users.ListRating:      "rating",
	users.ListDateAdded:   sortListAddedAt,
}

// reads is how a list's works are read: the query to run, and where. A
// manual list's selects the works in it, of any kind, in the list's order;
// a smart list's is its rules. ok is false for a smart list whose rules
// can't run, which has no works.
func (l List) reads() (q WorkQuery, rd reading, ok bool) {
	if l.Smart == nil {
		return WorkQuery{Kinds: listKinds, Sort: []SortKey{{Field: listOrders[l.Sort]}}}, reading{list: l.ID}, true
	}
	return l.Smart.Query, reading{}, len(l.Smart.Errors) == 0
}

// listSelect reads the user's lists with how many of their works the
// Viewer sees.
var listSelect = `
SELECT l.id, l.name, l.built_in,
  (SELECT count(*) FROM list_work lw JOIN work w ON w.id = lw.work_id WHERE lw.list_id = l.id AND ` + visible + `),
  l.query, l.sort
FROM list l
WHERE l.user_id = @user`

var listsOfUser = stmt("Lists", listSelect+`
ORDER BY l.built_in DESC, l.name COLLATE NOCASE, l.id`)

var listOne = stmt("List", listSelect+`
  AND l.id = @list`)

// workLists reads which of the user's lists hold a work.
var workLists = stmt("WorkLists", `
-- viewer-filter: user
SELECT lw.list_id
FROM list_work lw
JOIN list l ON l.id = lw.list_id
WHERE l.user_id = @user AND lw.work_id = @work
ORDER BY l.built_in DESC, l.name COLLATE NOCASE, l.id`)

func scanList(r scanner, l *List) error {
	var query sql.NullString
	if err := r.Scan(&l.ID, &l.Name, &l.BuiltIn, &l.Works, &query, &l.Sort); err != nil {
		return err
	}
	if query.Valid {
		l.Smart = parseSmart(query.String)
	}
	return nil
}

// parseSmart reads a stored query. Rules that no longer parse or validate
// are kept, with their errors, so the list stays editable and runs empty.
func parseSmart(stored string) *SmartList {
	var q WorkQuery
	if err := json.Unmarshal([]byte(stored), &q); err != nil {
		return &SmartList{Errors: []QueryProblem{{Pointer: "", Code: "invalid"}}}
	}
	return &SmartList{Query: q, Errors: q.Validate()}
}

// LogBrokenSmartLists logs a Warn for each saved query that no longer
// compiles, with its list and problem codes and never its rules. The server
// runs it once as it starts.
func LogBrokenSmartLists(ctx context.Context, saved []users.SavedQuery) {
	for _, q := range saved {
		if problems := parseSmart(q.Query).Errors; len(problems) > 0 {
			codes := make([]string, len(problems))
			for i, p := range problems {
				codes[i] = p.Code
			}
			slog.WarnContext(ctx, "lists: a smart list's rules no longer compile", "list", q.List, "problems", codes)
		}
	}
}

// Lists returns the user's lists, My List first. A smart list's Works is 0:
// counting it runs its rules, so only CountedLists does.
func (s *Service) Lists(ctx context.Context, v users.Viewer) ([]List, error) {
	var out []List
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = rowsOf(ctx, tx, listsOfUser, viewerOf(v).args(), scanList)
		return err
	})
	return out, err
}

// CountedLists returns the user's lists with each smart list's count and
// each list's first works, for the screen that shows them.
func (s *Service) CountedLists(ctx context.Context, v users.Viewer) ([]List, error) {
	out, err := s.Lists(ctx, v)
	if err != nil {
		return nil, err
	}
	vw := viewerOf(v)
	now := time.Now()
	firsts := make([][]WorkSummary, len(out))
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		for i := range out {
			var err error
			if firsts[i], err = firstWorks(ctx, tx, vw, &out[i], now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.decorate(ctx, vw, firsts...); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Items = append([]WorkSummary{}, firsts[i]...)
	}
	return out, nil
}

// firstWorks reads a list's first works as its page lists them, and counts
// a smart list's works with them. A smart list whose rules can't run has
// none.
func firstWorks(ctx context.Context, tx *sql.Tx, vw viewer, l *List, now time.Time) ([]WorkSummary, error) {
	q, rd, ok := l.reads()
	if !ok {
		return nil, nil
	}
	c := compile(q, vw, now, listSeed(l.ID, now), nil, rd)
	args := append(c.args, sql.Named("limit", rowLimit), sql.Named("offset", 0))
	if l.Smart == nil {
		return summaries(ctx, tx, statement{sql: c.page}, args)
	}
	works, total, _, err := countedSummaries(ctx, tx, statement{sql: c.counted}, args, false)
	l.Works = total
	return works, err
}

// List returns one of the user's lists, or ErrNotFound. A smart list's
// Works is 0, as in Lists: the first page of its works has the count.
func (s *Service) List(ctx context.Context, v users.Viewer, list id.ID) (List, error) {
	var out List
	vw := viewerOf(v)
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		return scanList(queryRow(ctx, tx, listOne, vw.args(sql.Named("list", list))), &out)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return List{}, ErrNotFound
	}
	if err != nil {
		return List{}, err
	}
	return out, nil
}

// ListWorks returns a page of a list's works: a manual list's in the order
// it is shown in, a smart list's as its rules select them. It lists those
// of kinds, or every kind when kinds is empty. The first page, at offset 0,
// has the Total; a later page's is 0.
func (s *Service) ListWorks(ctx context.Context, v users.Viewer, list id.ID, kinds []catalog.WorkKind, o GridOptions) (GridPage[WorkSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	vw := viewerOf(v)
	var items []WorkSummary
	var total int
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		l := List{ID: list}
		var saved sql.NullString
		if err := rowOf(ctx, tx, listOwner, vw.args(sql.Named("list", list)), &saved, &l.Sort); err != nil {
			return notFound(err)
		}
		if saved.Valid {
			l.Smart = parseSmart(saved.String)
		}
		q, rd, ok := l.reads()
		if !ok {
			return nil
		}
		now := time.Now()
		items, total, _, err = readQuery(ctx, tx, vw, q, listSeed(list, now), kinds, rd, o.Offset, limit, now)
		return err
	})
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return s.summaryPage(ctx, vw, items, total, nil)
}

// listOwner reads a list of the user's: its rules, NULL for a manual list,
// and its sort. A list that isn't the user's has no row.
var listOwner = stmt("ListOwner", `
-- viewer-filter: user
SELECT l.query, l.sort FROM list l WHERE l.id = @list AND l.user_id = @user`)
