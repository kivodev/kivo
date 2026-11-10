package query

import (
	"database/sql"
	"maps"
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

// seriesOfOld is the work a season or episode took its series' fields
// from, as the old registry wrote it.
const seriesOfOld = "CASE WHEN w.kind IN ('season', 'episode') THEN w.root_id ELSE w.id END"

// lastPlayedOld is the user's last play as the old registry read it, a
// series' and a season's by a subquery for each work.
const lastPlayedOld = `(CASE w.kind
    WHEN 'series' THEN (SELECT max(fu.last_played_at) FROM work fe CROSS JOIN user_state fu ON fu.work_id = fe.id
      WHERE fe.root_id = w.id AND fu.user_id = @user)
    WHEN 'season' THEN (SELECT max(fu.last_played_at) FROM work fe CROSS JOIN user_state fu ON fu.work_id = fe.id
      WHERE fe.parent_id = w.id AND fu.user_id = @user)
    ELSE us.last_played_at END)`

// oldFields are the fields the registry now reads another way to the same
// end, as it read them before: compileOld compiles with them, so the
// comparison proves the two ways equal. They are a series' genres, tags
// and companies, the last play, and the sort title.
//
// The fields whose meaning changed on purpose are not among them, so
// compileOld reads them as compile does: rating, which is unset without
// votes, and contentRating, which is a season's or episode's own before its
// series'. Their tests are TestRatingNeedsVotes and
// TestContentRatingIsAWorksOwnFirst.
var oldFields = func() map[string]*fieldDef {
	out := map[string]*fieldDef{}
	old := func(field string, change func(*fieldDef)) {
		f := *fieldsByKey[field]
		change(&f)
		out[field] = &f
	}
	for _, field := range []string{"genre", "tag", "company"} {
		old(field, func(f *fieldDef) { f.on = "x.work_id = " + seriesOfOld })
	}
	old("lastPlayedAt", func(f *fieldDef) {
		f.expr, f.sort, f.ms, f.rolled = dayOf(lastPlayedOld), lastPlayedOld, false, false
	})
	old("title", func(f *fieldDef) { f.sort = "w.sort_title" })
	return out
}()

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

// compileOld is compile as it was when it read every work on the server,
// counted in a statement of its own, and sorted every page: kept so
// TestCompileMatchesOld can check compile selects the same works in the
// same order. It compiles with oldFields.
//
// It departs from the compiler it was in what compile now means otherwise
// on purpose. It selects provisional works and the works under them, which
// TestQueryIncludesProvisionalWorks covers, and so reads each work's root
// r for the fields that inherit from it; and see oldFields.
func compileOld(q WorkQuery, vw viewer, now time.Time, seed string, only []catalog.WorkKind) compiledOld {
	current := maps.Clone(fieldsByKey)
	maps.Copy(fieldsByKey, oldFields)
	defer maps.Copy(fieldsByKey, current)

	c := &compiler{now: now, seed: seed, nested: true}
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
JOIN work r ON r.id = w.root_id
JOIN work_library wl ON wl.work_id = w.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.kind IN (SELECT value FROM json_each(` + c.bind(set(kinds)) + `))
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
