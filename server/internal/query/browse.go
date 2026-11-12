package query

import (
	"context"
	"database/sql"
	"slices"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
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

// Ranges a rule can ask for.
const (
	HDR         Range = "hdr" // HDR10 or HLG
	DolbyVision Range = "dolbyVision"
)

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
