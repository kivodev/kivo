package query

import (
	"database/sql"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/users"
)

// compiledOld is the two statements compileOld makes of a query, its count
// and its page, and their bound parameters; page also takes @limit and
// @offset.
type compiledOld struct {
	count, page string
	args        []any
}

// orderOld is a query's ORDER BY as compileOld wrote it.
func (c *compiler) orderOld(q WorkQuery) string {
	keys := q.Sort
	if len(keys) == 0 {
		keys = []SortKey{{Field: "title"}}
	}
	var parts []string
	for _, k := range keys {
		if k.Field == sortRandom {
			parts = append(parts, "sha256(w.id, "+c.bind(c.seed)+")")
			continue
		}
		f := fieldsByKey[k.Field]
		dir := k.Direction
		if dir == "" {
			dir = f.DefaultDirection
		}
		parts = append(parts, f.sort+" IS NULL", f.sort+" "+strings.ToUpper(dir))
	}
	parts = append(parts, "w.parent_number", "w.number", "w.sort_title", "w.id")
	return strings.Join(parts, ", ")
}

// compileOld is compile as it was when it read every work on the server
// and counted in a statement of its own, kept so TestCompileMatchesOld can
// check compile selects the same works.
func compileOld(q WorkQuery, vw viewer, now time.Time, seed string, only []catalog.WorkKind) compiledOld {
	c := &compiler{now: now, seed: seed}
	kinds := make([]string, len(q.Kinds))
	for i, k := range q.Kinds {
		kinds[i] = string(k)
	}
	where := ""
	if q.Where != nil {
		where = "\n  AND " + c.group(*q.Where)
	}
	sortBy := c.orderOld(q)
	from := `
FROM work w
JOIN work_library wl ON wl.work_id = w.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE NOT w.provisional
  AND NOT (SELECT r.provisional FROM work r WHERE r.id = w.root_id)
  AND w.kind IN (SELECT value FROM json_each(` + c.bind(set(kinds)) + `))
  AND ` + visible + where
	args := func() []any { return vw.args(append(c.params, sql.Named("only", set(only)))...) }
	if q.MaxWorks == nil {
		if only != nil {
			from += "\n  AND w.kind IN (SELECT value FROM json_each(@only))"
		}
		return compiledOld{
			count: "SELECT count(*)" + from,
			page:  "SELECT " + summaryColumns + from + "\nORDER BY " + sortBy + "\nLIMIT @limit OFFSET @offset",
			args:  args(),
		}
	}
	capped := `WITH matched AS (
SELECT w.id, row_number() OVER (ORDER BY ` + sortBy + `) AS o` + from + `
),
limited AS (SELECT id, o FROM matched ORDER BY o LIMIT ` + c.bind(*q.MaxWorks) + `),
filtered AS (SELECT l.id, l.o FROM limited l JOIN work fw ON fw.id = l.id
  WHERE @only IS NULL OR fw.kind IN (SELECT value FROM json_each(@only)))`
	return compiledOld{
		count: capped + `
SELECT count(*) FROM filtered`,
		page: capped + `, page AS (SELECT id, o FROM filtered ORDER BY o LIMIT @limit OFFSET @offset)
SELECT ` + summaryColumns + `
FROM page p
JOIN work w ON w.id = p.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
ORDER BY p.o`,
		args: args(),
	}
}

// CompiledOld is Compiled by compileOld.
func CompiledOld(q WorkQuery, v users.Viewer, now time.Time, only []catalog.WorkKind) (count, page string, args []any) {
	c := compileOld(q, viewerOf(v), now, seedOf(q, now), only)
	return c.count, c.page, append(c.args, sql.Named("limit", 50), sql.Named("offset", 0))
}
