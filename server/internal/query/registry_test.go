package query_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
)

// TestBitsAgreeWithTheView checks the bit each resolution and range has
// against the bits the work_browse_keys view sets, which work_library
// copies: a film of each width and range has its class's bit, and a rule
// on the class selects it.
func TestBitsAgreeWithTheView(t *testing.T) {
	h := newHousehold(t)
	resolutions, ranges := query.Bits()
	films := []struct {
		width  int
		info   map[string]any
		ranges []query.Range
	}{
		{3840, map[string]any{"range": "hdr10"}, []query.Range{query.HDR}},
		{3200, map[string]any{"range": "hlg"}, []query.Range{query.HDR}},
		{3199, map[string]any{"range": "sdr", "dolby_vision_profile": 5}, []query.Range{query.DolbyVision}},
		{1920, map[string]any{"range": "hdr10", "dolby_vision_profile": 8}, []query.Range{query.HDR, query.DolbyVision}},
		{1700, map[string]any{"range": "sdr"}, nil},
		{1699, nil, nil},
		{1200, nil, nil},
		{1199, nil, nil},
		{640, nil, nil},
	}
	seen := map[query.Resolution]bool{}
	for i, f := range films {
		m := h.movie(h.moviesFolder, fmt.Sprintf("Width %d", f.width), 2000+i, rankG)
		track := map[string]any{"width": f.width}
		for k, v := range f.info {
			track[k] = v
		}
		h.tracks(m.File, time.Hour, []catalog.Track{{Index: 0, Type: "video", Codec: "hevc", Info: info(track)}})
		class := query.ResolutionOf(f.width)
		seen[class] = true
		wantResolutions, wantRanges := resolutions[class], int64(0)
		for _, r := range f.ranges {
			wantRanges |= ranges[r]
		}
		for _, source := range []string{"work_browse_keys", "work_library"} {
			var gotResolutions, gotRanges int64
			err := h.b.DB.Reader().QueryRow(`SELECT resolutions, dynamic_ranges FROM `+source+` WHERE work_id = ?`, m.Work).Scan(&gotResolutions, &gotRanges)
			if err != nil || gotResolutions != wantResolutions || gotRanges != wantRanges {
				t.Errorf("%d wide, %v: %s has resolutions %d and ranges %d, %v; want %d (%s) and %d %v",
					f.width, f.info, source, gotResolutions, gotRanges, err, wantResolutions, class, wantRanges, f.ranges)
			}
		}
		rules := []query.Rule{{Field: "title", Op: "startsWith", Values: []string{"width "}}, {Field: "resolution", Op: "anyOf", Values: []string{string(class)}}}
		if len(f.ranges) > 0 {
			var names []string
			for _, r := range f.ranges {
				names = append(names, string(r))
			}
			rules = append(rules, query.Rule{Field: "dynamicRange", Op: "allOf", Values: names})
		} else {
			rules = append(rules, query.Rule{Field: "dynamicRange", Op: "isNotSet"})
		}
		if got := ids(h.runQuery(h.admin, movies(rules...)).Items); !slices.Contains(got, m.Work) {
			t.Errorf("%d wide, %v: the rules %+v select %v, not it", f.width, f.info, rules, got)
		}
	}
	if len(seen) != len(resolutions) || len(ranges) != 2 {
		t.Errorf("the films have %d of the %d resolutions, and there are %d ranges, want 2", len(seen), len(resolutions), len(ranges))
	}
	// The grid's filters read the same bits.
	p, err := h.q.Browse(ctx, h.admin, h.movies, query.BrowseOptions{Filter: query.Filter{
		Resolutions: []query.Resolution{query.Resolution720p}, Ranges: nil}})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, c := range p.Items {
		titles = append(titles, c.Title)
	}
	if !slices.Equal(titles, []string{"Width 1200", "Width 1699"}) {
		t.Errorf("the grid's 720p films: %v", titles)
	}
}

// TestSortIndexes checks each index the registry says holds a sort's order
// against the schema: a library's root rows by the field's column in the
// field's default direction, then by sort title and ID. A page that walks
// the index relies on it.
func TestSortIndexes(t *testing.T) {
	db := storetest.New(t)
	for column, f := range query.IndexedSorts() {
		index := query.SortIndex(column)
		if index == "" {
			t.Errorf("%s sorts by %s, which has no index", f.Field, column)
			continue
		}
		var definition string
		if err := db.Reader().QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&definition); err != nil {
			t.Errorf("%s: %v", index, err)
			continue
		}
		rows, err := db.Reader().Query(`SELECT name, desc FROM pragma_index_xinfo(?) WHERE key ORDER BY seqno`, index)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var name string
			var desc bool
			if err := rows.Scan(&name, &desc); err != nil {
				t.Fatal(err)
			}
			if desc {
				name += " desc"
			}
			got = append(got, name)
		}
		rows.Close()
		want := []string{"library_id", column, "sort_title", "work_id"}
		if f.DefaultDirection == "desc" {
			want[1] += " desc"
		}
		if column == "sort_title" {
			want = []string{"library_id", "sort_title", "work_id"}
		}
		if !slices.Equal(got, want) || !strings.HasSuffix(definition, "WHERE is_root") {
			t.Errorf("%s, for %s, is %v in %q; want %v of the root rows", index, f.Field, got, definition, want)
		}
	}
}

// TestGridSortsAreQueryFields checks that a library grid's sorts and a
// query's sortable fields are one vocabulary: each grid sort orders by the
// sortable field of its name, but for the one sort whose field has another.
func TestGridSortsAreQueryFields(t *testing.T) {
	renamed := map[query.Sort]string{query.ByRuntime: "runtimeMs"}
	fields := query.WorkFields()
	sorts := query.GridSorts()
	if len(sorts) != 9 {
		t.Errorf("the grid has %d sorts, want 9", len(sorts))
	}
	for sort, field := range sorts {
		name := string(sort)
		if other, ok := renamed[sort]; ok {
			name = other
		}
		if field != name || !slices.ContainsFunc(fields, func(f query.WorkField) bool { return f.Field == field && f.Sortable }) {
			t.Errorf("the grid's sort %s orders by %s, want the sortable field %s", sort, field, name)
		}
	}
}

// A page that walks an index lists the works its query's first page
// lists, where that page leaves off: for each sort an index holds, as a
// Viewer with one library of the works, and for a rule naming one library
// of several.
func TestWalkedPagesFollowTheFirst(t *testing.T) {
	db := storetest.New(t)
	movies, shows, _ := generate(t, db, shape{movies: 300, series: 80, seasons: 2, episodes: 3, airEvery: 86400, moviesElsewhere: 60, seriesElsewhere: 20})
	// Titles, dates and ratings repeat, so every sort breaks ties.
	err := db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		for _, stmt := range []string{
			`UPDATE work SET sort_title = printf('Title %d', rid % 40), added_at = 1000 * (rid % 30) WHERE parent_id IS NULL`,
			`UPDATE work_library SET sort_title = (SELECT w.sort_title FROM work w WHERE w.id = work_id),
			   added_at = (SELECT w.added_at FROM work w WHERE w.id = work_id), last_added_at = (SELECT w.rid % 20 FROM work w WHERE w.id = work_id),
			   release_key = (SELECT iif(w.rid % 9, printf('%04d-01-01', 1990 + w.rid % 25), '') FROM work w WHERE w.id = work_id),
			   runtime_sort = (SELECT iif(w.rid % 7, 60000 * (w.rid % 50), 9223372036854775807) FROM work w WHERE w.id = work_id),
			   creator_sort = (SELECT iif(w.rid % 5, printf('Author %d', w.rid % 6), '') FROM work w WHERE w.id = work_id)
			 WHERE is_root`,
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q := query.New(db)
	some, every := budgetViewer(t, db, "some", movies, shows), budgetViewer(t, db, "every")
	var walked int
	query.Observe(t, func(sql string) {
		if strings.Contains(sql, "FROM work_library wl INDEXED BY work_library_") {
			walked++
		}
	})
	inMovies := where(query.Rule{Field: "library", Op: "anyOf", Values: []string{movies.String()}})
	for _, f := range query.IndexedSorts() {
		for _, kind := range []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries} {
			wq := query.WorkQuery{Kinds: []catalog.WorkKind{kind}, Sort: []query.SortKey{{Field: f.Field}}}
			first, err := q.QueryWorks(ctx, some, wq, query.GridOptions{Limit: 200})
			if err != nil {
				t.Fatal(err)
			}
			if first.Total <= 30 || len(first.Items) != min(first.Total, 200) {
				t.Fatalf("%s by %s: %d works of %d", kind, f.Field, len(first.Items), first.Total)
			}
			walked = 0
			var got []id.ID
			for offset := 7; offset+13 <= len(first.Items); offset += 13 {
				p, err := q.QueryWorks(ctx, some, wq, query.GridOptions{Offset: offset, Limit: 13})
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, ids(p.Items)...)
			}
			if want := ids(first.Items)[7 : 7+len(got)]; !slices.Equal(got, want) || walked == 0 {
				t.Errorf("%s by %s: the later pages, %d of them walking an index, list\n%v, and the first page\n%v", kind, f.Field, walked, got, want)
			}
		}
		// Every library's movies are sorted, and one library's walked.
		for _, tt := range []struct {
			where *query.RuleGroup
			walks bool
		}{{nil, false}, {inMovies, true}} {
			wq := query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: tt.where, Sort: []query.SortKey{{Field: f.Field}}}
			first, err := q.QueryWorks(ctx, every, wq, query.GridOptions{Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			walked = 0
			later, err := q.QueryWorks(ctx, every, wq, query.GridOptions{Offset: 40, Limit: 60})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ids(later.Items), ids(first.Items)[40:]) || (walked > 0) != tt.walks {
				t.Errorf("movies by %s, in one library %v: the later page walked %d indexes and lists\n%v, and the first page\n%v",
					f.Field, tt.walks, walked, ids(later.Items), ids(first.Items)[40:])
			}
		}
	}
}
