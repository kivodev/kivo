package query

import (
	"cmp"
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
// works the query selects, and then for a read that asks for letters how
// many start with each character: a page with no works has no row to carry
// them, so it is for the first page, where no works means none selected.
type compiled struct {
	counted, page string
	args          []any
}

// reading is what a read asks of a query beyond the query itself: where to
// look for its works, and what to say of them besides.
type reading struct {
	// library is the one library to read, which the Viewer must be granted
	// like any other, or zero for every library that holds the query's
	// kinds.
	library   id.ID
	unmatched bool // only the works not yet matched
	letters   bool // counted also counts the works by first character
}

// libraryOf is the kind of library that holds each kind of work a query
// selects. A library holds works of its own kind alone: catalog refuses
// any other.
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
// rules: the works of kinds that the Viewer sees. It reads work_library wl
// in the libraries the Viewer is granted that hold the kinds, then each
// row's work w, and what the rules and the order read besides: the user's
// state us, and played pl.
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
//
// A read of one library reads @library's rows alone, and from says where
// it starts. With a column in sortIndexes, it reads the library's root rows
// through the column's index, so in its order. With "us" or "pl", it
// starts from the user's states or from played, and looks up the row of
// each one's work, so it reads only the works with one.
func (c *compiler) matching(kinds []catalog.WorkKind, rd reading, from string) string {
	works, libraries, nested := scope(kinds)
	rows, joins, age := "\n  AND wl.is_root", "", "w.content_rating_age"
	if nested {
		rows, joins = "", "\nCROSS JOIN work r ON r.id = w.root_id"
		age = "coalesce(w.content_rating_age, r.content_rating_age)"
	}
	if c.state && from != "us" {
		joins += "\nLEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id"
	}
	if c.played && from != "pl" {
		joins += "\nLEFT JOIN played pl ON pl.work = w.id"
	}
	in := "wl.library_id IN (SELECT gl.id FROM library gl WHERE gl.kind IN (" + libraries + ") AND " + catalog.Granted("gl.id") + ")"
	if !rd.library.IsZero() {
		in = "wl.library_id = @library"
	}
	const byKey = "work_library wl INDEXED BY sqlite_autoindex_work_library_1"
	first := "FROM " + byKey
	switch from {
	case "":
	case "us":
		first, in = "FROM user_state us\nCROSS JOIN "+byKey+" ON wl.work_id = us.work_id", in+"\n  AND us.user_id = @user"
	case "pl":
		first = "FROM played pl\nCROSS JOIN " + byKey + " ON wl.work_id = pl.work"
	default:
		first = "FROM work_library wl INDEXED BY " + sortIndexes[from]
	}
	unmatched := ""
	if rd.unmatched {
		unmatched = "\n  AND w.provisional"
	}
	return `
` + first + `
CROSS JOIN work w ON w.id = wl.work_id` + joins + `
WHERE ` + in + rows + `
  AND w.kind IN (` + works + `)` + unmatched + `
  AND ( -- viewer-filter: wl
    ` + catalog.Granted("wl.library_id") + `
    AND ` + catalog.RatingVisible(age) + `)`
}

// narrowed is the kinds a statement of q selects: q's, or those of them in
// only, when it has any, for an uncapped query's page of only's kinds. A
// capped query's page keeps only's kinds from its capped works instead.
func narrowed(q WorkQuery, only []catalog.WorkKind) []catalog.WorkKind {
	if q.MaxWorks != nil || len(only) == 0 {
		return q.Kinds
	}
	return slices.DeleteFunc(slices.Clone(q.Kinds), func(k catalog.WorkKind) bool { return !slices.Contains(only, k) })
}

// ordered is the field a query's works of kinds are ordered by, when a
// page of one library's can be read in that order without reading them
// all: root works alone, ordered by one key in its field's default
// direction, of a field with an index of that order (see sortIndexes) or
// one the user's own state gives (see mine). It is nil for any other
// query.
func ordered(q WorkQuery, kinds []catalog.WorkKind) *fieldDef {
	if _, _, nested := scope(kinds); nested || len(q.Sort) > 1 {
		return nil
	}
	key := SortKey{Field: "title"}
	if len(q.Sort) == 1 {
		key = q.Sort[0]
	}
	f := fieldsByKey[key.Field]
	if f == nil || f.indexed == "" && !f.state || key.Direction != "" && key.Direction != f.DefaultDirection {
		return nil
	}
	return f
}

// mine is where the works of kinds with a sort key of f's are, when the
// user's own rows give the key: the user's states us, or played pl for
// series, whose key is rolled up there. It is "" when no one table has
// them.
func (c *compiler) mine(f *fieldDef, kinds []catalog.WorkKind) string {
	switch {
	case !f.state:
		return ""
	case !f.rolled || !c.rolls:
		return "us"
	case slices.Equal(kinds, []catalog.WorkKind{catalog.KindSeries}):
		return "pl"
	}
	return ""
}

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
//
// A statement reads every work the query selects and sorts them, but for a
// read of one library in an order a page can be read in (see ordered).
// There it reads no further than the page's end: through the order's
// index, or for an order of the user's own, first the works that have a
// key, from the user's rows, and then the rest by title. Its counted
// statement counts the works in a pass of its own, unless the query has a
// cap.
func compile(q WorkQuery, vw viewer, now time.Time, seed string, only []catalog.WorkKind, rd reading) compiled {
	kinds := narrowed(q, only)
	_, _, nested := scope(kinds)
	c := &compiler{now: now, seed: seed, nested: nested,
		rolls: slices.Contains(kinds, catalog.KindSeries) || slices.Contains(kinds, catalog.KindSeason)}
	where := ""
	if q.Where != nil {
		where = "\n  AND " + c.group(*q.Where)
	}
	rules := *c // reads what the rules read, and not what only the order does
	columns, by := c.order(q)
	limit := ""
	if q.MaxWorks != nil {
		limit = "\nLIMIT " + c.bind(*q.MaxWorks)
	}
	with := "WITH "
	if c.played {
		with += played(kinds) + ",\n"
	}
	// selected selects the works with columns, reading work_library as
	// matching does from, and keeping those that also pass and.
	selected := func(from, columns, and string) string {
		return "SELECT wl.work_id AS id, w.kind AS kind, " + columns + c.matching(kinds, rd, from) + where + and
	}
	// capped cuts selected works in the order by at the query's cap.
	capped := func(works, by string) string {
		if limit == "" {
			return works
		}
		works += "\nORDER BY " + by + limit
		if len(only) > 0 {
			works = "SELECT * FROM (\n" + works + "\n) WHERE kind IN (SELECT value FROM json_each(@only))"
		}
		return works
	}
	// statement cuts its page from matched in the order by. It counts the
	// works of counted, a table of them with their sort titles t, when one
	// is given: how many there are, or for letters how many start with each
	// character, a code point or 0 for an empty title, and then their sum.
	statement := func(matched, by, counted string) string {
		rail, counts := "", ""
		switch {
		case counted != "" && rd.letters:
			rail = "rail AS MATERIALIZED (SELECT coalesce(unicode(t), 0) AS initial, count(*) AS n FROM " + counted + " GROUP BY 1),\n"
			counts = ", (SELECT sum(n) FROM rail), (SELECT json_group_array(json_array(initial, n)) FROM rail)"
		case counted != "":
			counts = ", (SELECT count(*) FROM " + counted + ")"
		}
		return with + "matched AS " + matched + ",\n" + rail + `page AS (SELECT * FROM matched ORDER BY ` + by + ` LIMIT @limit OFFSET @offset)
SELECT ` + summaryColumns + counts + `
FROM page p
JOIN work w ON w.id = p.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
ORDER BY p.` + strings.ReplaceAll(by, ", ", ", p.")
	}
	titled := columns
	if rd.letters {
		titled += ", wl.sort_title AS t"
	}
	out := compiled{
		counted: statement("MATERIALIZED (\n"+capped(selected("", titled, ""), by)+"\n)", by, "matched"),
		page:    statement("(\n"+capped(selected("", columns, ""), by)+"\n)", by, ""),
		args:    vw.args(append(c.params, sql.Named("only", set(only)), sql.Named("library", rd.library))...),
	}
	f := ordered(q, kinds)
	if f == nil || rd.library.IsZero() {
		return out
	}
	matched := ""
	if f.indexed != "" {
		columns, by = f.walk()
		matched = capped(selected(f.indexed, columns, ""), by)
	} else if from := c.mine(f, kinds); from != "" && limit == "" {
		// The works with a key, in order, and then those without, by title:
		// order names the key k0 and the sort title k1.
		key, ends := c.value(f, f.sort), "\nLIMIT @limit + @offset\n)"
		matched = "SELECT * FROM (\n" + selected(from, columns, "\n  AND "+key+" IS NOT NULL") + "\nORDER BY " + by + ends +
			"\nUNION ALL\nSELECT * FROM (\n" + selected("sort_title", columns, "\n  AND "+key+" IS NULL") + "\nORDER BY k1, id" + ends
	}
	if matched == "" {
		return out
	}
	out.page = statement("(\n"+matched+"\n)", by, "")
	if limit == "" {
		out.counted = statement("(\n"+matched+"\n)", by, "(SELECT wl.sort_title AS t"+rules.matching(kinds, rd, "")+where+")")
	}
	return out
}

// walk is the order of f's index, as order gives a query's: the columns
// it reads and the ORDER BY over their names and id.
func (f *fieldDef) walk() (columns, by string) {
	columns, by = "wl."+f.indexed+" AS k0", "k0"
	if f.DefaultDirection == "desc" {
		by += " DESC"
	}
	if f.indexed != "sort_title" {
		columns, by = columns+", wl.sort_title AS k1", by+", k1"
	}
	return columns, by + ", id"
}

// compiler builds one statement's WHERE clause and order, with their
// parameters, and notes what they read besides each work and its
// work_library row.
type compiler struct {
	params []any
	now    time.Time
	seed   string

	nested bool // the works include seasons or episodes, so each one's root r is read
	rolls  bool // the works include series or seasons
	state  bool // a rule or the order reads the user's state us
	played bool // a rule or the order reads played pl
}

// bind adds a parameter and returns its name.
func (c *compiler) bind(v any) string {
	name := "p" + strconv.Itoa(len(c.params))
	c.params = append(c.params, sql.Named(name, v))
	return "@" + name
}

// value is a field's expr or sort as the statement's works have it: a
// season's or episode's own or else its series', and a series' or season's
// rolled up from its works.
func (c *compiler) value(f *fieldDef, expr string) string {
	c.state = c.state || f.state
	switch {
	case f.inherit != "" && c.nested:
		return "coalesce(" + expr + ", " + f.inherit + ")"
	case f.rolled && c.rolls:
		c.played = true
		return "CASE WHEN w.kind IN ('series', 'season') THEN pl.at ELSE " + expr + " END"
	}
	return expr
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
	}
	expr := c.value(f, f.expr)
	switch {
	case f.Type == FieldText:
		return c.textRule(expr, r)
	case f.ms:
		return c.scalarRule(f, dayOf(expr), r)
	case f.Type == FieldNumber || f.Type == FieldDate:
		return c.scalarRule(f, expr, r)
	}
	return c.choiceRule(f, expr, r)
}

func (c *compiler) textRule(expr string, r Rule) string {
	p := c.bind(store.Fold(r.Values[0]))
	fold := "fold(" + expr + ")"
	switch r.Op {
	case "contains":
		return "instr(" + fold + ", " + p + ") > 0"
	case "notContains":
		return "(" + expr + " IS NULL OR instr(" + fold + ", " + p + ") = 0)"
	}
	return "substr(" + fold + ", 1, length(" + p + ")) = " + p
}

func (c *compiler) scalarRule(f *fieldDef, expr string, r Rule) string {
	switch r.Op {
	case "isSet":
		return expr + " IS NOT NULL"
	case "isNotSet":
		return expr + " IS NULL"
	case "within":
		return expr + " BETWEEN " + c.bind(c.cutoff(r.Values[0])) + " AND " + c.bind(c.today())
	case "notWithin":
		return "(" + expr + " IS NULL OR " + expr + " NOT BETWEEN " + c.bind(c.cutoff(r.Values[0])) + " AND " + c.bind(c.today()) + ")"
	case "between":
		return expr + " BETWEEN " + c.bind(f.scalar(r.Values[0])) + " AND " + c.bind(f.scalar(r.Values[1]))
	case "gte":
		return expr + " >= " + c.bind(f.scalar(r.Values[0]))
	case "lte":
		return expr + " <= " + c.bind(f.scalar(r.Values[0]))
	}
	return expr + " = " + c.bind(f.scalar(r.Values[0]))
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

func (c *compiler) choiceRule(f *fieldDef, expr string, r Rule) string {
	in := expr + " IN " + c.members(f, r.Values)
	if r.Op == "noneOf" {
		return "(" + expr + " IS NULL OR NOT " + in + ")"
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

// members is the SQL list of a rule's values, bound as one parameter.
func (c *compiler) members(f *fieldDef, values []string) string {
	if !f.ids {
		return "(SELECT value FROM json_each(" + c.bind(set(values)) + "))"
	}
	return "(SELECT unhex(value) FROM json_each(" + c.bind(id.Set(parsed(values)...)) + "))"
}

// parsed is a valid rule's values as the IDs they are.
func parsed(values []string) []id.ID {
	ids := make([]id.ID, len(values))
	for i, v := range values {
		ids[i], _ = id.Parse(v)
	}
	return ids
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
// season and episode number, which only works under a root have, sort
// title, then ID.
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
		name := column(c.value(f, f.sort))
		terms = append(terms, name+" IS NULL", name+" "+strings.ToUpper(dir))
	}
	if c.nested {
		terms = append(terms, column("w.parent_number"), column("w.number"))
	}
	terms = append(terms, column("wl.sort_title"), "id")
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

// holdingLibraries reads the libraries of @kinds the Viewer is granted,
// among @among when it's set, and no more than two: what matters is
// whether there is one.
var holdingLibraries = stmt("HoldingLibraries", `
SELECT l.id
FROM library l
WHERE l.kind IN (SELECT value FROM json_each(@kinds))
  AND (@among IS NULL OR l.id IN (SELECT unhex(value) FROM json_each(@among)))
  AND `+libraryVisible+`
LIMIT 2`)

// compilePage compiles q for a page read without its count. When a page
// of one library's works can be read in the query's order (see ordered)
// and the read names no library, it looks for the one library that holds
// the query's works, for the page to be read so there: the only library of
// their kinds the Viewer is granted, among those a rule every work must
// pass names.
func compilePage(ctx context.Context, db querier, q WorkQuery, vw viewer, now time.Time, seed string, only []catalog.WorkKind, rd reading) (compiled, error) {
	kinds := narrowed(q, only)
	if ordered(q, kinds) == nil || !rd.library.IsZero() {
		return compile(q, vw, now, seed, only, rd), nil
	}
	var holding []catalog.LibraryKind
	for _, k := range kinds {
		if l, ok := libraryOf[k]; ok && !slices.Contains(holding, l) {
			holding = append(holding, l)
		}
	}
	var among any
	if q.Where != nil && q.Where.Match == "all" {
		if i := slices.IndexFunc(q.Where.Rules, func(r Rule) bool { return r.Field == "library" && r.Op == "anyOf" }); i >= 0 {
			among = id.Set(parsed(q.Where.Rules[i].Values)...)
		}
	}
	libraries, err := rowsOf(ctx, db, holdingLibraries, vw.args(sql.Named("kinds", set(holding)), sql.Named("among", among)),
		func(r scanner, l *id.ID) error { return r.Scan(l) })
	if err != nil {
		return compiled{}, err
	}
	if len(libraries) == 1 {
		rd.library = libraries[0]
	}
	return compile(q, vw, now, seed, only, rd), nil
}

// runQuery reads a page of a valid query, from one snapshot.
func (s *Service) runQuery(ctx context.Context, vw viewer, q WorkQuery, seed string, only []catalog.WorkKind, o GridOptions, now time.Time) (GridPage[WorkSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	var items []WorkSummary
	total := 0
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		items, total, _, err = readQuery(ctx, tx, vw, q, seed, only, reading{}, o.Offset, limit, now)
		return err
	})
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return s.summaryPage(ctx, vw, items, total, nil)
}

// readQuery reads the page of a valid query's works at offset. The first
// page, at offset 0, counts the query's works in the statement that reads
// it, and has the letters the read asks for. A later page doesn't count:
// its total is 0, and it has no letters.
func readQuery(ctx context.Context, tx *sql.Tx, vw viewer, q WorkQuery, seed string, only []catalog.WorkKind, rd reading,
	offset, limit int, now time.Time) (items []WorkSummary, total int, letters []Letter, err error) {
	page := []any{sql.Named("limit", limit), sql.Named("offset", offset)}
	if offset == 0 {
		c := compile(q, vw, now, seed, only, rd)
		return countedSummaries(ctx, tx, statement{sql: c.counted}, append(c.args, page...), rd.letters)
	}
	c, err := compilePage(ctx, tx, q, vw, now, seed, only, rd)
	if err != nil {
		return nil, 0, nil, err
	}
	items, err = summaries(ctx, tx, statement{sql: c.page}, append(c.args, page...))
	return items, 0, nil, err
}

// countedSummaries runs a compiled query's counted statement and returns
// its summaries and its count, 0 when it has no rows. With letters, it
// returns where each letter starts, which is nowhere when it has no rows.
func countedSummaries(ctx context.Context, q querier, st statement, args []any, letters bool) ([]WorkSummary, int, []Letter, error) {
	total := 0
	var counts sql.NullString
	more := []any{&total}
	if letters {
		more = append(more, &counts)
	}
	out, err := rowsOf(ctx, q, st, args, func(r scanner, c *WorkSummary) (err error) {
		*c, err = scanSummary(r, more...)
		return err
	})
	if err != nil || !letters {
		return out, total, nil, err
	}
	// The rail's rows are each a character and how many works start with
	// it. In the characters' order they are in the works' order, because
	// UTF-8 sorts by code point.
	var rows [][2]int64
	if counts.Valid {
		if err := json.Unmarshal([]byte(counts.String), &rows); err != nil {
			return nil, 0, nil, err
		}
	}
	slices.SortFunc(rows, func(a, b [2]int64) int { return cmp.Compare(a[0], b[0]) })
	byLetter := make([]letterCount, len(rows))
	for i, r := range rows {
		byLetter[i] = letterCount{Initial: r[0], N: r[1]}
	}
	starts, _ := railLetters(byLetter)
	return out, total, starts, nil
}
