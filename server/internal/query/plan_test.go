package query_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/store/storetest"
)

// largeTables grow with the library or with use. A query may search them
// through an index, but must never scan one whole.
var largeTables = []string{
	"work", "work_library", "work_external_id", "media_file", "media_track", "media_chapter", "media_attachment", "media_version",
	"media_version_work", "person", "credit", "work_genre", "work_tag", "work_company", "collection_work",
	"user_state", "play_event", "match", "image", "artwork", "provider_cache", "job", "scan_dir", "audit", "activity",
	"book", "book_series", "book_series_book",
}

// wholeLibrary lists the statements allowed to scan a large table, keyed by
// package/file or by statement key, with the reason. They are the nightly
// and whole-library tasks, which visit every row anyway, counts over a
// partial index that holds only the rows counted, the rule builder's reads
// of a field's values, and the test suite's
// invariants.
var wholeLibrary = map[string]string{
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
}

var (
	narg      = regexp.MustCompile(`sqlc\.(?:narg|slice)\('(\w+)'\)`)
	embed     = regexp.MustCompile(`sqlc\.embed\((\w+)\)`)
	paramName = regexp.MustCompile(`@(\w+)`)
	alias     = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+(\w+)(?:\s+(?:AS\s+)?(\w+))?`)
	fullScan  = regexp.MustCompile(`^SCAN (\w+)(?: USING (?:COVERING )?INDEX \w+)?$`)
)

// TestQueryPlans records the EXPLAIN QUERY PLAN of every package's
// statements, both sqlc's queries and query's own, in
// testdata/plans/<key>.txt. Running go test -update rewrites those files.
// The test fails on a full scan of a large table, by table or by index,
// outside the whole-library tasks. Each @name is bound to NULL.
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
			got, err := plan(db.Reader(), stmt, args)
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
			path := filepath.Join("testdata", "plans", st.key()+".txt")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("%s changed (go test -update to accept):\n%s", path, got)
			}
		})
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
