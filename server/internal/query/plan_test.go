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
	"book", "book_series", "book_series_book",
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

	"identify/catalog.sql.CountUnidentified": "how many files the identify task has left in each library, for Tasks and the Libraries page: media_file_behind holds only files not yet probed or identified",
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
			count, page, args := query.Compiled(s.q, h.friend, now, only)
			prefix := ""
			if only != nil {
				prefix = "only/"
			}
			file := "smart/" + s.name
			for _, st := range []statement{{"query", file, prefix + "count", count}, {"query", file, prefix + "page", page}} {
				t.Run(st.key(), func(t *testing.T) { checkScans(t, h.b.DB.Reader(), st, st.sql, args) })
			}
		}
	}
}

// TestSmartQueriesStartAtWorkLibrary checks where a compiled query starts
// reading, on generated libraries: at work_library, searched by library
// through its primary key. Each work is looked up from there, and then its
// root when seasons or episodes are asked for. It checks without and with
// the statistics SQLite gathers, which the nightly maintenance has it do.
func TestSmartQueriesStartAtWorkLibrary(t *testing.T) {
	const (
		rows = "SEARCH wl USING PRIMARY KEY (library_id=?)"
		work = "SEARCH w USING INDEX sqlite_autoindex_work_1 (id=?)"
		root = "SEARCH r USING INDEX sqlite_autoindex_work_1 (id=?)"
	)
	for _, statistics := range []bool{false, true} {
		t.Run(fmt.Sprintf("statistics=%v", statistics), func(t *testing.T) {
			db := storetest.New(t)
			movies, shows, _ := generate(t, db, shape{movies: 500, series: 150, seasons: 5, episodes: 14, airEvery: 86400, moviesElsewhere: 100, seriesElsewhere: 30})
			viewers := []users.Viewer{budgetViewer(t, db, "some", movies, shows), budgetViewer(t, db, "every")}
			if statistics {
				err := db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
					_, err := tx.ExecContext(ctx, "ANALYZE")
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				// A connection plans with the statistics it found when it
				// first read the schema, so plan on new connections.
				var path string
				if err := db.Reader().QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path); err != nil {
					t.Fatal(err)
				}
				if db, err = store.Open(ctx, path); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
			}
			for _, d := range smartDefs() {
				want := []string{rows, work}
				if slices.Contains(d.q.Kinds, catalog.KindEpisode) {
					want = append(want, root)
				}
				for _, v := range viewers {
					count, page, args := query.Compiled(d.q, v, time.Now(), nil)
					for _, stmt := range []string{count, page} {
						got, err := plan(db.Reader(), stmt, args)
						if err != nil {
							t.Fatal(err)
						}
						// The plan's first steps on wl, w and r are the
						// FROM's, in the order it reads them.
						var reads []string
						for line := range strings.SplitSeq(got, "\n") {
							if step := strings.TrimSpace(line); tableRead.MatchString(step) && len(reads) < len(want) {
								reads = append(reads, step)
							}
						}
						if !slices.Equal(reads, want) {
							t.Errorf("%s: reads %q, want %q in:\n%s", d.name, reads, want, got)
						}
					}
				}
			}
		})
	}
}

// tableRead matches a plan's step that reads wl, w or r.
var tableRead = regexp.MustCompile(`^(SEARCH|SCAN) (wl|w|r)( |$)`)

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
