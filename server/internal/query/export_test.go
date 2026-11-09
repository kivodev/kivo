package query

import (
	"context"
	"time"

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
