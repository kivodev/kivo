package query_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

// TestLibraryValues checks the values a library's grid offers its filters:
// those its top-level works the Viewer sees have.
func TestLibraryValues(t *testing.T) {
	h := newHousehold(t)
	values := func(v users.Viewer, lib id.ID, field string) []string {
		t.Helper()
		got, err := h.q.FieldValues(ctx, v, field, lib, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(got))
		for i, fv := range got {
			out[i] = fv.Value
		}
		return out
	}
	if genres := values(h.admin, h.movies, "genre"); !slices.Contains(genres, "Science Fiction") || !slices.IsSorted(genres) {
		t.Errorf("admin genres %v", genres)
	}
	if got, want := values(h.admin, h.movies, "contentRating"), []string{"G", "PG-13", "R"}; !slices.Equal(got, want) { // youngest first
		t.Errorf("admin ratings %v, want %v", got, want)
	}
	if years := values(h.admin, h.movies, "year"); !slices.Contains(years, "2021") || !slices.IsSorted(years) {
		t.Errorf("admin years %v", years)
	}
	if got, ranges := values(h.admin, h.movies, "resolution"), values(h.admin, h.movies, "dynamicRange"); !slices.Equal(got, []string{"1080p"}) || len(ranges) != 0 {
		t.Errorf("admin resolutions %v, ranges %v; want 1080p, none", got, ranges)
	}

	if got, want := values(h.kid, h.movies, "contentRating"), []string{"G"}; !slices.Equal(got, want) {
		t.Errorf("kid ratings %v, want %v", got, want)
	}
	if years := values(h.kid, h.movies, "year"); slices.Contains(years, "2021") {
		t.Errorf("kid sees Dune's year: %v", years)
	}
	// Dune is the 1080p film, and the kid doesn't see it.
	if got := values(h.kid, h.movies, "resolution"); len(got) != 0 {
		t.Errorf("kid resolutions %v", got)
	}

	if got, ranges := values(h.admin, h.uhd, "resolution"), values(h.admin, h.uhd, "dynamicRange"); !slices.Equal(got, []string{"4k"}) || !slices.Equal(ranges, []string{"hdr", "dolbyVision"}) {
		t.Errorf("4K library resolutions %v, ranges %v", got, ranges)
	}
	// A library's values are its own: the movies library has no narrators,
	// and the books library has the books' alone.
	if got := values(h.admin, h.movies, "narrator"); len(got) != 0 {
		t.Errorf("the movies library's narrators %v", got)
	}
	if got := values(h.admin, h.books, "narrator"); !slices.Contains(got, h.porter.String()) {
		t.Errorf("the books library's narrators %v, want Porter among them", got)
	}
	if every, own := values(h.admin, id.ID{}, "genre"), values(h.admin, h.shows, "genre"); len(every) == 0 || len(own) != 0 {
		t.Errorf("the shows library's genres %v, of every library's %v", own, every)
	}
	// The values a rule can only name, not the works', are every library's.
	if got := values(h.admin, h.movies, "library"); len(got) < 2 {
		t.Errorf("the libraries, asked in one: %v", got)
	}
	if got := values(h.admin, h.movies, "playState"); len(got) != 3 {
		t.Errorf("the play states, asked in one library: %v", got)
	}

	for _, field := range []string{"genre", "resolution", "playState"} {
		if _, err := h.q.FieldValues(ctx, h.nobody, field, h.movies, "", nil); !errors.Is(err, query.ErrNotFound) {
			t.Errorf("no grants, %s: %v", field, err)
		}
	}
	if _, err := h.q.FieldValues(ctx, h.friend, "genre", h.private, "", nil); !errors.Is(err, query.ErrNotFound) {
		t.Errorf("a library not granted: %v", err)
	}
}

// TestBrowseLetters checks that the first page of a grid sorted by title
// says where each letter starts. Letters follow the sort title's rule, and
// each offset is the letter's index in the grid.
func TestBrowseLetters(t *testing.T) {
	h := newHousehold(t)
	h.movie(h.moviesFolder, "12 Monkeys", 1995, rankR)
	h.movie(h.moviesFolder, "The Abyss", 1989, rankPG13)
	h.movie(h.moviesFolder, "Zodiac", 2007, rankR)
	p, err := h.browse(h.admin, h.movies, grid{})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, c := range p.Items {
		titles = append(titles, c.Title)
	}
	// The unmatched some.film.2023 sorts after Z because its sort title is
	// lower case, so S starts there.
	wantTitles := []string{"12 Monkeys", "The Abyss", "Blade Runner", "Dune", "Home Movie", "Toy Story", "Toy Story 2", "Zodiac", "some.film.2023"}
	if !slices.Equal(titles, wantTitles) || p.Total != len(wantTitles) {
		t.Fatalf("grid %v, total %d", titles, p.Total)
	}
	if got, want := starts(p.Letters), "#0 A1 B2 D3 H4 T5 Z7 S8"; got != want {
		t.Errorf("letters %s, want %s", got, want)
	}
	// The page at each letter's offset starts with it.
	for _, l := range p.Letters {
		g, err := h.browse(h.admin, h.movies, grid{GridOptions: query.GridOptions{Offset: l.Offset, Limit: 1}})
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Items) != 1 || g.Items[0].ID != p.Items[l.Offset].ID {
			t.Errorf("%s's page starts at %v, want %s", l.Letter, ids(g.Items), titles[l.Offset])
		}
	}

	// The grid's filters and Viewer apply. Of 1995's films, the kid sees
	// only Toy Story.
	years := filter{YearFrom: 1995, YearTo: 1995}
	for _, tt := range []struct {
		v     users.Viewer
		want  string
		total int
	}{
		{h.admin, "#0 T1", 2},
		{h.kid, "T0", 1},
	} {
		g, err := h.browse(tt.v, h.movies, grid{Filter: years})
		if err != nil {
			t.Fatal(err)
		}
		if got := starts(g.Letters); got != tt.want || g.Total != tt.total {
			t.Errorf("letters %s of %d, want %s of %d", got, g.Total, tt.want, tt.total)
		}
	}

	// Only the first page of the title sort has them.
	two, err := h.browse(h.admin, h.movies, grid{GridOptions: query.GridOptions{Limit: 2}})
	if err != nil || two.Letters == nil {
		t.Fatalf("a short first page: letters %v, %v", two.Letters, err)
	}
	for _, o := range []grid{{Sort: "addedAt"}, {GridOptions: query.GridOptions{Offset: 3}}} {
		g, err := h.browse(h.admin, h.movies, o)
		if err != nil {
			t.Fatal(err)
		}
		if g.Letters != nil {
			t.Errorf("Browse(%+v) has letters %v", o, g.Letters)
		}
	}
}

// TestNamedListLetters checks that an audiobooks library's Authors and
// Series lists say where each letter starts on their first page, by the
// sort name's first character. It also checks that the page at each
// letter's offset starts with that letter.
func TestNamedListLetters(t *testing.T) {
	h := newHousehold(t)
	book := func(title, author, series string) {
		h.book([]string{author + "/" + title + "/" + title + ".m4b"}, catalog.WorkMetadata{
			Title: title,
			Book: &catalog.BookMetadata{Authors: []string{author}, Series: []catalog.SeriesRef{
				{Key: catalog.LocalSeriesKey(author, series), Name: series, Position: "1", Primary: true},
			}},
		})
	}
	book("Children of Time", "Adrian Tchaikovsky", "Children of Time")
	book("The Long Way to a Small, Angry Planet", "Becky Chambers", "Wayfarers")
	book("Into the Deep", "50 Cent", "1% Lifesteal")

	authors, err := h.q.Authors(ctx, h.admin, h.books, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range authors.Items {
		names = append(names, a.Name)
	}
	if want := []string{"50 Cent", "Adrian Tchaikovsky", "Andy Weir", "Becky Chambers", "James S. A. Corey"}; !slices.Equal(names, want) || authors.Total != len(want) {
		t.Fatalf("authors %v of %d", names, authors.Total)
	}
	if got, want := starts(authors.Letters), "#0 A1 B3 J4"; got != want {
		t.Errorf("author letters %s, want %s", got, want)
	}
	for _, l := range authors.Letters {
		p, err := h.q.Authors(ctx, h.admin, h.books, query.GridOptions{Offset: l.Offset, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Items) != 1 || p.Items[0].ID != authors.Items[l.Offset].ID || p.Total != authors.Total {
			t.Errorf("%s's page starts at %v of %d, want %s", l.Letter, p.Items, p.Total, names[l.Offset])
		}
		if l.Offset > 0 && p.Letters != nil {
			t.Errorf("a later page of authors has letters %v", p.Letters)
		}
	}

	series, err := h.q.SeriesList(ctx, h.admin, h.books, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	names = nil
	for _, s := range series.Items {
		names = append(names, s.Name)
	}
	if want := []string{"1% Lifesteal", "Children of Time", "The Expanse", "Wayfarers"}; !slices.Equal(names, want) || series.Total != len(want) {
		t.Fatalf("series %v of %d", names, series.Total)
	}
	if got, want := starts(series.Letters), "#0 C1 E2 W3"; got != want {
		t.Errorf("series letters %s, want %s", got, want)
	}
	for _, l := range series.Letters {
		p, err := h.q.SeriesList(ctx, h.admin, h.books, query.GridOptions{Offset: l.Offset, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Items) != 1 || p.Items[0].ID != series.Items[l.Offset].ID || p.Total != series.Total {
			t.Errorf("%s's page starts at %v of %d, want %s", l.Letter, p.Items, p.Total, names[l.Offset])
		}
	}
	if p, err := h.q.SeriesList(ctx, h.admin, h.books, query.GridOptions{Offset: 2}); err != nil || p.Letters != nil {
		t.Errorf("an offset page of series: letters %v, %v", p.Letters, err)
	}
}

// starts renders letters with their offsets, such as "A0 B3".
func starts(letters []query.Letter) string {
	var out []string
	for _, l := range letters {
		out = append(out, fmt.Sprintf("%s%d", l.Letter, l.Offset))
	}
	return strings.Join(out, " ")
}
