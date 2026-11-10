package query_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

var queryAllKinds = []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindSeason, catalog.KindEpisode, catalog.KindVideo, catalog.KindBook}

func (h *household) runQuery(v users.Viewer, q query.WorkQuery) query.GridPage[query.WorkSummary] {
	h.t.Helper()
	p, err := h.q.QueryWorks(ctx, v, q, query.GridOptions{Limit: 200})
	if err != nil {
		h.t.Fatalf("%+v: %v", q, err)
	}
	return p
}

func where(rules ...query.Rule) *query.RuleGroup { return &query.RuleGroup{Match: "all", Rules: rules} }

// fieldValues are values to feed each field's operators.
func (h *household) fieldValues(t *testing.T, f query.WorkField, mine id.ID) []string {
	t.Helper()
	switch f.Field {
	case "library":
		return []string{h.movies.String()}
	case "contentRating":
		return []string{"G"}
	case "playState":
		return []string{"played"}
	case "genre":
		return []string{"Adventure"}
	case "tag":
		return []string{"Space"}
	case "company":
		return []string{"Legendary"}
	case "resolution":
		return []string{"1080p"}
	case "dynamicRange":
		return []string{"hdr"}
	case "collection":
		return []string{h.toyStoryCollection.String()}
	case "bookSeries":
		return []string{h.expanseBooks.String()}
	case "inList":
		return []string{mine.String()}
	case "rootWork":
		return []string{h.dune.Work.String()}
	case "actor", "director":
		return []string{h.villeneuve.String()}
	case "author":
		return []string{h.weir.String()}
	case "narrator":
		return []string{h.porter.String()}
	case "rating", "myRating", "year", "runtimeMs":
		return []string{"1", "9999999"}
	case "title":
		return []string{"toy"}
	default: // dates
		return []string{"2000-01-01", "2030-01-01"}
	}
}

// ruleFor builds a rule for op with the field's values cut to the op's arity.
func ruleFor(f query.WorkField, op query.FieldOp, values []string) query.Rule {
	r := query.Rule{Field: f.Field, Op: op.Op}
	switch {
	case op.Op == "within" || op.Op == "notWithin":
		r.Values = []string{"P30Y"}
	case op.Arity == 0:
	case op.Arity < 0:
		r.Values = values[:1]
	case op.Arity == 1:
		r.Values = values[:1]
	case op.Arity == 2:
		r.Values = []string{values[0], values[len(values)-1]}
	}
	return r
}

// TestQueryEveryFieldAndOperator runs every field with every operator
// against the fixture, over every kind.
func TestQueryEveryFieldAndOperator(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	mine := h.lists(h.admin)[0].ID
	for _, f := range query.WorkFields() {
		values := h.fieldValues(t, f, mine)
		for _, op := range f.Ops {
			t.Run(f.Field+"/"+op.Op, func(t *testing.T) {
				q := query.WorkQuery{Kinds: queryAllKinds, Where: where(ruleFor(f, op, values))}
				if problems := q.Validate(); len(problems) != 0 {
					t.Fatalf("invalid: %v", problems)
				}
				h.runQuery(h.admin, q)
			})
		}
		if f.Sortable {
			h.runQuery(h.admin, query.WorkQuery{Kinds: queryAllKinds, Sort: []query.SortKey{{Field: f.Field}}})
		}
	}
}

// TestQueryHidesWhatTheViewerCantSee checks that no field and operator shows
// a restricted Viewer a work it can't see, and that every value a field
// offers one matches a work it can.
func TestQueryHidesWhatTheViewerCantSee(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	mine := h.lists(h.friend)[0].ID
	seen := func(v users.Viewer) map[id.ID]bool {
		out := map[id.ID]bool{}
		for _, c := range h.runQuery(v, query.WorkQuery{Kinds: queryAllKinds}).Items {
			out[c.ID] = true
		}
		return out
	}
	for _, viewer := range []struct {
		name string
		v    users.Viewer
		hide id.ID
	}{{"kid", h.kid, h.bladeRunner.Work}, {"friend", h.friend, h.hailMary}} {
		allowed := seen(viewer.v)
		if allowed[viewer.hide] {
			t.Fatalf("%s sees the work that should be hidden", viewer.name)
		}
		for _, f := range query.WorkFields() {
			values := h.fieldValues(t, f, mine)
			for _, op := range f.Ops {
				q := query.WorkQuery{Kinds: queryAllKinds, Where: where(ruleFor(f, op, values))}
				for _, c := range h.runQuery(viewer.v, q).Items {
					if !allowed[c.ID] {
						t.Errorf("%s/%s: %s saw %s", f.Field, op.Op, viewer.name, h.names[c.ID])
					}
				}
			}
			offered, err := h.q.FieldValues(ctx, viewer.v, f.Field, "", nil)
			if err != nil {
				t.Fatalf("%s values: %v", f.Field, err)
			}
			if f.Field == "library" || f.Field == "inList" || !slices.ContainsFunc(f.Ops, func(o query.FieldOp) bool { return o.Op == "anyOf" }) || f.Field == "resolution" || f.Field == "dynamicRange" || f.Field == "playState" {
				continue
			}
			for _, v := range offered {
				q := query.WorkQuery{Kinds: queryAllKinds, Where: where(query.Rule{Field: f.Field, Op: "anyOf", Values: []string{v.Value}})}
				if h.runQuery(viewer.v, q).Total == 0 {
					t.Errorf("%s offered %s %q, matching no work it can see", viewer.name, f.Field, v.Name)
				}
			}
		}
	}
	// A library the friend isn't granted isn't offered, even by its ID.
	got, err := h.q.FieldValues(ctx, h.friend, "library", "", []string{h.private.String()})
	if err != nil || len(got) != 0 {
		t.Errorf("a library the friend isn't granted: %v, %v", got, err)
	}
	// Another user's list isn't offered.
	others := h.lists(h.admin)[0].ID
	if got, _ := h.q.FieldValues(ctx, h.friend, "inList", "", []string{others.String()}); len(got) != 0 {
		t.Errorf("another user's list is offered: %v", got)
	}
	// A rule on a library the friend isn't granted matches nothing.
	r := query.Rule{Field: "library", Op: "anyOf", Values: []string{h.uhd.String()}}
	if p := h.runQuery(h.friend, query.WorkQuery{Kinds: queryAllKinds, Where: where(r)}); p.Total != 0 {
		t.Errorf("the friend reached a library it isn't granted: %v", ids(p.Items))
	}
	// A malformed ID is refused, naming the parameter.
	if _, err := h.q.FieldValues(ctx, h.friend, "rootWork", "", []string{"nope"}); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("a malformed id: %v", err)
	}
}

func TestQueryRandom(t *testing.T) {
	h := newHousehold(t)
	q := query.WorkQuery{Kinds: queryAllKinds, Sort: []query.SortKey{{Field: "random"}}}
	titled := ids(h.runQuery(h.admin, query.WorkQuery{Kinds: queryAllKinds}).Items)
	day := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	on := func(at time.Time) []id.ID {
		p, err := h.q.QueryWorksAt(ctx, h.admin, q, query.GridOptions{Limit: 200}, at)
		if err != nil {
			t.Fatal(err)
		}
		return ids(p.Items)
	}
	first := on(day)
	if slices.Equal(first, titled) {
		t.Error("random came back in title order")
	}
	if !slices.Equal(first, on(day.Add(6*time.Hour))) {
		t.Error("the order moved within a UTC day")
	}
	if slices.Equal(first, on(day.AddDate(0, 0, 1))) && slices.Equal(first, on(day.AddDate(0, 0, 2))) {
		t.Error("the order didn't change across days")
	}
}

// TestQueryFieldsByKind checks what a field means for each kind.
func TestQueryFieldsByKind(t *testing.T) {
	h := newHousehold(t)
	series := []catalog.WorkKind{catalog.KindSeries}
	seasons := []catalog.WorkKind{catalog.KindSeason}
	episodes := []catalog.WorkKind{catalog.KindEpisode}
	run := func(kinds []catalog.WorkKind, r query.Rule) []id.ID {
		t.Helper()
		return ids(h.runQuery(h.admin, query.WorkQuery{Kinds: kinds, Where: where(r)}).Items)
	}
	// A season and an episode take the content rating from their series.
	rating := query.Rule{Field: "contentRating", Op: "anyOf", Values: []string{certs[rankPG13]}}
	if got := run(series, rating); !slices.Contains(got, h.expanse) {
		t.Errorf("series rated PG-13: %v", got)
	}
	if got := run(seasons, rating); !slices.Contains(got, h.expanseS1) {
		t.Errorf("season inheriting PG-13: %v", got)
	}
	if got := run(episodes, rating); !slices.Contains(got, h.expanse1.Work) {
		t.Errorf("episode inheriting PG-13: %v", got)
	}
	// Resolution is unset on a series and set on an episode with a file.
	if got := run(series, query.Rule{Field: "resolution", Op: "isSet"}); len(got) != 0 {
		t.Errorf("a series with a resolution: %v", got)
	}
	if got := run(series, query.Rule{Field: "resolution", Op: "isNotSet"}); !slices.Contains(got, h.expanse) {
		t.Errorf("a series' resolution isn't unset: %v", got)
	}
	// rootWork is a series' own ID for its episodes.
	if got := run(episodes, query.Rule{Field: "rootWork", Op: "anyOf", Values: []string{h.expanse.String()}}); !slices.Contains(got, h.expanse1.Work) || slices.Contains(got, h.bluey3.Work) {
		t.Errorf("rootWork: %v", got)
	}
}

func TestQueryWithin(t *testing.T) {
	h := newHousehold(t)
	movies := []catalog.WorkKind{catalog.KindMovie}
	run := func(op, period string) []id.ID {
		t.Helper()
		return ids(h.runQuery(h.admin, query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "releaseDate", Op: op, Values: []string{period}})}).Items)
	}
	h.metadata(h.homeMovie.Work, catalog.WorkMetadata{Year: time.Now().Year(), ReleaseDate: time.Now().UTC().AddDate(0, 0, -10).Format(time.DateOnly)})
	h.metadata(h.toyStory2.Work, catalog.WorkMetadata{Year: 2099, ReleaseDate: "2099-06-01"})
	if got := run("within", "P30D"); !slices.Contains(got, h.homeMovie.Work) || slices.Contains(got, h.toyStory2.Work) {
		t.Errorf("within P30D: %v", got)
	}
	got := run("notWithin", "P30D")
	if slices.Contains(got, h.homeMovie.Work) || !slices.Contains(got, h.toyStory2.Work) || !slices.Contains(got, h.dune.Work) {
		t.Errorf("notWithin P30D: %v", got)
	}
}

func TestQuerySortOrder(t *testing.T) {
	h := newHousehold(t)
	h.metadata(h.homeMovie.Work, catalog.WorkMetadata{Community: &catalog.CommunityRating{Rating: 9.9, Votes: 1}})
	movies := []catalog.WorkKind{catalog.KindMovie}
	run := func(dir string) []query.WorkSummary {
		t.Helper()
		return h.runQuery(h.admin, query.WorkQuery{Kinds: movies, Sort: []query.SortKey{{Field: "rating", Direction: dir}}}).Items
	}
	for _, dir := range []string{"desc", "asc"} {
		items := run(dir)
		sawUnrated := false
		for _, c := range items {
			if c.Community == nil {
				sawUnrated = true
			} else if sawUnrated {
				t.Errorf("a rated film after an unrated one, %s: %v", dir, ids(items))
			}
		}
		if !sawUnrated {
			t.Fatalf("the fixture has no unrated film, %s", dir)
		}
	}
	// Rating sorts by the vote-weighted score, so one 9.9 vote ranks below
	// many 8s, though its raw rating is higher.
	desc := ids(run("desc"))
	if slices.Index(desc, h.homeMovie.Work) < slices.Index(desc, h.toyStory.Work) {
		t.Errorf("a one-vote 9.9 outranks a well-rated film: %v", desc)
	}
	// Ties: a show's episodes in season and episode order, under one key.
	eps := ids(h.runQuery(h.admin, query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindEpisode}, Where: where(query.Rule{Field: "rootWork", Op: "anyOf", Values: []string{h.expanse.String()}}), Sort: []query.SortKey{{Field: "rating"}}}).Items)
	if !slices.Equal(eps, []id.ID{h.expanse1.Work, h.expanse2.Work, h.expanse3.Work}) {
		t.Errorf("ties: %v", eps)
	}
}

func TestQueryExcludesProvisionalWorks(t *testing.T) {
	h := newHousehold(t)
	all := ids(h.runQuery(h.admin, query.WorkQuery{Kinds: queryAllKinds}).Items)
	if slices.Contains(all, h.unmatchedShow1.Work) {
		t.Errorf("a provisional work came back: %v", all)
	}
}

func TestQuerySemantics(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	movies := []catalog.WorkKind{catalog.KindMovie}
	run := func(q query.WorkQuery) []id.ID { return ids(h.runQuery(h.admin, q).Items) }

	// Mixed kinds, and a rule on a series-level field reaching an episode.
	mixed := query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries}, Where: where(query.Rule{Field: "contentRating", Op: "anyOf", Values: []string{"G"}})}
	got := run(mixed)
	if !slices.Contains(got, h.toyStory.Work) || !slices.Contains(got, h.bluey) {
		t.Errorf("mixed movies and shows: %v", got)
	}
	// A title search ignores case and diacritics.
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "title", Op: "contains", Values: []string{"TOY"}})}); !slices.Contains(got, h.toyStory.Work) {
		t.Errorf("title contains: %v", got)
	}
	// An unset field matches noneOf but not anyOf.
	noTags := query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "tag", Op: "noneOf", Values: []string{"Space"}})}
	if got := run(noTags); !slices.Contains(got, h.dune.Work) {
		t.Errorf("noneOf on an unset field: %v", got)
	}
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "tag", Op: "anyOf", Values: []string{"Space"}})}); len(got) != 0 {
		t.Errorf("anyOf with no such tag: %v", got)
	}
	// A value that no longer exists is one no work has.
	gone := id.New().String()
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "collection", Op: "anyOf", Values: []string{gone}})}); len(got) != 0 {
		t.Errorf("a collection that's gone: %v", got)
	}
	// noneOf on it keeps everything.
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "collection", Op: "noneOf", Values: []string{gone}})}); len(got) == 0 {
		t.Error("noneOf of a collection that's gone")
	}
	// isNotSet and isSet on a rating.
	unset := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "rating", Op: "isNotSet"})})
	set := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "rating", Op: "isSet"})})
	if len(unset)+len(set) != len(run(query.WorkQuery{Kinds: movies})) {
		t.Errorf("isSet and isNotSet: %v and %v", unset, set)
	}
	// A year between bounds is inclusive.
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "year", Op: "between", Values: []string{"1995", "1999"}})}); !slices.Equal(got, []id.ID{h.toyStory.Work, h.toyStory2.Work}) {
		t.Errorf("year between: %v", got)
	}
	// Sorting by year descending, with the unset sorting last.
	years := run(query.WorkQuery{Kinds: movies, Sort: []query.SortKey{{Field: "year", Direction: "desc"}}})
	if years[0] != h.dune.Work {
		t.Errorf("year desc: %v", years)
	}
	// An any group mixes.
	any := query.WorkQuery{Kinds: movies, Where: &query.RuleGroup{Match: "any", Rules: []query.Rule{
		{Field: "title", Op: "startsWith", Values: []string{"dune"}},
		{Field: "title", Op: "startsWith", Values: []string{"blade"}},
	}}}
	if got := run(any); !slices.Contains(got, h.dune.Work) || !slices.Contains(got, h.bladeRunner.Work) {
		t.Errorf("any: %v", got)
	}
	// Max works caps total.
	two := 2
	if p := h.runQuery(h.admin, query.WorkQuery{Kinds: movies, MaxWorks: &two}); p.Total != 2 || len(p.Items) != 2 {
		t.Errorf("maxWorks: %d, %d", p.Total, len(p.Items))
	}
	// In a list.
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "inList", Op: "anyOf", Values: []string{h.lists(h.admin)[0].ID.String()}})}); !slices.Equal(got, []id.ID{h.toyStory.Work}) {
		t.Errorf("inList: %v", got)
	}
	// rootWork of an episode is its series.
	if got := run(query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindEpisode}, Where: where(query.Rule{Field: "rootWork", Op: "anyOf", Values: []string{h.bluey.String()}})}); len(got) == 0 {
		t.Error("rootWork found no episode of the series")
	}
	// playState rolls a series up from its episodes.
	if got := run(query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindSeries}, Where: where(query.Rule{Field: "playState", Op: "anyOf", Values: []string{"inProgress"}})}); !slices.Contains(got, h.parent(h.expanseS1)) {
		t.Errorf("a series in progress: %v", got)
	}
	// A partial release date compares as its earliest day.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work SET release_date = '2021' WHERE id = ?`, h.dune.Work)
		return err
	})
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "releaseDate", Op: "lte", Values: []string{"2021-01-01"}})}); !slices.Contains(got, h.dune.Work) {
		t.Errorf("partial date: %v", got)
	}
	if got := run(query.WorkQuery{Kinds: movies, Where: where(query.Rule{Field: "releaseDate", Op: "gte", Values: []string{"2021-01-02"}})}); slices.Contains(got, h.dune.Work) {
		t.Errorf("partial date as its first day: %v", got)
	}
}

func TestQueryValidation(t *testing.T) {
	cases := []struct {
		name string
		q    query.WorkQuery
		at   string
	}{
		{"no kinds", query.WorkQuery{}, "/kinds"},
		{"extra refused", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindExtra}}, "/kinds/0"},
		{"newer version", query.WorkQuery{Version: 99, Kinds: []catalog.WorkKind{catalog.KindMovie}}, "/version"},
		{"empty group", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: &query.RuleGroup{Match: "all"}}, "/where"},
		{"bad match", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: &query.RuleGroup{Match: "x", Rules: []query.Rule{{Field: "year", Op: "isSet"}}}}, "/where/match"},
		{"unknown field", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: where(query.Rule{Field: "nope", Op: "isSet"})}, "/where/rules/0/field"},
		{"unknown op", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: where(query.Rule{Field: "year", Op: "contains", Values: []string{"1"}})}, "/where/rules/0/op"},
		{"wrong arity", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: where(query.Rule{Field: "year", Op: "between", Values: []string{"1"}})}, "/where/rules/0/values"},
		{"bad value", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: where(query.Rule{Field: "year", Op: "eq", Values: []string{"x"}})}, "/where/rules/0/values/0"},
		{"nested too deep", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: &query.RuleGroup{Match: "all", Groups: []query.RuleGroup{{Match: "all", Groups: []query.RuleGroup{{Match: "all", Rules: []query.Rule{{Field: "year", Op: "isSet"}}}}}}}}, "/where/groups/0/groups"},
		{"unsortable", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Sort: []query.SortKey{{Field: "genre"}}}, "/sort/0/field"},
		{"random with another", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Sort: []query.SortKey{{Field: "random"}, {Field: "year"}}}, "/sort/0/field"},
		{"bad within", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: where(query.Rule{Field: "addedAt", Op: "within", Values: []string{"30"}})}, "/where/rules/0/values/0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.q.Validate()
			if !slices.ContainsFunc(got, func(p query.QueryProblem) bool { return p.Pointer == c.at }) {
				t.Errorf("want a problem at %s; got %v", c.at, got)
			}
		})
	}
	zero, kinds := 0, []catalog.WorkKind{catalog.KindMovie}
	groups := make([]query.RuleGroup, 5)
	for i := range groups {
		groups[i] = query.RuleGroup{Match: "all", Rules: []query.Rule{{Field: "year", Op: "isSet"}}}
	}
	values := make([]string, 101)
	for i := range values {
		values[i] = "x"
	}
	for _, c := range []struct {
		name, at, code string
		q              query.WorkQuery
	}{
		{"maxWorks 0", "/maxWorks", "invalid", query.WorkQuery{Kinds: kinds, MaxWorks: &zero}},
		{"negative version", "/version", "invalid", query.WorkQuery{Version: -1, Kinds: kinds}},
		{"NaN", "/where/rules/0/values/0", "invalid", query.WorkQuery{Kinds: kinds, Where: where(query.Rule{Field: "year", Op: "eq", Values: []string{"NaN"}})}},
		{"Inf", "/where/rules/0/values/0", "invalid", query.WorkQuery{Kinds: kinds, Where: where(query.Rule{Field: "year", Op: "eq", Values: []string{"Inf"}})}},
		{"only punctuation", "/where/rules/0/values/0", "invalid", query.WorkQuery{Kinds: kinds, Where: where(query.Rule{Field: "title", Op: "contains", Values: []string{"!!!"}})}},
		{"101 values", "/where/rules/0/values", "tooLarge", query.WorkQuery{Kinds: kinds, Where: where(query.Rule{Field: "genre", Op: "anyOf", Values: values})}},
		{"six groups", "/where", "tooLarge", query.WorkQuery{Kinds: kinds, Where: &query.RuleGroup{Match: "all", Groups: groups}}},
		{"four sort keys", "/sort", "tooLarge", query.WorkQuery{Kinds: kinds, Sort: []query.SortKey{{Field: "year"}, {Field: "title"}, {Field: "rating"}, {Field: "addedAt"}}}},
	} {
		if got := c.q.Validate(); !slices.Contains(got, query.QueryProblem{Pointer: c.at, Code: c.code}) {
			t.Errorf("%s: want %s %s; got %v", c.name, c.at, c.code, got)
		}
	}

	// Past the rule limit.
	many := make([]query.Rule, 21)
	for i := range many {
		many[i] = query.Rule{Field: "year", Op: "isSet"}
	}
	if got := (query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: where(many...)}).Validate(); !slices.ContainsFunc(got, func(p query.QueryProblem) bool { return p.Code == "tooLarge" }) {
		t.Errorf("21 rules: %v", got)
	}
	var invalid *query.InvalidQueryError
	if _, err := query.New(nil).QueryWorks(context.Background(), users.Viewer{}, query.WorkQuery{}, query.GridOptions{}); !errors.As(err, &invalid) {
		t.Errorf("an invalid query: %v", err)
	}
}

func TestReleaseSortUsesTheFullDate(t *testing.T) {
	h := newHousehold(t)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		for _, u := range []struct {
			w    id.ID
			date string
		}{{h.toyStory.Work, "1995-11-22"}, {h.toyStory2.Work, "1995-02-01"}} {
			if _, err := tx.ExecContext(ctx, `UPDATE work SET release_date = ?, year = 1995 WHERE id = ?`, u.date, u.w); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE work_library SET release_key = ?, year = 1995 WHERE work_id = ?`, u.date, u.w); err != nil {
				return err
			}
		}
		return nil
	})
	p, err := h.q.Browse(ctx, h.admin, h.movies, query.BrowseOptions{Sort: query.ByRelease})
	if err != nil {
		t.Fatal(err)
	}
	a, b := slices.Index(ids(p.Items), h.toyStory.Work), slices.Index(ids(p.Items), h.toyStory2.Work)
	if a > b {
		t.Errorf("newest first within a year: Toy Story at %d, Toy Story 2 at %d", a, b)
	}
}
