package query_test

import (
	"strconv"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// grid is a library's grid as a client asks for it: a sort and filters,
// which it sends as a query read in the library.
type grid struct {
	Sort   string // a sortable field; the title when empty
	Filter filter
	query.GridOptions
}

// filter narrows a grid. Its fields combine with AND. A list matches any
// of its values, and an empty list matches everything.
type filter struct {
	Played         string // a play state; empty for any
	Genres         []string
	YearFrom       int // the first year, inclusive; zero for no bound
	YearTo         int // the last year, inclusive; zero for no bound
	ContentRatings []string
	Resolutions    []query.Resolution
	Ranges         []query.Range
	Unmatched      bool  // only provisional works
	Narrator       id.ID // only the books this person narrated; zero for any
}

// gridSorts are the sorts the clients' grids offer, each a field of a
// query in its default direction.
var gridSorts = []string{"title", "addedAt", "releaseDate", "rating", "myRating", "runtimeMs", "lastPlayedAt", "author", "lastEpisodeAddedAt"}

// gridKinds is the kind of a library's top-level works, which its grid
// holds.
var gridKinds = map[catalog.LibraryKind]catalog.WorkKind{
	catalog.Movies: catalog.KindMovie, catalog.Shows: catalog.KindSeries,
	catalog.Videos: catalog.KindVideo, catalog.Audiobooks: catalog.KindBook,
}

// query is the grid's sort and filters as a query of kind: the rules
// every work must pass, and the sort's field as the one sort key.
func (g grid) query(kind catalog.WorkKind) query.WorkQuery {
	f := g.Filter
	var rules []query.Rule
	rule := func(field, op string, values ...string) {
		if len(values) > 0 {
			rules = append(rules, query.Rule{Field: field, Op: op, Values: values})
		}
	}
	if f.Played != "" {
		rule("playState", "anyOf", f.Played)
	}
	rule("genre", "anyOf", f.Genres...)
	from, to := strconv.Itoa(f.YearFrom), strconv.Itoa(f.YearTo)
	switch {
	case f.YearFrom != 0 && f.YearTo != 0:
		rule("year", "between", from, to)
	case f.YearFrom != 0:
		rule("year", "gte", from)
	case f.YearTo != 0:
		rule("year", "lte", to)
	}
	rule("contentRating", "anyOf", f.ContentRatings...)
	rule("resolution", "anyOf", names(f.Resolutions)...)
	rule("dynamicRange", "anyOf", names(f.Ranges)...)
	if !f.Narrator.IsZero() {
		rule("narrator", "anyOf", f.Narrator.String())
	}
	q := query.WorkQuery{Kinds: []catalog.WorkKind{kind}}
	if g.Sort != "" {
		q.Sort = []query.SortKey{{Field: g.Sort}}
	}
	if len(rules) > 0 {
		q.Where = &query.RuleGroup{Match: "all", Rules: rules}
	}
	return q
}

// names is a filter's values as a rule's.
func names[V ~string](values []V) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// kindIn is the kind of lib's top-level works, or movies for a library
// that isn't there.
func kindIn(db *store.DB, lib id.ID) catalog.WorkKind {
	var kind catalog.LibraryKind
	if err := db.Reader().QueryRow(`SELECT kind FROM library WHERE id = ?`, lib).Scan(&kind); err != nil {
		return catalog.KindMovie
	}
	return gridKinds[kind]
}

// browse reads a page of lib's grid as v: the grid's query, read in the
// library.
func browse(db *store.DB, svc *query.Service, v users.Viewer, lib id.ID, g grid) (query.GridPage[query.WorkSummary], error) {
	return svc.QueryWorks(ctx, v, g.query(kindIn(db, lib)), query.Scope{Library: lib, Unmatched: g.Filter.Unmatched}, g.GridOptions)
}

// browse reads a page of lib's grid as v.
func (h *household) browse(v users.Viewer, lib id.ID, g grid) (query.GridPage[query.WorkSummary], error) {
	return browse(h.b.DB, h.q, v, lib, g)
}
