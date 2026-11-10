package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// The limits of a WorkQuery, and its current format's version.
const (
	queryVersion = 1
	maxRules     = 20
	maxGroups    = 5 // including the root
	maxSortKeys  = 3
	maxValues    = 100
)

// sortRandom is the SortKey field that shuffles the works.
const sortRandom = "random"

// WorkQuery is what a query selects: kinds of works, a tree of rules, how
// they sort, and how they're capped. Run a WorkQuery with
// QueryWorks, which applies the Viewer whatever the rules say.
type WorkQuery struct {
	Version  int                `json:"version"`
	Kinds    []catalog.WorkKind `json:"kinds"`
	Where    *RuleGroup         `json:"where,omitempty"`
	Sort     []SortKey          `json:"sort,omitempty"`
	MaxWorks *int               `json:"maxWorks,omitempty"`
}

// RuleGroup is a group of rules and sub-groups, joined by Match: "all" or
// "any". The root may hold groups, and those groups rules only.
type RuleGroup struct {
	Match  string      `json:"match"`
	Rules  []Rule      `json:"rules,omitempty"`
	Groups []RuleGroup `json:"groups,omitempty"`
}

// Rule is one condition: an operator on a field, with its values. Values
// are in the field type's canonical form.
type Rule struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Values []string `json:"values,omitempty"`
}

// SortKey is one key of the order: a sortable field, or sortRandom, with
// its direction, "asc" or "desc", or the field's default when empty.
type SortKey struct {
	Field     string `json:"field"`
	Direction string `json:"direction,omitempty"`
}

// QueryProblem is one invalid part of a query. Pointer is a JSON pointer
// from the query's root, such as /where/rules/2/op.
type QueryProblem struct {
	Pointer string
	Code    string
}

// InvalidQueryError says why a query can't run.
type InvalidQueryError struct {
	Problems []QueryProblem
}

func (e *InvalidQueryError) Error() string { return fmt.Sprintf("invalid query: %v", e.Problems) }

// Validate checks a query against the field registry and the limits.
func (q WorkQuery) Validate() []QueryProblem {
	var v validator
	if q.Version < 0 || q.Version > queryVersion {
		v.add("/version", "invalid")
	}
	if len(q.Kinds) == 0 {
		v.add("/kinds", "required")
	}
	for i, k := range q.Kinds {
		if !slices.Contains(allKinds, k) {
			v.add(fmt.Sprintf("/kinds/%d", i), "invalid")
		}
	}
	if q.Where != nil {
		v.groups++
		v.group(*q.Where, "/where", true)
	}
	if v.rules > maxRules {
		v.add("/where", "tooLarge")
	}
	if v.groups > maxGroups {
		v.add("/where", "tooLarge")
	}
	v.sort(q.Sort)
	if q.MaxWorks != nil && *q.MaxWorks < 1 {
		v.add("/maxWorks", "invalid")
	}
	return v.problems
}

type validator struct {
	problems      []QueryProblem
	rules, groups int
}

func (v *validator) add(pointer, code string) {
	v.problems = append(v.problems, QueryProblem{pointer, code})
}

func (v *validator) group(g RuleGroup, at string, root bool) {
	if g.Match != "all" && g.Match != "any" {
		v.add(at+"/match", "invalid")
	}
	if len(g.Rules) == 0 && len(g.Groups) == 0 {
		v.add(at, "required")
	}
	for i, r := range g.Rules {
		v.rules++
		v.rule(r, fmt.Sprintf("%s/rules/%d", at, i))
	}
	if !root && len(g.Groups) > 0 {
		v.add(at+"/groups", "invalid")
		return
	}
	for i, sub := range g.Groups {
		v.groups++
		v.group(sub, fmt.Sprintf("%s/groups/%d", at, i), false)
	}
}

func (v *validator) rule(r Rule, at string) {
	f, ok := fieldsByKey[r.Field]
	if !ok {
		v.add(at+"/field", "invalid")
		return
	}
	i := slices.IndexFunc(f.Ops, func(o FieldOp) bool { return o.Op == r.Op })
	if i < 0 {
		v.add(at+"/op", "invalid")
		return
	}
	arity := f.Ops[i].Arity
	if len(r.Values) > maxValues {
		v.add(at+"/values", "tooLarge")
		return
	}
	if (arity >= 0 && len(r.Values) != arity) || (arity < 0 && len(r.Values) == 0) {
		v.add(at+"/values", "invalid")
		return
	}
	for j, value := range r.Values {
		if !f.validValue(r.Op, value) {
			v.add(fmt.Sprintf("%s/values/%d", at, j), "invalid")
		}
	}
}

func (v *validator) sort(keys []SortKey) {
	if len(keys) > maxSortKeys {
		v.add("/sort", "tooLarge")
	}
	for i, k := range keys {
		at := fmt.Sprintf("/sort/%d", i)
		if k.Direction != "" && k.Direction != "asc" && k.Direction != "desc" {
			v.add(at+"/direction", "invalid")
		}
		if k.Field == sortRandom {
			if len(keys) > 1 {
				v.add(at+"/field", "invalid")
			}
			continue
		}
		if f, ok := fieldsByKey[k.Field]; !ok || !f.Sortable {
			v.add(at+"/field", "invalid")
		}
	}
}

// durationRe is the form within and notWithin take: P30D, P2W, P6M, P1Y.
var durationRe = regexp.MustCompile(`^P[1-9][0-9]{0,3}[DWMY]$`)

// validValue reports whether value is well formed for f and op.
func (f *fieldDef) validValue(op, value string) bool {
	if op == "within" || op == "notWithin" {
		return durationRe.MatchString(value)
	}
	switch {
	case f.Type == FieldNumber:
		n, err := strconv.ParseFloat(value, 64)
		return err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
	case f.Type == FieldDate:
		_, err := time.Parse(time.DateOnly, value)
		return err == nil
	case f.Type == FieldText:
		return store.Fold(value) != ""
	case f.bits != nil:
		_, ok := f.bits[value]
		return ok
	case f.ids:
		_, err := id.Parse(value)
		return err == nil
	case len(f.inline) > 0:
		return slices.ContainsFunc(f.inline, func(v FieldValue) bool { return v.Value == value })
	}
	return value != ""
}

// compiled is a query's statements and their bound parameters; both also
// take @limit and @offset. page reads a page of the query's works. counted
// reads the same page with, after each work's summary columns, how many
// works the query selects: a page with no works has no row to carry the
// count, so it is for the first page, where no works means none selected.
type compiled struct {
	counted, page string
	args          []any
}

// libraryOf is the kind of library that holds each kind of work a query
// selects.
var libraryOf = map[catalog.WorkKind]catalog.LibraryKind{
	catalog.KindMovie: catalog.Movies, catalog.KindSeries: catalog.Shows, catalog.KindSeason: catalog.Shows,
	catalog.KindEpisode: catalog.Shows, catalog.KindVideo: catalog.Videos, catalog.KindBook: catalog.Audiobooks,
}

// scope is where a query's works are, as SQL literals: the kinds, the
// kinds of library that hold them, and whether any is under a root work.
// It writes the names in allKinds and libraryOf, never the query's own
// text, so a kind that isn't one of them selects nothing. With none of
// them it writes NULL, which no kind is in.
func scope(kinds []catalog.WorkKind) (works, libraries string, nested bool) {
	var ws, ls []string
	for _, k := range allKinds {
		if !slices.Contains(kinds, k) {
			continue
		}
		ws = append(ws, "'"+string(k)+"'")
		if l := "'" + string(libraryOf[k]) + "'"; !slices.Contains(ls, l) {
			ls = append(ls, l)
		}
		nested = nested || k == catalog.KindSeason || k == catalog.KindEpisode
	}
	if len(ws) == 0 {
		return "NULL", "NULL", false
	}
	return strings.Join(ws, ", "), strings.Join(ls, ", "), nested
}

// matching is the FROM and WHERE that select a query's works, before its
// rules: the works of kinds, not provisional, that the Viewer sees. It
// reads work_library wl in the libraries the Viewer is granted that hold
// the kinds, then each row's work w, and with state the user's state us.
//
// A query with seasons or episodes also reads each work's root r, for what
// a work under a root takes from it. Any other work is its own root, so a
// query without them looks up the works of the libraries' root rows alone.
//
// The Viewer predicate is in the form for a statement on wl:
// catalog.Granted on the row's library and catalog.RatingVisible, which
// together are catalog.VisibleWork.
//
// CROSS JOIN and INDEXED BY fix the plan: wl by its primary key, which
// SQLite names sqlite_autoindex_work_library_1, then w, then r. Left to
// choose, SQLite with statistics scans work for its kinds, or reads wl
// through an index that has to look every row up again.
func matching(kinds []catalog.WorkKind, state bool) string {
	works, libraries, nested := scope(kinds)
	rows, root, matched, age := "\n  AND wl.is_root", "", "NOT w.provisional", "w.content_rating_age"
	if nested {
		rows, root = "", "\nCROSS JOIN work r ON r.id = w.root_id"
		matched, age = "NOT w.provisional AND NOT r.provisional", "coalesce(w.content_rating_age, r.content_rating_age)"
	}
	if state {
		root += "\nLEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id"
	}
	return `
FROM work_library wl INDEXED BY sqlite_autoindex_work_library_1
CROSS JOIN work w ON w.id = wl.work_id` + root + `
WHERE wl.library_id IN (SELECT gl.id FROM library gl WHERE gl.kind IN (` + libraries + `) AND ` + catalog.Granted("gl.id") + `)` + rows + `
  AND w.kind IN (` + works + `)
  AND ` + matched + `
  AND ( -- viewer-filter: wl
    ` + catalog.Granted("wl.library_id") + `
    AND ` + catalog.RatingVisible(age) + `)`
}

// stateRead matches SQL that reads the user's state us.
var stateRead = regexp.MustCompile(`\bus\.`)

// compile turns a valid query into SQL. It is the only way a WorkQuery
// becomes a statement: the SQL always carries the Viewer predicate, before
// any cap or page is cut, and values go in as bound parameters. seed fixes
// a random sort's order, and only narrows the query to those kinds, after
// any cap, when it's set.
//
// A statement selects in matched the ID of each work the query selects,
// with the columns its order reads, and joins the user's state there only
// when a rule or the order reads it. It cuts the page from those, and
// builds summaries for the page's works alone.
func compile(q WorkQuery, vw viewer, now time.Time, seed string, only []catalog.WorkKind) compiled {
	c := &compiler{now: now, seed: seed}
	where := ""
	if q.Where != nil {
		where = "\n  AND " + c.group(*q.Where)
	}
	columns, by := c.order(q)
	kinds := q.Kinds
	if q.MaxWorks == nil && only != nil {
		kinds = slices.DeleteFunc(slices.Clone(kinds), func(k catalog.WorkKind) bool { return !slices.Contains(only, k) })
	}
	matched := "SELECT w.id AS id, w.kind AS kind, " + columns + matching(kinds, stateRead.MatchString(columns+where)) + where
	if q.MaxWorks != nil {
		matched += "\nORDER BY " + by + "\nLIMIT " + c.bind(*q.MaxWorks)
		if len(only) > 0 {
			matched = "SELECT * FROM (\n" + matched + "\n) WHERE kind IN (SELECT value FROM json_each(@only))"
		}
	}
	read := func(materialized, total string) string {
		return "WITH matched AS " + materialized + "(\n" + matched + `
),
page AS (SELECT * FROM matched ORDER BY ` + by + ` LIMIT @limit OFFSET @offset)
SELECT ` + summaryColumns + total + `
FROM page p
JOIN work w ON w.id = p.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
ORDER BY p.` + strings.ReplaceAll(by, ", ", ", p.")
	}
	return compiled{
		counted: read("MATERIALIZED ", ", (SELECT count(*) FROM matched)"),
		page:    read("", ""),
		args:    vw.args(append(c.params, sql.Named("only", set(only)))...),
	}
}

// compiler builds one statement's WHERE clause and its parameters.
type compiler struct {
	params []any
	now    time.Time
	seed   string
}

// bind adds a parameter and returns its name.
func (c *compiler) bind(v any) string {
	name := "p" + strconv.Itoa(len(c.params))
	c.params = append(c.params, sql.Named(name, v))
	return "@" + name
}

func (c *compiler) group(g RuleGroup) string {
	parts := make([]string, 0, len(g.Rules)+len(g.Groups))
	for _, r := range g.Rules {
		parts = append(parts, c.rule(r))
	}
	for _, sub := range g.Groups {
		parts = append(parts, c.group(sub))
	}
	joiner := " AND "
	if g.Match == "any" {
		joiner = " OR "
	}
	return "(" + strings.Join(parts, joiner) + ")"
}

func (c *compiler) rule(r Rule) string {
	f := fieldsByKey[r.Field]
	switch {
	case f.bits != nil:
		return c.bitsRule(f, r)
	case f.from != "":
		return c.setRule(f, r)
	case f.Type == FieldText:
		return c.textRule(f, r)
	case f.Type == FieldNumber || f.Type == FieldDate:
		return c.scalarRule(f, r)
	}
	return c.choiceRule(f, r)
}

func (c *compiler) textRule(f *fieldDef, r Rule) string {
	p := c.bind(store.Fold(r.Values[0]))
	fold := "fold(" + f.expr + ")"
	switch r.Op {
	case "contains":
		return "instr(" + fold + ", " + p + ") > 0"
	case "notContains":
		return "(" + f.expr + " IS NULL OR instr(" + fold + ", " + p + ") = 0)"
	}
	return "substr(" + fold + ", 1, length(" + p + ")) = " + p
}

func (c *compiler) scalarRule(f *fieldDef, r Rule) string {
	switch r.Op {
	case "isSet":
		return f.expr + " IS NOT NULL"
	case "isNotSet":
		return f.expr + " IS NULL"
	case "within":
		return f.expr + " BETWEEN " + c.bind(c.cutoff(r.Values[0])) + " AND " + c.bind(c.today())
	case "notWithin":
		return "(" + f.expr + " IS NULL OR " + f.expr + " NOT BETWEEN " + c.bind(c.cutoff(r.Values[0])) + " AND " + c.bind(c.today()) + ")"
	case "between":
		return f.expr + " BETWEEN " + c.bind(f.scalar(r.Values[0])) + " AND " + c.bind(f.scalar(r.Values[1]))
	case "gte":
		return f.expr + " >= " + c.bind(f.scalar(r.Values[0]))
	case "lte":
		return f.expr + " <= " + c.bind(f.scalar(r.Values[0]))
	}
	return f.expr + " = " + c.bind(f.scalar(r.Values[0]))
}

// scalar is value as its field's expression compares: a number, or a day.
func (f *fieldDef) scalar(value string) any {
	if f.Type == FieldNumber {
		n, _ := strconv.ParseFloat(value, 64)
		return n
	}
	return value
}

// today is the current UTC day.
func (c *compiler) today() string { return c.now.UTC().Format(time.DateOnly) }

// cutoff is the day that period, such as P30D, before now.
func (c *compiler) cutoff(period string) string {
	n, _ := strconv.Atoi(period[1 : len(period)-1])
	t := c.now.UTC()
	switch period[len(period)-1] {
	case 'D':
		t = t.AddDate(0, 0, -n)
	case 'W':
		t = t.AddDate(0, 0, -7*n)
	case 'M':
		t = t.AddDate(0, -n, 0)
	default:
		t = t.AddDate(-n, 0, 0)
	}
	return t.Format(time.DateOnly)
}

func (c *compiler) choiceRule(f *fieldDef, r Rule) string {
	in := f.expr + " IN (SELECT value FROM json_each(" + c.bind(set(r.Values)) + "))"
	if r.Op == "noneOf" {
		return "(" + f.expr + " IS NULL OR NOT " + in + ")"
	}
	return in
}

// setRule is a rule on a field whose values are rows joined to the work.
func (c *compiler) setRule(f *fieldDef, r Rule) string {
	exists := func(where string) string {
		return "EXISTS (SELECT 1 FROM " + f.from + " WHERE " + f.on + where + ")"
	}
	switch r.Op {
	case "isSet":
		return exists("")
	case "isNotSet":
		return "NOT " + exists("")
	case "allOf":
		wanted := distinct(r.Values)
		return "(SELECT count(DISTINCT " + f.value + ") FROM " + f.from + " WHERE " + f.on + " AND " + f.value +
			" IN " + c.members(f, wanted) + ") = " + c.bind(len(wanted))
	case "noneOf":
		return "NOT " + exists(" AND "+f.value+" IN "+c.members(f, r.Values))
	}
	return exists(" AND " + f.value + " IN " + c.members(f, r.Values))
}

// members is the SQL list of a set field's values, bound as one parameter.
func (c *compiler) members(f *fieldDef, values []string) string {
	if !f.ids {
		return "(SELECT value FROM json_each(" + c.bind(set(values)) + "))"
	}
	ids := make([]id.ID, len(values))
	for i, v := range values {
		ids[i], _ = id.Parse(v)
	}
	return "(SELECT unhex(value) FROM json_each(" + c.bind(id.Set(ids...)) + "))"
}

func distinct(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

// bitsRule is a rule on a field stored as a bit mask in work_library.
func (c *compiler) bitsRule(f *fieldDef, r Rule) string {
	column := "wl." + f.column
	switch r.Op {
	case "isSet":
		return column + " <> 0"
	case "isNotSet":
		return column + " = 0"
	}
	var mask int64
	for _, v := range r.Values {
		mask |= f.bits[v]
	}
	m := strconv.FormatInt(mask, 10)
	switch r.Op {
	case "allOf":
		return "(" + column + " & " + m + ") = " + m
	case "noneOf":
		return "(" + column + " & " + m + ") = 0"
	}
	return "(" + column + " & " + m + ") <> 0"
}

// order is a query's order: the columns it reads, named k0 onwards, and
// the ORDER BY over those names and id, each term starting with a name. A
// work with no value sorts last whichever way the key runs; ties go by
// season and episode number, title, then ID.
func (c *compiler) order(q WorkQuery) (columns, by string) {
	keys := q.Sort
	if len(keys) == 0 {
		keys = []SortKey{{Field: "title"}}
	}
	var cols, terms []string
	column := func(expr string) string {
		name := "k" + strconv.Itoa(len(cols))
		cols = append(cols, expr+" AS "+name)
		return name
	}
	for _, k := range keys {
		if k.Field == sortRandom {
			terms = append(terms, column("sha256(w.id, "+c.bind(c.seed)+")"))
			continue
		}
		f := fieldsByKey[k.Field]
		dir := k.Direction
		if dir == "" {
			dir = f.DefaultDirection
		}
		name := column(f.sort)
		terms = append(terms, name+" IS NULL", name+" "+strings.ToUpper(dir))
	}
	terms = append(terms, column("w.parent_number"), column("w.number"), column("w.sort_title"), "id")
	return strings.Join(cols, ", "), strings.Join(terms, ", ")
}

// seedOf makes the random order of an unsaved query the same all UTC day.
func seedOf(q WorkQuery, now time.Time) string {
	b, _ := json.Marshal(q)
	h := fnv.New32a()
	h.Write(b)
	return fmt.Sprintf("%d/%d", h.Sum32(), now.UTC().Unix()/86400)
}

// listSeed makes a saved list's random order the same all UTC day.
func listSeed(list id.ID, now time.Time) string {
	return fmt.Sprintf("%s/%d", list, now.UTC().Unix()/86400)
}

// Saved is q stamped with the current version as stored JSON.
func (q WorkQuery) Saved() (string, error) {
	q.Version = queryVersion
	b, err := json.Marshal(q)
	return string(b), err
}

// QueryWorks runs a query as the Viewer. An invalid query returns an
// *InvalidQueryError saying what's wrong.
func (s *Service) QueryWorks(ctx context.Context, v users.Viewer, q WorkQuery, o GridOptions) (GridPage[WorkSummary], error) {
	return s.queryWorksAt(ctx, v, q, o, time.Now())
}

func (s *Service) queryWorksAt(ctx context.Context, v users.Viewer, q WorkQuery, o GridOptions, now time.Time) (GridPage[WorkSummary], error) {
	if problems := q.Validate(); len(problems) > 0 {
		return GridPage[WorkSummary]{}, &InvalidQueryError{problems}
	}
	return s.runQuery(ctx, viewerOf(v), q, seedOf(q, now), nil, o, now)
}

// runQuery reads a page of a valid query. The first page, at offset 0,
// counts the query's works in the statement that reads it; a later page
// doesn't count, and its Total is 0.
func (s *Service) runQuery(ctx context.Context, vw viewer, q WorkQuery, seed string, only []catalog.WorkKind, o GridOptions, now time.Time) (GridPage[WorkSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	c := compile(q, vw, now, seed, only)
	args := append(c.args, sql.Named("limit", limit), sql.Named("offset", o.Offset))
	var items []WorkSummary
	total := 0
	if o.Offset == 0 {
		items, total, err = countedSummaries(ctx, s.db.Reader(), statement{sql: c.counted}, args)
	} else {
		items, err = summaries(ctx, s.db.Reader(), statement{sql: c.page}, args)
	}
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return s.summaryPage(ctx, vw, items, total, nil)
}

// countedSummaries runs a compiled query's counted statement and returns
// its summaries and its count, 0 when it has no rows.
func countedSummaries(ctx context.Context, q querier, st statement, args []any) ([]WorkSummary, int, error) {
	total := 0
	out, err := rowsOf(ctx, q, st, args, func(r scanner, c *WorkSummary) (err error) {
		*c, err = scanSummary(r, &total)
		return err
	})
	return out, total, err
}
