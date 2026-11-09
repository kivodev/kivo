package metadata

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

func TestSearch(t *testing.T) {
	e := newEnv(t)
	got, err := e.s.SearchMovies(ctx, Query{Title: "Dune", Year: 2021})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != (catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "438631"}) ||
		got[0].Year != 2021 || got[1].Year != 1984 || got[2].Year != 0 {
		t.Fatalf("movies: %+v", got)
	}
	series, err := e.s.SearchSeries(ctx, Query{Title: "The Expanse"})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].ID.Kind != string(catalog.KindSeries) || series[0].Title != "The Expanse" || series[0].Year != 2015 {
		t.Fatalf("series: %+v", series)
	}
}

func TestResolve(t *testing.T) {
	e := newEnv(t)
	for _, x := range []catalog.ExternalID{
		{Provider: "tmdb", Kind: "movie", ID: "438631"},
		{Provider: "imdb", Kind: "movie", ID: "tt1160419"},
	} {
		c, err := e.s.Resolve(ctx, x, "")
		if err != nil {
			t.Fatalf("%v: %v", x, err)
		}
		if c.ID.ID != "438631" || c.Title != "Dune" || c.Year != 2021 ||
			!slices.Equal(c.AltTitles, []string{"Dune: Part One", "Dune : Première partie"}) || !slices.Equal(c.OtherYears, []int{2020}) {
			t.Fatalf("%v: %+v", x, c)
		}
	}
	for _, x := range []catalog.ExternalID{
		{Provider: "imdb", Kind: "movie", ID: "tt0000000"}, // the find fixture has no such file, so TMDb answers 404
		{Provider: "tmdb", Kind: "movie", ID: "../../etc"},
		{Provider: "anidb", Kind: "series", ID: "69"},
	} {
		if _, err := e.s.Resolve(ctx, x, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("%v: got %v, want ErrNotFound", x, err)
		}
	}
}

func TestEpisodes(t *testing.T) {
	e := newEnv(t)
	series := catalog.ExternalID{Provider: "tmdb", Kind: string(catalog.KindSeries), ID: "63639"}
	type ep struct {
		id             string
		season, number int
	}
	list := func(o Ordering) []ep {
		t.Helper()
		eps, err := e.s.Episodes(ctx, series, o, "", 0)
		if err != nil {
			t.Fatalf("%s: %v", o, err)
		}
		var out []ep
		for _, x := range eps {
			out = append(out, ep{x.ID.ID, x.Season, x.Number})
		}
		return out
	}
	if got, want := list(Aired), []ep{{"9001", 0, 1}, {"1001", 1, 1}, {"1002", 1, 2}, {"2001", 2, 1}, {"2002", 2, 2}}; !slices.Equal(got, want) {
		t.Errorf("aired: %v, want %v", got, want)
	}
	// The only DVD group, with its discs and episodes in their own order.
	if got, want := list(DVD), []ep{{"1002", 1, 1}, {"1001", 1, 2}, {"2001", 2, 1}, {"2002", 2, 2}}; !slices.Equal(got, want) {
		t.Errorf("dvd: %v, want %v", got, want)
	}
	// There is no usable absolute group, because the only one is empty. So
	// the result is aired order without specials, numbered continuously.
	if got, want := list(Absolute), []ep{{"1001", 1, 1}, {"1002", 1, 2}, {"2001", 1, 3}, {"2002", 1, 4}}; !slices.Equal(got, want) {
		t.Errorf("absolute: %v, want %v", got, want)
	}
	if got, want := list("5e5b7b4f1b7c4c0013a1d3f2"), list(DVD); !slices.Equal(got, want) {
		t.Errorf("group by ID: %v, want %v", got, want)
	}
	// A group TMDb doesn't list isn't an ordering, so aired order stands in.
	if got, want := list("5e5b7b4f1b7c4c00deadbeef"), list(Aired); !slices.Equal(got, want) {
		t.Errorf("a missing group: %v, want aired %v", got, want)
	}
	if groups, err := e.s.EpisodeGroups(ctx, series, ""); err != nil || !slices.Contains(groups, "5e5b7b4f1b7c4c0013a1d3f2") {
		t.Errorf("episode groups %v, %v", groups, err)
	}

	// maxAge refetches anything older. Zero keeps the cache.
	before := e.fx.count("/3/tv/63639/season/1")
	list(Aired)
	if e.fx.count("/3/tv/63639/season/1") != before {
		t.Error("a cached season was fetched again")
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := e.s.Episodes(ctx, series, Aired, "", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if e.fx.count("/3/tv/63639/season/1") != before+1 {
		t.Error("maxAge didn't refetch")
	}
}

func TestTrailerOf(t *testing.T) {
	trailer := func(key, lang string, official bool, published string) tmdbVideo {
		return tmdbVideo{Key: key, Site: "YouTube", Type: "Trailer", Official: official, Lang: lang, Published: published}
	}
	const older, newer = "2020-01-01T00:00:00.000Z", "2021-01-01T00:00:00.000Z"
	for _, tc := range []struct {
		name, lang string
		videos     []tmdbVideo
		want       string
	}{
		{"none", "en-US", nil, ""},
		{"official over language", "de-DE", []tmdbVideo{trailer("de", "de", false, newer), trailer("en", "en", true, older)}, "en"},
		{"metadata language over English", "de-DE", []tmdbVideo{trailer("en", "en", true, newer), trailer("de", "de", true, older)}, "de"},
		{"English when the language has none", "fr", []tmdbVideo{trailer("es", "es", true, newer), trailer("en", "en", true, older)}, "en"},
		{"newest within a language", "en-US", []tmdbVideo{trailer("old", "en", true, older), trailer("new", "en", true, newer)}, "new"},
		{"another language when nothing else", "fr", []tmdbVideo{trailer("es", "es", true, older)}, "es"},
		{"no teasers or clips", "en", []tmdbVideo{
			{Key: "teaser", Site: "YouTube", Type: "Teaser", Official: true, Lang: "en", Published: newer},
			{Key: "clip", Site: "YouTube", Type: "Clip", Official: true, Lang: "en", Published: newer},
		}, ""},
		{"YouTube only", "en", []tmdbVideo{{Key: "vimeo", Site: "Vimeo", Type: "Trailer", Official: true, Lang: "en", Published: newer}}, ""},
	} {
		want := tc.want
		if want != "" {
			want = "https://www.youtube.com/watch?v=" + want
		}
		if got := trailerOf(tc.videos, tc.lang); got != want {
			t.Errorf("%s: %q, want %q", tc.name, got, want)
		}
	}
}

func TestContentRating(t *testing.T) {
	for _, tc := range []struct {
		country, cert, name string
		age                 int // -1 for none
	}{
		{"US", "PG-13", "PG-13", 13}, {"US", "R", "R", 17}, {"US", "TV-MA", "TV-MA", 17}, {"US", "G", "G", 0},
		{"GB", "12A", "12A", 12}, {"GB", "PG", "PG", 8}, {"DE", "FSK 16", "FSK 16", 16}, {"DE", "12", "12", 12},
		{"AU", "MA15+", "MA15+", 15}, {"FR", "", "", -1}, {"US", "NR", "", -1}, {"US", "Unrated", "", -1},
		{"US", "US:PG-13", "PG-13", 13}, {"GB", "gb:15", "15", 15}, {"GB", "US:PG-13", "", -1}, {"DE", "US:R", "", -1}, {"US", "Rated R", "R", 17}, {"US", "pg-13", "PG-13", 13},
		{"BR", "L", "L", 0}, {"SE", "Btl", "", -1},
	} {
		name, age := contentRating(tc.country, tc.cert)
		if name != tc.name || (age == nil) != (tc.age < 0) || age != nil && *age != tc.age {
			t.Errorf("contentRating(%s, %q) = %q, %v, want %q, %d", tc.country, tc.cert, name, age, tc.name, tc.age)
		}
	}
}

func TestClean(t *testing.T) {
	if got := clean("  Dune\x00\x1b[31m\nPart  ", 100); got != "Dune[31m\nPart" {
		t.Errorf("clean: %q", got)
	}
	if got := clean("Ünïcödé title", 5); got != "Ünïcö" {
		t.Errorf("clean caps runes: %q", got)
	}
	if got := clean("bad \xff utf8", 100); got != "bad  utf8" {
		t.Errorf("clean drops invalid UTF-8: %q", got)
	}
}
