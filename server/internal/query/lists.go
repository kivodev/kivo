package query

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// List is a list the user keeps. Works counts the works the Viewer sees.
type List struct {
	ID      id.ID
	Name    string
	BuiltIn bool
	Works   int
}

// listWorksPage reads a list's works from @offset, most recently added
// first; works added at the same time are listed by title.
var listWorksPage = stmt("ListWorks", `
SELECT `+summaryColumns+`
FROM list_work lw
JOIN work w ON w.id = lw.work_id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE lw.list_id = @list
  AND `+visible+`
ORDER BY lw.added_at DESC, w.sort_title, w.id
LIMIT @limit OFFSET @offset`)

var listWorksCount = stmt("ListWorksCount", `
SELECT count(*)
FROM list_work lw
JOIN work w ON w.id = lw.work_id
WHERE lw.list_id = @list
  AND `+visible)

// listSelect reads the user's lists with how many of their works the
// Viewer sees.
var listSelect = `
SELECT l.id, l.name, l.built_in,
  (SELECT count(*) FROM list_work lw JOIN work w ON w.id = lw.work_id WHERE lw.list_id = l.id AND ` + visible + `)
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

func scanList(r scanner, l *List) error { return r.Scan(&l.ID, &l.Name, &l.BuiltIn, &l.Works) }

// Lists returns the user's lists, My List first.
func (s *Service) Lists(ctx context.Context, v users.Viewer) ([]List, error) {
	var out []List
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = rowsOf(ctx, tx, listsOfUser, viewerOf(v).args(), scanList)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List returns one of the user's lists, or ErrNotFound.
func (s *Service) List(ctx context.Context, v users.Viewer, list id.ID) (List, error) {
	var out List
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		return rowOf(ctx, tx, listOne, viewerOf(v).args(sql.Named("list", list)), &out.ID, &out.Name, &out.BuiltIn, &out.Works)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return List{}, ErrNotFound
	}
	return out, err
}

// ListWorks returns a page of a list's works, most recently added first.
func (s *Service) ListWorks(ctx context.Context, v users.Viewer, list id.ID, o GridOptions) (GridPage[WorkSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	vw := viewerOf(v)
	var items []WorkSummary
	var total int
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		var owner id.ID
		if err := rowOf(ctx, tx, listOwner, vw.args(sql.Named("list", list)), &owner); err != nil {
			return notFound(err)
		}
		if err := rowOf(ctx, tx, listWorksCount, vw.args(sql.Named("list", list)), &total); err != nil {
			return err
		}
		var err error
		items, err = summaries(ctx, tx, listWorksPage, vw.args(sql.Named("list", list),
			sql.Named("offset", o.Offset), sql.Named("limit", limit)))
		return err
	})
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return s.summaryPage(ctx, vw, items, total, nil)
}

// listOwner confirms a list is the user's.
var listOwner = stmt("ListOwner", `
-- viewer-filter: user
SELECT l.user_id FROM list l WHERE l.id = @list AND l.user_id = @user`)
