package query

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// Sort orders a library grid. Its values are the API's. Each sort's order
// is defined in gridSorts.
type Sort string

// Sorts, each in its default direction.
const (
	ByTitle            Sort = "title"              // A to Z, ignoring leading articles
	ByAdded            Sort = "addedAt"            // newest first
	ByRelease          Sort = "releaseDate"        // newest first
	ByCommunityRating  Sort = "rating"             // the community rating, weighted by its votes, highest first
	ByMyRating         Sort = "myRating"           // the user's own rating, highest first
	ByRuntime          Sort = "runtime"            // shortest first
	ByLastPlayed       Sort = "lastPlayedAt"       // most recent first
	ByAuthor           Sort = "author"             // A to Z by the first author's sort name, then by title
	ByLastEpisodeAdded Sort = "lastEpisodeAddedAt" // a series by its newest episode in the library, newest first, then by title
)

// PlayState filters by the user's watch state. Its values are the API's. A
// series is played when every episode is played, and in progress when some
// are.
type PlayState string

// Play states.
const (
	Unplayed   PlayState = "unplayed"
	InProgress PlayState = "inProgress"
	Played     PlayState = "played"
)

// Resolution classes a version's video by its width.
type Resolution string

// Resolutions.
const (
	Resolution4K    Resolution = "4k"    // 3200 pixels wide or more
	Resolution1080p Resolution = "1080p" // 1700 pixels wide or more
	Resolution720p  Resolution = "720p"  // 1200 pixels wide or more
	ResolutionSD    Resolution = "sd"
)

// ResolutionOf classes a video by its width, not its height, because a
// widescreen film is shorter than its class's height.
func ResolutionOf(width int) Resolution {
	switch {
	case width >= 3200:
		return Resolution4K
	case width >= 1700:
		return Resolution1080p
	case width >= 1200:
		return Resolution720p
	}
	return ResolutionSD
}

// Range is a video's dynamic range. Its values are the API's.
type Range string

// Ranges a filter can ask for.
const (
	HDR         Range = "hdr" // HDR10 or HLG
	DolbyVision Range = "dolbyVision"
)

// Filter narrows a library grid. Its fields combine with AND. A list
// matches any of its values, and an empty list matches everything.
// Resolutions and Ranges apply to works with versions (movies and videos),
// not to series.
type Filter struct {
	Played         PlayState // empty for any
	Genres         []string
	YearFrom       int // the first year, inclusive; zero for no bound
	YearTo         int // the last year, inclusive; zero for no bound
	ContentRatings []string
	Resolutions    []Resolution
	Ranges         []Range
	Unmatched      bool  // only provisional works
	Narrator       id.ID // only the books this person narrated; zero for any
}

// BrowseOptions picks a page of a library grid.
type BrowseOptions struct {
	Sort   Sort // ByTitle when empty
	Filter Filter
	GridOptions
}

// Library grids use one statement per sort, all of the same shape. The page
// CTE orders the grid's rows by the sort and cuts the page at its offset.
// Summaries are then built for the page's rows alone. A sort with an index
// on work_library reads the page in the index's order. An offset skips rows
// in that same order, so it costs no more than the count.

// grid selects a library grid's rows, over work_library wl and work w. The
// rows are the library's top-level works that the Viewer sees and the
// filters keep.
var grid = `wl.library_id = @library AND wl.is_root
  AND ` + visible + `
  ` + filters

// filters are a library grid's filters, each in the (@x IS NULL OR ...)
// form, so a filter not asked for matches every row.
var filters = `AND (@played IS NULL OR @played = CASE
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

// rollupPlayState is a series' or season's play state from its episodes,
// those with column pointing at w.
func rollupPlayState(column string) string {
	return `CASE
      WHEN NOT EXISTS (SELECT 1 FROM work fe WHERE ` + column + ` = w.id AND fe.kind = 'episode'
        AND EXISTS (SELECT 1 FROM work_library fl WHERE fl.work_id = fe.id AND ` + catalog.Granted("fl.library_id") + `)
        AND NOT EXISTS (SELECT 1 FROM user_state fu WHERE fu.user_id = @user AND fu.work_id = fe.id AND fu.played))
        THEN 'played'
      WHEN EXISTS (SELECT 1 FROM work fe JOIN user_state fu ON fu.user_id = @user AND fu.work_id = fe.id
        WHERE ` + column + ` = w.id AND fe.kind = 'episode' AND (fu.played OR fu.resumable))
        THEN 'inProgress'
      ELSE 'unplayed' END`
}

// gridSort is how a library grid orders one sort, using the sort's index on
// work_library.
//
// key is the sort key, the page's k column. order orders the page's id, k
// and t (the sort title), with ties broken by title and then ID. letters
// counts the grid by letter for the letter rail.
//
// A per-user sort reads in two parts, each in its own order and cut at the
// page's end, so a page near the top reads little of a large library. It
// first reads the user's works that have a key, from the user's states:
// from is that part's FROM clause, and mine holds its own conditions,
// placed before the grid's. It then reads the works matching rest by
// title, with no key.
type gridSort struct {
	key, order       string
	from, mine, rest string
	letters          bool
}

var gridSorts = map[Sort]gridSort{
	ByTitle:   {key: "NULL", order: "t, id", letters: true},
	ByAdded:   {key: "wl.added_at", order: "k DESC, t, id"},
	ByRelease: {key: "wl.release_key", order: "k DESC, t, id"},
	// rating_sort is the community rating weighted by its votes, as if 50
	// more votes of 6.5 were cast (see work_browse_keys). So a work few
	// people rated ranks below a well-rated work many people rated. Works
	// with no rating follow, by title.
	ByCommunityRating: {key: "wl.rating_sort", order: "k DESC, t, id"},
	// Sorts by the running time of the work's first version. Works with no
	// version, such as series, follow by title.
	ByRuntime: {key: "wl.runtime_sort", order: "k, t, id"},
	// Works with no author first.
	ByAuthor: {key: "wl.creator_sort", order: "k, t, id"},
	// Sorts by the newest added_at among the work and the descendants that
	// make it available in the library. So a series sorts by its newest
	// episode in the library.
	ByLastEpisodeAdded: {key: "wl.last_added_at", order: "k DESC, t, id"},
	ByMyRating: {
		key: "us.rating", order: "k DESC, t, id",
		from: `FROM user_state us -- the user's rated works, first: CROSS JOIN keeps the order
    CROSS JOIN work_library wl ON wl.library_id = @library AND wl.work_id = us.work_id
    JOIN work w ON w.id = wl.work_id`,
		mine: "us.user_id = @user AND us.rating IS NOT NULL AND ",
		rest: "NOT EXISTS (SELECT 1 FROM user_state ru WHERE ru.user_id = @user AND ru.work_id = w.id AND ru.rating IS NOT NULL)",
	},
	// A series sorts by its most recently played episode.
	ByLastPlayed: {
		key: "lp.at", order: "k DESC, t, id",
		from: `FROM ( -- the user's played works, first: CROSS JOIN keeps the order
      SELECT le.root_id AS root, max(lu.last_played_at) AS at
      FROM user_state lu JOIN work le ON le.id = lu.work_id
      WHERE lu.user_id = @user AND lu.last_played_at IS NOT NULL
      GROUP BY le.root_id) lp
    CROSS JOIN work_library wl ON wl.library_id = @library AND wl.work_id = lp.root
    JOIN work w ON w.id = wl.work_id`,
		// CROSS JOIN reads the work and its episodes before their states.
		rest: `NOT EXISTS (SELECT 1 FROM work le CROSS JOIN user_state lu ON lu.user_id = @user AND lu.work_id = le.id
        WHERE le.root_id = w.id AND lu.last_played_at IS NOT NULL)`,
	},
}

// page returns the sort's page CTE: the IDs of the page's works, with their
// k and t. In a per-user sort, the works with no key have a NULL key, which
// sorts last when descending.
func (g gridSort) page() string {
	if g.from == "" {
		return `SELECT wl.work_id AS id, ` + g.key + ` AS k, wl.sort_title AS t
  FROM work_library wl
  JOIN work w ON w.id = wl.work_id
  WHERE ` + grid + `
  ORDER BY ` + g.order + `
  LIMIT @limit OFFSET @offset`
	}
	return `SELECT id, k, t FROM (
    SELECT wl.work_id AS id, ` + g.key + ` AS k, wl.sort_title AS t
    ` + g.from + `
    WHERE ` + g.mine + grid + `
    ORDER BY ` + g.order + `
    LIMIT @limit + @offset
  ) keyed
  UNION ALL
  SELECT id, k, t FROM (
    SELECT wl.work_id AS id, NULL AS k, wl.sort_title AS t
    FROM work_library wl
    JOIN work w ON w.id = wl.work_id
    WHERE ` + g.rest + ` AND ` + grid + `
    ORDER BY t, id
    LIMIT @limit + @offset
  ) rest
  ORDER BY ` + g.order + `
  LIMIT @limit OFFSET @offset`
}

// browse is each sort's grid statement.
var browse = func() map[Sort]statement {
	out := map[Sort]statement{}
	for _, sort := range slices.Sorted(maps.Keys(gridSorts)) {
		g := gridSorts[sort]
		out[sort] = stmt("Browse."+string(sort), `
WITH page (id, k, t) AS (
  `+g.page()+`
)
SELECT `+summaryColumns+`
FROM page p
JOIN work w ON w.id = p.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
ORDER BY p.`+strings.ReplaceAll(g.order, ", ", ", p."))
	}
	return out
}()

var browseCount = stmt("BrowseCount", `
SELECT count(*)
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE `+grid)

// browseLetters counts BrowseCount's rows by the first character of their
// sort title, for the letter rail. The character is a code point, or 0 for
// an empty title. The groups come in the grid's order, because UTF-8 sorts
// by code point.
var browseLetters = stmt("BrowseLetters", `
SELECT coalesce(unicode(wl.sort_title), 0) AS initial, count(*) AS n
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE `+grid+`
GROUP BY 1
ORDER BY 1`)

// Browse returns a page of a library's grid. The grid holds the library's
// top-level works: movies, series, videos or books.
func (s *Service) Browse(ctx context.Context, v users.Viewer, lib id.ID, o BrowseOptions) (GridPage[WorkSummary], error) {
	filter, err := o.Filter.args()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	if o.Sort == "" {
		o.Sort = ByTitle
	}
	g, ok := gridSorts[o.Sort]
	if !ok {
		return GridPage[WorkSummary]{}, invalid("sort", "invalid", fmt.Sprintf("sort %q", o.Sort))
	}
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	vw := viewerOf(v)
	count := vw.args(append(filter, sql.Named("library", lib))...)
	pageArgs := vw.args(append(filter, sql.Named("library", lib), sql.Named("offset", o.Offset), sql.Named("limit", limit))...)

	var items []WorkSummary
	var letters []Letter
	total := 0
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := library(ctx, tx, vw, lib); err != nil {
			return notFound(err)
		}
		var err error
		if g.letters {
			// The letters' counts add up to the total.
			letters, total, err = rail(ctx, tx, browseLetters, count, o.GridOptions)
		} else {
			err = rowOf(ctx, tx, browseCount, count, &total)
		}
		if err != nil {
			return err
		}
		items, err = summaries(ctx, tx, browse[o.Sort], pageArgs)
		return err
	})
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return s.summaryPage(ctx, vw, items, total, letters)
}

// Resolutions and ranges are stored as bit sets in work_library, a bit for
// each a work's versions have. The work_browse_keys view sets the bits.
var (
	resolutionBits = map[Resolution]int64{ResolutionSD: 1, Resolution720p: 2, Resolution1080p: 4, Resolution4K: 8}
	rangeBits      = map[Range]int64{HDR: 1, DolbyVision: 2}
)

// named is a bit set's bits by their values' names, as a rule gives them.
func named[V ~string](bits map[V]int64) map[string]int64 {
	out := make(map[string]int64, len(bits))
	for v, bit := range bits {
		out[string(v)] = bit
	}
	return out
}

// args returns the filter's statement arguments.
func (f Filter) args() ([]any, error) {
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

// notHidden leaves out the episodes a multi-episode version covers after
// its first episode. Those are part of the first episode's summary.
const notHidden = `(NOT EXISTS (SELECT 1 FROM media_version_work h WHERE h.work_id = w.id AND h.ord > 0)
    OR EXISTS (SELECT 1 FROM media_version_work h WHERE h.work_id = w.id AND h.ord = 0))`

// seasonEpisodeCounts is how many episodes each of a series' seasons lists,
// counted the way Children lists a season's episodes.
var seasonEpisodeCounts = stmt("SeasonEpisodeCounts", `
SELECT w.parent_id, count(*) AS episodes
FROM work w
WHERE w.root_id = @series AND w.kind = 'episode'
  AND `+notHidden+`
  AND `+visible+`
GROUP BY w.parent_id`)

// episodeOverviews is a season's episodes' synopses, for its list.
var episodeOverviews = stmt("EpisodeOverviews", `
SELECT w.id, w.overview
FROM work w
WHERE w.parent_id = @season AND w.kind = 'episode' AND coalesce(w.overview, '') <> ''
  AND `+visible)

// children is a series' seasons or a season's episodes, specials last.
var children = stmt("Children", `
SELECT `+summaryColumns+`
FROM work w
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.parent_id = @parent AND w.kind IN ('season', 'episode')
  AND `+notHidden+`
  AND `+visible+`
ORDER BY w.kind = 'season' AND coalesce(w.number, 0) = 0, coalesce(w.number, 0), w.id`)

// Children returns all of a series' seasons or a season's episodes, in
// order with specials last. They aren't paged, because one parent has a
// bounded number of children. Any other work has no children. An episode
// that a multi-episode version covers after its first episode is part of
// the first episode's summary, not listed on its own.
func (s *Service) Children(ctx context.Context, v users.Viewer, parent id.ID) ([]WorkSummary, error) {
	vw := viewerOf(v)
	var items []WorkSummary
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		p, err := summary(ctx, tx, vw, parent)
		if err != nil {
			return notFound(err)
		}
		if items, err = summaries(ctx, tx, children, vw.args(sql.Named("parent", parent))); err != nil {
			return err
		}
		if p.Kind == catalog.KindSeason {
			return addOverviews(ctx, tx, vw, parent, items)
		}
		if p.Kind != catalog.KindSeries {
			return nil
		}
		// A series' seasons carry how many episodes each lists.
		type count struct {
			season id.ID
			n      int
		}
		counts, err := rowsOf(ctx, tx, seasonEpisodeCounts, vw.args(sql.Named("series", parent)), func(r scanner, c *count) error {
			return r.Scan(&c.season, &c.n)
		})
		if err != nil {
			return err
		}
		for _, c := range counts {
			if i := slices.IndexFunc(items, func(s WorkSummary) bool { return s.ID == c.season }); i >= 0 {
				items[i].Episodes = c.n
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.decorate(ctx, vw, items); err != nil {
		return nil, err
	}
	return items, nil
}

// addOverviews gives a season's episode summaries their synopses.
func addOverviews(ctx context.Context, tx *sql.Tx, vw viewer, season id.ID, items []WorkSummary) error {
	type overview struct {
		episode id.ID
		text    string
	}
	overviews, err := rowsOf(ctx, tx, episodeOverviews, vw.args(sql.Named("season", season)), func(r scanner, o *overview) error {
		return r.Scan(&o.episode, &o.text)
	})
	for _, o := range overviews {
		if i := slices.IndexFunc(items, func(e WorkSummary) bool { return e.ID == o.episode }); i >= 0 {
			items[i].Overview = o.text
		}
	}
	return err
}

// CollectionSource is where a collection comes from. Its values are the
// API's.
type CollectionSource string

// Collection sources.
const (
	FromProvider CollectionSource = "provider" // a movie collection
	FromFolder   CollectionSource = "folder"   // a videos library's folder
)

// Collection is a movie collection or a videos library's folder.
type Collection struct {
	ID     id.ID
	Name   string
	Source CollectionSource
	Items  int    // how many of its works the Viewer sees
	Images Images // its poster and backdrop
}

// CollectionPage is a collection's page: the collection and its works.
type CollectionPage struct {
	Collection Collection
	Items      []WorkSummary // in release order
}

// collectionColumns are a collection c's ID, name and source. The source
// comes from the key's prefix.
const collectionColumns = `c.id, c.name, CASE WHEN c.key LIKE 'folder:%' THEN 'folder' ELSE 'provider' END`

// scanCollection scans collectionColumns and the collection's work count.
func scanCollection(r scanner, c *Collection) error {
	return r.Scan(&c.ID, &c.Name, &c.Source, &c.Items)
}

// inCollection counts a collection c's works that the Viewer sees in the
// library. Provisional works are never in collections.
var inCollection = `SELECT count(*) FROM collection_work cw JOIN work w ON w.id = cw.work_id
    WHERE cw.collection_id = c.id AND NOT w.provisional
      AND EXISTS (SELECT 1 FROM work_library cl WHERE cl.library_id = @library AND cl.work_id = w.id)
      AND ` + visible

// collectionLetters counts the collections with a visible work in the
// library by the first character of their sort name, for the list's letter
// rail. The character is a code point, or 0 for an empty name. The groups
// come in the list's order, and the counts add up to the list's total.
var collectionLetters = stmt("CollectionLetters", `
SELECT coalesce(unicode(c.sort_name), 0) AS initial, count(*) AS n
FROM collection c
WHERE (`+inCollection+`) > 0
GROUP BY 1
ORDER BY 1`)

// collections lists the collections with a visible work in the library, by
// sort name, with how many of their works the Viewer sees.
var collections = stmt("Collections", `
SELECT `+collectionColumns+`, (`+inCollection+`) AS items
FROM collection c
WHERE (`+inCollection+`) > 0
ORDER BY c.sort_name, c.id
LIMIT @limit OFFSET @offset`)

// seenInCollection counts a collection c's works that the Viewer sees, in
// any library.
var seenInCollection = `SELECT count(*) FROM collection_work cw JOIN work w ON w.id = cw.work_id
    WHERE cw.collection_id = c.id AND NOT w.provisional
      AND ` + visible

// collection is a collection, with how many of its works the Viewer sees,
// in any library.
var collection = stmt("Collection", `
SELECT `+collectionColumns+`, (
    `+seenInCollection+`) AS items
FROM collection c
WHERE c.id = @collection`)

// collectionItems is a collection's works in release order.
var collectionItems = stmt("CollectionItems", `
SELECT `+summaryColumns+`
FROM collection_work cw
JOIN work w ON w.id = cw.work_id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE cw.collection_id = @collection AND NOT w.provisional
  AND `+visible+`
ORDER BY coalesce(w.year, 0), w.sort_title, w.id`)

// Collections returns a page of the collections with works in a library that
// the Viewer sees, by name.
func (s *Service) Collections(ctx context.Context, v users.Viewer, lib id.ID, o GridOptions) (GridPage[Collection], error) {
	vw := viewerOf(v)
	g, err := nameList(ctx, s, vw, lib, o, collectionLetters, collections, scanCollection)
	if err != nil {
		return GridPage[Collection]{}, err
	}
	if err := s.withCollectionImages(ctx, vw, g.Items); err != nil {
		return GridPage[Collection]{}, err
	}
	return g, nil
}

// Collection returns a collection's page, read from one snapshot. The page
// lists all of the collection's works that the Viewer sees. A collection
// with no such works is ErrNotFound.
func (s *Service) Collection(ctx context.Context, v users.Viewer, coll id.ID) (CollectionPage, error) {
	vw := viewerOf(v)
	args := vw.args(sql.Named("collection", coll))
	var p CollectionPage
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := scanCollection(queryRow(ctx, tx, collection, args), &p.Collection); err != nil {
			return notFound(err)
		}
		if p.Collection.Items == 0 {
			return ErrNotFound
		}
		var err error
		p.Items, err = summaries(ctx, tx, collectionItems, args)
		return err
	})
	if err != nil {
		return CollectionPage{}, err
	}
	if err := s.decorate(ctx, vw, p.Items); err != nil {
		return CollectionPage{}, err
	}
	one := []Collection{p.Collection}
	if err := s.withCollectionImages(ctx, vw, one); err != nil {
		return CollectionPage{}, err
	}
	p.Collection = one[0]
	return p, nil
}
