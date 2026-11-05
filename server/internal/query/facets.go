package query

import (
	"context"
	"database/sql"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// Facets are the values a library grid's filters offer. They are the values
// that the library's top-level works the Viewer sees actually have.
type Facets struct {
	Genres         []string     // by name
	ContentRatings []string     // youngest first
	Years          []int        // ascending
	Narrators      []PersonRef  // an audiobooks library's narrators, by name
	Resolutions    []Resolution // largest first
	Ranges         []Range      // HDR, then Dolby Vision
	Collections    bool         // whether the library lists any collections
}

var facetGenres = stmt("FacetGenres", `
SELECT DISTINCT g.name
FROM work_library wl
JOIN work w ON w.id = wl.work_id
JOIN work_genre wg ON wg.work_id = w.id
JOIN genre g ON g.id = wg.genre_id
WHERE `+libraryWorks+`
ORDER BY g.name`)

// facetContentRatings lists the content ratings, youngest first by the age
// rank each one means.
var facetContentRatings = stmt("FacetContentRatings", `
SELECT w.content_rating
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE `+libraryWorks+` AND w.content_rating IS NOT NULL
GROUP BY w.content_rating
ORDER BY min(w.content_rating_age), w.content_rating`)

var facetYears = stmt("FacetYears", `
SELECT DISTINCT w.year
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE `+libraryWorks+` AND w.year IS NOT NULL
ORDER BY 1`)

// facetNarrators lists the narrators of the books in the library, by sort
// name.
var facetNarrators = stmt("FacetNarrators", `
SELECT DISTINCT p.id, p.name, p.sort_name
FROM work_library wl
JOIN work w ON w.id = wl.work_id
JOIN credit c ON c.work_id = w.id AND c.role = 'narrator'
JOIN person p ON p.id = c.person_id
WHERE `+libraryWorks+`
ORDER BY p.sort_name, p.id`)

// facetVideo lists the resolution and dynamic range bit sets of the
// library's works, which Facets combines.
var facetVideo = stmt("FacetVideo", `
SELECT DISTINCT wl.resolutions, wl.dynamic_ranges
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE `+libraryWorks)

// facetCollections says whether the library lists any collections.
var facetCollections = stmt("FacetCollections", `
SELECT EXISTS (SELECT 1 FROM collection c WHERE (`+inCollection+`) > 0)`)

// Facets returns a library's filter values, read from one snapshot.
func (s *Service) Facets(ctx context.Context, v users.Viewer, lib id.ID) (Facets, error) {
	vw := viewerOf(v)
	args := vw.args(sql.Named("library", lib))
	f := Facets{Genres: []string{}, ContentRatings: []string{}, Years: []int{}, Narrators: []PersonRef{}, Resolutions: []Resolution{}, Ranges: []Range{}}
	var resolutions, ranges int64
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := library(ctx, tx, vw, lib); err != nil {
			return notFound(err)
		}
		err := each(ctx, tx, facetGenres, args, func(r scanner) error {
			var g string
			err := r.Scan(&g)
			f.Genres = append(f.Genres, g)
			return err
		})
		if err != nil {
			return err
		}
		err = each(ctx, tx, facetContentRatings, args, func(r scanner) error {
			var c string
			err := r.Scan(&c)
			f.ContentRatings = append(f.ContentRatings, c)
			return err
		})
		if err != nil {
			return err
		}
		err = each(ctx, tx, facetYears, args, func(r scanner) error {
			var y int
			err := r.Scan(&y)
			f.Years = append(f.Years, y)
			return err
		})
		if err != nil {
			return err
		}
		err = each(ctx, tx, facetNarrators, args, func(r scanner) error {
			var p PersonRef
			var sortName string
			err := r.Scan(&p.ID, &p.Name, &sortName)
			f.Narrators = append(f.Narrators, p)
			return err
		})
		if err != nil {
			return err
		}
		err = each(ctx, tx, facetVideo, args, func(r scanner) error {
			var res, rng int64
			err := r.Scan(&res, &rng)
			resolutions |= res
			ranges |= rng
			return err
		})
		if err != nil {
			return err
		}
		return rowOf(ctx, tx, facetCollections, args, &f.Collections)
	})
	if err != nil {
		return Facets{}, err
	}
	for _, r := range []Resolution{Resolution4K, Resolution1080p, Resolution720p, ResolutionSD} {
		if resolutions&resolutionBits[r] != 0 {
			f.Resolutions = append(f.Resolutions, r)
		}
	}
	for _, r := range []Range{HDR, DolbyVision} {
		if ranges&rangeBits[r] != 0 {
			f.Ranges = append(f.Ranges, r)
		}
	}
	return f, nil
}
