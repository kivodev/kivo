package query_test

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
	"github.com/mmcdole/kivo/internal/users"
)

// largeTables grow with the library or with use. A query may search them
// through an index, but must never scan one whole.
var largeTables = []string{
	"work", "work_library", "work_external_id", "media_file", "media_track", "media_chapter", "media_attachment", "media_version",
	"media_version_work", "person", "credit", "work_genre", "work_tag", "work_company", "collection_work",
	"user_state", "play_event", "match", "image", "artwork", "provider_cache", "scan_dir", "activity",
	"book", "book_series", "book_series_book", "work_recommendation",
}

// wholeLibrary lists the statements allowed to scan a large table, keyed by
// package/file or by statement key, with the reason. They are the nightly
// and whole-library tasks, which visit every row anyway, counts over a
// partial index that holds only the rows counted, the rule builder's reads
// of a field's values, and the test suite's
// invariants.
var wholeLibrary = map[string]string{
	"users/users.sql.SavedListQueries":  "once at startup, to log smart lists that no longer compile",
	"catalog/invariants.sql":            "CheckInvariants, run after every test",
	"previews/previews.sql.Pending":     "what the previews task has left, or Make preview thumbnails on a library",
	"subtitles/subtitles.sql.Pending":   "media_track_extracted holds only ASS, SSA and PGS tracks",
	"markers/markers.sql.CountPending":  "how many files the markers task has left, for Tasks",
	"markers/markers.sql.MarkerSeasons": "what the markers task has left, or Find intro and credits on a library",
	"markers/markers.sql.PruneMarkers":  "the nightly cache prune",
	"scan/scan.sql.AllDirs":             "the file watcher's startup listing of every directory",
	"query/MissingCount":                "the Missing files total: media_file_gone holds only gone files",
	"catalog/work.sql.EmptyWorks":       "the nightly deletion of works with nothing left",
	"scan/scan.sql.Behind":              "media_file_behind holds only files a scan hasn't caught up with",
	"scan/scan.sql.ProbeFailures":       "media_file_probe_failed holds only files that failed to probe",
	"metadata/work.sql.FailedRefreshes": "work_refresh_failed holds only works whose refresh failed",
	"query/ValuesActor":                 "a field's values for a rule, read when the builder opens the field",
	"query/ValuesAuthor":                "a field's values for a rule, read when the builder opens the field",
	"query/ValuesBookSeries":            "a field's values for a rule, read when the builder opens the field",
	"query/ValuesCollection":            "a field's values for a rule, read when the builder opens the field",
	"query/ValuesCompany":               "a field's values for a rule, read when the builder opens the field",
	"query/ValuesContentRating":         "a field's values for a rule, read when the builder opens the field",
	"query/ValuesDirector":              "a field's values for a rule, read when the builder opens the field",
	"query/ValuesGenre":                 "a field's values for a rule, read when the builder opens the field",
	"query/ValuesNarrator":              "a field's values for a rule, read when the builder opens the field",
	"query/ValuesTag":                   "a field's values for a rule, read when the builder opens the field",

	"identify/catalog.sql.CountUnidentified": "how many files the identify task has left, for Tasks: media_file_behind holds only files not yet probed or identified",
}

var (
	narg      = regexp.MustCompile(`sqlc\.(?:narg|slice)\('(\w+)'\)`)
	embed     = regexp.MustCompile(`sqlc\.embed\((\w+)\)`)
	paramName = regexp.MustCompile(`@(\w+)`)
	alias     = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(\w+)(?:\s+(?:AS\s+)?(\w+))?`)
	fullScan  = regexp.MustCompile(`^SCAN (\w+)(?: USING (?:COVERING )?INDEX \w+)?$`)
)

// TestQueryPlans checks the EXPLAIN QUERY PLAN of every package's
// statements, both sqlc's queries and query's own. It fails on a full scan
// of a large table, by table or by index, outside the whole-library tasks.
// Each @name is bound to NULL.
func TestQueryPlans(t *testing.T) {
	db := storetest.New(t)
	for _, st := range append(statements(t, otherQueries), own()...) {
		t.Run(st.key(), func(t *testing.T) {
			stmt := embed.ReplaceAllString(narg.ReplaceAllString(st.sql, "@$1"), "$1.*")
			var args []any
			for _, m := range paramName.FindAllStringSubmatch(stmt, -1) {
				if !slices.ContainsFunc(args, func(a any) bool { return a.(sql.NamedArg).Name == m[1] }) {
					args = append(args, sql.Named(m[1], nil))
				}
			}
			for range strings.Count(stmt, "?") {
				args = append(args, nil)
			}
			checkScans(t, db.Reader(), st, stmt, args)
		})
	}
}

// smartSample is a WorkQuery the plan and Viewer tests compile.
type smartSample struct {
	name string
	q    query.WorkQuery
}

// smartSamples are queries over the fixture that between them use each
// kind of field, groups, a cap, and kinds with and without a root above
// them.
func smartSamples(h *household) []smartSample {
	mine := h.lists(h.admin)[0].ID
	movies := []catalog.WorkKind{catalog.KindMovie}
	shows := []catalog.WorkKind{catalog.KindSeries, catalog.KindSeason, catalog.KindEpisode}
	ten := 10
	samples := []smartSample{
		{"title", query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "title", Op: "contains", Values: []string{"toy"}})}},
		{"year", query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "year", Op: "between", Values: []string{"1990", "2020"}}),
			Sort: []query.SortKey{{Field: "year"}}}},
		{"playState", query.WorkQuery{Kinds: shows, Where: where(query.Rule{Field: "playState", Op: "anyOf", Values: []string{"inProgress"}}),
			Sort: []query.SortKey{{Field: "lastPlayedAt"}}}},
		{"people", query.WorkQuery{Kinds: movies, Where: where(
			query.Rule{Field: "genre", Op: "allOf", Values: []string{"Adventure", "Science Fiction"}},
			query.Rule{Field: "actor", Op: "anyOf", Values: []string{h.villeneuve.String()}},
			query.Rule{Field: "director", Op: "noneOf", Values: []string{h.villeneuve.String()}})}},
		{"resolution", query.WorkQuery{Kinds: movies, Where: where(
			query.Rule{Field: "resolution", Op: "anyOf", Values: []string{"4k"}},
			query.Rule{Field: "dynamicRange", Op: "allOf", Values: []string{"hdr"}}),
			Sort: []query.SortKey{{Field: "addedAt"}}}},
		{"within", query.WorkQuery{Kinds: movies, Where: where(
			query.Rule{Field: "releaseDate", Op: "within", Values: []string{"P30D"}},
			query.Rule{Field: "lastPlayedAt", Op: "notWithin", Values: []string{"P1Y"}}),
			Sort: []query.SortKey{{Field: "releaseDate"}}}},
		{"any", query.WorkQuery{Kinds: movies, Where: &query.RuleGroup{Match: "any", Rules: []query.Rule{
			{Field: "rating", Op: "gte", Values: []string{"8"}},
			{Field: "myRating", Op: "gte", Values: []string{"4"}}}},
			Sort: []query.SortKey{{Field: "rating"}, {Field: "myRating"}}}},
		{"nested", query.WorkQuery{Kinds: movies, Where: &query.RuleGroup{Match: "all",
			Rules: []query.Rule{{Field: "library", Op: "anyOf", Values: []string{h.movies.String()}}},
			Groups: []query.RuleGroup{
				{Match: "any", Rules: []query.Rule{{Field: "tag", Op: "anyOf", Values: []string{"Space"}}, {Field: "company", Op: "isSet"}}},
				{Match: "any", Rules: []query.Rule{{Field: "collection", Op: "anyOf", Values: []string{h.toyStoryCollection.String()}},
					{Field: "inList", Op: "anyOf", Values: []string{mine.String()}}}},
			}},
			Sort: []query.SortKey{{Field: "random"}}}},
		{"maxWorks", query.WorkQuery{Kinds: movies, Sort: []query.SortKey{{Field: "addedAt"}}, MaxWorks: &ten}},
		{"series", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindSeries}, Where: where(query.Rule{Field: "contentRating", Op: "noneOf", Values: []string{"TV-MA"}}),
			Sort: []query.SortKey{{Field: "lastEpisodeAddedAt"}}}},
		{"books", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindBook}, Where: where(
			query.Rule{Field: "author", Op: "anyOf", Values: []string{h.weir.String()}},
			query.Rule{Field: "bookSeries", Op: "isNotSet"}),
			Sort: []query.SortKey{{Field: "runtimeMs"}}}},
	}
	for _, s := range samples {
		if problems := s.q.Validate(); len(problems) > 0 {
			h.t.Fatalf("%s: %v", s.name, problems)
		}
	}
	return samples
}

// TestSmartQueryPlans checks the statements smart lists compile to at run
// time as TestQueryPlans checks the fixed ones. Each sample compiles as a
// Viewer granted some libraries, alone and narrowed to its kinds as a
// list's page narrows it.
func TestSmartQueryPlans(t *testing.T) {
	h := newHousehold(t)
	now := time.Now()
	for _, s := range smartSamples(h) {
		for _, only := range [][]catalog.WorkKind{nil, s.q.Kinds} {
			counted, page, args := h.q.Compiled(t, s.q, h.friend, now, only)
			prefix := ""
			if only != nil {
				prefix = "only/"
			}
			file := "smart/" + s.name
			for _, st := range []statement{{"query", file, prefix + "counted", counted}, {"query", file, prefix + "page", page}} {
				t.Run(st.key(), func(t *testing.T) { checkScans(t, h.b.DB.Reader(), st, st.sql, args) })
			}
		}
	}
}

// gridSample is a library grid the plan and Viewer tests compile.
type gridSample struct {
	name string
	lib  id.ID
	o    grid
}

// compiled is the statements the grid's read compiles to as the friend.
func (s gridSample) compiled(h *household) (counted, page string, args []any) {
	return h.q.CompiledIn(s.o.query(kindIn(h.b.DB, s.lib)), h.friend, query.Scope{Library: s.lib, Unmatched: s.o.Filter.Unmatched})
}

// gridSamples are the grids of a movies and a shows library by each sort,
// bare and with every filter at once.
func gridSamples(h *household) []gridSample {
	every := filter{Played: "inProgress", Genres: []string{"Drama", "Crime"}, YearFrom: 1990, YearTo: 2020,
		ContentRatings: []string{"PG-13"}, Resolutions: []query.Resolution{query.Resolution4K}, Ranges: []query.Range{query.HDR},
		Unmatched: true, Narrator: h.porter}
	var out []gridSample
	for _, sort := range gridSorts {
		for name, lib := range map[string]id.ID{"movies": h.movies, "shows": h.shows} {
			out = append(out,
				gridSample{name + " by " + sort, lib, grid{Sort: sort}},
				gridSample{name + " by " + sort + ", filtered", lib, grid{Sort: sort, Filter: every}})
		}
	}
	return out
}

// TestGridPlans checks the statements library grids compile to at run time
// as TestQueryPlans checks the fixed ones, as a Viewer granted some
// libraries.
func TestGridPlans(t *testing.T) {
	h := newHousehold(t)
	for _, s := range gridSamples(h) {
		counted, page, args := s.compiled(h)
		file := "grid/" + s.name
		for _, st := range []statement{{"query", file, "counted", counted}, {"query", file, "page", page}} {
			t.Run(st.key(), func(t *testing.T) { checkScans(t, h.b.DB.Reader(), st, st.sql, args) })
		}
	}
}

// TestSmartQueriesStartAtWorkLibrary checks where a compiled query starts
// reading, on generated libraries: at work_library, searched by library
// through its primary key. Each work is looked up from there, and then its
// root when seasons or episodes are asked for. It checks without and with
// the statistics SQLite gathers, which the nightly maintenance has it do.
//
// A page without its count, of a query an index holds in order, for a
// Viewer with one library of its works, starts at that index instead, takes
// each row from the index without the table, and sorts nothing before it
// cuts the page. One of a query ordered by the
// user's own state starts at the user's rows, and looks each one's work up
// in that library.
//
// A library's grid is a query read in that library, so both of its
// statements start as such a page does, bare or filtered.
func TestSmartQueriesStartAtWorkLibrary(t *testing.T) {
	const (
		rows = "SEARCH wl USING PRIMARY KEY (library_id=?)"
		work = "SEARCH w USING INDEX sqlite_autoindex_work_1 (id=?)"
		root = "SEARCH r USING INDEX sqlite_autoindex_work_1 (id=?)"
		mine = "SEARCH wl USING PRIMARY KEY (library_id=? AND work_id=?)" // from the user's rows
	)
	// The index each of smartDefs' queries walks, if any, or mine.
	walks := map[string]string{
		"movies, unplayed, by last played":   mine,
		"movies, title and rating, by title": "work_library_title",
		"movies, one actor, by added":        "work_library_added",
		"series, by last episode added":      "work_library_last_added",
		"movies, capped at 100":              "work_library_added",
	}
	for _, statistics := range []bool{false, true} {
		t.Run(fmt.Sprintf("statistics=%v", statistics), func(t *testing.T) {
			db := storetest.New(t)
			movies, shows, _ := generate(t, db, shape{movies: 500, series: 150, seasons: 5, episodes: 14, airEvery: 86400, moviesElsewhere: 100, seriesElsewhere: 30})
			viewers := []users.Viewer{budgetViewer(t, db, "some", movies, shows), budgetViewer(t, db, "every")}
			if statistics {
				db = analyzed(t, db)
			}
			// starts checks that stmt first reads want: its plan's first
			// steps on wl, w and r are the FROM's, in the order it reads
			// them. A statement that walks index starts there, reading the
			// index alone, and sorts nothing before it cuts its page; one
			// that starts at the user's rows reads wl as mine.
			starts := func(name, stmt string, args []any, index string, want ...string) {
				t.Helper()
				got, err := plan(db.Reader(), stmt, args)
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case index == mine:
					want = []string{mine, work}
				case index != "":
					want = []string{"SEARCH wl USING COVERING INDEX " + index + " (library_id=?)", work}
					if cut, _, _ := strings.Cut(got, "\nSCAN p\n"); strings.Contains(cut, "TEMP B-TREE") {
						t.Errorf("%s: the page sorts its works:\n%s", name, got)
					}
				}
				var reads []string
				for line := range strings.SplitSeq(got, "\n") {
					if step := strings.TrimSpace(line); tableRead.MatchString(step) && len(reads) < len(want) {
						reads = append(reads, step)
					}
				}
				if !slices.Equal(reads, want) {
					t.Errorf("%s: reads %q, want %q in:\n%s", name, reads, want, got)
				}
			}
			for _, d := range smartDefs() {
				want := []string{rows, work}
				if slices.Contains(d.q.Kinds, catalog.KindEpisode) {
					want = append(want, root)
				}
				for vi, v := range viewers {
					counted, page, args := query.New(db).Compiled(t, d.q, v, time.Now(), nil)
					starts(d.name, counted, args, "", want...)
					// Only the first Viewer has one library of each kind.
					index := ""
					if vi == 0 {
						index = walks[d.name]
					}
					starts(d.name, page, args, index, want...)
				}
			}
			indexes := map[string]string{"myRating": mine, "lastPlayedAt": mine} // by field
			for column, f := range query.IndexedSorts() {
				indexes[f.Field] = query.SortIndex(column)
			}
			unplayedDrama := filter{Genres: []string{"Drama"}, Played: "unplayed"}
			for _, sort := range gridSorts {
				for lib, kind := range map[id.ID]catalog.WorkKind{movies: catalog.KindMovie, shows: catalog.KindSeries} {
					for _, filter := range []filter{{}, unplayedDrama} {
						for _, v := range viewers {
							name := fmt.Sprintf("the grid by %s, filter %+v", sort, filter)
							counted, page, args := query.New(db).CompiledIn(grid{Sort: sort, Filter: filter}.query(kind), v, query.Scope{Library: lib})
							starts(name, counted, args, indexes[sort], rows, work)
							starts(name, page, args, indexes[sort], rows, work)
						}
					}
				}
			}
		})
	}
}

// TestManualListsStartAtTheList checks where the read of a manual list
// starts, in each sort, on generated libraries, without and with the
// statistics SQLite gathers: at the list's own rows, by its primary key or
// its index of places. Each one's work is looked up from there, and its
// root, so the read never goes through a library's works. In the custom
// order it walks the index of places and cuts its page as it goes: it
// sorts only works left at one place, by ID.
func TestManualListsStartAtTheList(t *testing.T) {
	want := []string{
		"SEARCH lw (list_id=?)",
		"SEARCH wl USING INDEX work_library_work (work_id=?)",
		"SEARCH w USING INDEX sqlite_autoindex_work_1 (id=?)",
		"SEARCH r USING INDEX sqlite_autoindex_work_1 (id=?)",
	}
	listRead := regexp.MustCompile(`^(SEARCH|SCAN) (lw|wl|w|r)( |$)`)
	listIndex := regexp.MustCompile(`^SEARCH lw USING (PRIMARY KEY|COVERING INDEX list_work_position) `)
	covering := strings.NewReplacer("USING COVERING INDEX work_library_work", "USING INDEX work_library_work")
	for _, statistics := range []bool{false, true} {
		t.Run(fmt.Sprintf("statistics=%v", statistics), func(t *testing.T) {
			db := storetest.New(t)
			movies, shows, _ := generate(t, db, shape{movies: 500, series: 150, seasons: 5, episodes: 14, airEvery: 86400, moviesElsewhere: 100, seriesElsewhere: 30})
			v, lists := manualViewer(t, db, movies, shows)
			if statistics {
				db = analyzed(t, db)
			}
			q := query.New(db)
			for _, sort := range []users.ListSort{users.ListCustom, users.ListTitle, users.ListReleaseDate, users.ListRating, users.ListDateAdded} {
				for _, only := range [][]catalog.WorkKind{nil, {catalog.KindMovie}} {
					counted, page, args := q.CompiledList(v, lists[0].id, sort, only)
					for name, stmt := range map[string]string{"counted": counted, "page": page} {
						st := statement{"query", fmt.Sprintf("list/%s/%v", sort, only), name, stmt}
						checkScans(t, db.Reader(), st, stmt, args)
						got, err := plan(db.Reader(), stmt, args)
						if err != nil {
							t.Fatal(err)
						}
						// A read of movies alone has no works under a root.
						expect := want[:len(want)-len(only)]
						var reads []string
						for line := range strings.SplitSeq(got, "\n") {
							if step := strings.TrimSpace(line); listRead.MatchString(step) && len(reads) < len(expect) {
								reads = append(reads, covering.Replace(listIndex.ReplaceAllString(step, "SEARCH lw ")))
							}
						}
						if cut, _, _ := strings.Cut(got, "\nSCAN p\n"); sort == users.ListCustom &&
							(strings.Contains(cut, "TEMP B-TREE FOR ORDER BY") || !strings.Contains(cut, "SEARCH lw USING COVERING INDEX list_work_position (list_id=?)")) {
							t.Errorf("%s: a page in the custom order doesn't walk the places unsorted:\n%s", st.key(), got)
						}
						if !slices.Equal(reads, expect) {
							t.Errorf("%s: reads %q, want %q in:\n%s", st.key(), reads, expect, got)
						}
					}
				}
			}
		})
	}
}

// analyzed gathers db's statistics and opens it again: a connection plans
// with the statistics it found when it first read the schema.
func analyzed(t *testing.T, db *store.DB) *store.DB {
	t.Helper()
	err := db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, "ANALYZE")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var path string
	if err := db.Reader().QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if db, err = store.Open(ctx, path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// tableRead matches a plan's step that reads wl, w or r.
var tableRead = regexp.MustCompile(`^(SEARCH|SCAN) (wl|w|r)( |$)`)

// TestSmartReadsSelectOnce counts the statements that select a query's
// works as the reads of smart lists and of library grids run, and how many
// of them count the works. Each is a pass over the works of the query's
// kinds, so a read runs one for each list it shows: a list's screen one
// that counts, with its header running none, a later page one that doesn't
// count, the Lists screen one for each smart list's count and first works, and
// Home one for each smart row, which has no count. A grid's first page
// runs one that counts, and a later page one that doesn't.
func TestSmartReadsSelectOnce(t *testing.T) {
	db := storetest.New(t)
	movies, shows, _ := generate(t, db, shape{movies: 500, series: 150, seasons: 5, episodes: 14, airEvery: 86400, moviesElsewhere: 100, seriesElsewhere: 30})
	q := query.New(db)
	v, smart := smartViewer(t, db, q, movies, shows)
	list, rows := smart[0].id, 5 // smartViewer shows five of the lists on Home
	var selects, counts int
	query.Observe(t, func(sql string) {
		if strings.Contains(sql, "FROM work_library wl INDEXED BY") {
			selects++
			if strings.Contains(sql, "(SELECT count(*) FROM matched)") || strings.Contains(sql, "(SELECT count(*) FROM (SELECT wl.sort_title") ||
				strings.Contains(sql, "sum(n) FROM rail") {
				counts++
			}
		}
	})
	paged := func(o grid, total bool) func() error {
		return func() error {
			p, err := browse(db, q, v, movies, o)
			if err == nil && (len(p.Items) == 0 || (p.Total > 0) != total || (p.Letters != nil) != (total && o.Sort == "")) {
				t.Errorf("the grid's page %+v has %d works, the total %d and the letters %v", o, len(p.Items), p.Total, p.Letters)
			}
			return err
		}
	}
	later := query.GridOptions{Offset: 60}
	for _, tt := range []struct {
		name            string
		selects, counts int
		read            func() error
	}{
		{"a list's screen", 1, 1, func() error {
			if _, err := q.List(ctx, v, list); err != nil {
				return err
			}
			p, err := q.ListWorks(ctx, v, list, nil, "", query.GridOptions{Limit: 20})
			if err == nil && (p.Total <= 20 || len(p.Items) != 20) {
				t.Errorf("the first page has %d works of %d, want 20 of more", len(p.Items), p.Total)
			}
			return err
		}},
		{"a list's later page", 1, 0, func() error {
			p, err := q.ListWorks(ctx, v, list, nil, "", query.GridOptions{Offset: 20, Limit: 20})
			if err == nil && (p.Total != 0 || len(p.Items) == 0) {
				t.Errorf("a later page has %d works and the total %d, want works and no total", len(p.Items), p.Total)
			}
			return err
		}},
		{"a query", 1, 1, func() error {
			_, err := q.QueryWorks(ctx, v, smartDefs()[0].q, query.Scope{}, query.GridOptions{})
			return err
		}},
		{"a query's later page", 1, 0, func() error {
			_, err := q.QueryWorks(ctx, v, smartDefs()[0].q, query.Scope{}, query.GridOptions{Offset: 20})
			return err
		}},
		{"the Lists screen", len(smart), len(smart), func() error { _, err := q.CountedLists(ctx, v); return err }},
		{"a grid by title", 1, 1, paged(grid{}, true)},
		{"a grid by title, later page", 1, 0, paged(grid{GridOptions: later}, false)},
		{"a filtered grid by rating", 1, 1, paged(grid{Sort: "rating", Filter: filter{Genres: []string{"Drama"}}}, true)},
		{"a grid by my rating", 1, 1, paged(grid{Sort: "myRating"}, true)},
		{"a grid by my rating, later page", 1, 0, paged(grid{Sort: "myRating", GridOptions: later}, false)},
		{"Home", rows, 0, func() error { _, _, err := q.Home(ctx, v); return err }},
	} {
		selects, counts = 0, 0
		if err := tt.read(); err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if selects != tt.selects || counts != tt.counts {
			t.Errorf("%s selects the works %d times and counts them %d times, want %d and %d", tt.name, selects, counts, tt.selects, tt.counts)
		}
	}
}

// checkScans fails on each full scan of a large table in stmt's plan,
// unless wholeLibrary allows st.
func checkScans(t *testing.T, db *sql.DB, st statement, stmt string, args []any) {
	t.Helper()
	got, err := plan(db, stmt, args)
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]string{}
	for _, m := range alias.FindAllStringSubmatch(stmt, -1) {
		tables[m[1]] = m[1]
		if m[2] != "" && !slices.Contains([]string{"ON", "WHERE", "JOIN", "LEFT", "USING", "GROUP", "ORDER", "SET", "LIMIT"}, strings.ToUpper(m[2])) {
			tables[m[2]] = m[1]
		}
	}
	_, allowed := wholeLibrary[st.pkg+"/"+st.file]
	if !allowed {
		_, allowed = wholeLibrary[st.key()]
	}
	for line := range strings.SplitSeq(got, "\n") {
		m := fullScan.FindStringSubmatch(strings.TrimLeft(line, " |`-"))
		if m != nil && slices.Contains(largeTables, tables[m[1]]) && !allowed {
			t.Errorf("full scan of %s: %s", tables[m[1]], strings.TrimSpace(line))
		}
	}
}

// plan renders EXPLAIN QUERY PLAN as an indented tree.
func plan(db *sql.DB, stmt string, args []any) (string, error) {
	rows, err := db.Query("EXPLAIN QUERY PLAN "+stmt, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	depth := map[int]int{}
	var b strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			return "", err
		}
		depth[id] = depth[parent] + 1
		fmt.Fprintf(&b, "%s%s\n", strings.Repeat("  ", depth[id]-1), detail)
	}
	return b.String(), rows.Err()
}
