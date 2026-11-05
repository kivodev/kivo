package metadata

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const audnexusAPI = "https://api.audnex.us"

// audibleHosts maps each region Audnexus serves to Audible's store there.
var audibleHosts = map[string]string{
	"us": "api.audible.com", "uk": "api.audible.co.uk", "ca": "api.audible.ca", "au": "api.audible.com.au",
	"de": "api.audible.de", "fr": "api.audible.fr", "es": "api.audible.es", "it": "api.audible.it",
	"jp": "api.audible.co.jp", "in": "api.audible.in",
}

// audibleRegions maps a content rating country to its Audible region. A
// country without a store of its own uses the US one.
var audibleRegions = map[string]string{
	"US": "us", "GB": "uk", "CA": "ca", "AU": "au", "DE": "de", "FR": "fr", "ES": "es", "IT": "it", "JP": "jp", "IN": "in",
}

func audibleRegion(country string) string { return cmp.Or(audibleRegions[country], "us") }

// audible is the Audible book provider. It searches Audible's catalog in
// the country's store, and looks up books and authors by ASIN from
// Audnexus, which normalizes Audible's records. Each request is a single GET
// through the fetch pipeline. Neither service needs a key.
type audible struct{ f *fetcher }

func newAudible(f *fetcher) *audible { return &audible{f: f} }

func (a *audible) Name() string { return provAudible }

// audibleSearchGroups lists the parts of a product a search returns. That is
// enough to match on and to fill a Match.
const audibleSearchGroups = "contributors,product_attrs,product_desc,series"

type audiblePerson struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
}

type audibleProduct struct {
	ASIN        string          `json:"asin"`
	Title       string          `json:"title"`
	Subtitle    string          `json:"subtitle"`
	Authors     []audiblePerson `json:"authors"`
	Narrators   []audiblePerson `json:"narrators"`
	Publisher   string          `json:"publisher_name"`
	ReleaseDate string          `json:"release_date"`
	Summary     string          `json:"publisher_summary"`
	Runtime     scalar          `json:"runtime_length_min"`
	ContentType string          `json:"content_type"`
	Series      []struct {
		ASIN     string `json:"asin"`
		Title    string `json:"title"`
		Sequence scalar `json:"sequence"`
	} `json:"series"`
}

// Search searches Audible's catalog by title and author, in Audible's order
// of relevance. Podcasts are left out.
func (a *audible) Search(ctx context.Context, country string, q BookQuery) ([]Book, error) {
	region := audibleRegion(country)
	host, ok := audibleHosts[region]
	if !ok {
		return nil, fmt.Errorf("%w: audible region %q", ErrNotFound, region)
	}
	p := url.Values{"num_results": {"10"}, "products_sort_by": {"Relevance"}, "response_groups": {audibleSearchGroups}}
	if q.Title != "" {
		p.Set("title", q.Title)
	}
	if q.Author != "" {
		p.Set("author", q.Author)
	}
	path := "/1.0/catalog/products?" + p.Encode()
	body, err := a.f.fetch(ctx, request{provider: provAudible, url: "https://" + host + path, key: region + path, ttl: fixed(ttlSearch)})
	if err != nil {
		return nil, err
	}
	var res struct {
		Products []audibleProduct `json:"products"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("decode audible search: %w", err)
	}
	out := []Book{}
	for _, r := range res.Products {
		if r.ASIN == "" || r.Title == "" || strings.EqualFold(r.ContentType, "Podcast") {
			continue
		}
		b := Book{
			ID: r.ASIN, Title: clean(r.Title, maxTitle), Subtitle: optText(r.Subtitle, maxTitle),
			Authors: bookAuthors(r.Authors), Narrators: []string{},
			Description: optText(plainText(r.Summary), maxOverview), Publisher: optText(r.Publisher, maxName),
			ReleaseDate: optDate(r.ReleaseDate), Duration: r.Runtime.minutes(), Series: []BookSeries{},
		}
		for _, n := range r.Narrators {
			b.Narrators = appendNew(b.Narrators, clean(n.Name, maxName))
		}
		for _, s := range r.Series {
			if s.Title != "" {
				b.Series = append(b.Series, BookSeries{ID: s.ASIN, Name: clean(s.Title, maxTitle), Position: positionLabel(string(s.Sequence))})
			}
		}
		out = append(out, b)
	}
	return out, nil
}

type audnexusSeries struct {
	ASIN     string `json:"asin"`
	Name     string `json:"name"`
	Position scalar `json:"position"`
}

type audnexusBook struct {
	ASIN      string          `json:"asin"`
	Title     string          `json:"title"`
	Subtitle  string          `json:"subtitle"`
	Authors   []audiblePerson `json:"authors"`
	Narrators []audiblePerson `json:"narrators"`
	Publisher string          `json:"publisherName"`
	Released  string          `json:"releaseDate"`
	Summary   string          `json:"summary"`
	Image     string          `json:"image"`
	Runtime   scalar          `json:"runtimeLengthMin"`
	Genres    []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"genres"`
	SeriesPrimary   *audnexusSeries `json:"seriesPrimary"`
	SeriesSecondary *audnexusSeries `json:"seriesSecondary"`
}

// Book returns a book by its ASIN from Audnexus, in the country's store.
// Only Audnexus genres of type "genre" become the book's genres. Its tags
// aren't used.
func (a *audible) Book(ctx context.Context, country, asin string) (*Book, error) {
	var r audnexusBook
	if err := a.audnexus(ctx, "books", audibleRegion(country), asin, func(body []byte) time.Duration {
		var d struct {
			Released string `json:"releaseDate"`
		}
		_ = json.Unmarshal(body, &d)
		return movieTTL(optDateString(d.Released), time.Now()) // a book is refetched on a movie's schedule
	}, &r); err != nil {
		return nil, err
	}
	b := &Book{
		ID: r.ASIN, Title: clean(r.Title, maxTitle), Subtitle: optText(r.Subtitle, maxTitle),
		Authors: bookAuthors(r.Authors), Narrators: []string{}, Genres: []string{}, Series: []BookSeries{},
		Description: optText(plainText(r.Summary), maxOverview), Publisher: optText(r.Publisher, maxName),
		ReleaseDate: optDate(r.Released), Duration: r.Runtime.minutes(),
	}
	if b.ID == "" {
		b.ID = asin
	}
	for _, n := range r.Narrators {
		b.Narrators = appendNew(b.Narrators, clean(n.Name, maxName))
	}
	for _, g := range r.Genres {
		if g.Type == "genre" {
			b.Genres = appendNew(b.Genres, clean(g.Name, maxName))
		}
	}
	for _, s := range []*audnexusSeries{r.SeriesPrimary, r.SeriesSecondary} {
		if s != nil && s.Name != "" {
			b.Series = append(b.Series, BookSeries{ID: s.ASIN, Name: clean(s.Name, maxTitle), Position: positionLabel(string(s.Position))})
		}
	}
	if r.Image != "" {
		b.Cover = &r.Image
	}
	return b, nil
}

// Author returns an author's biography and photo by their ASIN from
// Audnexus, in the country's store.
func (a *audible) Author(ctx context.Context, country, asin string) (*Author, error) {
	var r struct {
		ASIN        string `json:"asin"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Image       string `json:"image"`
	}
	if err := a.audnexus(ctx, "authors", audibleRegion(country), asin, fixed(ttlAuthor), &r); err != nil {
		return nil, err
	}
	out := &Author{ID: asin, Name: clean(r.Name, maxName), Bio: optText(plainText(r.Description), maxOverview)}
	if r.Image != "" {
		out.Image = &r.Image
	}
	return out, nil
}

// audnexus GETs a book or an author by ASIN in a region.
func (a *audible) audnexus(ctx context.Context, what, region, asin string, ttl func([]byte) time.Duration, v any) error {
	if _, ok := audibleHosts[region]; !ok || !IsASIN(asin) {
		return fmt.Errorf("%w: audnexus %s %q in %q", ErrNotFound, what, asin, region)
	}
	path := "/" + what + "/" + asin + "?" + url.Values{"region": {region}}.Encode()
	body, err := a.f.fetch(ctx, request{provider: provAudnexus, url: audnexusAPI + path, key: path, ttl: ttl})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode audnexus %s %s: %w", what, asin, err)
	}
	return nil
}

func bookAuthors(ps []audiblePerson) []BookAuthor {
	out := []BookAuthor{}
	for _, p := range ps {
		if n := clean(p.Name, maxName); n != "" {
			out = append(out, BookAuthor{ID: p.ASIN, Name: n})
		}
	}
	return out
}

// scalar is a JSON value that Audible and Audnexus send as either a number
// or a string ("970" or 970, "1.5" or 1.5).
type scalar string

func (s *scalar) UnmarshalJSON(b []byte) error {
	var str string
	if json.Unmarshal(b, &str) == nil {
		*s = scalar(str)
		return nil
	}
	var n json.Number
	*s = "" // any other JSON type has no value
	if json.Unmarshal(b, &n) == nil {
		*s = scalar(n.String())
	}
	return nil
}

// minutes reads a running time in minutes. It returns nil when there is
// none.
func (s scalar) minutes() *time.Duration {
	n, err := strconv.ParseFloat(string(s), 64)
	if err != nil || n <= 0 {
		return nil
	}
	d := time.Duration(n * float64(time.Minute))
	return &d
}

// optText returns cleaned provider text, or nil when there is none.
func optText(s string, n int) *string {
	if s = clean(s, n); s == "" {
		return nil
	}
	return &s
}

// optDate returns the date a timestamp or date starts with, or nil when
// there is none.
func optDate(s string) *string {
	if d := optDateString(s); d != "" {
		return &d
	}
	return nil
}

func optDateString(s string) string {
	if len(s) < 10 {
		return ""
	}
	return releaseDate(s[:10])
}
