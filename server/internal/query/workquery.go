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

// compiled is a query's two statements and their bound parameters; page
// also takes @limit and @offset.
type compiled struct {
	count, page string
	args        []any
}

// compile turns a valid query into SQL. It is the only way a WorkQuery
// becomes a statement: the SQL always carries the visible predicate, and
// values go in as bound parameters. seed fixes a random sort's order, and
// only narrows the query to those kinds, after any cap, when it's set.
func compile(q WorkQuery, vw viewer, now time.Time, seed string, only []catalog.WorkKind) compiled {
	c := &compiler{now: now, seed: seed}
	kinds := make([]string, len(q.Kinds))
	for i, k := range q.Kinds {
		kinds[i] = string(k)
	}
	where := ""
	if q.Where != nil {
		where = "\n  AND " + c.group(*q.Where)
	}
	sortBy := c.order(q)
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
		return compiled{
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
	return compiled{
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

// order is a query's ORDER BY. A work with no value sorts last whichever
// way the key runs; ties go by season and episode number, title, then ID.
func (c *compiler) order(q WorkQuery) string {
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

// runQuery reads a page of a valid query.
func (s *Service) runQuery(ctx context.Context, vw viewer, q WorkQuery, seed string, only []catalog.WorkKind, o GridOptions, now time.Time) (GridPage[WorkSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	c := compile(q, vw, now, seed, only)
	var total int
	var items []WorkSummary
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := rowOf(ctx, tx, statement{sql: c.count}, c.args, &total); err != nil {
			return err
		}
		args := append(slices.Clone(c.args), sql.Named("limit", limit), sql.Named("offset", o.Offset))
		var err error
		items, err = summaries(ctx, tx, statement{sql: c.page}, args)
		return err
	})
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return s.summaryPage(ctx, vw, items, total, nil)
}
