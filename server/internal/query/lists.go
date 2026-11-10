package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// List is a list the user keeps. Works counts the works the Viewer sees.
type List struct {
	ID      id.ID
	Name    string
	BuiltIn bool
	Works   int
	// Smart is a smart list's rules, nil for a manual list.
	Smart *SmartList
	// Covers are the posters of the list's first works, up to three, in
	// its order; a work without a poster gives none. Only CountedLists sets
	// them.
	Covers []Image
}

// listCovers is how many of a list's first works give it covers.
const listCovers = 3

// SmartList is a smart list's rules, and why they can't run if they can't.
type SmartList struct {
	Query  WorkQuery
	Errors []QueryProblem
}

// listWorksPage reads a list's works from @offset, most recently added
// first; works added at the same time are listed by title.
var listWorksPage = stmt("ListWorks", `
SELECT `+summaryColumns+`
FROM list_work lw
JOIN work w ON w.id = lw.work_id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE lw.list_id = @list
  AND (@kinds IS NULL OR w.kind IN (SELECT value FROM json_each(@kinds)))
  AND `+visible+`
ORDER BY lw.added_at DESC, w.sort_title, w.id
LIMIT @limit OFFSET @offset`)

var listWorksCount = stmt("ListWorksCount", `
SELECT count(*)
FROM list_work lw
JOIN work w ON w.id = lw.work_id
WHERE lw.list_id = @list
  AND (@kinds IS NULL OR w.kind IN (SELECT value FROM json_each(@kinds)))
  AND `+visible)

// listSelect reads the user's lists with how many of their works the
// Viewer sees.
var listSelect = `
SELECT l.id, l.name, l.built_in,
  (SELECT count(*) FROM list_work lw JOIN work w ON w.id = lw.work_id WHERE lw.list_id = l.id AND ` + visible + `),
  l.query
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
	if err := r.Scan(&l.ID, &l.Name, &l.BuiltIn, &l.Works, &query); err != nil {
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
// each list's covers, for the screen that shows them.
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
	if err := s.withImages(ctx, vw, firsts...); err != nil {
		return nil, err
	}
	for i, works := range firsts {
		out[i].Covers = []Image{}
		for _, w := range works {
			if w.Images.Poster != nil {
				out[i].Covers = append(out[i].Covers, *w.Images.Poster)
			}
		}
	}
	return out, nil
}

// firstWorks reads a list's first works as its page lists them, and counts
// a smart list's works. A smart list whose rules can't run has none.
func firstWorks(ctx context.Context, tx *sql.Tx, vw viewer, l *List, now time.Time) ([]WorkSummary, error) {
	if l.Smart == nil {
		return summaries(ctx, tx, listWorksPage, vw.args(sql.Named("list", l.ID), sql.Named("kinds", nil),
			sql.Named("offset", 0), sql.Named("limit", listCovers)))
	}
	if len(l.Smart.Errors) > 0 {
		return nil, nil
	}
	c := compile(l.Smart.Query, vw, now, listSeed(l.ID, now), nil)
	if err := rowOf(ctx, tx, statement{sql: c.count}, c.args, &l.Works); err != nil {
		return nil, err
	}
	return summaries(ctx, tx, statement{sql: c.page}, append(slices.Clone(c.args), sql.Named("limit", listCovers), sql.Named("offset", 0)))
}

// countSmart sets a smart list's Works to how many works its rules select.
func (s *Service) countSmart(ctx context.Context, vw viewer, l *List) error {
	if l.Smart == nil || len(l.Smart.Errors) > 0 {
		return nil
	}
	now := time.Now()
	c := compile(l.Smart.Query, vw, now, listSeed(l.ID, now), nil)
	return s.db.Read(ctx, func(tx *sql.Tx) error {
		return rowOf(ctx, tx, statement{sql: c.count}, c.args, &l.Works)
	})
}

// List returns one of the user's lists, or ErrNotFound.
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
	return out, s.countSmart(ctx, vw, &out)
}

// ListWorks returns a page of a list's works: a manual list's most recently
// added first, a smart list's as its rules select them. It lists those of
// kinds, or every kind when kinds is empty.
func (s *Service) ListWorks(ctx context.Context, v users.Viewer, list id.ID, kinds []catalog.WorkKind, o GridOptions) (GridPage[WorkSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	vw := viewerOf(v)
	var saved sql.NullString
	var items []WorkSummary
	var total int
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := rowOf(ctx, tx, listOwner, vw.args(sql.Named("list", list)), &saved); err != nil {
			return notFound(err)
		}
		if saved.Valid {
			return nil
		}
		if err := rowOf(ctx, tx, listWorksCount, vw.args(sql.Named("list", list), sql.Named("kinds", set(kinds))), &total); err != nil {
			return err
		}
		var err error
		items, err = summaries(ctx, tx, listWorksPage, vw.args(sql.Named("list", list), sql.Named("kinds", set(kinds)),
			sql.Named("offset", o.Offset), sql.Named("limit", limit)))
		return err
	})
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	if saved.Valid {
		return s.smartWorks(ctx, vw, list, parseSmart(saved.String), kinds, o)
	}
	return s.summaryPage(ctx, vw, items, total, nil)
}

// smartWorks runs a smart list's rules, keeping only kinds from the set
// the rules cap. A list whose rules can't run is empty.
func (s *Service) smartWorks(ctx context.Context, vw viewer, list id.ID, smart *SmartList, kinds []catalog.WorkKind, o GridOptions) (GridPage[WorkSummary], error) {
	if len(smart.Errors) > 0 {
		return s.summaryPage(ctx, vw, nil, 0, nil)
	}
	now := time.Now()
	return s.runQuery(ctx, vw, smart.Query, listSeed(list, now), kinds, o, now)
}

// listOwner reads a list of the user's rules, NULL for a manual list; a list
// that isn't the user's has no row.
var listOwner = stmt("ListOwner", `
-- viewer-filter: user
SELECT l.query FROM list l WHERE l.id = @list AND l.user_id = @user`)
