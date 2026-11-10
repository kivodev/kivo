package query

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// The library grid as it read before it ran a query: one statement per
// sort, a count, and the letters' counts. It is kept so
// TestBrowseMatchesOld can check Browse lists the same works in the same
// order, counts the same total and starts the letters at the same places.

// gridOld selects a library grid's rows, over work_library wl and work w:
// the library's top-level works that the Viewer sees and the filters keep.
var gridOld = `wl.library_id = @library AND wl.is_root
  AND ` + visible + `
  ` + filtersOld

// filtersOld are a library grid's filters, each in the (@x IS NULL OR ...)
// form, so a filter not asked for matches every row.
var filtersOld = `AND (@played IS NULL OR @played = CASE
    WHEN w.kind = 'series' THEN ` + rollupPlayState("fe.root_id") + `
    ELSE coalesce((SELECT CASE WHEN fu.played THEN 'played' WHEN fu.resumable THEN 'inProgress' END
      FROM user_state fu WHERE fu.user_id = @user AND fu.work_id = w.id), 'unplayed') END)
  AND (@genres IS NULL OR EXISTS (SELECT 1 FROM work_genre fg JOIN genre fgn ON fgn.id = fg.genre_id
    WHERE fg.work_id = w.id AND fgn.name IN (SELECT value FROM json_each(@genres))))
  AND (@year_from IS NULL OR w.year >= @year_from)
  AND (@year_to IS NULL OR w.year <= @year_to)
  AND (@content_ratings IS NULL OR w.content_rating IN (SELECT value FROM json_each(@content_ratings)))
  AND (@resolutions = 0 OR wl.resolutions & @resolutions)
  AND (@dynamic_ranges = 0 OR wl.dynamic_ranges & @dynamic_ranges)
  AND (NOT @unmatched OR w.provisional)
  AND (@narrator IS NULL OR EXISTS (SELECT 1 FROM credit fn
    WHERE fn.work_id = w.id AND fn.role = 'narrator' AND fn.person_id = @narrator))`

// gridSortOld is how the grid ordered one sort, using the sort's index on
// work_library.
//
// key is the sort key, the page's k column. order orders the page's id, k
// and t (the sort title), with ties broken by title and then ID. letters
// counts the grid by letter for the letter rail.
//
// A per-user sort reads in two parts, each in its own order and cut at the
// page's end. It first reads the user's works that have a key, from the
// user's states: from is that part's FROM clause, and mine holds its own
// conditions, placed before the grid's. It then reads the works matching
// rest by title, with no key.
type gridSortOld struct {
	key, order       string
	from, mine, rest string
	letters          bool
}

// playedOld keeps, of the states lu of the works le, those a root's last
// play is taken from: a series' are its own and those of every work under
// it, and any other root's its own alone. The grid took every root's from
// the works under it too, so a film's from its extras: the one place where
// the query means otherwise on purpose, which
// TestLastPlayedIsAWorksOwn covers.
const playedOld = `(le.id = le.root_id OR (SELECT lr.kind FROM work lr WHERE lr.id = le.root_id) = 'series')`

var gridSortsOld = map[Sort]gridSortOld{
	ByTitle:            {key: "NULL", order: "t, id", letters: true},
	ByAdded:            {key: "wl.added_at", order: "k DESC, t, id"},
	ByRelease:          {key: "wl.release_key", order: "k DESC, t, id"},
	ByCommunityRating:  {key: "wl.rating_sort", order: "k DESC, t, id"},
	ByRuntime:          {key: "wl.runtime_sort", order: "k, t, id"},
	ByAuthor:           {key: "wl.creator_sort", order: "k, t, id"},
	ByLastEpisodeAdded: {key: "wl.last_added_at", order: "k DESC, t, id"},
	ByMyRating: {
		key: "us.rating", order: "k DESC, t, id",
		from: `FROM user_state us -- the user's rated works, first: CROSS JOIN keeps the order
    CROSS JOIN work_library wl ON wl.library_id = @library AND wl.work_id = us.work_id
    JOIN work w ON w.id = wl.work_id`,
		mine: "us.user_id = @user AND us.rating IS NOT NULL AND ",
		rest: "NOT EXISTS (SELECT 1 FROM user_state ru WHERE ru.user_id = @user AND ru.work_id = w.id AND ru.rating IS NOT NULL)",
	},
	ByLastPlayed: {
		key: "lp.at", order: "k DESC, t, id",
		from: `FROM ( -- the user's played works, first: CROSS JOIN keeps the order
      SELECT le.root_id AS root, max(lu.last_played_at) AS at
      FROM user_state lu JOIN work le ON le.id = lu.work_id
      WHERE lu.user_id = @user AND lu.last_played_at IS NOT NULL AND ` + playedOld + `
      GROUP BY le.root_id) lp
    CROSS JOIN work_library wl ON wl.library_id = @library AND wl.work_id = lp.root
    JOIN work w ON w.id = wl.work_id`,
		rest: `NOT EXISTS (SELECT 1 FROM work le CROSS JOIN user_state lu ON lu.user_id = @user AND lu.work_id = le.id
        WHERE le.root_id = w.id AND lu.last_played_at IS NOT NULL AND ` + playedOld + `)`,
	},
}

// page returns the sort's page CTE: the IDs of the page's works, with their
// k and t. In a per-user sort, the works with no key have a NULL key, which
// sorts last when descending.
func (g gridSortOld) page() string {
	if g.from == "" {
		return `SELECT wl.work_id AS id, ` + g.key + ` AS k, wl.sort_title AS t
  FROM work_library wl
  JOIN work w ON w.id = wl.work_id
  WHERE ` + gridOld + `
  ORDER BY ` + g.order + `
  LIMIT @limit OFFSET @offset`
	}
	return `SELECT id, k, t FROM (
    SELECT wl.work_id AS id, ` + g.key + ` AS k, wl.sort_title AS t
    ` + g.from + `
    WHERE ` + g.mine + gridOld + `
    ORDER BY ` + g.order + `
    LIMIT @limit + @offset
  ) keyed
  UNION ALL
  SELECT id, k, t FROM (
    SELECT wl.work_id AS id, NULL AS k, wl.sort_title AS t
    FROM work_library wl
    JOIN work w ON w.id = wl.work_id
    WHERE ` + g.rest + ` AND ` + gridOld + `
    ORDER BY t, id
    LIMIT @limit + @offset
  ) rest
  ORDER BY ` + g.order + `
  LIMIT @limit OFFSET @offset`
}

// browseOld is a sort's grid statement.
func browseOld(g gridSortOld) statement {
	return statement{sql: `
WITH page (id, k, t) AS (
  ` + g.page() + `
)
SELECT ` + summaryColumns + `
FROM page p
JOIN work w ON w.id = p.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
ORDER BY p.` + strings.ReplaceAll(g.order, ", ", ", p.")}
}

var browseCountOld = statement{sql: `
SELECT count(*)
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE ` + gridOld}

// browseLettersOld counts browseCountOld's rows by the first character of
// their sort title, in the grid's order.
var browseLettersOld = statement{sql: `
SELECT coalesce(unicode(wl.sort_title), 0) AS initial, count(*) AS n
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE ` + gridOld + `
GROUP BY 1
ORDER BY 1`}

// argsOld returns the filter's statement arguments.
func (f Filter) argsOld() ([]any, error) {
	if f.Played != "" && !slices.Contains([]PlayState{Unplayed, InProgress, Played}, f.Played) {
		return nil, invalid("played", "invalid", fmt.Sprintf("played filter %q", f.Played))
	}
	var resolutions, dynamicRanges int64
	for _, r := range f.Resolutions {
		bit, ok := resolutionBits[r]
		if !ok {
			return nil, invalid("resolution", "invalid", fmt.Sprintf("resolution %q", r))
		}
		resolutions |= bit
	}
	for _, r := range f.Ranges {
		bit, ok := rangeBits[r]
		if !ok {
			return nil, invalid("hdr", "invalid", fmt.Sprintf("range %q", r))
		}
		dynamicRanges |= bit
	}
	if f.YearFrom != 0 && f.YearTo != 0 && f.YearTo < f.YearFrom {
		return nil, invalid("yearTo", "outOfRange", fmt.Sprintf("years %d to %d", f.YearFrom, f.YearTo))
	}
	var played any
	if f.Played != "" {
		played = string(f.Played)
	}
	return []any{
		sql.Named("played", played),
		sql.Named("genres", set(f.Genres)),
		sql.Named("year_from", sql.Null[int]{V: f.YearFrom, Valid: f.YearFrom != 0}),
		sql.Named("year_to", sql.Null[int]{V: f.YearTo, Valid: f.YearTo != 0}),
		sql.Named("content_ratings", set(f.ContentRatings)),
		sql.Named("resolutions", resolutions),
		sql.Named("dynamic_ranges", dynamicRanges),
		sql.Named("unmatched", f.Unmatched),
		sql.Named("narrator", f.Narrator),
	}, nil
}

// BrowseOld is Browse as it read the grid before: the IDs of the page's
// works in order, the grid's total, which every page had, and the letters,
// on a first page sorted by title.
func (s *Service) BrowseOld(ctx context.Context, v users.Viewer, lib id.ID, o BrowseOptions) (works []id.ID, total int, letters []Letter, err error) {
	filter, err := o.Filter.argsOld()
	if err != nil {
		return nil, 0, nil, err
	}
	if o.Sort == "" {
		o.Sort = ByTitle
	}
	g, ok := gridSortsOld[o.Sort]
	if !ok {
		return nil, 0, nil, invalid("sort", "invalid", fmt.Sprintf("sort %q", o.Sort))
	}
	limit, err := o.limit()
	if err != nil {
		return nil, 0, nil, err
	}
	vw := viewerOf(v)
	count := vw.args(append(filter, sql.Named("library", lib))...)
	pageArgs := vw.args(append(filter, sql.Named("library", lib), sql.Named("offset", o.Offset), sql.Named("limit", limit))...)
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := library(ctx, tx, vw, lib); err != nil {
			return notFound(err)
		}
		var err error
		if g.letters {
			letters, total, err = rail(ctx, tx, browseLettersOld, count, o.GridOptions)
		} else {
			err = rowOf(ctx, tx, browseCountOld, count, &total)
		}
		if err != nil {
			return err
		}
		items, err := summaries(ctx, tx, browseOld(g), pageArgs)
		for _, c := range items {
			works = append(works, c.ID)
		}
		return err
	})
	return works, total, letters, err
}
