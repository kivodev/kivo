package query

import (
	"context"
	"database/sql"
	"path"
	"path/filepath"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// unmatchedWorks reads a page of the Unmatched list: the unmatched works of
// every library, as the libraries' grids hold them, library by library and
// then by title. Each comes with the first file, by path, of the work or
// its descendants in that library.
var unmatchedWorks = stmt("UnmatchedWorks", `
WITH page AS (
  SELECT w.id AS id, wl.library_id AS lib, wl.sort_title AS t
  FROM work_library wl
  JOIN library l ON l.id = wl.library_id
  JOIN work w ON w.id = wl.work_id
  WHERE wl.is_root AND w.provisional AND l.deleting_at IS NULL
    AND (wl.library_id, wl.sort_title, wl.work_id) > (@after_library, @after_title, @after_id)
    AND `+libraryVisible+`
    AND `+visible+`
  ORDER BY wl.library_id, wl.sort_title, wl.work_id
  LIMIT @limit
)
SELECT p.id, p.lib, p.t, coalesce(r.path, ''), coalesce(f.rel_path, '')
FROM page p
LEFT JOIN media_file f ON f.id = (
  -- CROSS JOIN reads the work's tree first, not the library's files.
  SELECT df.id FROM work d
  CROSS JOIN media_version_work dv ON dv.work_id = d.id
  CROSS JOIN media_file df ON df.version_id = dv.version_id
  CROSS JOIN library_folder dr ON dr.id = df.folder_id
  WHERE d.root_id = p.id AND dr.library_id = p.lib
  ORDER BY dr.path, df.rel_path LIMIT 1)
LEFT JOIN library_folder r ON r.id = f.folder_id
ORDER BY p.lib, p.t, p.id`)

// Unmatched pages the Unmatched list: the unmatched works of every library
// the Viewer sees, library by library and then by title. Each summary
// carries its Path. The page holds at most limit works, or the default
// number when limit is 0. It has no total.
func (s *Service) Unmatched(ctx context.Context, v users.Viewer, after Cursor, limit int) (Page[WorkSummary], error) {
	vw := viewerOf(v)
	const list = "unmatched"
	limit, err := pageLimit(limit)
	if err != nil {
		return Page[WorkSummary]{}, err
	}
	pos, _, err := after.decode(list, 0)
	if err != nil {
		return Page[WorkSummary]{}, err
	}
	var items []WorkSummary
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		type row struct {
			work, library id.ID
			sortTitle     string
			folder, rel   string
		}
		rows, err := rowsOf(ctx, tx, unmatchedWorks, vw.args(sql.Named("after_library", pos.Library[:]),
			sql.Named("after_title", pos.Title), sql.Named("after_id", pos.ID[:]), sql.Named("limit", limit+1)),
			func(r scanner, x *row) error {
				return r.Scan(&x.work, &x.library, &x.sortTitle, &x.folder, &x.rel)
			})
		if err != nil {
			return err
		}
		works := make([]id.ID, len(rows))
		for i, r := range rows {
			works[i] = r.work
		}
		byWork, err := summariesByID(ctx, tx, vw, works)
		if err != nil {
			return err
		}
		for _, r := range rows {
			c := byWork[r.work]
			c.SortTitle, c.library, c.Path = r.sortTitle, r.library, parsedFrom(c.Kind, r.folder, r.rel)
			items = append(items, c)
		}
		return nil
	})
	if err != nil {
		return Page[WorkSummary]{}, err
	}
	g := page(items, limit, 0, func(c WorkSummary) position {
		return position{List: list, Library: c.library, Title: c.SortTitle, ID: c.ID}
	})
	if err := s.decorate(ctx, vw, g.Items); err != nil {
		return Page[WorkSummary]{}, err
	}
	return g, nil
}

// parsedFrom returns what an unmatched work was parsed from. For a series,
// that is its top folder under the library folder. For a book, it is the
// book's folder. Otherwise it is the file itself. It is empty for a work
// with no file.
func parsedFrom(kind catalog.WorkKind, folder, rel string) string {
	if rel == "" {
		return ""
	}
	dir := path.Dir(rel)
	switch {
	case dir == ".":
	case kind == catalog.KindSeries:
		rel, _, _ = strings.Cut(rel, "/")
	case kind == catalog.KindBook:
		rel = dir
	}
	return filepath.Join(folder, rel)
}
