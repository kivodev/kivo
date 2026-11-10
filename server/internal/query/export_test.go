package query

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/users"
)

// Statements returns every statement the package runs, its SQL by name.
func Statements() map[string]string {
	out := make(map[string]string, len(statements))
	for _, s := range statements {
		if _, dup := out[s.name]; dup {
			panic("query: two statements named " + s.name)
		}
		out[s.name] = s.sql
	}
	return out
}

// QueryWorksAt is QueryWorks as of now.
func (s *Service) QueryWorksAt(ctx context.Context, v users.Viewer, q WorkQuery, o GridOptions, now time.Time) (GridPage[WorkSummary], error) {
	return s.queryWorksAt(ctx, v, q, o, now)
}

// Compiled returns the statements q compiles to as the Viewer, the first
// page's with its count and a page's as a read of one compiles it, with
// their arguments for the first page.
func (s *Service) Compiled(t *testing.T, q WorkQuery, v users.Viewer, now time.Time, only []catalog.WorkKind) (counted, page string, args []any) {
	t.Helper()
	c, err := compilePage(context.Background(), s.db.Reader(), q, viewerOf(v), now, seedOf(q, now), only)
	if err != nil {
		t.Fatal(err)
	}
	return c.counted, c.page, append(c.args, sql.Named("limit", 50), sql.Named("offset", 0))
}

// Bits returns the bit each resolution and each range has in work_library.
func Bits() (map[Resolution]int64, map[Range]int64) { return resolutionBits, rangeBits }

// IndexedSorts returns the fields whose sort an index holds, by the
// work_library column they sort by.
func IndexedSorts() map[string]WorkField {
	out := map[string]WorkField{}
	for _, f := range workFields {
		if f.indexed != "" {
			out[f.indexed] = f.WorkField
		}
	}
	return out
}

// SortIndex returns the index that holds a work_library column's order,
// or "" when none does.
func SortIndex(column string) string { return sortIndexes[column] }

// GridSorts returns the sorts a library grid takes.
func GridSorts() []Sort { return slices.Collect(maps.Keys(gridSorts)) }

// Observe calls fn with the SQL of each statement the package runs, until
// the test ends.
func Observe(t *testing.T, fn func(sql string)) {
	observe = func(st statement) { fn(st.sql) }
	t.Cleanup(func() { observe = nil })
}
