package metadata

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

var (
	// ErrNotFound means there is no such movie, series, episode, work or image.
	ErrNotFound = fault.New(fault.NotFound, "not found")
	// ErrUnavailable means a provider is down, is limiting our requests, or
	// rejected our key, and nothing cached can stand in. Try again later.
	ErrUnavailable = fault.New(fault.Unavailable, "provider unavailable")
	// ErrBadImage means an image can't be used. It may not be a raster type
	// we accept (SVG never is), or it may be too large, have too many
	// pixels, or fail to decode. A refresh tries the next image instead.
	ErrBadImage = fault.New(fault.Invalid, "unusable image")
	// ErrNoSuchArtwork means a work has no artwork of the kind asked for.
	ErrNoSuchArtwork = fault.New(fault.Invalid, "no artwork of that kind")
)

// Imager re-encodes artwork. media's ffmpeg runner implements it.
type Imager interface {
	Image(ctx context.Context, src string, outs []media.ImageOut) ([]byte, error)
}

// Tasks asks for items to be done now. tasks.Runner implements it.
type Tasks interface {
	Ask(task string, items ...id.ID)
}

// Deps are the service's collaborators and folders.
type Deps struct {
	Catalog  *catalog.Catalog
	Imager   Imager
	Settings *config.Store
	Events   *events.Bus
	Tasks    Tasks
	CacheDir string // the server's cache directory; artwork goes in its images folder
	ImageDir string // data/images, where uploaded artwork's originals are kept
}

// Options configures the providers.
type Options struct {
	Audible bool   // also describe books from Audible; otherwise only from their files
	TMDbKey string // replaces the built-in TMDb key when set

	// Video, Books and Anime replace TMDb, Audible and the Anime-Lists
	// mapping. A nil one keeps the default. Books is used whether or not
	// Audible is set. Their answers bypass the fetch pipeline.
	Video VideoProvider
	Books BookProvider
	Anime AnimeMapping
}

// Service fetches provider data, normalizes it into catalog, and keeps each
// work's artwork. Every fetch goes through one pipeline: cache, outage check,
// single-flight, rate limit, egress, then cache write. Service never decides
// which work a file belongs to. identify does that, using the read-only
// searches here.
type Service struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps

	imageCache string // cache/images

	fetcher *fetcher
	video   VideoProvider
	books   BookProvider // nil when there is no book provider
	anime   AnimeMapping
}

// New returns the service.
func New(db *store.DB, d Deps, opt Options) *Service {
	f := newFetcher(db, egress.New())
	s := &Service{
		db: db, q: dbq.New(db.Reader()), deps: d,
		imageCache: filepath.Join(d.CacheDir, "images"),
		fetcher:    f,
	}
	s.video, s.books, s.anime = opt.Video, opt.Books, opt.Anime
	if s.video == nil {
		s.video = newTMDb(f, opt.TMDbKey)
	}
	if s.books == nil && opt.Audible {
		s.books = newAudible(f)
	}
	if s.anime == nil {
		s.anime = newAnimeLists(f)
	}
	return s
}

// taskPruneProviderCache nightly removes long-expired provider responses and
// image files that nothing refers to.
const taskPruneProviderCache = "prune-provider-cache"

// Tasks returns metadata's tasks, for cmd/kivo to add.
func (s *Service) Tasks() []tasks.Task {
	return []tasks.Task{
		{Name: TaskRefreshMetadata, Nightly: true, Limit: func(config.Settings) int { return 4 }, Wanted: s.refreshWanted,
			Pending: s.pendingRefreshes, Do: s.refreshWork, Left: s.refreshesLeft, Failed: s.failedRefreshes, Retry: s.retryRefreshes},
		{Name: taskPruneProviderCache, Nightly: true, Maintenance: true, Run: s.prune},
	}
}

// Health implements health.Reporter. It reports providers that are down or
// have refused our key.
func (s *Service) Health(context.Context) []health.Problem {
	return s.fetcher.health()
}

// The provider names used in external IDs, the cache and health problems.
// An external ID's kind is the catalog work kind it names.
const (
	provTMDb       = "tmdb"
	provIMDb       = "imdb"
	provTVDB       = "tvdb"
	provAudible    = "audible"
	provAudnexus   = "audnexus"
	provAnimeLists = "animelists"
)

// Query is what identify searches for.
type Query struct {
	Title    string
	Year     int    // zero searches every year
	Language string // the library's BCP 47 language; empty means the server's metadata language
}

// Match is one movie or series a provider offers for a search or an ID.
type Match struct {
	ID            catalog.ExternalID
	Title         string
	OriginalTitle string
	Year          int
	Countries     []string // ISO codes ("US") of where a series was made; set by search
	// Resolve fills these; search results leave them empty.
	AltTitles  []string
	OtherYears []int // release years elsewhere, which count as the year too
	// The book provider fills these for a book. They are empty for a movie
	// or series.
	Subtitle, Overview *string
	Authors, Narrators []string
	Duration           time.Duration
}

// Ordering is how a series' episodes are numbered: Aired, DVD, Absolute,
// or any other value naming a provider episode group by its ID. catalog
// stores it per series.
type Ordering string

// Orderings with a name. In Absolute order every episode is in season 1,
// numbered from 1 across the series.
const (
	Aired    Ordering = "aired"
	DVD      Ordering = "dvd"
	Absolute Ordering = "absolute"
)

// Episode is one episode in an ordering. ID is the provider's episode ID,
// which stays the same in every ordering.
type Episode struct {
	ID             catalog.ExternalID
	Season, Number int
	Title, AirDate string // AirDate is YYYY-MM-DD
}

// SearchMovies returns the provider's matches for q, best first by the
// provider's own ranking.
func (s *Service) SearchMovies(ctx context.Context, q Query) ([]Match, error) {
	return s.search(ctx, catalog.KindMovie, q)
}

// SearchSeries returns the provider's matches for q, best first by the
// provider's own ranking.
func (s *Service) SearchSeries(ctx context.Context, q Query) ([]Match, error) {
	return s.search(ctx, catalog.KindSeries, q)
}

// search backs SearchMovies and SearchSeries.
func (s *Service) search(ctx context.Context, kind catalog.WorkKind, q Query) ([]Match, error) {
	return s.video.Search(ctx, kind, q, s.language(q.Language))
}

// Resolve looks up a movie or series by an ID from a path, a hint file or
// an NFO: the video provider's own, or another it can find the work by
// (TMDb finds IMDb and TVDB IDs). The match always carries the provider's
// own ID. lang is the library's metadata language; empty is the server's.
func (s *Service) Resolve(ctx context.Context, x catalog.ExternalID, lang string) (Match, error) {
	return s.video.Resolve(ctx, x, s.language(lang))
}

// Episodes lists a series' episodes in the given ordering. series must be
// the video provider's series ID; another provider's ID returns ErrNotFound.
// Cached data older than maxAge is fetched again, and a zero maxAge keeps the
// cache's usual lifetime. identify passes a small maxAge for its one refetch
// when an episode is missing. lang is the library's metadata language; empty
// means the server's.
func (s *Service) Episodes(ctx context.Context, series catalog.ExternalID, o Ordering, lang string, maxAge time.Duration) ([]Episode, error) {
	return s.video.Episodes(ctx, series, o, s.language(lang), maxAge)
}

// EpisodeGroups returns the IDs of a series' episode groups. These are the
// orderings an admin can choose besides the named ones. series is the video
// provider's series ID. The groups are fetched in lang, as a refresh fetches
// them; empty lang means the server's metadata language.
func (s *Service) EpisodeGroups(ctx context.Context, series catalog.ExternalID, lang string) ([]string, error) {
	return s.video.EpisodeGroups(ctx, series, s.language(lang))
}

// language returns lang, a library's metadata language, or the server's
// when lang is empty.
func (s *Service) language(lang string) string {
	return cmp.Or(lang, s.deps.Settings.Get().MetadataLanguage)
}

// Locale returns a work's metadata language and content rating country. Each
// comes from the work's library when set there, else from the server.
func (s *Service) Locale(ctx context.Context, work id.ID) (lang, country string, err error) {
	return s.locale(ctx, work)
}

func (s *Service) locale(ctx context.Context, work id.ID) (lang, country string, err error) {
	var l catalog.Library
	lib, err := s.q.WorkLibrary(ctx, work)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return "", "", err
	default:
		if l, err = s.deps.Catalog.Library(ctx, lib); err != nil && !errors.Is(err, catalog.ErrNotFound) {
			return "", "", err
		}
	}
	st := s.deps.Settings.Get()
	return cmp.Or(l.MetadataLanguage, st.MetadataLanguage), cmp.Or(l.ContentRatingCountry, st.ContentRatingCountry), nil
}

// notFound maps catalog.ErrNotFound to this package's ErrNotFound. It
// returns any other error unchanged.
func notFound(err error) error {
	if errors.Is(err, catalog.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
