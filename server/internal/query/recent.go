package query

import (
	"bytes"
	"cmp"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// runGap is how far apart a show's additions may be and still share a
// "Recently Added" summary.
const runGap = 24 * time.Hour

// addition is a row of RecentlyAdded: a work the library added, with its
// season's and series' sort titles, and whether the user played it.
type addition struct {
	ID, ParentID, RootID                        id.ID
	Kind                                        catalog.WorkKind
	AddedAt                                     int64
	SortTitle, SeasonSortTitle, SeriesSortTitle string
	SeriesProvisional, Played                   bool
}

// bundle is a "Recently Added" summary before its summary is read.
type bundle struct {
	work      id.ID // the work the summary shows
	at        int64 // the newest addition, in Unix milliseconds
	sortTitle string
	episodes  int // how many episodes a season's or series' summary bundles
	// unmatched marks a season's summary of a provisional series. Seasons
	// are never provisional themselves, so the summary takes its series'
	// state.
	unmatched bool
	played    bool // the user played every work the summary covers
}

// bundles groups a library's newest additions into "Recently Added"
// summaries.
//
// A movie, video or book is one summary. A show's episodes are split into
// runs, newest first, where each addition is within runGap of the one
// before it. A run of one episode becomes that episode's summary. A run
// within one season becomes the season's summary, and a run across seasons
// becomes the series' summary.
//
// A summary whose works the user has all played is left out, so a show's
// older run onto the same summary can take its place.
//
// Summaries are ordered by their newest addition, then title, then ID, so
// a batch added together reads alphabetically.
func bundles(rows []addition) []bundle {
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b addition) int { return cmp.Compare(b.AddedAt, a.AddedAt) })

	var out []bundle
	seen := map[id.ID]bool{}
	add := func(b bundle) {
		if !b.played && !seen[b.work] { // skip a show's older run that lands on a summary already added
			seen[b.work] = true
			out = append(out, b)
		}
	}
	var shows []id.ID
	episodes := map[id.ID][]addition{}
	for _, r := range rows {
		switch {
		case r.Kind == catalog.KindEpisode:
			if episodes[r.RootID] == nil {
				shows = append(shows, r.RootID)
			}
			episodes[r.RootID] = append(episodes[r.RootID], r)
		default:
			add(bundle{work: r.ID, at: r.AddedAt, sortTitle: r.SortTitle, played: r.Played})
		}
	}
	for _, show := range shows {
		eps := episodes[show]
		start := 0
		for i := range eps {
			if i+1 == len(eps) || eps[i].AddedAt-eps[i+1].AddedAt > runGap.Milliseconds() {
				add(episodeRun(eps[start : i+1]))
				start = i + 1
			}
		}
	}
	slices.SortFunc(out, func(a, b bundle) int {
		return cmp.Or(cmp.Compare(b.at, a.at), cmp.Compare(a.sortTitle, b.sortTitle), bytes.Compare(a.work[:], b.work[:]))
	})
	return out
}

// episodeRun returns the summary for one run of a show's episodes, given
// newest first.
func episodeRun(eps []addition) bundle {
	first := eps[0]
	played := !slices.ContainsFunc(eps, func(e addition) bool { return !e.Played })
	if len(eps) == 1 {
		return bundle{work: first.ID, at: first.AddedAt, sortTitle: first.SortTitle, played: played}
	}
	for _, e := range eps[1:] {
		if e.ParentID != first.ParentID {
			return bundle{work: first.RootID, at: first.AddedAt, sortTitle: first.SeriesSortTitle, episodes: len(eps), played: played}
		}
	}
	return bundle{work: first.ParentID, at: first.AddedAt, sortTitle: first.SeasonSortTitle, episodes: len(eps),
		unmatched: first.SeriesProvisional, played: played}
}
