package query_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

func TestFacets(t *testing.T) {
	h := newHousehold(t)
	f, err := h.q.Facets(ctx, h.admin, h.movies)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.Genres, "Science Fiction") || !slices.IsSorted(f.Genres) {
		t.Errorf("admin genres %v", f.Genres)
	}
	if want := []string{"G", "PG-13", "R"}; !slices.Equal(f.ContentRatings, want) { // youngest first
		t.Errorf("admin ratings %v, want %v", f.ContentRatings, want)
	}
	if !slices.Contains(f.Years, 2021) || !slices.IsSorted(f.Years) {
		t.Errorf("admin years %v", f.Years)
	}
	if want := []query.Resolution{query.Resolution1080p}; !slices.Equal(f.Resolutions, want) || len(f.Ranges) != 0 || !f.Collections {
		t.Errorf("admin resolutions %v, ranges %v, collections %v; want %v, none, true", f.Resolutions, f.Ranges, f.Collections, want)
	}

	kid, err := h.q.Facets(ctx, h.kid, h.movies)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"G"}; !slices.Equal(kid.ContentRatings, want) {
		t.Errorf("kid ratings %v, want %v", kid.ContentRatings, want)
	}
	if slices.Contains(kid.Years, 2021) {
		t.Errorf("kid sees Dune's year: %v", kid.Years)
	}
	// Dune is the 1080p film, and the kid doesn't see it; Toy Story's collection stays.
	if len(kid.Resolutions) != 0 || !kid.Collections {
		t.Errorf("kid resolutions %v, collections %v", kid.Resolutions, kid.Collections)
	}

	uhd, err := h.q.Facets(ctx, h.admin, h.uhd)
	if err != nil {
		t.Fatal(err)
	}
	if want := []query.Range{query.HDR, query.DolbyVision}; !slices.Equal(uhd.Ranges, want) || !slices.Equal(uhd.Resolutions, []query.Resolution{query.Resolution4K}) {
		t.Errorf("4K library resolutions %v, ranges %v", uhd.Resolutions, uhd.Ranges)
	}
	shows, err := h.q.Facets(ctx, h.admin, h.shows)
	if err != nil {
		t.Fatal(err)
	}
	if shows.Collections {
		t.Error("the shows library lists collections")
	}
	if p, err := h.q.Collections(ctx, h.admin, h.shows, query.GridOptions{}); err != nil || p.Total != 0 {
		t.Errorf("the shows library's collections total %d, %v", p.Total, err)
	}

	if _, err := h.q.Facets(ctx, h.nobody, h.movies); !errors.Is(err, query.ErrNotFound) {
		t.Errorf("no grants: %v", err)
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
	p, err := h.q.Browse(ctx, h.admin, h.movies, query.BrowseOptions{})
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
		g, err := h.q.Browse(ctx, h.admin, h.movies, query.BrowseOptions{GridOptions: query.GridOptions{Offset: l.Offset, Limit: 1}})
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Items) != 1 || g.Items[0].ID != p.Items[l.Offset].ID {
			t.Errorf("%s's page starts at %v, want %s", l.Letter, ids(g.Items), titles[l.Offset])
		}
	}

	// The grid's filters and Viewer apply. Of 1995's films, the kid sees
	// only Toy Story.
	years := query.Filter{YearFrom: 1995, YearTo: 1995}
	for _, tt := range []struct {
		v     users.Viewer
		want  string
		total int
	}{
		{h.admin, "#0 T1", 2},
		{h.kid, "T0", 1},
	} {
		g, err := h.q.Browse(ctx, tt.v, h.movies, query.BrowseOptions{Filter: years})
		if err != nil {
			t.Fatal(err)
		}
		if got := starts(g.Letters); got != tt.want || g.Total != tt.total {
			t.Errorf("letters %s of %d, want %s of %d", got, g.Total, tt.want, tt.total)
		}
	}

	// Only the first page of the title sort has them.
	two, err := h.q.Browse(ctx, h.admin, h.movies, query.BrowseOptions{GridOptions: query.GridOptions{Limit: 2}})
	if err != nil || two.Letters == nil {
		t.Fatalf("a short first page: letters %v, %v", two.Letters, err)
	}
	for _, o := range []query.BrowseOptions{{Sort: query.ByAdded}, {GridOptions: query.GridOptions{Offset: 3}}} {
		g, err := h.q.Browse(ctx, h.admin, h.movies, o)
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
