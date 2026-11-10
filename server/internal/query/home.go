package query

import (
	"cmp"
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// rowLimit is how many summaries a Home row holds.
const rowLimit = 20

// seriesStaleAfter is how long a series' next episode stays in Continue
// Watching after the user's last play in the series, unless the series
// gained an episode since that play.
const seriesStaleAfter = 365 * 24 * time.Hour

// RowKind is what a Home row shows.
type RowKind string

// The Home rows.
const (
	// RowContinueWatching is what to keep watching, most recently played
	// first. It holds the videos in progress other than episodes, and each
	// series' episode up next. It includes works that can't play right now
	// (see WorkSummary.Unavailable), and leaves out what the user removed
	// from it and hasn't played since.
	RowContinueWatching RowKind = "continueWatching"
	// RowList is a list the user keeps, most recently added first.
	RowList RowKind = "list"
	// RowRecentlyAdded is one library's newest works that the user
	// hasn't played. A show's episodes added close together are bundled
	// into one summary (see bundles).
	RowRecentlyAdded RowKind = "recentlyAdded"
	// RowContinueListening is the books in progress, ordered as in Continue
	// Watching.
	RowContinueListening RowKind = "continueListening"
)

// HomeRow is one Home row. Library is set only on a Recently Added row,
// with the library's name and kind, and List only on a list's row.
type HomeRow struct {
	ID      string
	Kind    RowKind
	Library Library
	List    ListRef
	Items   []WorkSummary
}

// ListRef is a list as a row names it.
type ListRef struct {
	ID    id.ID
	Name  string
	Smart bool
}

// HomeRowInfo is a row of a user's Home layout, as the editor lists it.
type HomeRowInfo struct {
	ID      string
	Kind    RowKind
	Library Library
	List    ListRef
	Shown   bool
	smart   *SmartList // a smart list's rules, for its row
}

// homeRowStates reads what the user stored about their Home rows.
var homeRowStates = stmt("HomeRowStates", `
-- viewer-filter: user
SELECT row_id, position, shown FROM user_home_row WHERE user_id = @user`)

// Home rows. Provisional works appear in Recently Added but never in
// Continue Watching or collections.

// continueWatching lists what to keep watching, most recently played first.
// It holds the movies and other videos played past the Continue Watching
// floor (userstate keeps resumable), and each series' episode up next,
// ordered by when the user last played an episode of the series. A work
// whose files can't be reached right now stays in the row, marked
// unavailable.
//
// A work the user removed is left out until it's played again, and a
// series until any of its episodes is. A series also goes once it's
// stale: last played before @stale_before, with no episode added since.
var continueWatching = stmt("ContinueWatching", `
WITH followed (series, at, last) AS (
  SELECT fw.root_id, max(fu.last_played_at) FILTER (WHERE fu.played OR fu.resumable), max(fu.last_played_at)
  FROM user_state fu JOIN work fw ON fw.id = fu.work_id
  WHERE fu.user_id = @user AND fu.last_played_at IS NOT NULL AND fw.kind = 'episode'
  GROUP BY fw.root_id
),
picks (work, at) AS (
  SELECT pu.work_id, pu.last_played_at
  FROM user_state pu JOIN work pw ON pw.id = pu.work_id
  WHERE pu.user_id = @user AND pu.resumable AND pw.kind NOT IN ('episode', 'book')
    AND `+notRemoved("pu")+`
  UNION ALL
  SELECT `+upNext("f.series")+`, f.at FROM followed f
  WHERE f.at IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM user_state ru WHERE ru.user_id = @user AND ru.work_id = f.series AND ru.continue_removed_at >= f.last)
    AND (f.last >= @stale_before OR EXISTS (
      SELECT 1 FROM work ne JOIN work_library nl ON nl.work_id = ne.id
      WHERE ne.root_id = f.series AND ne.kind = 'episode' AND nl.added_at > f.last))
)
SELECT `+summaryColumns+`
FROM picks p
JOIN work w ON w.id = p.work
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE NOT w.provisional
  AND NOT (SELECT r.provisional FROM work r WHERE r.id = w.root_id)
  AND `+visible+`
ORDER BY p.at DESC, w.id
LIMIT @limit`)

// continueListening lists the books in progress, chosen and ordered as
// ContinueWatching does its videos.
var continueListening = stmt("ContinueListening", `
SELECT `+summaryColumns+`
FROM user_state us
JOIN work w ON w.id = us.work_id
WHERE us.user_id = @user AND us.resumable AND NOT w.provisional
  AND w.kind = 'book'
  AND `+notRemoved("us")+`
  AND `+notHidden+`
  AND `+visible+`
ORDER BY us.last_played_at DESC, us.work_id
LIMIT @limit`)

// recentlyAdded lists a library's newest 1,000 additions, newest first, for
// Home to bundle into summaries. Each row is a playable work with its season
// and series, and whether the user played it. Reading only the newest
// additions keeps the read small. A scan that adds more than that at once
// still shows the newest of them first.
var recentlyAdded = stmt("RecentlyAdded", `
SELECT w.id, w.kind, w.parent_id, w.root_id, ra.added_at, w.sort_title,
  coalesce(s.sort_title, ''), r.sort_title, r.provisional, coalesce(us.played, 0)
FROM (SELECT x.work_id, x.added_at FROM work_library x WHERE x.library_id = @library
      ORDER BY x.added_at DESC LIMIT 1000) ra
JOIN work w ON w.id = ra.work_id
JOIN work r ON r.id = w.root_id
LEFT JOIN work s ON s.id = w.parent_id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.kind IN ('movie', 'episode', 'video', 'book')
  AND `+visible+`
ORDER BY ra.added_at DESC, w.id`)

// notRemoved keeps the user's state u for a work unless the user removed the
// work from Continue Watching or Continue Listening and hasn't played it
// since.
func notRemoved(u string) string {
	return "(" + u + ".continue_removed_at IS NULL OR " + u + ".continue_removed_at < " + u + ".last_played_at)"
}

// rowID is a row's ID: its kind, followed by the library's or the list's
// ID for a Recently Added row or a list's row.
func rowID(kind RowKind, target id.ID) string {
	if target.IsZero() {
		return string(kind)
	}
	return string(kind) + ":" + target.String()
}

// ValidRowID reports whether s is well formed as a row's ID, whether or
// not the row exists.
func ValidRowID(s string) bool {
	kind, rest, cut := strings.Cut(s, ":")
	if !cut {
		return s == string(RowContinueWatching) || s == string(RowContinueListening)
	}
	if kind != string(RowRecentlyAdded) && kind != string(RowList) {
		return false
	}
	_, err := id.Parse(rest)
	return err == nil
}

// defaultRows lists every row of a user's Home in the default order:
// Continue Watching, My List, each video library's Recently Added in the
// server's library order, Continue Listening if the user has an audiobooks
// library, each audiobooks library's Recently Added, then the other lists.
// Every row is shown except the other lists.
func defaultRows(libs []Library, lists []List) []HomeRowInfo {
	rows := []HomeRowInfo{
		{ID: rowID(RowContinueWatching, id.ID{}), Kind: RowContinueWatching, Shown: true},
	}
	var others []HomeRowInfo
	for _, l := range lists {
		row := HomeRowInfo{ID: rowID(RowList, l.ID), Kind: RowList, Shown: l.BuiltIn, List: ListRef{ID: l.ID, Name: l.Name, Smart: l.Smart != nil}, smart: l.Smart}
		if l.BuiltIn {
			rows = append(rows, row)
		} else {
			others = append(others, row)
		}
	}
	var books []HomeRowInfo
	for _, lib := range libs {
		row := HomeRowInfo{ID: rowID(RowRecentlyAdded, lib.ID), Kind: RowRecentlyAdded, Shown: true,
			Library: Library{ID: lib.ID, Name: lib.Name, Kind: lib.Kind}}
		if lib.Kind == catalog.Audiobooks {
			books = append(books, row)
		} else {
			rows = append(rows, row)
		}
	}
	if len(books) > 0 {
		rows = append(rows, HomeRowInfo{ID: rowID(RowContinueListening, id.ID{}), Kind: RowContinueListening, Shown: true})
	}
	rows = append(rows, books...)
	return append(rows, others...)
}

// rowState is what the user stored about a row.
type rowState struct {
	position sql.NullInt64
	shown    bool
}

// arrange applies the user's stored state to the default rows. On a Home
// not arranged, the rows keep the default order and a stored row its shown.
// On an arranged Home, rows with a position come first in order. A row with
// none follows, keeping its stored shown, and with nothing stored it's shown
// only if it's a library's: a new library joins the end shown, and a row of
// a new kind joins off.
func arrange(rows []HomeRowInfo, states map[string]rowState) []HomeRowInfo {
	arranged := false
	for _, st := range states {
		arranged = arranged || st.position.Valid
	}
	for i := range rows {
		st, ok := states[rows[i].ID]
		switch {
		case ok:
			rows[i].Shown = st.shown
		case arranged:
			rows[i].Shown = rows[i].Kind == RowRecentlyAdded
		}
	}
	if !arranged {
		return rows
	}
	position := func(r HomeRowInfo) (int64, bool) {
		st, ok := states[r.ID]
		return st.position.Int64, ok && st.position.Valid
	}
	slices.SortStableFunc(rows, func(a, b HomeRowInfo) int {
		pa, oka := position(a)
		pb, okb := position(b)
		switch {
		case oka && okb:
			return cmp.Compare(pa, pb)
		case oka:
			return -1
		case okb:
			return 1
		default:
			return 0
		}
	})
	return rows
}

// Broken says whether the row is a smart list whose rules no longer run,
// so it never shows on Home.
func (r HomeRowInfo) Broken() bool { return r.smart != nil && len(r.smart.Errors) > 0 }

// layout returns every row of the user's Home in order, shown or not.
func layout(ctx context.Context, tx *sql.Tx, vw viewer) ([]HomeRowInfo, error) {
	libs, err := rowsOf(ctx, tx, homeLibraries, vw.args(), func(r scanner, l *Library) error {
		return r.Scan(&l.ID, &l.Name, &l.Kind)
	})
	if err != nil {
		return nil, err
	}
	lists, err := rowsOf(ctx, tx, listsOfUser, vw.args(), scanList)
	if err != nil {
		return nil, err
	}
	type stored struct {
		id string
		rowState
	}
	rows, err := rowsOf(ctx, tx, homeRowStates, vw.args(), func(r scanner, s *stored) error {
		return r.Scan(&s.id, &s.position, &s.shown)
	})
	if err != nil {
		return nil, err
	}
	states := make(map[string]rowState, len(rows))
	for _, r := range rows {
		states[r.id] = r.rowState
	}
	return arrange(defaultRows(libs, lists), states), nil
}

// HomeRows returns every row of the user's Home in order, shown or not,
// including rows with no works.
func (s *Service) HomeRows(ctx context.Context, v users.Viewer) ([]HomeRowInfo, error) {
	var out []HomeRowInfo
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = layout(ctx, tx, viewerOf(v))
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ArrangedHomeRows returns the user's whole layout with changes applied:
// the listed rows in their order, and the others after them in the order
// they're in, keeping their shown. A row that no longer exists is left out.
func (s *Service) ArrangedHomeRows(ctx context.Context, v users.Viewer, changes []users.HomeRow) ([]users.HomeRow, error) {
	current, err := s.HomeRows(ctx, v)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(current))
	for _, r := range current {
		known[r.ID] = true
	}
	out := make([]users.HomeRow, 0, len(current))
	listed := make(map[string]bool, len(changes))
	for _, c := range changes {
		if known[c.ID] {
			out = append(out, c)
			listed[c.ID] = true
		}
	}
	for _, r := range current {
		if !listed[r.ID] {
			out = append(out, users.HomeRow{ID: r.ID, Shown: r.Shown})
		}
	}
	return out, nil
}

// rowItems reads the summaries of a Home row.
func rowItems(ctx context.Context, tx *sql.Tx, vw viewer, r HomeRowInfo) ([]WorkSummary, error) {
	limit := sql.Named("limit", rowLimit)
	switch r.Kind {
	case RowContinueWatching:
		stale := sql.Named("stale_before", time.Now().Add(-seriesStaleAfter).UnixMilli())
		return summaries(ctx, tx, continueWatching, vw.args(limit, stale))
	case RowContinueListening:
		return summaries(ctx, tx, continueListening, vw.args(limit))
	case RowList:
		if r.smart != nil {
			return smartRow(ctx, tx, vw, r)
		}
		return summaries(ctx, tx, listWorksPage, vw.args(sql.Named("list", r.List.ID), sql.Named("kinds", nil), sql.Named("offset", 0), limit))
	case RowRecentlyAdded:
		added, err := rowsOf(ctx, tx, recentlyAdded, vw.args(sql.Named("library", r.Library.ID)), func(rs scanner, a *addition) error {
			return rs.Scan(&a.ID, &a.Kind, &a.ParentID, &a.RootID, &a.AddedAt, &a.SortTitle,
				&a.SeasonSortTitle, &a.SeriesSortTitle, &a.SeriesProvisional, &a.Played)
		})
		if err != nil {
			return nil, err
		}
		return bundleSummaries(ctx, tx, vw, bundles(added))
	}
	return nil, nil
}

// Home returns the Home rows that have summaries, in the user's layout,
// read from one snapshot. It returns the shown rows, or with kinds the
// rows of those kinds, shown or not. allHidden says the user hid every row
// of the layout; kinds never sets it.
func (s *Service) Home(ctx context.Context, v users.Viewer, kinds ...RowKind) (rows []HomeRow, allHidden bool, err error) {
	vw := viewerOf(v)
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		all, err := layout(ctx, tx, vw)
		if err != nil {
			return err
		}
		allHidden = len(kinds) == 0 && !slices.ContainsFunc(all, func(r HomeRowInfo) bool { return r.Shown })
		for _, r := range all {
			if len(kinds) > 0 && !slices.Contains(kinds, r.Kind) || len(kinds) == 0 && !r.Shown {
				continue
			}
			items, err := rowItems(ctx, tx, vw, r)
			if err != nil {
				if ctx.Err() != nil {
					return err
				}
				slog.WarnContext(ctx, "home: skipped a row that failed", "row", r.ID, "err", err)
				continue
			}
			if len(items) > 0 {
				rows = append(rows, HomeRow{ID: r.ID, Kind: r.Kind, Library: r.Library, List: r.List, Items: items})
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	lists := make([][]WorkSummary, len(rows))
	for i, r := range rows {
		lists[i] = r.Items
	}
	if err := s.decorate(ctx, vw, lists...); err != nil {
		return nil, false, err
	}
	return rows, allHidden, nil
}

// smartRow reads the first rowLimit works of a smart list's rules.
func smartRow(ctx context.Context, tx *sql.Tx, vw viewer, r HomeRowInfo) ([]WorkSummary, error) {
	if len(r.smart.Errors) > 0 {
		return nil, nil
	}
	now := time.Now()
	c, err := compilePage(ctx, tx, r.smart.Query, vw, now, listSeed(r.List.ID, now), nil)
	if err != nil {
		return nil, err
	}
	return summaries(ctx, tx, statement{sql: c.page}, append(slices.Clone(c.args), sql.Named("limit", rowLimit), sql.Named("offset", 0)))
}

// bundleSummaries reads the summaries of a library's "Recently Added"
// bundles, in their order, up to rowLimit. A bundle the Viewer may not see
// is skipped, and the next bundles are read in its place.
func bundleSummaries(ctx context.Context, q querier, vw viewer, bs []bundle) ([]WorkSummary, error) {
	items := []WorkSummary{}
	for len(bs) > 0 && len(items) < rowLimit {
		batch := bs[:min(rowLimit-len(items), len(bs))]
		bs = bs[len(batch):]
		ids := make([]id.ID, len(batch))
		for i, b := range batch {
			ids[i] = b.work
		}
		byWork, err := summariesByID(ctx, q, vw, ids)
		if err != nil {
			return nil, err
		}
		for _, b := range batch {
			c, ok := byWork[b.work]
			if !ok {
				continue
			}
			c.Bundled = b.episodes
			if b.unmatched {
				c.MatchState = Unmatched
			}
			items = append(items, c)
		}
	}
	return items, nil
}
