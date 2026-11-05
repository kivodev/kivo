package identify

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/parse"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/users"
)

// MatchSearch is an admin's "Fix match" search. A nil field defaults to
// the work's own value: its provider, its title, its year, or a book's first
// author. For a season or episode, the title is its series' title.
type MatchSearch struct {
	Provider *string // tmdb for a video, audible for a book
	Query    *string
	Year     *int
	Author   *string
}

// ErrNotMatchable means a "Fix match" search was for a work no provider
// matches, such as an extra or another video.
var ErrNotMatchable = fault.Coded(fault.Conflict, "kindMismatch", "only movies and series match a provider")

// SearchMatches is the "Fix match" search for a work the Viewer can see.
//
// For a movie or series, it searches TMDb's movies or series by title and
// year. A season or episode searches for its series. A book searches
// Audible's books by title and author.
//
// A query that is a provider ID, such as "tmdb-438631" or an ASIN for a
// book, looks up that ID instead. It finds nothing when the provider doesn't
// know the ID.
func (i *Identifier) SearchMatches(ctx context.Context, v users.Viewer, work id.ID, s MatchSearch) ([]metadata.Match, error) {
	w, err := i.visibleWork(ctx, v, work)
	if err != nil {
		return nil, err
	}
	if w.Kind == catalog.KindBook {
		return i.bookMatches(ctx, w, s)
	}
	return i.videoMatches(ctx, w, s)
}

// lookupQuery matches a search that names a provider ID.
var lookupQuery = regexp.MustCompile(`^(tmdb|tvdb|imdb)-(\w+)$`)

// videoMatches searches TMDb by title and year for a movie or series, or
// for the series of a season or episode. A "tmdb-", "tvdb-" or "imdb-" ID
// is looked up instead. A year narrows the search to exactly that year. A
// wrong year finds nothing, so then the search is tried again without it,
// as identify's own search does.
func (i *Identifier) videoMatches(ctx context.Context, w catalog.Work, s MatchSearch) ([]metadata.Match, error) {
	if err := provider(s, "tmdb"); err != nil {
		return nil, err
	}
	var kind catalog.WorkKind
	var q metadata.Query
	switch w.Kind {
	case catalog.KindMovie, catalog.KindSeries:
		kind, q = w.Kind, metadata.Query{Title: w.Title, Year: w.Year}
	case catalog.KindSeason, catalog.KindEpisode:
		series, err := i.deps.Catalog.Work(ctx, w.Root)
		if err != nil {
			return nil, err
		}
		kind, q = catalog.KindSeries, metadata.Query{Title: series.Title}
	default:
		return nil, ErrNotMatchable
	}
	if s.Query != nil {
		q = metadata.Query{Title: *s.Query}
	}
	q, country := searchText(q)
	if s.Year != nil {
		q.Year = *s.Year
	}
	var err error
	if q.Language, _, err = i.deps.Metadata.Locale(ctx, w.ID); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestWait)
	defer cancel()
	if m := lookupQuery.FindStringSubmatch(q.Title); m != nil {
		return found(i.deps.Metadata.Resolve(ctx, catalog.ExternalID{Provider: m[1], Kind: string(kind), ID: m[2]}, q.Language))
	}
	search := i.deps.Metadata.SearchSeries
	if kind == catalog.KindMovie {
		search = i.deps.Metadata.SearchMovies
	}
	matches, err := search(ctx, q)
	if err == nil && len(matches) == 0 && q.Year != 0 {
		matches, err = search(ctx, metadata.Query{Title: q.Title, Language: q.Language})
	}
	if err != nil {
		return nil, err
	}
	madeInFirst(matches, country)
	return matches, nil
}

// bookMatches searches Audible's books by title and author, or looks up the
// book an ASIN names.
func (i *Identifier) bookMatches(ctx context.Context, w catalog.Work, s MatchSearch) ([]metadata.Match, error) {
	if err := provider(s, "audible"); err != nil {
		return nil, err
	}
	q := metadata.BookQuery{Title: w.Title}
	if s.Query != nil {
		q = metadata.BookQuery{Title: strings.TrimSpace(*s.Query)}
	} else {
		author, err := i.deps.Catalog.FirstAuthor(ctx, w.ID)
		if err != nil {
			return nil, err
		}
		q.Author = author
	}
	if s.Author != nil {
		q.Author = strings.TrimSpace(*s.Author)
	}
	ctx, cancel := context.WithTimeout(ctx, requestWait)
	defer cancel()
	if asin := strings.ToUpper(q.Title); metadata.IsASIN(asin) {
		return found(i.deps.Metadata.LookupBook(ctx, w.ID, asin))
	}
	matches, err := i.deps.Metadata.SearchBooks(ctx, w.ID, q)
	if err != nil && !errors.Is(err, metadata.ErrNotFound) {
		return nil, err
	}
	return matches, nil
}

// provider checks that a search's provider is the work's provider, want.
func provider(s MatchSearch, want string) error {
	if s.Provider == nil || *s.Provider == want {
		return nil
	}
	// The message names the provider, so it needs a fault of its own.
	wrong := fault.New(fault.Invalid, "the provider is "+want)
	return fault.About(wrong, fault.Field{Name: "provider", Code: "invalid"})
}

// found turns a lookup's match into search results. There are none when
// the provider doesn't know the ID.
func found(m metadata.Match, err error) ([]metadata.Match, error) {
	switch {
	case errors.Is(err, metadata.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	}
	return []metadata.Match{m}, nil
}

// searchText parses a "Fix match" search the way a folder name is parsed.
// "Heat (1995)" is Heat with its year, and "Cash Cab (US)" is Cash Cab, made
// in the US. A provider ID ("tmdb-438631") is left as it is.
func searchText(q metadata.Query) (metadata.Query, string) {
	if lookupQuery.MatchString(q.Title) {
		return q, ""
	}
	g := (parse.Parser{}).Dir(q.Title)
	if g.Title == "" {
		return q, ""
	}
	q.Title = g.Title
	if g.Year != 0 {
		q.Year = g.Year
	}
	return q, g.Country
}

// madeInFirst moves the matches made in country to the front, otherwise
// keeping the provider's order. Names say UK where providers use GB.
func madeInFirst(matches []metadata.Match, country string) {
	if country == "UK" {
		country = "GB"
	}
	if country == "" {
		return
	}
	slices.SortStableFunc(matches, func(a, b metadata.Match) int {
		ia, ib := slices.Contains(a.Countries, country), slices.Contains(b.Countries, country)
		switch {
		case ia == ib:
			return 0
		case ia:
			return -1
		}
		return 1
	})
}
