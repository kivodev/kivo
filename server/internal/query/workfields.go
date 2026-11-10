package query

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// FieldType is what a field's values are, which decides its operators.
type FieldType string

// The field types.
const (
	FieldChoice FieldType = "choice"
	FieldSet    FieldType = "set"
	FieldPerson FieldType = "person"
	FieldNumber FieldType = "number"
	FieldDate   FieldType = "date"
	FieldText   FieldType = "text"
)

// FieldOp is an operator a field takes, with how many values: Arity is
// exact, or -1 for one or more.
type FieldOp struct {
	Op    string
	Arity int
}

// fieldOps are each field type's operators.
var fieldOps = map[FieldType][]FieldOp{
	FieldChoice: {{"anyOf", -1}, {"noneOf", -1}},
	FieldSet:    {{"anyOf", -1}, {"allOf", -1}, {"noneOf", -1}, {"isSet", 0}, {"isNotSet", 0}},
	FieldPerson: {{"anyOf", -1}, {"allOf", -1}, {"noneOf", -1}, {"isSet", 0}, {"isNotSet", 0}},
	FieldNumber: {{"eq", 1}, {"gte", 1}, {"lte", 1}, {"between", 2}, {"isSet", 0}, {"isNotSet", 0}},
	FieldDate:   {{"gte", 1}, {"lte", 1}, {"between", 2}, {"within", 1}, {"notWithin", 1}, {"isSet", 0}, {"isNotSet", 0}},
	FieldText:   {{"contains", 1}, {"notContains", 1}, {"startsWith", 1}},
}

// WorkField is a field rules can use, as the catalog lists it.
type WorkField struct {
	Field            string
	Name             string
	Type             FieldType
	Ops              []FieldOp
	Kinds            []catalog.WorkKind
	Sortable         bool
	DefaultDirection string // "asc" or "desc", for a sortable field
	Unit             string
	ValueSource      string // "inline" or "search", for a field with values to choose from
}

// FieldValue is a value a field can take, with what to call it.
type FieldValue struct {
	Value string
	Name  string
}

// fieldDef is a field with what compiling it needs. A scalar field reads
// expr, NULL when unset; a set field reads rows of from joined by on, each
// row's value in value; bits fields read a mask column of work_library. A
// sortable field orders by sort.
//
// A field of a series that its seasons and episodes share, such as its
// genres, joins on w.root_id, which is a root work's own ID.
type fieldDef struct {
	WorkField
	expr   string
	sort   string
	from   string
	on     string
	value  string
	ids    bool // value, or a scalar field's expr, is an ID
	bits   map[string]int64
	column string

	ms      bool   // expr is in epoch milliseconds, which a rule compares as its UTC day
	state   bool   // expr or sort reads the user's state us
	inherit string // what a season or episode with no expr takes: its series', read from the root r
	rolled  bool   // a series' or season's expr and sort are pl.at, rolled up in played
	// indexed is the work_library column the field sorts by, when an index
	// in sortIndexes holds a library's root works in the sort's order.
	indexed string

	inline []FieldValue
	stmt   *fieldValues // reads the field's values, for fields whose values the works have
}

// sortIndexes are the indexes that hold a library's root works in a sort's
// order, by the work_library column they order: by that column, in its
// field's default direction, then by sort title and ID. Each holds is_root
// too, so a read of the root rows that names no other column of
// work_library takes them from the index alone.
var sortIndexes = map[string]string{
	"sort_title":    "work_library_title",
	"added_at":      "work_library_added",
	"release_key":   "work_library_release",
	"rating_sort":   "work_library_rating",
	"runtime_sort":  "work_library_runtime",
	"creator_sort":  "work_library_creator",
	"last_added_at": "work_library_last_added",
}

// creditOf is the work to read a role's credits from: a season's or
// episode's own, or else its series'.
func creditOf(role string) string {
	return `x.work_id = CASE WHEN w.parent_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM credit own WHERE own.work_id = w.id AND own.role = '` + role + `') THEN w.root_id ELSE w.id END
    AND x.role = '` + role + `'`
}

// dayOf renders an epoch-milliseconds expression as a UTC day.
func dayOf(ms string) string { return "date(" + ms + " / 1000, 'unixepoch')" }

// played is the user's last play of each series and season among kinds,
// which have no state of their own: a series' is the latest of the works
// under it and itself, a season's of the works in it. A work is never both
// a root and a parent under one, so each has one row.
func played(kinds []catalog.WorkKind) string {
	var parts []string
	if slices.Contains(kinds, catalog.KindSeries) {
		parts = append(parts, `SELECT pw.root_id, max(pu.last_played_at)
  FROM user_state pu CROSS JOIN work pw ON pw.id = pu.work_id
  WHERE pu.user_id = @user AND pu.last_played_at IS NOT NULL
  GROUP BY pw.root_id`)
	}
	if slices.Contains(kinds, catalog.KindSeason) {
		parts = append(parts, `SELECT pw.parent_id, max(pu.last_played_at)
  FROM user_state pu CROSS JOIN work pw ON pw.id = pu.work_id
  WHERE pu.user_id = @user AND pu.last_played_at IS NOT NULL AND pw.parent_id <> pw.root_id
  GROUP BY pw.parent_id`)
	}
	return "played (work, at) AS MATERIALIZED (\n  " + strings.Join(parts, "\n  UNION ALL\n  ") + "\n)"
}

// rollupPlayState is a series' or season's play state from its episodes,
// those with column pointing at w: played when the user has played every
// one in a library the Viewer is granted, and in progress when the user
// has played or started any. The works with one played or started are read
// once for the statement, from the user's states.
func rollupPlayState(column string) string {
	return `CASE
      WHEN NOT EXISTS (SELECT 1 FROM work fe WHERE ` + column + ` = w.id AND fe.kind = 'episode'
        AND EXISTS (SELECT 1 FROM work_library fl WHERE fl.work_id = fe.id AND ` + catalog.Granted("fl.library_id") + `)
        AND NOT EXISTS (SELECT 1 FROM user_state fu WHERE fu.user_id = @user AND fu.work_id = fe.id AND fu.played))
        THEN 'played'
      WHEN w.id IN (SELECT ` + column + ` FROM user_state fu CROSS JOIN work fe ON fe.id = fu.work_id
        WHERE fu.user_id = @user AND (fu.played OR fu.resumable) AND fe.kind = 'episode')
        THEN 'inProgress'
      ELSE 'unplayed' END`
}

// playState is the user's state: a series' and season's roll up their
// episodes; no state means unplayed.
var playState = `CASE w.kind WHEN 'series' THEN ` + rollupPlayState("fe.root_id") + `
    WHEN 'season' THEN ` + rollupPlayState("fe.parent_id") + `
    ELSE coalesce(CASE WHEN us.played THEN 'played' WHEN us.resumable THEN 'inProgress' END, 'unplayed') END`

// fieldValues are the statements that read a field's values: every one the
// Viewer sees, at most valuesLimit of them, and those of one library's
// top-level works, @library's. A field whose values aren't works' has no
// library statement, and reads all in a library too. A field a rule takes
// no chosen values of has no all statement, and has values in a library
// alone.
type fieldValues struct {
	all, library *statement
}

// valuesLimit caps a search of a field's values in every library.
const valuesLimit = "100"

// workValues is the values statements of a field whose values come from
// works w. sql selects each value and its name, with {works} after the
// join of w, {seen} for the works to read and {limit} at its end.
func workValues(name, sql string) *fieldValues {
	all := stmt("Values"+name, strings.NewReplacer("{works}", "", "{seen}", visible, "{limit}", "\nLIMIT "+valuesLimit).Replace(sql))
	return &fieldValues{all: &all, library: libraryValuesOf(name, sql)}
}

// libraryValuesOf is workValues' statement for one library.
func libraryValuesOf(name, sql string) *statement {
	st := stmt("LibraryValues"+name, strings.NewReplacer(
		"{works}", "\nJOIN work_library wl ON wl.work_id = w.id", "{seen}", libraryWorks, "{limit}", "").Replace(sql))
	return &st
}

// allValues is the values statement of a field whose values aren't
// works'.
func allValues(name, sql string) *fieldValues {
	all := stmt(name, sql)
	return &fieldValues{all: &all}
}

// nameValues is the values statements for a field stored by name.
func nameValues(name, table, workTable, key string) *fieldValues {
	return workValues(name, `
SELECT DISTINCT n.name, n.name
FROM `+table+` n
JOIN `+workTable+` x ON x.`+key+` = n.id
JOIN work w ON w.id = x.work_id{works}
WHERE (@q IS NULL OR instr(fold(n.name), @q) > 0)
  AND (@names IS NULL OR n.name IN (SELECT value FROM json_each(@names)))
  AND {seen}
ORDER BY n.name{limit}`)
}

// personValues is the values statements for a person field.
func personValues(name, role string) *fieldValues {
	return workValues(name, `
SELECT DISTINCT p.id, p.name
FROM person p
JOIN credit x ON x.person_id = p.id AND x.role = '`+role+`'
JOIN work w ON w.id = x.work_id{works}
WHERE (@q IS NULL OR instr(fold(p.name), @q) > 0)
  AND (@ids IS NULL OR p.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND {seen}
ORDER BY p.name{limit}`)
}

var (
	collectionValues = workValues("Collection", `
SELECT DISTINCT c.id, c.name
FROM collection c
JOIN collection_work x ON x.collection_id = c.id
JOIN work w ON w.id = x.work_id{works}
WHERE (@q IS NULL OR instr(fold(c.name), @q) > 0)
  AND (@ids IS NULL OR c.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND {seen}
ORDER BY c.name{limit}`)
	bookSeriesValues = workValues("BookSeries", `
SELECT DISTINCT s.id, s.name
FROM book_series s
JOIN book_series_book x ON x.series_id = s.id
JOIN work w ON w.id = x.work_id{works}
WHERE (@q IS NULL OR instr(fold(s.name), @q) > 0)
  AND (@ids IS NULL OR s.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND {seen}
ORDER BY s.name{limit}`)
	rootWorkValues = workValues("RootWork", `
SELECT w.id, w.title
FROM work w{works}
WHERE w.parent_id IS NULL AND NOT w.provisional
  AND (@q IS NULL OR instr(fold(w.title), @q) > 0)
  AND (@ids IS NULL OR w.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND {seen}
ORDER BY w.sort_title{limit}`)
	// The years a library's works were released in, oldest first.
	yearValues = &fieldValues{library: libraryValuesOf("Year", `
SELECT cast(w.year AS TEXT), cast(w.year AS TEXT)
FROM work w{works}
WHERE w.year IS NOT NULL
  AND (@q IS NULL OR instr(cast(w.year AS TEXT), @q) > 0)
  AND (@names IS NULL OR cast(w.year AS TEXT) IN (SELECT value FROM json_each(@names)))
  AND {seen}
GROUP BY w.year
ORDER BY w.year{limit}`)}
	// The content ratings, youngest first by the age each one means.
	contentRatingValues = workValues("ContentRating", `
SELECT w.content_rating, w.content_rating
FROM work w{works}
WHERE w.content_rating IS NOT NULL
  AND (@q IS NULL OR instr(fold(w.content_rating), @q) > 0)
  AND (@names IS NULL OR w.content_rating IN (SELECT value FROM json_each(@names)))
  AND {seen}
GROUP BY w.content_rating
ORDER BY min(w.content_rating_age), w.content_rating{limit}`)
	libraryValues = allValues("ValuesLibrary", `
SELECT l.id, l.name
FROM library l
WHERE l.deleting_at IS NULL
  AND (@q IS NULL OR instr(fold(l.name), @q) > 0)
  AND (@ids IS NULL OR l.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND `+libraryVisible+`
ORDER BY l.name
LIMIT `+valuesLimit)
	listValues = allValues("ValuesInList", `
-- viewer-filter: user
SELECT l.id, l.name
FROM list l
WHERE l.user_id = @user AND l.query IS NULL
  AND (@q IS NULL OR instr(fold(l.name), @q) > 0)
  AND (@ids IS NULL OR l.id IN (SELECT unhex(value) FROM json_each(@ids)))
ORDER BY l.built_in DESC, l.name COLLATE NOCASE
LIMIT `+valuesLimit)
	// libraryVideo lists the resolution and dynamic range bit sets of a
	// library's top-level works.
	libraryVideo = stmt("LibraryValuesVideo", `
SELECT DISTINCT wl.resolutions, wl.dynamic_ranges
FROM work_library wl
JOIN work w ON w.id = wl.work_id
WHERE `+libraryWorks)
)

var (
	allKinds    = []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindSeason, catalog.KindEpisode, catalog.KindVideo, catalog.KindBook}
	videoKinds  = []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindSeason, catalog.KindEpisode, catalog.KindVideo}
	versioned   = []catalog.WorkKind{catalog.KindMovie, catalog.KindEpisode, catalog.KindVideo}
	booksOnly   = []catalog.WorkKind{catalog.KindBook}
	rangeValues = []FieldValue{{"hdr", "HDR"}, {"dolbyVision", "Dolby Vision"}}
	resolutions = []FieldValue{{"4k", "4K"}, {"1080p", "1080p"}, {"720p", "720p"}, {"sd", "SD"}}
)

// workFields is the registry: every field a rule or sort key can name, in
// the order clients offer them, the common ones first.
var workFields = []*fieldDef{
	{WorkField: WorkField{Field: "genre", Name: "Genre", Type: FieldSet, Kinds: allKinds, ValueSource: "inline"},
		from: "work_genre x JOIN genre n ON n.id = x.genre_id", on: "x.work_id = w.root_id", value: "n.name",
		stmt: nameValues("Genre", "genre", "work_genre", "genre_id")},
	{WorkField: WorkField{Field: "year", Name: "Year", Type: FieldNumber, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "nullif(wl.year, 0)", sort: "nullif(wl.year, 0)", stmt: yearValues},
	{WorkField: WorkField{Field: "playState", Name: "Play state", Type: FieldChoice, Kinds: allKinds, ValueSource: "inline"},
		expr: playState, state: true, inline: []FieldValue{{"unplayed", "Unplayed"}, {"inProgress", "In progress"}, {"played", "Played"}}},
	// The rating a card shows: a work's community rating when it has votes.
	{WorkField: WorkField{Field: "rating", Name: "Rating", Type: FieldNumber, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "CASE WHEN w.community_votes IS NOT NULL THEN w.community_rating END", sort: "nullif(wl.rating_sort, -1)", indexed: "rating_sort"},
	{WorkField: WorkField{Field: "releaseDate", Name: "Release date", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "nullif(wl.release_key, '')", sort: "nullif(wl.release_key, '')", indexed: "release_key"},
	{WorkField: WorkField{Field: "addedAt", Name: "Date added", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "wl.added_at", ms: true, sort: "wl.added_at", indexed: "added_at"},
	{WorkField: WorkField{Field: "actor", Name: "Actor", Type: FieldPerson, Kinds: videoKinds, ValueSource: "search"},
		from: "credit x", on: creditOf("actor"), value: "x.person_id", ids: true, stmt: personValues("Actor", "actor")},
	{WorkField: WorkField{Field: "director", Name: "Director", Type: FieldPerson, Kinds: videoKinds, ValueSource: "search"},
		from: "credit x", on: creditOf("director"), value: "x.person_id", ids: true, stmt: personValues("Director", "director")},

	{WorkField: WorkField{Field: "library", Name: "Library", Type: FieldChoice, Kinds: allKinds, ValueSource: "inline"},
		expr: "wl.library_id", ids: true, stmt: libraryValues},
	{WorkField: WorkField{Field: "contentRating", Name: "Content rating", Type: FieldChoice, Kinds: allKinds, ValueSource: "inline"},
		expr: "w.content_rating", inherit: "r.content_rating", stmt: contentRatingValues},

	{WorkField: WorkField{Field: "tag", Name: "Tag", Type: FieldSet, Kinds: allKinds, ValueSource: "search"},
		from: "work_tag x JOIN tag n ON n.id = x.tag_id", on: "x.work_id = w.root_id", value: "n.name",
		stmt: nameValues("Tag", "tag", "work_tag", "tag_id")},
	{WorkField: WorkField{Field: "company", Name: "Company", Type: FieldSet, Kinds: allKinds, ValueSource: "search"},
		from: "work_company x JOIN company n ON n.id = x.company_id", on: "x.work_id = w.root_id", value: "n.name",
		stmt: nameValues("Company", "company", "work_company", "company_id")},
	{WorkField: WorkField{Field: "resolution", Name: "Resolution", Type: FieldSet, Kinds: versioned, ValueSource: "inline"},
		bits: named(resolutionBits), column: "resolutions", inline: resolutions},
	{WorkField: WorkField{Field: "dynamicRange", Name: "Dynamic range", Type: FieldSet, Kinds: versioned, ValueSource: "inline"},
		bits: named(rangeBits), column: "dynamic_ranges", inline: rangeValues},
	{WorkField: WorkField{Field: "collection", Name: "Collection", Type: FieldSet, Kinds: []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindVideo}, ValueSource: "search"},
		from: "collection_work x", on: "x.work_id = w.id", value: "x.collection_id", ids: true, stmt: collectionValues},
	{WorkField: WorkField{Field: "bookSeries", Name: "Book series", Type: FieldSet, Kinds: booksOnly, ValueSource: "search"},
		from: "book_series_book x", on: "x.work_id = w.id", value: "x.series_id", ids: true, stmt: bookSeriesValues},
	{WorkField: WorkField{Field: "inList", Name: "In list", Type: FieldSet, Kinds: allKinds, ValueSource: "inline"},
		from: "list_work x JOIN list xl ON xl.id = x.list_id", on: "x.work_id = w.id AND xl.user_id = @user", value: "x.list_id", ids: true, stmt: listValues},
	{WorkField: WorkField{Field: "rootWork", Name: "Show or movie", Type: FieldSet, Kinds: allKinds, ValueSource: "search"},
		from: "work x", on: "x.id = w.root_id", value: "x.id", ids: true, stmt: rootWorkValues},

	// It sorts by the first author's sort name, a work with no author first.
	{WorkField: WorkField{Field: "author", Name: "Author", Type: FieldPerson, Kinds: booksOnly, Sortable: true, DefaultDirection: "asc", ValueSource: "search"},
		from: "credit x", on: "x.work_id = w.id AND x.role = 'author'", value: "x.person_id", ids: true, stmt: personValues("Author", "author"),
		sort: "wl.creator_sort", indexed: "creator_sort"},
	{WorkField: WorkField{Field: "narrator", Name: "Narrator", Type: FieldPerson, Kinds: booksOnly, ValueSource: "search"},
		from: "credit x", on: "x.work_id = w.id AND x.role = 'narrator'", value: "x.person_id", ids: true, stmt: personValues("Narrator", "narrator")},

	{WorkField: WorkField{Field: "myRating", Name: "My rating", Type: FieldNumber, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "us.rating", sort: "us.rating", state: true},
	{WorkField: WorkField{Field: "runtimeMs", Name: "Runtime", Type: FieldNumber, Kinds: append(slices.Clone(versioned), catalog.KindBook), Sortable: true, DefaultDirection: "asc", Unit: "ms"},
		expr: "nullif(wl.runtime_sort, 9223372036854775807)", sort: "nullif(wl.runtime_sort, 9223372036854775807)", indexed: "runtime_sort"},

	{WorkField: WorkField{Field: "lastEpisodeAddedAt", Name: "Latest addition", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "nullif(wl.last_added_at, 0)", ms: true, sort: "nullif(wl.last_added_at, 0)", indexed: "last_added_at"},
	{WorkField: WorkField{Field: "lastPlayedAt", Name: "Last played", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "us.last_played_at", ms: true, sort: "us.last_played_at", state: true, rolled: true},

	{WorkField: WorkField{Field: "title", Name: "Title", Type: FieldText, Kinds: allKinds, Sortable: true, DefaultDirection: "asc"},
		expr: "w.title", sort: "wl.sort_title", indexed: "sort_title"},
}

// fieldsByKey is the registry by field name, with each field's operators.
var fieldsByKey = func() map[string]*fieldDef {
	out := make(map[string]*fieldDef, len(workFields))
	for _, f := range workFields {
		f.Ops = fieldOps[f.Type]
		out[f.Field] = f
	}
	return out
}()

// WorkFields returns the fields rules can use.
func WorkFields() []WorkField {
	out := make([]WorkField, len(workFields))
	for i, f := range workFields {
		out[i] = f.WorkField
	}
	return out
}

// FieldValues returns a field's values the Viewer can see: those matching
// q, or with ids those values' names. With a library, they are the values
// that its top-level works have, all of them: what its grid's filters
// offer. A field with no values to choose returns none; an unknown field,
// or a library the Viewer doesn't see, returns ErrNotFound.
func (s *Service) FieldValues(ctx context.Context, v users.Viewer, field string, lib id.ID, q string, ids []string) ([]FieldValue, error) {
	f, ok := fieldsByKey[field]
	if !ok {
		return nil, ErrNotFound
	}
	vw := viewerOf(v)
	args, err := f.valueArgs(vw, lib, q, ids)
	if err != nil {
		return nil, err
	}
	var out []FieldValue
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if !lib.IsZero() {
			if err := library(ctx, tx, vw, lib); err != nil {
				return notFound(err)
			}
		}
		out, err = f.values(ctx, tx, lib, q, ids, args)
		return err
	})
	return out, err
}

// valueArgs is the arguments of f's values statements: the search q
// folded, the library, and ids as the names or the IDs they are.
func (f *fieldDef) valueArgs(vw viewer, lib id.ID, q string, ids []string) ([]any, error) {
	var search any
	if q != "" {
		search = store.Fold(q)
	}
	args := vw.args(sql.Named("q", search), sql.Named("library", lib))
	if !f.ids {
		return append(args, sql.Named("names", set(ids))), nil
	}
	parsed := make([]id.ID, 0, len(ids))
	for _, raw := range ids {
		u, err := id.Parse(raw)
		if err != nil {
			return nil, invalid("ids", "invalid", fmt.Sprintf("id %q", raw))
		}
		parsed = append(parsed, u)
	}
	var picked any
	if len(parsed) > 0 {
		picked = id.Set(parsed...)
	}
	return append(args, sql.Named("ids", picked)), nil
}

// values reads f's values with args: its fixed ones, those of them lib's
// works have for a bits field, or its statement's.
func (f *fieldDef) values(ctx context.Context, tx querier, lib id.ID, q string, ids []string, args []any) ([]FieldValue, error) {
	if f.stmt == nil {
		inline := f.inline
		if f.bits != nil && !lib.IsZero() {
			var err error
			if inline, err = f.present(ctx, tx, args); err != nil {
				return nil, err
			}
		}
		return inlineValues(inline, q, ids), nil
	}
	st := f.stmt.all
	if !lib.IsZero() && f.stmt.library != nil {
		st = f.stmt.library
	}
	if st == nil {
		return nil, nil
	}
	return rowsOf(ctx, tx, *st, args, func(r scanner, fv *FieldValue) error {
		if !f.ids {
			return r.Scan(&fv.Value, &fv.Name)
		}
		var u id.ID
		err := r.Scan(&u, &fv.Name)
		fv.Value = u.String()
		return err
	})
}

// present is the values of a bits field that @library's top-level works
// have, in the field's own order.
func (f *fieldDef) present(ctx context.Context, q querier, args []any) ([]FieldValue, error) {
	var has int64
	err := each(ctx, q, libraryVideo, args, func(r scanner) error {
		var resolutions, ranges int64
		err := r.Scan(&resolutions, &ranges)
		if f.column == "resolutions" {
			has |= resolutions
		} else {
			has |= ranges
		}
		return err
	})
	var out []FieldValue
	for _, v := range f.inline {
		if has&f.bits[v.Value] != 0 {
			out = append(out, v)
		}
	}
	return out, err
}

// inlineValues filters a field's fixed values by q or ids.
func inlineValues(values []FieldValue, q string, ids []string) []FieldValue {
	if len(values) == 0 {
		return nil
	}
	out := make([]FieldValue, 0, len(values))
	key := store.Fold(q)
	for _, v := range values {
		switch {
		case len(ids) > 0:
			for _, want := range ids {
				if v.Value == want {
					out = append(out, v)
				}
			}
		case key == "" || strings.Contains(store.Fold(v.Name), key):
			out = append(out, v)
		}
	}
	return out
}
