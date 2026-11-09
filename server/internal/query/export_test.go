package query

import (
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

// CompiledSQL is the statement a valid query compiles to for v.
func CompiledSQL(q WorkQuery, v users.Viewer, now time.Time) string {
	return compile(q, viewerOf(v), now).sql
}
