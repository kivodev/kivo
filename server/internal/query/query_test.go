package query_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

var update = flag.Bool("update", false, "rewrite golden files")

var ctx = context.Background()

// TestGolden renders what each user of the household sees, from Home to
// detail pages and search, and compares it with testdata/golden/<user>.json.
// IDs are replaced by the fixture's names and times by "<time>", so the
// output is stable.
func TestGolden(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	for _, tt := range []struct {
		name string
		v    users.Viewer
	}{{"admin", h.admin}, {"kid", h.kid}, {"friend", h.friend}, {"nobody", h.nobody}} {
		t.Run(tt.name, func(t *testing.T) {
			got := h.render(t, tt.v)
			path := filepath.Join("testdata", "golden", tt.name+".json")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s changed (go test -update to accept); got:\n%s", path, got)
			}
		})
	}
}

// render runs every method a client would for the fixture and returns the
// results as normalized JSON. A method that fails records its error, so the
// golden shows ErrNotFound where a user may not look.
func (h *household) render(t *testing.T, v users.Viewer) []byte {
	t.Helper()
	out := map[string]any{}
	record := func(name string, val any, err error) {
		if err != nil {
			out[name] = "error: " + err.Error()
			return
		}
		out[name] = val
	}
	libs, err := h.q.Libraries(ctx, v)
	record("libraries", libs, err)
	home, err := h.q.Home(ctx, v)
	record("home", home, err)
	for _, lib := range []id.ID{h.movies, h.shows, h.private, h.books} {
		p, err := h.q.Browse(ctx, v, lib, query.BrowseOptions{})
		record("browse "+h.names[lib], p, err)
	}
	byAuthor, err := h.q.Browse(ctx, v, h.books, query.BrowseOptions{Sort: query.ByAuthor})
	record("browse library:Audiobooks by author", byAuthor, err)
	narrated, err := h.q.Browse(ctx, v, h.books, query.BrowseOptions{Filter: query.Filter{Narrator: h.porter}})
	record("browse library:Audiobooks narrated by Ray Porter", narrated, err)
	authors, err := h.q.Authors(ctx, v, h.books, query.GridOptions{})
	record("authors library:Audiobooks", authors, err)
	series, err := h.q.SeriesList(ctx, v, h.books, query.GridOptions{})
	record("series library:Audiobooks", series, err)
	facets, err := h.q.Facets(ctx, v, h.books)
	record("facets library:Audiobooks", facets, err)
	for _, p := range []id.ID{h.corey, h.porter, h.villeneuve} {
		person, err := h.q.Person(ctx, v, p)
		record("person "+h.names[p], person, err)
	}
	expanse, err := h.q.Series(ctx, v, h.expanseBooks)
	record("series "+h.names[h.expanseBooks], expanse, err)
	for _, parent := range []id.ID{h.bluey, h.blueyS1, h.expanseS1} {
		p, err := h.q.Children(ctx, v, parent)
		record("children "+h.names[parent], p, err)
	}
	p, err := h.q.Collections(ctx, v, h.movies, query.GridOptions{})
	record("collections library:Movies", p, err)
	c, err := h.q.Collection(ctx, v, h.toyStoryCollection)
	record("collection "+h.names[h.toyStoryCollection], c, err)
	for _, work := range []id.ID{h.bladeRunner.Work, h.dune.Work, h.toyStory.Work, h.bluey, h.bluey1, h.expanse, h.secret.Work, h.leviathan} {
		d, err := h.q.Detail(ctx, v, work)
		record("detail "+h.names[work], d, err)
	}
	for _, q := range []string{"blade", "Toy", "zendaya", "Bl", "weir"} {
		r, err := h.q.Search(ctx, v, q, 0)
		record("search "+q, r, err)
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return h.normalize(b)
}

var (
	uuidPattern   = regexp.MustCompile(`"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"`)
	timePattern   = regexp.MustCompile(`"\d{4}-\d\d-\d\dT[^"]*"`)
	cursorPattern = regexp.MustCompile(`"Next": "[^"]+"`)
)

func (h *household) normalize(b []byte) []byte {
	for id, name := range h.names {
		b = bytes.ReplaceAll(b, []byte(`"`+id.String()+`"`), []byte(`"`+name+`"`))
	}
	b = uuidPattern.ReplaceAll(b, []byte(`"<id>"`))
	b = timePattern.ReplaceAllFunc(b, func(m []byte) []byte {
		if string(m) == `"0001-01-01T00:00:00Z"` {
			return m
		}
		return []byte(`"<time>"`)
	})
	return append(cursorPattern.ReplaceAll(b, []byte(`"Next": "<cursor>"`)), '\n')
}

// TestNoGrantsSeesNothing checks every method for a user with no grants,
// and for the zero Viewer: lists are empty and lookups are ErrNotFound.
func TestNoGrantsSeesNothing(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	for name, v := range map[string]users.Viewer{"no grants": h.nobody, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			libs, err := h.q.Libraries(ctx, v)
			if err != nil || len(libs) != 0 {
				t.Errorf("Libraries = %v, %v", libs, err)
			}
			home, err := h.q.Home(ctx, v)
			if err != nil || len(row(home, query.RowContinueWatching))+len(recentlyAdded(home))+len(row(home, query.RowFavorites)) != 0 {
				t.Errorf("Home = %+v, %v", home, err)
			}
			for _, q := range []string{"blade", "Bl"} {
				if r, err := h.q.Search(ctx, v, q, 0); err != nil || len(r.Works)+len(r.People) != 0 {
					t.Errorf("Search(%q) = %v, %v", q, r, err)
				}
			}
			notFound := map[string]error{}
			_, notFound["Browse"] = h.q.Browse(ctx, v, h.movies, query.BrowseOptions{})
			_, notFound["Children"] = h.q.Children(ctx, v, h.bluey)
			_, notFound["Detail"] = h.q.Detail(ctx, v, h.dune.Work)
			_, notFound["Collections"] = h.q.Collections(ctx, v, h.movies, query.GridOptions{})
			_, notFound["Collection"] = h.q.Collection(ctx, v, h.toyStoryCollection)
			for method, err := range notFound {
				if !errors.Is(err, query.ErrNotFound) {
					t.Errorf("%s: err = %v, want ErrNotFound", method, err)
				}
			}
		})
	}
}

// TestHiddenIsMissing checks that what a Viewer may not see fails exactly
// like what doesn't exist.
func TestHiddenIsMissing(t *testing.T) {
	h := newHousehold(t)
	_, hidden := h.q.Detail(ctx, h.friend, h.secret.Work)
	_, missing := h.q.Detail(ctx, h.friend, id.New())
	if !errors.Is(hidden, query.ErrNotFound) || hidden.Error() != missing.Error() {
		t.Errorf("hidden: %v, missing: %v", hidden, missing)
	}
	_, hidden = h.q.Browse(ctx, h.friend, h.private, query.BrowseOptions{})
	_, missing = h.q.Browse(ctx, h.friend, id.New(), query.BrowseOptions{})
	if !errors.Is(hidden, query.ErrNotFound) || hidden.Error() != missing.Error() {
		t.Errorf("hidden: %v, missing: %v", hidden, missing)
	}
}

// TestState checks that State matches the state on the work's summary, for
// a work the Viewer sees.
func TestState(t *testing.T) {
	h := newHousehold(t)
	if err := h.state.Update(ctx, h.friend, h.toyStory.Work, userstate.Change{Favorite: new(true), Played: new(true)}); err != nil {
		t.Fatal(err)
	}
	st, err := h.q.Userstate(ctx, h.friend, h.toyStory.Work)
	if err != nil {
		t.Fatal(err)
	}
	d, err := h.q.Detail(ctx, h.friend, h.toyStory.Work)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Favorite || !st.Played || st.PlayCount != 1 || st.Rating != nil || st.LastPlayed != d.Summary.Userstate.LastPlayed ||
		st.Position != d.Summary.Userstate.Position {
		t.Errorf("state %+v; detail's %+v", st, d.Summary.Userstate)
	}
	if _, err := h.q.Userstate(ctx, h.friend, h.secret.Work); !errors.Is(err, query.ErrNotFound) {
		t.Errorf("a work the Viewer can't see: %v", err)
	}
}

func TestRatingCeilingAndUnratedRule(t *testing.T) {
	h := newHousehold(t)
	titles := func(cs []query.WorkSummary) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.Title)
		}
		return out
	}
	p, err := h.q.Browse(ctx, h.kid, h.movies, query.BrowseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Dune is PG-13, Blade Runner is R, and Home Movie is unrated. The
	// provisional work has no rating either.
	if got, want := titles(p.Items), []string{"Toy Story", "Toy Story 2"}; !slices.Equal(got, want) {
		t.Errorf("kid's movies = %v, want %v", got, want)
	}
	// An episode with no rating of its own takes its series' rating. The
	// Expanse is PG-13, so its episodes are hidden too.
	if _, err := h.q.Detail(ctx, h.kid, h.expanse1.Work); !errors.Is(err, query.ErrNotFound) {
		t.Errorf("kid sees a PG-13 series' episode: %v", err)
	}
	if _, err := h.q.Detail(ctx, h.kid, h.bluey3.Work); err != nil {
		t.Errorf("kid can't see a G series' episode: %v", err)
	}
	// The admin shows unrated works.
	if _, err := h.q.Detail(ctx, h.admin, h.homeMovie.Work); err != nil {
		t.Errorf("admin can't see an unrated movie: %v", err)
	}
}
