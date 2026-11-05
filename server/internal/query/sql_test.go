package query_test

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

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

// TestViewerFilter checks that every statement carries the Viewer
// predicate behind a "-- viewer-filter" marker, and every copy of it is the
// same as WorkSummary's (or, for the statements that list libraries, as
// Libraries').
func TestViewerFilter(t *testing.T) {
	stmts := own()
	canonical := map[string]string{}
	for _, st := range stmts {
		switch st.name {
		case "WorkSummary":
			i := strings.Index(st.sql, "EXISTS ( -- viewer-filter")
			canonical["works"] = squeeze(group(st.sql, i+len("EXISTS ")))
		case "Libraries":
			i := strings.Index(st.sql, "-- viewer-filter: libraries")
			canonical["libraries"] = squeeze(group(st.sql, strings.Index(st.sql[i:], "(")+i))
		}
	}
	if canonical["works"] == "" || canonical["libraries"] == "" {
		t.Fatal("no canonical predicate in WorkSummary or Libraries")
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
			if got := squeeze(group(st.sql, open)); got != canonical[kind] {
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
