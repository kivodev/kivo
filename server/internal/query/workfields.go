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
// row's value in value; bits fields read a mask column of work_library.
type fieldDef struct {
	WorkField
	expr   string
	sort   string
	from   string
	on     string
	value  string
	ids    bool // value is an ID
	bits   map[string]int64
	column string

	inline []FieldValue
	stmt   *statement // reads the field's values, for fields with many
}

// seriesOf is the work a season or episode takes its series-level fields
// from: its series; any other work is its own.
const seriesOf = "CASE WHEN w.kind IN ('season', 'episode') THEN w.root_id ELSE w.id END"

// creditOf is the work to read a role's credits from: a season's or
// episode's own, or else its series'.
func creditOf(role string) string {
	return `x.work_id = CASE WHEN w.parent_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM credit own WHERE own.work_id = w.id AND own.role = '` + role + `') THEN w.root_id ELSE w.id END
    AND x.role = '` + role + `'`
}

// dayOf renders an epoch-milliseconds expression as a UTC day.
func dayOf(ms string) string { return "date(" + ms + " / 1000, 'unixepoch')" }

var (
	// lastPlayedMs is the user's last play: a series' or season's rolls up
	// its episodes.
	lastPlayedMs = `CASE w.kind
    WHEN 'series' THEN (SELECT max(fu.last_played_at) FROM work fe JOIN user_state fu ON fu.work_id = fe.id
      WHERE fe.root_id = w.id AND fu.user_id = @user)
    WHEN 'season' THEN (SELECT max(fu.last_played_at) FROM work fe JOIN user_state fu ON fu.work_id = fe.id
      WHERE fe.parent_id = w.id AND fu.user_id = @user)
    ELSE us.last_played_at END`
	// playState is the user's state: a series' and season's roll up their
	// episodes; no state means unplayed.
	playState = `CASE w.kind WHEN 'series' THEN ` + rollupPlayState("fe.root_id") + `
    WHEN 'season' THEN ` + rollupPlayState("fe.parent_id") + `
    ELSE coalesce(CASE WHEN us.played THEN 'played' WHEN us.resumable THEN 'inProgress' END, 'unplayed') END`
)

// nameValues is the values statement for a field stored by name.
func nameValues(name, table, workTable, key string) *statement {
	s := stmt("Values"+name, `
SELECT DISTINCT n.name, n.name
FROM `+table+` n
JOIN `+workTable+` x ON x.`+key+` = n.id
JOIN work w ON w.id = x.work_id
WHERE (@q IS NULL OR instr(fold(n.name), @q) > 0)
  AND (@names IS NULL OR n.name IN (SELECT value FROM json_each(@names)))
  AND `+visible+`
ORDER BY n.name
LIMIT `+valuesLimit)
	return &s
}

// personValues is the values statement for a person field.
func personValues(name, role string) *statement {
	s := stmt("Values"+name, `
SELECT DISTINCT p.id, p.name
FROM person p
JOIN credit x ON x.person_id = p.id AND x.role = '`+role+`'
JOIN work w ON w.id = x.work_id
WHERE (@q IS NULL OR instr(fold(p.name), @q) > 0)
  AND (@ids IS NULL OR p.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND `+visible+`
ORDER BY p.name
LIMIT `+valuesLimit)
	return &s
}

// valuesLimit caps a search of a field's values.
const valuesLimit = "100"

var (
	collectionValues = stmt("ValuesCollection", `
SELECT DISTINCT c.id, c.name
FROM collection c
JOIN collection_work x ON x.collection_id = c.id
JOIN work w ON w.id = x.work_id
WHERE (@q IS NULL OR instr(fold(c.name), @q) > 0)
  AND (@ids IS NULL OR c.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND `+visible+`
ORDER BY c.name
LIMIT `+valuesLimit)
	bookSeriesValues = stmt("ValuesBookSeries", `
SELECT DISTINCT s.id, s.name
FROM book_series s
JOIN book_series_book x ON x.series_id = s.id
JOIN work w ON w.id = x.work_id
WHERE (@q IS NULL OR instr(fold(s.name), @q) > 0)
  AND (@ids IS NULL OR s.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND `+visible+`
ORDER BY s.name
LIMIT `+valuesLimit)
	rootWorkValues = stmt("ValuesRootWork", `
SELECT w.id, w.title
FROM work w
WHERE w.parent_id IS NULL AND NOT w.provisional
  AND (@q IS NULL OR instr(fold(w.title), @q) > 0)
  AND (@ids IS NULL OR w.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND `+visible+`
ORDER BY w.sort_title
LIMIT `+valuesLimit)
	contentRatingValues = stmt("ValuesContentRating", `
SELECT DISTINCT w.content_rating, w.content_rating
FROM work w
WHERE w.content_rating IS NOT NULL
  AND (@q IS NULL OR instr(fold(w.content_rating), @q) > 0)
  AND (@names IS NULL OR w.content_rating IN (SELECT value FROM json_each(@names)))
  AND `+visible+`
ORDER BY w.content_rating
LIMIT `+valuesLimit)
	libraryValues = stmt("ValuesLibrary", `
SELECT l.id, l.name
FROM library l
WHERE l.deleting_at IS NULL
  AND (@q IS NULL OR instr(fold(l.name), @q) > 0)
  AND (@ids IS NULL OR l.id IN (SELECT unhex(value) FROM json_each(@ids)))
  AND `+libraryVisible+`
ORDER BY l.name
LIMIT `+valuesLimit)
	listValues = stmt("ValuesInList", `
-- viewer-filter: user
SELECT l.id, l.name
FROM list l
WHERE l.user_id = @user
  AND (@q IS NULL OR instr(fold(l.name), @q) > 0)
  AND (@ids IS NULL OR l.id IN (SELECT unhex(value) FROM json_each(@ids)))
ORDER BY l.built_in DESC, l.name COLLATE NOCASE
LIMIT `+valuesLimit)
)

var (
	allKinds    = []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindSeason, catalog.KindEpisode, catalog.KindVideo, catalog.KindBook}
	videoKinds  = []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindSeason, catalog.KindEpisode, catalog.KindVideo}
	versioned   = []catalog.WorkKind{catalog.KindMovie, catalog.KindEpisode, catalog.KindVideo}
	booksOnly   = []catalog.WorkKind{catalog.KindBook}
	rangeValues = []FieldValue{{"hdr", "HDR"}, {"dolbyVision", "Dolby Vision"}}
	resolutions = []FieldValue{{"4k", "4K"}, {"1080p", "1080p"}, {"720p", "720p"}, {"sd", "SD"}}
)

// workFields is the registry: every field a rule or sort key can name.
var workFields = []*fieldDef{
	{WorkField: WorkField{Field: "library", Name: "Library", Type: FieldChoice, Kinds: allKinds, ValueSource: "inline"},
		from: "work_library x", on: "x.work_id = w.id", value: "x.library_id", ids: true, stmt: &libraryValues},
	{WorkField: WorkField{Field: "contentRating", Name: "Content rating", Type: FieldChoice, Kinds: allKinds, ValueSource: "inline"},
		expr: "CASE WHEN w.kind IN ('season', 'episode') THEN (SELECT r.content_rating FROM work r WHERE r.id = w.root_id) ELSE w.content_rating END",
		stmt: &contentRatingValues},
	{WorkField: WorkField{Field: "playState", Name: "Play state", Type: FieldChoice, Kinds: allKinds, ValueSource: "inline"},
		expr: playState, inline: []FieldValue{{"unplayed", "Unplayed"}, {"inProgress", "In progress"}, {"played", "Played"}}},

	{WorkField: WorkField{Field: "genre", Name: "Genre", Type: FieldSet, Kinds: allKinds, ValueSource: "inline"},
		from: "work_genre x JOIN genre n ON n.id = x.genre_id", on: "x.work_id = " + seriesOf, value: "n.name",
		stmt: nameValues("Genre", "genre", "work_genre", "genre_id")},
	{WorkField: WorkField{Field: "tag", Name: "Tag", Type: FieldSet, Kinds: allKinds, ValueSource: "search"},
		from: "work_tag x JOIN tag n ON n.id = x.tag_id", on: "x.work_id = " + seriesOf, value: "n.name",
		stmt: nameValues("Tag", "tag", "work_tag", "tag_id")},
	{WorkField: WorkField{Field: "company", Name: "Company", Type: FieldSet, Kinds: allKinds, ValueSource: "search"},
		from: "work_company x JOIN company n ON n.id = x.company_id", on: "x.work_id = " + seriesOf, value: "n.name",
		stmt: nameValues("Company", "company", "work_company", "company_id")},
	{WorkField: WorkField{Field: "resolution", Name: "Resolution", Type: FieldSet, Kinds: versioned, ValueSource: "inline"},
		bits: map[string]int64{"sd": 1, "720p": 2, "1080p": 4, "4k": 8}, column: "resolutions", inline: resolutions},
	{WorkField: WorkField{Field: "dynamicRange", Name: "Dynamic range", Type: FieldSet, Kinds: versioned, ValueSource: "inline"},
		bits: map[string]int64{"hdr": 1, "dolbyVision": 2}, column: "dynamic_ranges", inline: rangeValues},
	{WorkField: WorkField{Field: "collection", Name: "Collection", Type: FieldSet, Kinds: []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindVideo}, ValueSource: "search"},
		from: "collection_work x", on: "x.work_id = w.id", value: "x.collection_id", ids: true, stmt: &collectionValues},
	{WorkField: WorkField{Field: "bookSeries", Name: "Book series", Type: FieldSet, Kinds: booksOnly, ValueSource: "search"},
		from: "book_series_book x", on: "x.work_id = w.id", value: "x.series_id", ids: true, stmt: &bookSeriesValues},
	{WorkField: WorkField{Field: "inList", Name: "In list", Type: FieldSet, Kinds: allKinds, ValueSource: "inline"},
		from: "list_work x JOIN list xl ON xl.id = x.list_id", on: "x.work_id = w.id AND xl.user_id = @user", value: "x.list_id", ids: true, stmt: &listValues},
	{WorkField: WorkField{Field: "rootWork", Name: "Show or movie", Type: FieldSet, Kinds: allKinds, ValueSource: "search"},
		from: "work x", on: "x.id = w.root_id", value: "x.id", ids: true, stmt: &rootWorkValues},

	{WorkField: WorkField{Field: "actor", Name: "Actor", Type: FieldPerson, Kinds: videoKinds, ValueSource: "search"},
		from: "credit x", on: creditOf("actor"), value: "x.person_id", ids: true, stmt: personValues("Actor", "actor")},
	{WorkField: WorkField{Field: "director", Name: "Director", Type: FieldPerson, Kinds: videoKinds, ValueSource: "search"},
		from: "credit x", on: creditOf("director"), value: "x.person_id", ids: true, stmt: personValues("Director", "director")},
	{WorkField: WorkField{Field: "author", Name: "Author", Type: FieldPerson, Kinds: booksOnly, ValueSource: "search"},
		from: "credit x", on: "x.work_id = w.id AND x.role = 'author'", value: "x.person_id", ids: true, stmt: personValues("Author", "author")},
	{WorkField: WorkField{Field: "narrator", Name: "Narrator", Type: FieldPerson, Kinds: booksOnly, ValueSource: "search"},
		from: "credit x", on: "x.work_id = w.id AND x.role = 'narrator'", value: "x.person_id", ids: true, stmt: personValues("Narrator", "narrator")},

	{WorkField: WorkField{Field: "rating", Name: "Rating", Type: FieldNumber, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "w.community_rating", sort: "nullif(wl.rating_sort, -1)"},
	{WorkField: WorkField{Field: "myRating", Name: "My rating", Type: FieldNumber, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "us.rating", sort: "us.rating"},
	{WorkField: WorkField{Field: "runtimeMs", Name: "Runtime", Type: FieldNumber, Kinds: append(slices.Clone(versioned), catalog.KindBook), Sortable: true, DefaultDirection: "asc", Unit: "ms"},
		expr: "nullif(wl.runtime_sort, 9223372036854775807)", sort: "nullif(wl.runtime_sort, 9223372036854775807)"},
	{WorkField: WorkField{Field: "year", Name: "Year", Type: FieldNumber, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "nullif(wl.year, 0)", sort: "nullif(wl.year, 0)"},

	{WorkField: WorkField{Field: "addedAt", Name: "Date added", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: dayOf("wl.added_at"), sort: "wl.added_at"},
	{WorkField: WorkField{Field: "lastEpisodeAddedAt", Name: "Latest addition", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: dayOf("nullif(wl.last_added_at, 0)"), sort: "nullif(wl.last_added_at, 0)"},
	{WorkField: WorkField{Field: "releaseDate", Name: "Release date", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: "nullif(wl.release_key, '')", sort: "nullif(wl.release_key, '')"},
	{WorkField: WorkField{Field: "lastPlayedAt", Name: "Last played", Type: FieldDate, Kinds: allKinds, Sortable: true, DefaultDirection: "desc"},
		expr: dayOf("(" + lastPlayedMs + ")"), sort: "(" + lastPlayedMs + ")"},

	{WorkField: WorkField{Field: "title", Name: "Title", Type: FieldText, Kinds: allKinds, Sortable: true, DefaultDirection: "asc"},
		expr: "w.title", sort: "w.sort_title"},
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
// q, or with ids those values' names. A field with no values to choose
// returns none; an unknown field returns ErrNotFound.
func (s *Service) FieldValues(ctx context.Context, v users.Viewer, field, q string, ids []string) ([]FieldValue, error) {
	f, ok := fieldsByKey[field]
	if !ok {
		return nil, ErrNotFound
	}
	if f.stmt == nil {
		return f.inlineValues(q, ids), nil
	}
	vw := viewerOf(v)
	var search any
	if q != "" {
		search = store.Fold(q)
	}
	args := vw.args(sql.Named("q", search))
	if f.ids {
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
		args = append(args, sql.Named("ids", picked))
	} else {
		args = append(args, sql.Named("names", set(ids)))
	}
	var out []FieldValue
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var err error
		if f.ids {
			out, err = rowsOf(ctx, tx, *f.stmt, args, func(r scanner, fv *FieldValue) error {
				var u id.ID
				err := r.Scan(&u, &fv.Name)
				fv.Value = u.String()
				return err
			})
			return err
		}
		out, err = rowsOf(ctx, tx, *f.stmt, args, func(r scanner, fv *FieldValue) error { return r.Scan(&fv.Value, &fv.Name) })
		return err
	})
	return out, err
}

// inlineValues filters a field's fixed values by q or ids.
func (f *fieldDef) inlineValues(q string, ids []string) []FieldValue {
	if len(f.inline) == 0 {
		return nil
	}
	out := make([]FieldValue, 0, len(f.inline))
	key := store.Fold(q)
	for _, v := range f.inline {
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
