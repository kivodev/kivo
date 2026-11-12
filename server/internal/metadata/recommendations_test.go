package metadata

import (
	"context"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
)

// recommendations lists a work's stored recommendations in order.
func (e *env) recommendations(work id.ID) []string {
	e.t.Helper()
	return e.strings(`SELECT ext_id FROM work_recommendation WHERE work_id = ? ORDER BY ord`, work)
}

// A movie's and a series' refresh store the titles TMDb recommends, in
// TMDb's order, from the details the refresh fetches anyway. Unmatching
// clears them.
func TestRefreshRecommendations(t *testing.T) {
	e := newEnv(t)
	_, movies := e.b.Library(catalog.Movies)
	movie := e.movie(movies, "Dune (2021)/Dune (2021).mkv")
	_, shows := e.b.Library(catalog.Shows)
	series := e.b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: "The Expanse",
		ExternalIDs: []catalog.ExternalID{catalogID("tmdb", "series", "63639")}})
	e.b.Episode(shows, series, 1, 1)

	e.mustRefresh(movie)
	e.mustRefresh(series)

	if got, want := e.recommendations(movie), []string{"693134", "335984", "329865"}; !slices.Equal(got, want) {
		t.Errorf("the movie's recommendations %v, want %v", got, want)
	}
	if got, want := e.recommendations(series), []string{"1399", "60059"}; !slices.Equal(got, want) {
		t.Errorf("the series' recommendations %v, want %v", got, want)
	}
	for _, p := range []string{"/3/movie/438631/recommendations", "/3/tv/63639/recommendations"} {
		if n := e.fx.count(p); n != 0 {
			t.Errorf("%s requested %d times", p, n)
		}
	}

	// A second refresh writes the same list again.
	e.mustRefresh(movie)
	if got := e.recommendations(movie); len(got) != 3 {
		t.Errorf("after a second refresh: %v", got)
	}

	for _, w := range []struct {
		id    id.ID
		title string
	}{{movie, "Dune"}, {series, "The Expanse"}} {
		e.b.Write(func(ctx context.Context, tx *store.Tx) error {
			return e.b.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error { return b.Unmatch(ctx, w.id, w.title, 0) })
		})
	}
	if got := e.strings(`SELECT ext_id FROM work_recommendation`); len(got) != 0 {
		t.Errorf("after unmatching: %v", got)
	}
}
