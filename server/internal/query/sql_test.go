package query_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/query"
)

// statement is one named statement, either from another package's queries/
// file or one of query's own.
type statement struct{ pkg, file, name, sql string }

// key names the statement in its package as package/file.name. Query's own
// statements have no file, so they are named query/name.
func (s statement) key() string {
	if s.file == "" {
		return s.pkg + "/" + s.name
	}
	return s.pkg + "/" + s.file + "." + s.name
}

// own returns query's statements, by name.
func own() []statement {
	stmts := query.Statements()
	var out []statement
	for _, name := range slices.Sorted(maps.Keys(stmts)) {
		out = append(out, statement{pkg: "query", name: name, sql: stmts[name]})
	}
	return out
}

// statements returns the named statements of the query files matching
// pattern.
func statements(t *testing.T, pattern string) []statement {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		t.Fatal("no query files", err)
	}
	var out []statement
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(string(b), "-- name: ")
		for _, p := range parts[1:] {
			name, body, _ := strings.Cut(p, "\n")
			out = append(out, statement{
				pkg: filepath.Base(filepath.Dir(filepath.Dir(f))), file: filepath.Base(f),
				name: strings.Fields(name)[0], sql: untilEnd(body),
			})
		}
	}
	return out
}

// otherQueries matches the query files of every package but query. sqlc
// compiles those files.
var otherQueries = filepath.Join("..", "*", "queries", "*.sql")

// untilEnd cuts a statement at its closing semicolon, skipping any
// semicolons in comments.
func untilEnd(body string) string {
	at := 0
	for line := range strings.SplitAfterSeq(body, "\n") {
		code, _, _ := strings.Cut(line, "--")
		if i := strings.Index(code, ";"); i >= 0 {
			return body[:at+i]
		}
		at += len(line)
	}
	return body
}

// group returns the parenthesized text that opens at s[open], or "" if it
// isn't closed.
func group(s string, open int) string {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[open : i+1]
			}
		}
	}
	return ""
}

var space = regexp.MustCompile(`\s+`)

func squeeze(s string) string { return strings.TrimSpace(space.ReplaceAllString(s, " ")) }

// canonicalFilter returns the Viewer predicate on works as WorkSummary
// carries it, from its opening parenthesis to its closing one.
func canonicalFilter(t *testing.T) string {
	t.Helper()
	sql := query.Statements()["WorkSummary"]
	i := strings.Index(sql, "EXISTS ( -- viewer-filter")
	if i < 0 {
		t.Fatal("no canonical predicate in WorkSummary")
	}
	return squeeze(group(sql, i+len("EXISTS ")))
}

// checkCompiledFilter checks that a statement a WorkQuery compiled to
// carries the Viewer predicate on every read of the works it selects from,
// once each, before any cap or page is cut.
//
// A compiled statement reads each work w from its work_library row wl, so
// it carries the predicate behind a "-- viewer-filter: wl" marker without
// the canonical one's lookup of that row, and tests wl in its place. It
// reads the rating of w's root from the root it joined as r, or leaves it
// out when it reads only root rows, which have no other root. The check
// rewrites the canonical predicate in exactly those ways and requires each
// read's to be the same text, and requires the joins and the root rows
// condition that make the rewrites mean the same.
func checkCompiledFilter(t *testing.T, canonical, sql string) {
	t.Helper()
	const (
		marker     = "-- viewer-filter: wl"
		lookup     = "( -- viewer-filter SELECT 1 FROM work_library vl WHERE vl.work_id = w.id AND "
		rootAge    = "(SELECT vr.content_rating_age FROM work vr WHERE vr.id = CASE WHEN w.parent_id IS NOT NULL THEN w.root_id END)"
		ownOrRoots = "coalesce(w.content_rating_age, " + rootAge + ")"
		work       = "\nCROSS JOIN work w ON w.id = wl.work_id\n"
		root       = "CROSS JOIN work r ON r.id = w.root_id\n"
	)
	if !strings.HasPrefix(canonical, lookup) || strings.Count(canonical, "vl.library_id") != 1 || strings.Count(canonical, ownOrRoots) != 1 {
		t.Fatalf("the canonical predicate changed, and this check with it:\n%s", canonical)
	}
	// Each read of the works is a row of work_library wl and its work w,
	// and what follows them up to the next read.
	reads := strings.Split(sql, work)
	if len(reads) < 2 || strings.Count(sql, " work_library wl ") != len(reads)-1 || strings.Count(sql, "-- viewer-filter") != len(reads)-1 {
		t.Fatalf("the statement doesn't read each work from its work_library row, with the Viewer predicate once for each read:\n%s", sql)
	}
	for _, read := range reads[1:] {
		want := strings.ReplaceAll(strings.TrimPrefix(canonical, lookup), "vl.library_id", "wl.library_id")
		at := strings.Index(read, marker)
		if at < 0 {
			t.Fatalf("a read of the works doesn't carry the Viewer predicate:\n%s", sql)
		}
		switch {
		case strings.HasPrefix(read, root):
			want = strings.Replace(want, rootAge, "r.content_rating_age", 1)
		case strings.Contains(read[:at], "\n  AND wl.is_root\n"):
			want = strings.Replace(want, ownOrRoots, "w.content_rating_age", 1)
		default:
			t.Errorf("a read of the works reads neither the root of each work nor only root rows:\n%s", sql)
		}
		want = "( " + marker + " " + want
		if got := squeeze(group(read, strings.LastIndex(read[:at], "("))); got != want {
			t.Errorf("viewer predicate differs from the canonical one:\n got %s\nwant %s", got, want)
		}
		if strings.Contains(read[:at], "LIMIT") {
			t.Errorf("the Viewer predicate comes after a cut:\n%s", sql)
		}
	}
}

// TestViewerFilter checks that every statement carries the Viewer
// predicate behind a "-- viewer-filter" marker, and every copy of it is the
// same as WorkSummary's (or, for the statements that list libraries, as
// Libraries'). The statements a WorkQuery compiles to, a library grid's
// among them, carry it in the form checkCompiledFilter checks.
func TestViewerFilter(t *testing.T) {
	stmts := own()
	canonical := map[string]string{"works": canonicalFilter(t)}
	for _, st := range stmts {
		if st.name == "Libraries" {
			i := strings.Index(st.sql, "-- viewer-filter: libraries")
			canonical["libraries"] = squeeze(group(st.sql, strings.Index(st.sql[i:], "(")+i))
		}
	}
	if canonical["libraries"] == "" {
		t.Fatal("no canonical predicate in Libraries")
	}
	h := newHousehold(t)
	now := time.Now()
	for _, s := range smartSamples(h) {
		for _, only := range [][]catalog.WorkKind{nil, s.q.Kinds} {
			counted, page, _ := h.q.Compiled(t, s.q, h.friend, now, only)
			checkCompiledFilter(t, canonical["works"], counted)
			checkCompiledFilter(t, canonical["works"], page)
		}
	}
	for _, s := range gridSamples(h) {
		counted, page, _ := h.q.CompiledGrid(t, h.friend, s.lib, s.o)
		checkCompiledFilter(t, canonical["works"], counted)
		checkCompiledFilter(t, canonical["works"], page)
	}
	for _, st := range stmts {
		n := 0
		for i := 0; ; {
			j := strings.Index(st.sql[i:], "-- viewer-filter")
			if j < 0 {
				break
			}
			at := i + j
			kind, open := "works", strings.LastIndex(st.sql[:at], "(")
			if strings.HasPrefix(st.sql[at:], "-- viewer-filter: libraries") {
				kind, open = "libraries", at+strings.Index(st.sql[at:], "(")
			}
			if strings.HasPrefix(st.sql[at:], "-- viewer-filter: user") {
				if !strings.Contains(st.sql, "user_id = @user") {
					t.Errorf("%s: read of the user's own rows doesn't name @user", st.name)
				}
			} else if got := squeeze(group(st.sql, open)); got != canonical[kind] {
				t.Errorf("%s: viewer predicate differs from the canonical one:\n%s", st.name, got)
			}
			n++
			i = at + 1
		}
		if n == 0 {
			t.Errorf("%s: no -- viewer-filter", st.name)
		}
	}
}

var (
	lineComment = regexp.MustCompile(`--[^\n]*`)
	readKeyword = regexp.MustCompile(`(?i)^\s*(select|with)\b`)
	writes      = regexp.MustCompile(`(?i)\b(insert|update|delete|replace|create|drop|alter|pragma|attach|vacuum)\b`)
)

// TestReadOnly checks that every statement only reads: query owns no
// tables.
func TestReadOnly(t *testing.T) {
	for _, st := range own() {
		code := lineComment.ReplaceAllString(st.sql, "")
		if !readKeyword.MatchString(code) || writes.MatchString(code) {
			t.Errorf("%s may only read", st.name)
		}
	}
}

// TestStatementsRun checks that only sql.go's helpers run SQL. That way
// every statement the package runs was made by stmt, so the plan and
// Viewer tests see it.
func TestStatementsRun(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	run := regexp.MustCompile(`\.(Query|QueryRow|Exec|Prepare)(Context)?\(`)
	for _, f := range files {
		if f == "sql.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if run.Match(b) {
			t.Errorf("%s runs SQL itself: run a statement through sql.go's helpers", f)
		}
	}
}
