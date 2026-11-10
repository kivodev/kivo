package query_test

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store/storetest"
	"github.com/mmcdole/kivo/internal/users"
)

// TestQueryReadsAgree runs a matrix of queries on generated works and
// checks that every way a query is read gives the same works in the same
// order. A query's first page reads all of its works and sorts them. A
// later page reads no further than its end where it can: through the
// order's index, or from the user's own rows. So the matrix reads each
// query whole, as a first page, and checks against that:
//
//   - its total is how many works it has;
//   - its pages, read as later pages are, are runs of it, to its end and
//     past it;
//   - with a cap it is the first works up to the cap, and counts them;
//   - narrowed to kinds it is its works of those kinds, after any cap;
//   - each of its works is one the Viewer may see, by the rules as sees
//     has them, and a query without rules has every such work of its kinds;
//   - a query that reads nothing of the user's own has, for any Viewer,
//     the works it has for one who sees everything, less those hidden.
//
// The matrix is every field with every operator, groups and every sort
// both ways, over each set of kinds, as Viewers with every, some and no
// libraries, with and without a rating ceiling and unrated works. It runs
// one query in 97, and besides those every sort a page can be read in the
// order of, with each rule in turn, as the Viewers with one library of a
// kind: the reads whose pages take another way than their first. Under the
// race detector it runs a tenth of the first and a fifth of the second.
func TestQueryReadsAgree(t *testing.T) {
	db := storetest.New(t)
	g := newGenerated(t, db)
	svc := query.New(db)
	canonical := canonicalFilter(t)
	now := time.Now()
	every := 97
	if raceDetector {
		every = 971
	}
	// read is a query's works, whole: its first page of any length, and
	// its total.
	read := func(q query.WorkQuery, v users.Viewer, only []catalog.WorkKind) ([]id.ID, int) {
		counted, _, args := svc.Compiled(t, q, v, now, only)
		checkCompiledFilter(t, canonical, counted)
		return readCompiled(t, db.Reader(), counted, paged(args, 100000, 0), true)
	}
	var ran, found, walked, keyed int
	check := func(name string, v users.Viewer, sees map[id.ID]catalog.WorkKind, q query.WorkQuery, n int) {
		t.Helper()
		if problems := q.Validate(); len(problems) > 0 {
			t.Fatalf("%s: %v", describe(q), problems)
		}
		at := fmt.Sprintf("as %s, %s", name, describe(q))
		all, total := read(q, v, nil)
		ran++
		if len(all) > 0 {
			found++
		}
		if total != len(all) {
			t.Errorf("%s: the total is %d, of %d works", at, total, len(all))
		}

		// The Viewer.
		visible := 0
		for _, kind := range sees {
			if slices.Contains(q.Kinds, kind) {
				visible++
			}
		}
		if slices.ContainsFunc(all, func(work id.ID) bool { _, ok := sees[work]; return !ok }) {
			t.Errorf("%s: has a work the Viewer may not see", at)
		}
		if q.Where == nil && len(all) != visible {
			t.Errorf("%s: has %d works, and the Viewer may see %d of its kinds", at, len(all), visible)
		}
		if !readsOwn(q) {
			everything, _ := read(q, g.viewers[0].v, nil)
			want := slices.DeleteFunc(everything, func(work id.ID) bool { _, ok := sees[work]; return !ok })
			if !slices.Equal(all, want) {
				t.Errorf("%s: has %d works, and %d of those of a Viewer who sees everything", at, len(all), len(want))
			}
		}

		// Pages.
		_, page, args := svc.Compiled(t, q, v, now, nil)
		checkCompiledFilter(t, canonical, page)
		switch {
		case strings.Contains(page, "FROM user_state us\nCROSS JOIN") || strings.Contains(page, "FROM played pl\nCROSS JOIN"):
			keyed++
		case strings.Contains(page, "INDEXED BY work_library_"):
			walked++
		}
		for _, p := range [][2]int{{100000, 0}, {7, 0}, {5, 3}, {4, len(all) - 2}, {50, len(all)}} {
			limit, offset := p[0], max(p[1], 0)
			got, _ := readCompiled(t, db.Reader(), page, paged(args, limit, offset), false)
			if want := run(all, offset, limit); !slices.Equal(got, want) {
				t.Errorf("%s: the page of %d at %d has %d works, want %d, or others, in %s", at, limit, offset, len(got), len(want), page)
			}
		}

		// A list page's kinds, and a cap.
		only := [][]catalog.WorkKind{q.Kinds[:1], {catalog.KindEpisode, catalog.KindMovie}, {}}[n%3]
		ofKinds := func(works []id.ID) []id.ID {
			return slices.DeleteFunc(slices.Clone(works), func(work id.ID) bool { return len(only) > 0 && !slices.Contains(only, sees[work]) })
		}
		if got, total := read(q, v, only); !slices.Equal(got, ofKinds(all)) || total != len(got) {
			t.Errorf("%s: of %v it has %d works and the total %d, want %d", at, only, len(got), total, len(ofKinds(all)))
		}
		if len(q.Sort) > 0 && q.Sort[0].Field == "random" {
			return // a cap is part of what seeds an unsaved query's shuffle
		}
		limit := []int{1, 7, 40, 5000}[n%4]
		q.MaxWorks = &limit
		want := run(all, 0, limit)
		if got, total := read(q, v, nil); !slices.Equal(got, want) || total != len(want) {
			t.Errorf("%s: capped at %d it has %d works and the total %d, want %d", at, limit, len(got), total, len(want))
		}
		if got, total := read(q, v, only); !slices.Equal(got, ofKinds(want)) || total != len(got) {
			t.Errorf("%s: capped at %d, of %v it has %d works and the total %d, want %d", at, limit, only, len(got), total, len(ofKinds(want)))
		}
		_, page, args = svc.Compiled(t, q, v, now, nil)
		if got, _ := readCompiled(t, db.Reader(), page, paged(args, 3, 2), false); !slices.Equal(got, run(want, 2, 3)) {
			t.Errorf("%s: capped at %d, the page of 3 at 2 has %d works, want %d, or others", at, limit, len(got), len(run(want, 2, 3)))
		}
	}
	for vi, v := range g.viewers {
		sees := g.sees(t, v.v)
		for ki, kinds := range generatedKinds {
			for qi, q := range g.queries {
				n := vi*len(generatedKinds)*len(g.queries) + ki*len(g.queries) + qi
				if n%every != 0 {
					continue
				}
				q.Kinds = kinds
				// Rules take each sort in turn, and sorts each rule.
				if q.Sort == nil {
					q.Sort = g.sorts[n%len(g.sorts)]
				} else if q.Where == nil && n%2 == 0 {
					q.Where = where(g.rules[n%len(g.rules)])
				}
				check(v.name, v.v, sees, q, n)
			}
		}
		if len(v.v.Libraries()) == 0 {
			continue
		}
		// The orders a page can be read in, of each kind a library's grid
		// holds: these Viewers have one library of a kind.
		n := vi
		for _, kind := range []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindVideo, catalog.KindBook} {
			for _, f := range query.WorkFields() {
				if !f.Sortable {
					continue
				}
				if n++; raceDetector && n%5 != 0 {
					continue
				}
				q := query.WorkQuery{Kinds: []catalog.WorkKind{kind}, Sort: []query.SortKey{{Field: f.Field}}}
				if n%3 > 0 {
					q.Where = where(g.rules[n*7%len(g.rules)])
				}
				check(v.name, v.v, sees, q, n)
			}
		}
	}
	t.Logf("%d queries read, %d with works; %d paged through an index and %d from the user's rows", ran, found, walked, keyed)
	if found < ran/4 || walked == 0 || keyed == 0 {
		t.Errorf("the matrix compares too little")
	}
}

// TestGridReadsAgree reads library grids on generated works and checks
// each against the grid's query, read whole across every library with a
// rule for the grid's own: the grid has the same works in the same order,
// its first page counts them and has the letters when it is sorted by
// title, and its later pages are runs of them with no total. A grid's
// pages read no further than their end where they can, and the query read
// whole sorts every work, so the two are read in different ways.
//
// The matrix is every sort over every filter alone and several together,
// in each library, as TestQueryReadsAgree's Viewers. It runs one grid in
// 29, or one in 293 under the race detector.
func TestGridReadsAgree(t *testing.T) {
	db := storetest.New(t)
	g := newGenerated(t, db)
	svc := query.New(db)
	now := time.Now()
	every := 29
	if raceDetector {
		every = 293
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
	libraries := []struct {
		name string
		kind catalog.WorkKind
	}{{"movies1", catalog.KindMovie}, {"movies2", catalog.KindMovie}, {"shows1", catalog.KindSeries}, {"shows2", catalog.KindSeries},
		{"videos", catalog.KindVideo}, {"books", catalog.KindBook}}
	unmatched := map[id.ID]bool{}
	rows, err := db.Reader().Query(`SELECT id FROM work WHERE provisional`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var work id.ID
		if err := rows.Scan(&work); err != nil {
			t.Fatal(err)
		}
		unmatched[work] = true
	}

	var ran, found, hidden, n int
	for _, v := range g.viewers {
		for _, l := range libraries {
			lib := g.libs[l.name]
			for _, sort := range sorts {
				for fi, filter := range filters {
					if n++; n%every != 0 {
						continue
					}
					o := query.BrowseOptions{Sort: sort, Filter: filter}
					at := fmt.Sprintf("as %s, %s by %q, filter %d %+v", v.name, l.name, sort, fi, filter)
					ran++
					first, err := svc.Browse(ctx, v.v, lib, query.BrowseOptions{Sort: sort, Filter: filter, GridOptions: query.GridOptions{Limit: 200}})
					if !v.v.AllLibraries() && !slices.Contains(v.v.Libraries(), lib) {
						// A Viewer without the library finds none, whatever the grid.
						if !errors.Is(err, query.ErrNotFound) {
							t.Fatalf("%s: %v, want not found", at, err)
						}
						hidden++
						continue
					}
					if err != nil {
						t.Fatalf("%s: %v", at, err)
					}

					// The grid's query, in every library, and sorted by a
					// second key that changes no order, so that it is read
					// whole and sorted.
					q := query.GridQuery(t, o)
					q.Kinds = []catalog.WorkKind{l.kind}
					in := query.Rule{Field: "library", Op: "anyOf", Values: []string{lib.String()}}
					if q.Where == nil {
						q.Where = where(in)
					} else {
						q.Where.Rules = append(q.Where.Rules, in)
					}
					q.Sort = append(q.Sort, query.SortKey{Field: "title"})
					counted, _, args := svc.Compiled(t, q, v.v, now, nil)
					want, _ := readCompiled(t, db.Reader(), counted, paged(args, 100000, 0), true)
					if filter.Unmatched {
						want = slices.DeleteFunc(want, func(work id.ID) bool { return !unmatched[work] })
					}
					if len(want) > 0 {
						found++
					}

					titled := sort == "" || sort == query.ByTitle
					if !slices.Equal(ids(first.Items), want) || first.Total != len(want) || (first.Letters != nil) != titled {
						t.Fatalf("%s: has %d works, the total %d and the letters %v, want %d works", at, len(first.Items), first.Total, first.Letters, len(want))
					}
					for _, p := range []query.GridOptions{{Limit: 7}, {Offset: 7, Limit: 7}, {Offset: 3, Limit: 1},
						{Offset: max(len(want)-2, 1), Limit: 5}, {Offset: 100000, Limit: 50}} {
						o.GridOptions = p
						got, err := svc.Browse(ctx, v.v, lib, o)
						if err != nil {
							t.Fatalf("%s, page %+v: %v", at, p, err)
						}
						total := 0
						if p.Offset == 0 {
							total = len(want)
						}
						if !slices.Equal(ids(got.Items), run(want, p.Offset, p.Limit)) || got.Total != total || (got.Letters != nil) != (titled && p.Offset == 0) {
							t.Fatalf("%s, page %+v: has %d works, the total %d and the letters %v, want %d of %d", at, p,
								len(got.Items), got.Total, got.Letters, len(run(want, p.Offset, p.Limit)), len(want))
						}
					}
				}
			}
		}
	}
	t.Logf("%d grids read, %d of libraries the Viewer has none of, %d with works", ran, hidden, found)
	if found < ran/8 {
		t.Errorf("only %d of %d grids listed any work: the matrix compares too little", found, ran)
	}
}

// run is the works of a page of limit at offset.
func run(works []id.ID, offset, limit int) []id.ID {
	offset = min(offset, len(works))
	return works[offset:min(offset+limit, len(works))]
}

// readsOwn reports whether q reads what is the user's own, so that two
// Viewers who see the same works may still have different ones.
func readsOwn(q query.WorkQuery) bool {
	text := describe(q)
	return slices.ContainsFunc([]string{"playState", "myRating", "lastPlayedAt", "inList"}, func(field string) bool {
		return strings.Contains(text, `"`+field+`"`)
	})
}

// sees is the works in a library that v may see, with their kinds, by the
// Viewer's rules as Go: a work must be in a library v is granted, and its
// rating rank, its own or else its series', must be within v's ceiling. A
// work with no rank is seen unless v blocks unrated works and it is video.
func (g *generated) sees(t *testing.T, v users.Viewer) map[id.ID]catalog.WorkKind {
	t.Helper()
	rows, err := g.db.Reader().Query(`
		SELECT w.id, w.kind, wl.library_id, w.content_rating_age, r.content_rating_age
		FROM work w JOIN work_library wl ON wl.work_id = w.id JOIN work r ON r.id = w.root_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[id.ID]catalog.WorkKind{}
	ceiling, capped := v.RatingCeiling()
	for rows.Next() {
		var work, lib id.ID
		var kind catalog.WorkKind
		var own, root sql.NullInt64
		if err := rows.Scan(&work, &kind, &lib, &own, &root); err != nil {
			t.Fatal(err)
		}
		if !v.AllLibraries() && !slices.Contains(v.Libraries(), lib) {
			continue
		}
		age := own
		if !age.Valid {
			age = root
		}
		if age.Valid && capped && age.Int64 > ceiling || !age.Valid && !v.ShowsUnrated() && kind != catalog.KindBook {
			continue
		}
		out[work] = kind
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
