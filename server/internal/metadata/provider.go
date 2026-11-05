package metadata

import (
	"context"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

// VideoProvider is a source of movie and series metadata. It searches, looks
// up a movie or series by ID, describes a work with its artwork, lists images
// to choose from, and lists a series' episodes in its orderings.
//
// Its answers use this package's types, so nothing outside its client reads
// the provider's own types. TMDb is the default. Without a key, it makes no
// requests and returns ErrUnavailable, as it does in an outage.
type VideoProvider interface {
	// Name is the provider's name in external IDs, the cache and the fetch
	// pipeline.
	Name() string
	Search(ctx context.Context, kind catalog.WorkKind, q Query, lang string) ([]Match, error)
	// Resolve looks a movie or series up by any ID the provider can find
	// it by. The match carries the provider's own ID.
	Resolve(ctx context.Context, x catalog.ExternalID, lang string) (Match, error)
	// Describe returns what the provider says about a movie, series, season
	// or episode, in lang. It returns ErrNotFound for one it doesn't know.
	Describe(ctx context.Context, r Ref, lang string) (*Description, error)
	// Images returns the artwork an admin chooses from. For a movie or
	// series, these are the images Describe gives. For a season or episode,
	// they are every image the provider has.
	Images(ctx context.Context, r Ref, lang string) (Images, error)
	// Episodes lists a series' episodes in an ordering. Cached data older
	// than maxAge is fetched again. A zero maxAge keeps the cache's usual
	// lifetime.
	Episodes(ctx context.Context, series catalog.ExternalID, o Ordering, lang string, maxAge time.Duration) ([]Episode, error)
	// EpisodeGroups lists the IDs of a series' episode groups. These are the
	// orderings an admin can choose besides the named ones.
	EpisodeGroups(ctx context.Context, series catalog.ExternalID, lang string) ([]string, error)
	// Owns reports whether an image URL is one of the provider's.
	Owns(url string) bool
	// Preview returns an image URL at the size a preview of an artwork kind
	// is made from. A URL that isn't the provider's is returned unchanged.
	Preview(url, kind string) string
}

// Ref names a work for a video provider by its external IDs. The provider
// uses the IDs it knows.
//
// A movie or series is named by its own IDs. A season is named by its
// series' IDs and its number. An episode is named by its series' IDs, its
// season and number in the library, and its own IDs, which are the same in
// every ordering.
type Ref struct {
	Kind       catalog.WorkKind
	IDs        []catalog.ExternalID // a movie's or series' own IDs; for a season or episode, its series' IDs
	Season     int                  // a season's number, or the season an episode is numbered in
	Episode    int                  // an episode's number in Season
	EpisodeIDs []catalog.ExternalID // an episode's own IDs
}

// Description is what a video provider says about a movie, series, season
// or episode, as the provider gives it. A season sets its title, overview,
// release date and poster. An episode sets those, plus its still, credits
// and community rating. A movie or series can set every field.
type Description struct {
	Title, OriginalTitle string
	OriginalLanguage     string // the provider's language code
	ReleaseDate          string // YYYY-MM-DD; a movie's release, a series' first air date, or a season's or episode's air date
	Overview             string
	AltTitles            []string
	Genres               []string
	Companies            []string // a movie's production companies, or a series' networks
	Tags                 []string
	ContentRatings       map[string]string // keyed by ISO 3166-1 country
	Cast                 []Person          // in billing order
	Crew                 []Person          // directors and writers
	Community            *catalog.CommunityRating
	Collection           *Collection // set only for a movie
	Images               Images
}

// Collection is the collection a movie is in, as a video provider gives it.
// Ref holds the movie's place in it, ordered by release. Images holds the
// collection's own posters and backdrops.
type Collection struct {
	Ref    catalog.CollectionRef
	Images Images
}

// Person is someone a work credits. Role is roleActor, roleDirector or
// roleWriter.
type Person struct {
	IDs       []catalog.ExternalID // the provider's IDs for them; none when it has no ID
	Name      string
	Role      string
	Character string // set only for an actor
	Photo     string // the photo's URL; empty for none
}

// Images holds a work's artwork by kind, each list in the provider's order.
// Original is the language the work was made in, which is used to rank
// them.
type Images struct {
	Original                          string
	Posters, Backdrops, Logos, Stills []Image
}

// Image is one image a provider offers.
type Image struct {
	URL           string
	Language      string  // ISO 639-1; empty, or "00", when textless
	Score         float64 // the provider's rating of the image; higher is better
	Width, Height int
}

// BookProvider is a source of audiobook metadata. It searches by title and
// author, and looks up a book or an author by ID. Each call reads the store
// of a content rating country. Audible, through Audnexus, is the default.
type BookProvider interface {
	// Name is the provider's name in external IDs and the fetch pipeline.
	Name() string
	Search(ctx context.Context, country string, q BookQuery) ([]Book, error)
	Book(ctx context.Context, country, id string) (*Book, error)
	Author(ctx context.Context, country, id string) (*Author, error)
}

// AnimeMapping maps an AniDB ID to the series a video provider knows. The
// Anime-Lists mapping file is the default.
type AnimeMapping interface {
	// AniDB returns what the mapping knows of an AniDB ID. It returns
	// ErrNotFound when the mapping has no usable entry for it.
	AniDB(ctx context.Context, anidb string) (AniDB, error)
}

// BookQuery is a book search by title and author. Either may be empty.
type BookQuery struct{ Title, Author string }

// Book is what a provider says about a book. An optional field is nil when
// the provider doesn't say, and empty when it says there is none. This tells
// missing data apart from empty data.
type Book struct {
	ID          string // the provider's ID, an ASIN
	Title       string
	Subtitle    *string
	Authors     []BookAuthor
	Narrators   []string
	Description *string // plain text
	Publisher   *string
	ReleaseDate *string // YYYY-MM-DD
	Genres      []string
	Series      []BookSeries // the primary series first
	Duration    *time.Duration
	Cover       *string // the cover's URL, which never reaches a client
}

// BookAuthor is a book's author as a provider names them, with the
// provider's ID.
type BookAuthor struct{ ID, Name string }

// BookSeries is a book's place in a series, as a provider gives it.
type BookSeries struct {
	ID, Name string
	Position string // as written, such as "1" or "1.5"; empty for none
}

// Author is an author as a provider describes them.
type Author struct {
	ID, Name string
	Bio      *string // plain text
	Image    *string // the photo's URL, which never reaches a client
}
