package query

import (
	"context"
	"database/sql"
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
// page's with its count and a page's, with their arguments for the first
// page.
func Compiled(q WorkQuery, v users.Viewer, now time.Time, only []catalog.WorkKind) (counted, page string, args []any) {
	c := compile(q, viewerOf(v), now, seedOf(q, now), only)
	return c.counted, c.page, append(c.args, sql.Named("limit", 50), sql.Named("offset", 0))
}

// Observe calls fn with the SQL of each statement the package runs, until
// the test ends.
func Observe(t *testing.T, fn func(sql string)) {
	observe = func(st statement) { fn(st.sql) }
	t.Cleanup(func() { observe = nil })
}
