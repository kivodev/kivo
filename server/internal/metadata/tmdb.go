package metadata

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

// tmdbKey is Kivo's own TMDb API key, registered with TMDb for the project. It
// is built in so every server can match media with no setup. A server admin can
// set their own key with tmdb_key in server.toml or KIVO_TMDB_KEY.
const tmdbKey = "5afcef223358863e223207a6f2ba14d6"

const (
	tmdbAPI    = "https://api.themoviedb.org/3"
	tmdbImages = "https://image.tmdb.org/t/p/original"
	// tmdbProfiles is the size people's photos are fetched at. It is TMDb's
	// smallest size that is at least as wide as a photo's widest encoding
	// (370).
	tmdbProfiles = "https://image.tmdb.org/t/p/h632"
)

// TMDb's episode group types that name an ordering.
const (
	groupAbsolute = 2
	groupDVD      = 3
)

// tmdb is the TMDb video provider. It reads movies, series, seasons, episode
// groups, collections, images and external IDs. Each is a single GET through
// the fetch pipeline, with the key as a bearer header, and the answer is
// mapped into this package's types.
type tmdb struct {
	f      *fetcher
	apiKey string
	fresh  Freshness // of each request; set per call by with
}

// with returns a copy of t whose requests use cached answers only as fr
// allows.
func (t *tmdb) with(fr Freshness) *tmdb {
	c := *t
	c.fresh = fr
	return &c
}

// newTMDb returns a provider that uses key, or the built-in key when key is
// empty. A test binary leaves the built-in key out, so tests never reach
// TMDb.
func newTMDb(f *fetcher, key string) *tmdb {
	builtin := tmdbKey
	if testing.Testing() {
		builtin = ""
	}
	return &tmdb{f: f, apiKey: cmp.Or(key, builtin)}
}

type tmdbName struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tmdbCast struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profile_path"`
}

type tmdbCrew struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Job         string `json:"job"`
	ProfilePath string `json:"profile_path"`
}

type tmdbCredits struct {
	Cast []tmdbCast `json:"cast"`
	Crew []tmdbCrew `json:"crew"`
}

type tmdbImage struct {
	FilePath string  `json:"file_path"`
	Lang     string  `json:"iso_639_1"`
	Vote     float64 `json:"vote_average"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
}

type tmdbImageSet struct {
	Posters   []tmdbImage `json:"posters"`
	Backdrops []tmdbImage `json:"backdrops"`
	Logos     []tmdbImage `json:"logos"`
	Stills    []tmdbImage `json:"stills"`
}

type tmdbTranslations struct {
	Translations []tmdbTranslation `json:"translations"`
}

type tmdbTranslation struct {
	Lang string `json:"iso_639_1"`
	Data struct {
		Title    string `json:"title"`
		Name     string `json:"name"`
		Overview string `json:"overview"`
	} `json:"data"`
}

type tmdbContentRating struct {
	Country string `json:"iso_3166_1"`
	Rating  string `json:"rating"`
}

type tmdbPart struct {
	ID          int    `json:"id"`
	ReleaseDate string `json:"release_date"`
}

// tmdbVideo is one of the videos TMDb lists for a movie.
type tmdbVideo struct {
	Key       string `json:"key"`
	Site      string `json:"site"`
	Type      string `json:"type"`
	Official  bool   `json:"official"`
	Lang      string `json:"iso_639_1"`
	Published string `json:"published_at"` // RFC 3339 in one format, so the text sorts by time
}

type tmdbExternalIDs struct {
	IMDb string `json:"imdb_id"`
	TVDB int    `json:"tvdb_id"`
}

type tmdbMovie struct {
	ID                  int        `json:"id"`
	Title               string     `json:"title"`
	OriginalTitle       string     `json:"original_title"`
	OriginalLanguage    string     `json:"original_language"`
	Overview            string     `json:"overview"`
	ReleaseDate         string     `json:"release_date"`
	Genres              []tmdbName `json:"genres"`
	ProductionCompanies []tmdbName `json:"production_companies"`
	BelongsToCollection *tmdbName  `json:"belongs_to_collection"`
	AlternativeTitles   struct {
		Titles []struct {
			Title string `json:"title"`
		} `json:"titles"`
	} `json:"alternative_titles"`
	ReleaseDates struct {
		Results []struct {
			Country      string `json:"iso_3166_1"`
			ReleaseDates []struct {
				ContentRating string `json:"certification"`
				ReleaseDate   string `json:"release_date"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	Credits  tmdbCredits `json:"credits"`
	Keywords struct {
		Keywords []tmdbName `json:"keywords"`
	} `json:"keywords"`
	ExternalIDs  tmdbExternalIDs  `json:"external_ids"`
	Images       tmdbImageSet     `json:"images"`
	Translations tmdbTranslations `json:"translations"`
	Videos       struct {
		Results []tmdbVideo `json:"results"`
	} `json:"videos"`
	tmdbVotes
}

// tmdbVotes is TMDb's community rating of a movie, series or episode.
type tmdbVotes struct {
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}

// community converts the votes to catalog's form. It returns nil when there
// are no votes.
func (v tmdbVotes) community() *catalog.CommunityRating {
	if v.VoteCount <= 0 {
		return nil
	}
	return &catalog.CommunityRating{Rating: min(max(v.VoteAverage, 0), 10), Votes: v.VoteCount}
}

type tmdbSeries struct {
	ID               int        `json:"id"`
	Name             string     `json:"name"`
	OriginalName     string     `json:"original_name"`
	OriginalLanguage string     `json:"original_language"`
	Overview         string     `json:"overview"`
	FirstAirDate     string     `json:"first_air_date"`
	Status           string     `json:"status"`
	Genres           []tmdbName `json:"genres"`
	Networks         []tmdbName `json:"networks"`
	Seasons          []struct {
		SeasonNumber int `json:"season_number"`
	} `json:"seasons"`
	AlternativeTitles struct {
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	} `json:"alternative_titles"`
	ContentRatings struct {
		Results []tmdbContentRating `json:"results"`
	} `json:"content_ratings"`
	Credits  tmdbCredits `json:"credits"`
	Keywords struct {
		Results []tmdbName `json:"results"`
	} `json:"keywords"`
	ExternalIDs  tmdbExternalIDs  `json:"external_ids"`
	Images       tmdbImageSet     `json:"images"`
	Translations tmdbTranslations `json:"translations"`
	tmdbVotes
}

// ended reports whether a series will get no new episodes, so its data can
// be cached for longer.
func (s *tmdbSeries) ended() bool { return s.Status == "Ended" || s.Status == "Canceled" }

type tmdbSeason struct {
	SeasonNumber int           `json:"season_number"`
	Name         string        `json:"name"`
	Overview     string        `json:"overview"`
	AirDate      string        `json:"air_date"`
	PosterPath   string        `json:"poster_path"`
	Episodes     []tmdbEpisode `json:"episodes"`
}

type tmdbEpisode struct {
	ID            int        `json:"id"`
	Name          string     `json:"name"`
	Overview      string     `json:"overview"`
	AirDate       string     `json:"air_date"`
	StillPath     string     `json:"still_path"`
	SeasonNumber  int        `json:"season_number"`
	EpisodeNumber int        `json:"episode_number"`
	Crew          []tmdbCrew `json:"crew"`
	GuestStars    []tmdbCast `json:"guest_stars"`
	tmdbVotes
}

type tmdbCollection struct {
	PosterPath   string     `json:"poster_path"`
	BackdropPath string     `json:"backdrop_path"`
	Parts        []tmdbPart `json:"parts"`
}

func (t *tmdb) Name() string { return provTMDb }

// Owns reports whether u is on TMDb's image server.
func (t *tmdb) Owns(u string) bool { return strings.HasPrefix(u, "https://image.tmdb.org/") }

// tmdbPreview maps an artwork kind to the TMDb size its preview is made
// from. Each is the smallest TMDb size that is about as wide as the kind's
// smallest width, or wider.
var tmdbPreview = map[string]string{artworkPoster: "w342", artworkBackdrop: "w780", artworkLogo: "w500", artworkStill: "w300"}

// lookupID picks, from a movie's or series' IDs, the one to look it up by.
// It prefers TMDb's own ID, then one TMDb can find it by.
func lookupID(kind catalog.WorkKind, ids []catalog.ExternalID) (catalog.ExternalID, bool) {
	var other *catalog.ExternalID
	for _, x := range ids {
		if x.Kind != string(kind) {
			continue
		}
		switch x.Provider {
		case provTMDb:
			return x, true
		case provIMDb, provTVDB:
			if other == nil {
				other = &x
			}
		}
	}
	if other == nil {
		return catalog.ExternalID{}, false
	}
	return *other, true
}

// lookup returns the kind and TMDb ID of the movie or series r names. For a
// season or episode, it returns those of the series.
func (t *tmdb) lookup(ctx context.Context, r Ref) (catalog.WorkKind, string, error) {
	kind := r.Kind
	if kind == catalog.KindSeason || kind == catalog.KindEpisode {
		kind = catalog.KindSeries
	}
	x, ok := lookupID(kind, r.IDs)
	if !ok {
		return "", "", fmt.Errorf("%w: no %s ID to look up", ErrNotFound, kind)
	}
	tmdbID, err := t.tmdbID(ctx, x)
	return kind, tmdbID, err
}

func (t *tmdb) Describe(ctx context.Context, r Ref, lang string, fr Freshness) (*Description, error) {
	t = t.with(fr)
	kind, tmdbID, err := t.lookup(ctx, r)
	if err != nil {
		return nil, err
	}
	if kind == catalog.KindMovie {
		return t.describeMovie(ctx, tmdbID, lang)
	}
	sr, err := t.series(ctx, tmdbID, lang)
	if err != nil {
		return nil, err
	}
	switch r.Kind {
	case catalog.KindSeries:
		return t.describeSeries(ctx, sr, lang)
	case catalog.KindSeason:
		sn, err := t.season(ctx, sr, r.Season, lang)
		if err != nil {
			return nil, err
		}
		d := &Description{Title: sn.Name, Overview: sn.Overview, ReleaseDate: sn.AirDate}
		if sn.PosterPath != "" {
			d.Images.Posters = []Image{{URL: tmdbImages + sn.PosterPath}}
		}
		return d, nil
	}
	_, e, err := t.episodeOf(ctx, sr, r, lang)
	if err != nil {
		return nil, err
	}
	d := &Description{
		Title: e.Name, Overview: e.Overview, ReleaseDate: e.AirDate,
		Cast: castOf(e.GuestStars), Crew: crewOf(e.Crew), Community: e.community(),
	}
	if e.StillPath != "" {
		d.Images.Stills = []Image{{URL: tmdbImages + e.StillPath}}
	}
	return d, nil
}

func (t *tmdb) describeMovie(ctx context.Context, tmdbID, lang string) (*Description, error) {
	m, err := t.movie(ctx, tmdbID, lang)
	if err != nil {
		return nil, err
	}
	imgs, err := t.movieImages(ctx, m, lang)
	if err != nil {
		return nil, err
	}
	tr := translated(m.Translations, m.OriginalLanguage)
	d := &Description{
		Title:         cmp.Or(m.Title, tr.Data.Title, m.OriginalTitle),
		OriginalTitle: m.OriginalTitle, OriginalLanguage: m.OriginalLanguage,
		ReleaseDate: m.ReleaseDate, Overview: cmp.Or(m.Overview, tr.Data.Overview),
		Genres: tmdbNames(m.Genres), Companies: tmdbNames(m.ProductionCompanies), Tags: tmdbNames(m.Keywords.Keywords),
		ContentRatings: map[string]string{},
		Cast:           castOf(m.Credits.Cast), Crew: crewOf(m.Credits.Crew),
		Community: m.community(), Images: imagesOf(imgs, m.OriginalLanguage),
		TrailerURL: trailerOf(m.Videos.Results, lang),
	}
	for _, a := range m.AlternativeTitles.Titles {
		d.AltTitles = append(d.AltTitles, a.Title)
	}
	for _, r := range m.ReleaseDates.Results {
		for _, rd := range r.ReleaseDates {
			if rd.ContentRating != "" && d.ContentRatings[r.Country] == "" {
				d.ContentRatings[r.Country] = rd.ContentRating // from the first release that has one
			}
		}
	}
	if c := m.BelongsToCollection; c != nil && c.Name != "" {
		d.Collection = &Collection{Ref: catalog.CollectionRef{Key: provTMDb + ":" + strconv.Itoa(c.ID), Name: c.Name}}
		coll, err := t.collection(ctx, c.ID, lang)
		switch {
		case err == nil:
			d.Collection.Ref.Order = collectionOrder(coll, m.ID)
			if coll.PosterPath != "" {
				d.Collection.Images.Posters = []Image{{URL: tmdbImages + coll.PosterPath}}
			}
			if coll.BackdropPath != "" {
				d.Collection.Images.Backdrops = []Image{{URL: tmdbImages + coll.BackdropPath}}
			}
		case !errors.Is(err, ErrNotFound):
			return nil, err
		}
	}
	return d, nil
}

func (t *tmdb) describeSeries(ctx context.Context, sr *tmdbSeries, lang string) (*Description, error) {
	imgs, err := t.seriesImages(ctx, sr, lang)
	if err != nil {
		return nil, err
	}
	tr := translated(sr.Translations, sr.OriginalLanguage)
	d := &Description{
		Title:         cmp.Or(sr.Name, tr.Data.Name, sr.OriginalName),
		OriginalTitle: sr.OriginalName, OriginalLanguage: sr.OriginalLanguage,
		ReleaseDate: sr.FirstAirDate, Overview: cmp.Or(sr.Overview, tr.Data.Overview),
		Genres: tmdbNames(sr.Genres), Companies: tmdbNames(sr.Networks), Tags: tmdbNames(sr.Keywords.Results),
		ContentRatings: map[string]string{},
		Cast:           castOf(sr.Credits.Cast), Crew: crewOf(sr.Credits.Crew),
		Community: sr.community(), Images: imagesOf(imgs, sr.OriginalLanguage),
	}
	for _, a := range sr.AlternativeTitles.Results {
		d.AltTitles = append(d.AltTitles, a.Title)
	}
	for _, r := range sr.ContentRatings.Results {
		if r.Rating != "" && d.ContentRatings[r.Country] == "" {
			d.ContentRatings[r.Country] = r.Rating
		}
	}
	return d, nil
}

func (t *tmdb) Images(ctx context.Context, r Ref, lang string) (Images, error) {
	kind, tmdbID, err := t.lookup(ctx, r)
	if err != nil {
		return Images{}, err
	}
	if kind == catalog.KindMovie {
		m, err := t.movie(ctx, tmdbID, lang)
		if err != nil {
			return Images{}, err
		}
		set, err := t.movieImages(ctx, m, lang)
		return imagesOf(set, m.OriginalLanguage), err
	}
	sr, err := t.series(ctx, tmdbID, lang)
	if err != nil {
		return Images{}, err
	}
	var set tmdbImageSet
	switch r.Kind {
	case catalog.KindSeries:
		set, err = t.seriesImages(ctx, sr, lang)
	case catalog.KindSeason:
		set, err = t.childImages(ctx, sr, fmt.Sprintf("/tv/%d/season/%d/images", sr.ID, r.Season), lang)
	default:
		sn, e, ferr := t.episodeOf(ctx, sr, r, lang)
		if ferr != nil {
			return Images{}, ferr
		}
		set, err = t.childImages(ctx, sr, fmt.Sprintf("/tv/%d/season/%d/episode/%d/images", sr.ID, sn.SeasonNumber, e.EpisodeNumber), lang)
	}
	return imagesOf(set, sr.OriginalLanguage), err
}

// episodeOf finds an episode in its series' aired seasons. It matches by the
// TMDb episode ID, which is the same in every ordering, and falls back to
// the episode number only when there is no ID.
//
// It looks first in the season the episode is numbered in, which is where
// the episode is in aired order. Only when the episode isn't there, because
// it is numbered in another ordering, does it read every season.
func (t *tmdb) episodeOf(ctx context.Context, sr *tmdbSeries, r Ref, lang string) (*tmdbSeason, *tmdbEpisode, error) {
	var epID int
	for _, x := range r.EpisodeIDs {
		if x.Provider == provTMDb && x.Kind == string(catalog.KindEpisode) {
			epID, _ = strconv.Atoi(x.ID)
		}
	}
	find := func(sn *tmdbSeason) *tmdbEpisode {
		for i, e := range sn.Episodes {
			byID := epID != 0 && e.ID == epID
			byNumber := epID == 0 && sn.SeasonNumber == r.Season && e.EpisodeNumber == r.Episode
			if byID || byNumber {
				return &sn.Episodes[i]
			}
		}
		return nil
	}
	for _, ref := range sr.Seasons {
		if ref.SeasonNumber != r.Season {
			continue
		}
		sn, err := t.season(ctx, sr, r.Season, lang)
		if err != nil {
			return nil, nil, err
		}
		if e := find(sn); e != nil {
			return sn, e, nil
		}
		break
	}
	if epID == 0 {
		return nil, nil, ErrNotFound // matching by number, only its own season can have it
	}
	seasons, err := t.aired(ctx, sr, lang)
	if err != nil {
		return nil, nil, err
	}
	for _, sn := range seasons {
		if e := find(sn); e != nil {
			return sn, e, nil
		}
	}
	return nil, nil, ErrNotFound
}

// imagesOf converts a TMDb image set to Images, keeping only images with a
// path. Each image's preview is the same image at its kind's preview size.
func imagesOf(set tmdbImageSet, original string) Images {
	list := func(imgs []tmdbImage, kind string) []Image {
		var out []Image
		for _, img := range imgs {
			if strings.HasPrefix(img.FilePath, "/") {
				out = append(out, Image{
					URL: tmdbImages + img.FilePath, Preview: strings.TrimSuffix(tmdbImages, "original") + tmdbPreview[kind] + img.FilePath,
					Language: img.Lang, Score: img.Vote, Width: img.Width, Height: img.Height,
				})
			}
		}
		return out
	}
	return Images{
		Original: original, Posters: list(set.Posters, artworkPoster), Backdrops: list(set.Backdrops, artworkBackdrop),
		Logos: list(set.Logos, artworkLogo), Stills: list(set.Stills, artworkStill),
	}
}

// translated returns the translation into the original language, or else
// into English. These are the fallbacks for text missing in the metadata
// language.
func translated(t tmdbTranslations, original string) tmdbTranslation {
	for _, want := range []string{original, "en"} {
		for _, tr := range t.Translations {
			if tr.Lang == want && (tr.Data.Overview != "" || tr.Data.Title != "" || tr.Data.Name != "") {
				return tr
			}
		}
	}
	return tmdbTranslation{}
}

// collectionOrder returns a movie's place in its collection by release
// date, counting from 1. It returns zero if the movie isn't listed.
func collectionOrder(c *tmdbCollection, movie int) int {
	parts := slices.Clone(c.Parts)
	slices.SortStableFunc(parts, func(a, b tmdbPart) int {
		switch {
		case a.ReleaseDate == b.ReleaseDate:
			return 0
		case a.ReleaseDate == "": // unreleased last
			return 1
		case b.ReleaseDate == "":
			return -1
		}
		return strings.Compare(a.ReleaseDate, b.ReleaseDate)
	})
	for i, p := range parts {
		if p.ID == movie {
			return i + 1
		}
	}
	return 0
}

// tmdbNames returns the names of TMDb genres, companies, networks or
// keywords.
func tmdbNames(ns []tmdbName) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Name
	}
	return out
}

// castOf converts TMDb's cast to people, in billing order.
func castOf(cast []tmdbCast) []Person {
	cast = slices.Clone(cast)
	slices.SortStableFunc(cast, func(a, b tmdbCast) int { return a.Order - b.Order })
	out := make([]Person, len(cast))
	for i, c := range cast {
		out[i] = Person{IDs: tmdbPerson(c.ID), Name: c.Name, Role: roleActor, Character: c.Character, Photo: profile(c.ProfilePath)}
	}
	return out
}

// crewOf returns the directors and writers in TMDb's crew.
func crewOf(crew []tmdbCrew) []Person {
	var out []Person
	for _, c := range crew {
		if role := map[string]string{"Director": roleDirector, "Writer": roleWriter, "Screenplay": roleWriter}[c.Job]; role != "" {
			out = append(out, Person{IDs: tmdbPerson(c.ID), Name: c.Name, Role: role, Photo: profile(c.ProfilePath)})
		}
	}
	return out
}

// tmdbPerson returns a person's TMDb external ID, or nil when id is zero.
func tmdbPerson(id int) []catalog.ExternalID {
	if id == 0 {
		return nil
	}
	return []catalog.ExternalID{{Provider: provTMDb, Kind: "person", ID: strconv.Itoa(id)}}
}

// profile returns the URL of a person's photo, or empty when there is none.
func profile(path string) string {
	if path == "" {
		return ""
	}
	return tmdbProfiles + path
}

// get fetches path through the pipeline and decodes it into v. Query
// parameters are encoded sorted, so the cache key is stable.
func (t *tmdb) get(ctx context.Context, path string, params url.Values, ttl func([]byte) time.Duration, v any) error {
	apiKey := t.apiKey
	if apiKey == "" {
		return fmt.Errorf("%w: no TMDb API key", ErrUnavailable)
	}
	key := path
	if len(params) > 0 {
		key += "?" + params.Encode()
	}
	// A v4 read access token goes in a header. An older v3 API key can only
	// go in the query string. So it is kept out of the cache key, and it is
	// scrubbed from any error, which would otherwise carry the URL into
	// logs.
	u, header := tmdbAPI+key, http.Header{"Accept": {"application/json"}}
	if v3Key(apiKey) {
		sep := "?"
		if len(params) > 0 {
			sep = "&"
		}
		u += sep + "api_key=" + url.QueryEscape(apiKey)
	} else {
		header.Set("Authorization", "Bearer "+apiKey)
	}
	body, err := t.f.fetch(ctx, request{provider: provTMDb, url: u, key: key, ttl: ttl, fresh: t.fresh, header: header})
	if err != nil {
		return scrub(err, apiKey)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode tmdb %s: %w", path, err)
	}
	return nil
}

func (t *tmdb) Search(ctx context.Context, kind catalog.WorkKind, q Query, lang string) ([]Match, error) {
	p := url.Values{"query": {q.Title}, "language": {lang}, "include_adult": {"false"}}
	path := "/search/movie"
	if kind == catalog.KindSeries {
		path = "/search/tv"
		if q.Year > 0 {
			p.Set("first_air_date_year", strconv.Itoa(q.Year))
		}
	} else if q.Year > 0 {
		p.Set("year", strconv.Itoa(q.Year))
	}
	var res struct {
		Results []struct {
			ID            int      `json:"id"`
			Title         string   `json:"title"`
			Name          string   `json:"name"`
			OriginalTitle string   `json:"original_title"`
			OriginalName  string   `json:"original_name"`
			ReleaseDate   string   `json:"release_date"`
			FirstAirDate  string   `json:"first_air_date"`
			OriginCountry []string `json:"origin_country"`
		} `json:"results"`
	}
	if err := t.get(ctx, path, p, fixed(ttlSearch), &res); err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(res.Results))
	for _, r := range res.Results {
		out = append(out, Match{
			ID:            catalog.ExternalID{Provider: provTMDb, Kind: string(kind), ID: strconv.Itoa(r.ID)},
			Title:         clean(cmp.Or(r.Title, r.Name), maxTitle),
			OriginalTitle: clean(cmp.Or(r.OriginalTitle, r.OriginalName), maxTitle),
			Year:          year(cmp.Or(r.ReleaseDate, r.FirstAirDate)),
			Countries:     r.OriginCountry,
		})
	}
	return out, nil
}

// Resolve turns any supported ID into a match that carries the TMDb ID.
func (t *tmdb) Resolve(ctx context.Context, x catalog.ExternalID, lang string) (Match, error) {
	tmdbID, err := t.tmdbID(ctx, x)
	if err != nil {
		return Match{}, err
	}
	switch catalog.WorkKind(x.Kind) {
	case catalog.KindMovie:
		m, err := t.movie(ctx, tmdbID, lang)
		if err != nil {
			return Match{}, err
		}
		c := Match{
			ID:    catalog.ExternalID{Provider: provTMDb, Kind: string(catalog.KindMovie), ID: tmdbID},
			Title: clean(m.Title, maxTitle), OriginalTitle: clean(m.OriginalTitle, maxTitle), Year: year(m.ReleaseDate),
		}
		for _, a := range m.AlternativeTitles.Titles {
			c.AltTitles = appendNew(c.AltTitles, clean(a.Title, maxTitle))
		}
		for _, r := range m.ReleaseDates.Results {
			for _, d := range r.ReleaseDates {
				if y := year(d.ReleaseDate); y != 0 && y != c.Year && !slices.Contains(c.OtherYears, y) {
					c.OtherYears = append(c.OtherYears, y)
				}
			}
		}
		slices.Sort(c.OtherYears)
		return c, nil
	case catalog.KindSeries:
		s, err := t.series(ctx, tmdbID, lang)
		if err != nil {
			return Match{}, err
		}
		c := Match{
			ID:    catalog.ExternalID{Provider: provTMDb, Kind: string(catalog.KindSeries), ID: tmdbID},
			Title: clean(s.Name, maxTitle), OriginalTitle: clean(s.OriginalName, maxTitle), Year: year(s.FirstAirDate),
		}
		for _, a := range s.AlternativeTitles.Results {
			c.AltTitles = appendNew(c.AltTitles, clean(a.Title, maxTitle))
		}
		return c, nil
	}
	return Match{}, fmt.Errorf("%w: %s %s %s", ErrNotFound, x.Provider, x.Kind, x.ID)
}

// tmdbID returns TMDb's ID for x, looking IMDb and TVDB IDs up through
// TMDb's find endpoint.
func (t *tmdb) tmdbID(ctx context.Context, x catalog.ExternalID) (string, error) {
	var source string
	switch x.Provider {
	case provTMDb:
		if _, err := strconv.Atoi(x.ID); err != nil {
			return "", fmt.Errorf("%w: TMDb ID %q", ErrNotFound, x.ID) // an ID from a filename must never shape the URL
		}
		return x.ID, nil
	case provIMDb:
		source = "imdb_id"
	case provTVDB:
		source = "tvdb_id"
	default:
		return "", fmt.Errorf("%w: %s ID", ErrNotFound, x.Provider)
	}
	var res struct {
		Movies []tmdbName `json:"movie_results"`
		Series []tmdbName `json:"tv_results"`
	}
	err := t.get(ctx, "/find/"+url.PathEscape(x.ID), url.Values{"external_source": {source}}, fixed(ttlSearch), &res)
	if err != nil {
		return "", err
	}
	found := res.Movies
	if x.Kind == string(catalog.KindSeries) {
		found = res.Series
	}
	if len(found) == 0 {
		return "", fmt.Errorf("%w: %s %s %s on TMDb", ErrNotFound, x.Provider, x.Kind, x.ID)
	}
	return strconv.Itoa(found[0].ID), nil
}

func detailParams(lang string, appends ...string) url.Values {
	return url.Values{
		"language":               {lang},
		"append_to_response":     {strings.Join(appends, ",")},
		"include_image_language": {imageLangs(lang)},
	}
}

// imageLangs returns the image languages a request includes: the metadata
// language, English, and textless.
func imageLangs(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return strings.Join(slices.Compact([]string{base, "en", "null"}), ",")
}

// videoLangs returns the video languages a movie's request includes: the
// metadata language and English.
func videoLangs(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return strings.Join(slices.Compact([]string{base, "en"}), ",")
}

// trailerOf returns the YouTube URL of a movie's best trailer, or "" when it
// has none. Official trailers come first, then the metadata language, then
// English, then the newest.
func trailerOf(videos []tmdbVideo, lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	rank := func(v tmdbVideo) int {
		switch v.Lang {
		case base:
			return 0
		case "en":
			return 1
		}
		return 2
	}
	videos = slices.DeleteFunc(slices.Clone(videos), func(v tmdbVideo) bool {
		return v.Site != "YouTube" || v.Type != "Trailer" || v.Key == ""
	})
	if len(videos) == 0 {
		return ""
	}
	best := slices.MinFunc(videos, func(a, b tmdbVideo) int {
		if a.Official != b.Official {
			if a.Official {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(b.Published, a.Published))
	})
	return "https://www.youtube.com/watch?v=" + url.QueryEscape(best.Key)
}

// allLanguages reports whether a work's images are fetched in every
// language. That is so for a work made in a language other than the
// metadata language and English, because images in its own language may be
// the only ones it has.
func allLanguages(lang, original string) bool {
	base, _, _ := strings.Cut(lang, "-")
	return original != "" && original != base && original != "en"
}

func (t *tmdb) movie(ctx context.Context, tmdbID, lang string) (*tmdbMovie, error) {
	ttl := func(b []byte) time.Duration {
		var m struct {
			ReleaseDate string `json:"release_date"`
		}
		_ = json.Unmarshal(b, &m)
		return movieTTL(m.ReleaseDate, time.Now())
	}
	var m tmdbMovie
	p := detailParams(lang, "alternative_titles", "credits", "external_ids", "images", "keywords", "release_dates", "translations", "videos")
	p.Set("include_video_language", videoLangs(lang)) // without it TMDb lists only videos in the request's language
	if err := t.get(ctx, "/movie/"+tmdbID, p, ttl, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// movieImages returns a movie's images. Usually these are the images in its
// details. For a movie made in another language (see allLanguages), it
// fetches every image TMDb has, cached as long as the details.
func (t *tmdb) movieImages(ctx context.Context, m *tmdbMovie, lang string) (tmdbImageSet, error) {
	if !allLanguages(lang, m.OriginalLanguage) {
		return m.Images, nil
	}
	var out tmdbImageSet
	ttl := fixed(movieTTL(m.ReleaseDate, time.Now()))
	err := t.get(ctx, "/movie/"+strconv.Itoa(m.ID)+"/images", nil, ttl, &out)
	return out, err
}

func (t *tmdb) series(ctx context.Context, tmdbID, lang string) (*tmdbSeries, error) {
	ttl := func(b []byte) time.Duration {
		var s tmdbSeries
		_ = json.Unmarshal(b, &s)
		return seriesTTL(s.ended())
	}
	var s tmdbSeries
	p := detailParams(lang, "alternative_titles", "content_ratings", "credits", "external_ids", "images", "keywords", "translations")
	if err := t.get(ctx, "/tv/"+tmdbID, p, ttl, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// seriesImages returns a series' images, as movieImages does for a movie.
func (t *tmdb) seriesImages(ctx context.Context, s *tmdbSeries, lang string) (tmdbImageSet, error) {
	if !allLanguages(lang, s.OriginalLanguage) {
		return s.Images, nil
	}
	var out tmdbImageSet
	err := t.get(ctx, fmt.Sprintf("/tv/%d/images", s.ID), nil, fixed(seriesTTL(s.ended())), &out)
	return out, err
}

func (t *tmdb) season(ctx context.Context, s *tmdbSeries, n int, lang string) (*tmdbSeason, error) {
	var out tmdbSeason
	path := fmt.Sprintf("/tv/%d/season/%d", s.ID, n)
	if err := t.get(ctx, path, url.Values{"language": {lang}}, fixed(seriesTTL(s.ended())), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// childImages lists a season's posters or an episode's stills from path. It
// asks for the same languages the series' own images are fetched in.
func (t *tmdb) childImages(ctx context.Context, s *tmdbSeries, path, lang string) (tmdbImageSet, error) {
	var out tmdbImageSet
	var p url.Values
	if !allLanguages(lang, s.OriginalLanguage) {
		p = url.Values{"include_image_language": {imageLangs(lang)}}
	}
	err := t.get(ctx, path, p, fixed(seriesTTL(s.ended())), &out)
	return out, err
}

func (t *tmdb) collection(ctx context.Context, collID int, lang string) (*tmdbCollection, error) {
	var out tmdbCollection
	if err := t.get(ctx, "/collection/"+strconv.Itoa(collID), url.Values{"language": {lang}}, fixed(ttlOlder), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// aired returns every season of a series, specials included, in order.
func (t *tmdb) aired(ctx context.Context, s *tmdbSeries, lang string) ([]*tmdbSeason, error) {
	var out []*tmdbSeason
	for _, sn := range s.Seasons {
		season, err := t.season(ctx, s, sn.SeasonNumber, lang)
		if err != nil {
			return nil, err
		}
		out = append(out, season)
	}
	slices.SortFunc(out, func(a, b *tmdbSeason) int { return a.SeasonNumber - b.SeasonNumber })
	return out, nil
}

// Episodes lists a TMDb series' episodes in an ordering.
//
// An ordering that can't be resolved falls back to aired order, so one bad
// setting doesn't stop the series' new episodes from being placed. Examples
// are a DVD order when the series has none or several, or an episode group
// TMDb no longer lists. Absolute order without an absolute group numbers the
// aired episodes instead.
func (t *tmdb) Episodes(ctx context.Context, series catalog.ExternalID, o Ordering, lang string, fr Freshness) ([]Episode, error) {
	if !tmdbSeriesID(series) {
		return nil, fmt.Errorf("%w: %s %s", ErrNotFound, series.Provider, series.Kind)
	}
	t = t.with(fr)
	s, err := t.series(ctx, series.ID, lang)
	if err != nil {
		return nil, err
	}
	var eps []Episode
	switch o {
	case Aired, "":
		return t.airedEpisodes(ctx, s, lang)
	case Absolute, DVD:
		want := map[Ordering]int{Absolute: groupAbsolute, DVD: groupDVD}[o]
		var gid string
		if gid, err = t.groupOfType(ctx, s, want); err == nil {
			eps, err = t.group(ctx, s, gid, o, lang)
		}
		// An empty group, such as Evangelion's absolute "Specials", is not
		// an ordering.
		if err == nil && len(eps) == 0 && o == Absolute {
			err = ErrNotFound
		}
	default:
		eps, err = t.group(ctx, s, string(o), o, lang)
	}
	switch {
	case !errors.Is(err, ErrNotFound):
		return eps, err
	case o == Absolute:
		return t.absoluteFromAired(ctx, s, lang)
	}
	slog.InfoContext(ctx, "metadata: episode ordering not found; using aired order", "series", s.ID, "ordering", o)
	return t.airedEpisodes(ctx, s, lang)
}

// airedEpisodes lists a series' episodes in aired order.
func (t *tmdb) airedEpisodes(ctx context.Context, s *tmdbSeries, lang string) ([]Episode, error) {
	seasons, err := t.aired(ctx, s, lang)
	if err != nil {
		return nil, err
	}
	var out []Episode
	for _, sn := range seasons {
		for _, e := range sn.Episodes {
			out = append(out, episode(e.ID, sn.SeasonNumber, e.EpisodeNumber, e.Name, e.AirDate))
		}
	}
	return out, nil
}

// EpisodeGroups returns the IDs of a TMDb series' episode groups.
func (t *tmdb) EpisodeGroups(ctx context.Context, series catalog.ExternalID, lang string) ([]string, error) {
	if !tmdbSeriesID(series) {
		return nil, fmt.Errorf("%w: %s %s", ErrNotFound, series.Provider, series.Kind)
	}
	s, err := t.series(ctx, series.ID, lang)
	if err != nil {
		return nil, err
	}
	groups, err := t.groups(ctx, s)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = g.ID
	}
	return out, nil
}

// tmdbSeriesID reports whether x is a TMDb series ID.
func tmdbSeriesID(x catalog.ExternalID) bool {
	return x.Provider == provTMDb && x.Kind == string(catalog.KindSeries)
}

type tmdbGroup struct {
	ID   string `json:"id"`
	Type int    `json:"type"`
}

// groups lists a series' episode groups.
func (t *tmdb) groups(ctx context.Context, s *tmdbSeries) ([]tmdbGroup, error) {
	var res struct {
		Results []tmdbGroup `json:"results"`
	}
	path := fmt.Sprintf("/tv/%d/episode_groups", s.ID)
	if err := t.get(ctx, path, nil, fixed(seriesTTL(s.ended())), &res); err != nil {
		return nil, err
	}
	return res.Results, nil
}

// groupOfType returns the series' only episode group of a type. It returns
// ErrNotFound when there are none or several. The first of several is often
// the wrong DVD order, so in that case the admin names one by ID instead.
func (t *tmdb) groupOfType(ctx context.Context, s *tmdbSeries, typ int) (string, error) {
	groups, err := t.groups(ctx, s)
	if err != nil {
		return "", err
	}
	var found []string
	for _, g := range groups {
		if g.Type == typ {
			found = append(found, g.ID)
		}
	}
	if len(found) != 1 {
		return "", ErrNotFound
	}
	return found[0], nil
}

// group lists an episode group's episodes. Each subgroup becomes a season,
// numbered by its order. In Absolute order, the subgroups run on as one
// season.
func (t *tmdb) group(ctx context.Context, s *tmdbSeries, gid string, o Ordering, lang string) ([]Episode, error) {
	type groupEpisode struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		AirDate string `json:"air_date"`
		Order   int    `json:"order"`
	}
	type subgroup struct {
		Order    int            `json:"order"`
		Episodes []groupEpisode `json:"episodes"`
	}
	var res struct {
		Groups []subgroup `json:"groups"`
	}
	path := "/tv/episode_group/" + url.PathEscape(gid)
	if err := t.get(ctx, path, url.Values{"language": {lang}}, fixed(seriesTTL(s.ended())), &res); err != nil {
		return nil, err
	}
	slices.SortFunc(res.Groups, func(a, b subgroup) int { return a.Order - b.Order })
	var out []Episode
	for _, g := range res.Groups {
		slices.SortFunc(g.Episodes, func(a, b groupEpisode) int { return a.Order - b.Order })
		for _, e := range g.Episodes {
			if o == Absolute {
				out = append(out, episode(e.ID, 1, len(out)+1, e.Name, e.AirDate))
			} else {
				out = append(out, episode(e.ID, g.Order, e.Order+1, e.Name, e.AirDate))
			}
		}
	}
	return out, nil
}

// absoluteFromAired numbers the aired episodes from 1 across the series,
// leaving out specials. It is used for a series with no absolute episode
// group.
func (t *tmdb) absoluteFromAired(ctx context.Context, s *tmdbSeries, lang string) ([]Episode, error) {
	seasons, err := t.aired(ctx, s, lang)
	if err != nil {
		return nil, err
	}
	var out []Episode
	for _, sn := range seasons {
		if sn.SeasonNumber == 0 {
			continue
		}
		for _, e := range sn.Episodes {
			out = append(out, episode(e.ID, 1, len(out)+1, e.Name, e.AirDate))
		}
	}
	return out, nil
}

func episode(tmdbID, season, number int, title, airDate string) Episode {
	return Episode{
		ID:     catalog.ExternalID{Provider: provTMDb, Kind: string(catalog.KindEpisode), ID: strconv.Itoa(tmdbID)},
		Season: season, Number: number, Title: clean(title, maxTitle), AirDate: airDate,
	}
}

// v3Key reports whether k is a TMDb v3 API key (32 hex digits) rather than
// a v4 read access token (a JWT).
func v3Key(k string) bool {
	if len(k) != 32 {
		return false
	}
	for _, c := range k {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// scrub removes a secret from an error's text.
func scrub(err error, secret string) error {
	if secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	return &scrubbed{err: err, msg: strings.ReplaceAll(err.Error(), secret, "REDACTED")}
}

type scrubbed struct {
	err error
	msg string
}

func (s *scrubbed) Error() string { return s.msg }
func (s *scrubbed) Unwrap() error { return s.err }
