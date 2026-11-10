package query_test

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store/storetest"
)

// TestBrowseMatchesOld reads library grids through Browse and through the
// statements the grid ran before it ran a query, on the same generated
// works, and checks that both list the same works in the same order, count
// the same total, and start the letters at the same places. The matrix is
// every sort over every filter alone and several together, in each
// library, at first pages, later ones and pages past the end, as Viewers
// with every, some and no libraries, with and without a rating ceiling and
// unrated works.
//
// A later page of Browse has no total, so its total is checked to be 0.
// BrowseOld's comment says where the old grid is made to mean what Browse
// now means on purpose.
//
// The whole matrix takes minutes, so it runs only when KIVO_DIFFERENTIAL=1.
// Otherwise one read in 31 runs, which still takes each sort, filter and
// page through several Viewers and libraries, or one in 311 under the race
// detector.
func TestBrowseMatchesOld(t *testing.T) {
	db := storetest.New(t)
	d := newDifferential(t, db)
	svc := query.New(db)
	every := 31
	switch {
	case os.Getenv("KIVO_DIFFERENTIAL") == "1":
		every = 1
	case raceDetector:
		every = 311
	}
	sorts := []query.Sort{"", query.ByTitle, query.ByAdded, query.ByRelease, query.ByCommunityRating, query.ByMyRating,
		query.ByRuntime, query.ByLastPlayed, query.ByAuthor, query.ByLastEpisodeAdded}
	if len(sorts) != len(query.GridSorts())+1 {
		t.Fatalf("the matrix has %d sorts, and the grid %d", len(sorts)-1, len(query.GridSorts()))
	}
	resolutions := func(rs ...query.Resolution) query.Filter { return query.Filter{Resolutions: rs} }
	filters := []query.Filter{
		{},
		{Played: query.Unplayed}, {Played: query.InProgress}, {Played: query.Played},
		{Genres: []string{"Drama"}}, {Genres: []string{"Comedy", "Action"}}, {Genres: []string{"Western"}},
		{YearFrom: 1990}, {YearTo: 1980}, {YearFrom: 1965, YearTo: 2000}, {YearFrom: 2001, YearTo: 2001},
		{ContentRatings: []string{"PG-13"}}, {ContentRatings: []string{"G", "R", "TV-MA"}},
		resolutions(query.Resolution4K), resolutions(query.Resolution1080p), resolutions(query.Resolution720p),
		resolutions(query.ResolutionSD), resolutions(query.Resolution4K, query.ResolutionSD),
		{Ranges: []query.Range{query.HDR}}, {Ranges: []query.Range{query.DolbyVision}}, {Ranges: []query.Range{query.HDR, query.DolbyVision}},
		{Unmatched: true},
		{Narrator: blobID('p', 6)},
		{Played: query.Played, Genres: []string{"Drama", "Horror"}},
		{Unmatched: true, YearFrom: 1970, Played: query.Unplayed},
		{Genres: []string{"Comedy"}, YearFrom: 1960, YearTo: 2010, ContentRatings: []string{"PG-13", "R"}, Resolutions: []query.Resolution{query.Resolution1080p, query.Resolution4K}},
		{Narrator: blobID('p', 5), Played: query.InProgress},
		{Ranges: []query.Range{query.HDR}, Resolutions: []query.Resolution{query.Resolution720p}, YearTo: 1999},
	}
	pages := []query.GridOptions{{}, {Limit: 7}, {Offset: 5, Limit: 7}, {Offset: 3, Limit: 1}, {Offset: 40, Limit: 200}, {Offset: 100000, Limit: 50}}
	libraries := []string{"movies1", "movies2", "shows1", "shows2", "videos", "books"}

	var ran, found, rows, hidden, differed, n int
	for _, v := range d.viewers {
	libraries:
		for _, name := range libraries {
			lib := d.libs[name]
			for _, sort := range sorts {
				for fi, filter := range filters {
					for _, page := range pages {
						if n++; n%every != 0 {
							continue
						}
						o := query.BrowseOptions{Sort: sort, Filter: filter, GridOptions: page}
						at := fmt.Sprintf("as %s, %s by %q, filter %d %+v, page %+v", v.name, name, sort, fi, filter, page)
						want, wantTotal, wantLetters, wantErr := svc.BrowseOld(ctx, v.v, lib, o)
						got, err := svc.Browse(ctx, v.v, lib, o)
						ran++
						if wantErr != nil || err != nil {
							// A Viewer without the library finds none, whatever
							// the grid.
							if !errors.Is(wantErr, query.ErrNotFound) || !errors.Is(err, query.ErrNotFound) {
								t.Fatalf("%s: %v, and the old grid %v", at, err, wantErr)
							}
							hidden++
							continue libraries
						}
						rows += len(want)
						if len(want) > 0 {
							found++
						}
						if page.Offset > 0 {
							wantTotal = 0
						}
						if !slices.Equal(ids(got.Items), want) || got.Total != wantTotal ||
							(got.Letters == nil) != (wantLetters == nil) || !slices.Equal(got.Letters, wantLetters) {
							t.Errorf("%s:\n got %d of %d, letters %v\nwant %d of %d, letters %v", at,
								len(got.Items), got.Total, got.Letters, len(want), wantTotal, wantLetters)
							if differed++; differed == 10 {
								t.Fatal("and more")
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d grid reads compared, %d of libraries the Viewer has none of, %d with works, %d works in all", ran, hidden, found, rows)
	if found < ran/8 {
		t.Errorf("only %d of %d grid reads listed any work: the matrix compares too little", found, ran)
	}
}
