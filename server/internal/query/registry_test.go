package query_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/query"
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
	p, err := h.browse(h.admin, h.movies, grid{Filter: filter{
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
// field's default direction, then by sort title and ID, and last is_root,
// which a read of the root rows names. A page that walks the index relies
// on its order, and reads the index alone.
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
		want := []string{"library_id", column, "sort_title", "work_id", "is_root"}
		if f.DefaultDirection == "desc" {
			want[1] += " desc"
		}
		if column == "sort_title" {
			want = []string{"library_id", "sort_title", "work_id", "is_root"}
		}
		if !slices.Equal(got, want) || !strings.HasSuffix(definition, "WHERE is_root") {
			t.Errorf("%s, for %s, is %v in %q; want %v of the root rows", index, f.Field, got, definition, want)
		}
	}
}

// TestGridSortsAreQueryFields checks that each sort a library's grid
// offers is a sortable field of a query.
func TestGridSortsAreQueryFields(t *testing.T) {
	fields := query.WorkFields()
	for _, sort := range gridSorts {
		if !slices.ContainsFunc(fields, func(f query.WorkField) bool { return f.Field == sort && f.Sortable }) {
			t.Errorf("the grid's sort %s is no sortable field", sort)
		}
	}
}
