package metadata

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/media"
)

// Provider text is stored as length-capped plain text.
const (
	maxTitle    = 500
	maxOverview = 10000
	maxName     = 200
	maxCast     = 50
	// maxCastPhotos is how many of the billed cast have their photos
	// fetched, in addition to every director and writer. These are the
	// faces a detail page leads with.
	maxCastPhotos = 20
)

// remoteImage is an image a provider offers as artwork.
type remoteImage struct{ provider, url string }

// remotePhoto is a provider's photo of a person a work credits.
type remotePhoto struct {
	person catalog.ExternalID
	image  remoteImage
}

// found is what the providers say about a work.
type found struct {
	meta       catalog.WorkMetadata
	artwork    map[string][]remoteImage // by kind, best first
	photos     []remotePhoto            // of the people the work credits
	collection map[string][]remoteImage // a movie's collection's artwork, by kind
}

// fromProviders gathers a work's provider data. It returns nil with no
// error for a work no provider knows. That is a work with no usable external
// ID, or one the provider has removed.
func (s *Service) fromProviders(ctx context.Context, w catalog.Work, lang, country string) (*found, error) {
	r, ok, err := s.ref(ctx, w)
	if err != nil || !ok {
		return nil, err
	}
	d, err := s.video.Describe(ctx, r, lang)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f := &found{artwork: s.providerArtwork(d.Images, lang)}
	f.meta = catalog.WorkMetadata{
		Title: clean(d.Title, maxTitle), Overview: clean(d.Overview, maxOverview),
		ReleaseDate: releaseDate(d.ReleaseDate), Year: year(d.ReleaseDate),
	}
	switch w.Kind {
	case catalog.KindEpisode:
		f.meta.Credits, f.meta.Community = credits(d.Cast, d.Crew), d.Community
		f.photos = s.photosOf(d.Cast, d.Crew)
	case catalog.KindMovie, catalog.KindSeries:
		f.meta.OriginalTitle = clean(d.OriginalTitle, maxTitle)
		f.meta.OriginalLanguage = languageCode(d.OriginalLanguage)
		f.meta.Genres, f.meta.Companies, f.meta.Tags = names(d.Genres), names(d.Companies), names(d.Tags)
		f.meta.Credits, f.meta.Community = credits(d.Cast, d.Crew), d.Community
		f.meta.TrailerURL = d.TrailerURL
		f.photos = s.photosOf(d.Cast, d.Crew)
		for _, a := range d.AltTitles {
			f.meta.AltTitles = appendNew(f.meta.AltTitles, clean(a, maxTitle))
		}
		if r := d.ContentRatings[country]; r != "" {
			f.meta.ContentRating, f.meta.ContentRatingAge = contentRating(country, r)
		}
		if c := d.Collection; c != nil {
			f.meta.Collection = &catalog.CollectionRef{Key: c.Ref.Key, Name: clean(c.Ref.Name, maxTitle), Order: c.Ref.Order}
			f.collection = s.providerArtwork(c.Images, lang)
		}
	}
	return f, nil
}

// ref names a movie, series, season or episode for the video provider. ok
// is false for other kinds of work.
func (s *Service) ref(ctx context.Context, w catalog.Work) (_ Ref, ok bool, err error) {
	switch w.Kind {
	case catalog.KindMovie, catalog.KindSeries:
		return Ref{Kind: w.Kind, IDs: w.ExternalIDs}, true, nil
	case catalog.KindSeason, catalog.KindEpisode:
		root, err := s.deps.Catalog.Work(ctx, w.Root)
		if err != nil {
			return Ref{}, false, notFound(err)
		}
		r := Ref{Kind: w.Kind, IDs: root.ExternalIDs, Season: w.Number}
		if w.Kind == catalog.KindEpisode {
			r.Season, r.Episode, r.EpisodeIDs = w.ParentNumber, w.Number, w.ExternalIDs
		}
		return r, true, nil
	}
	return Ref{}, false, nil
}

// providerArtwork converts a provider's images to artwork by kind, best
// first.
func (s *Service) providerArtwork(imgs Images, lang string) map[string][]remoteImage {
	out := map[string][]remoteImage{}
	for kind, list := range imagesByKind(imgs) {
		for _, img := range ranked(kind, list, lang, imgs.Original) {
			out[kind] = append(out[kind], remoteImage{provider: s.video.Name(), url: img.URL})
		}
	}
	return out
}

// imagesByKind groups a provider's images by artwork kind.
func imagesByKind(imgs Images) map[string][]Image {
	return map[string][]Image{artworkPoster: imgs.Posters, artworkBackdrop: imgs.Backdrops, artworkLogo: imgs.Logos, artworkStill: imgs.Stills}
}

// ranked orders a provider's images of a kind best first, by language and
// then by score. It leaves out images in a language the kind is never shown
// in.
func ranked(kind string, imgs []Image, lang, original string) []Image {
	imgs = slices.DeleteFunc(slices.Clone(imgs), func(img Image) bool {
		_, ok := langRank(kind, img.Language, lang, original)
		return !ok
	})
	slices.SortStableFunc(imgs, func(a, b Image) int {
		ra, _ := langRank(kind, a.Language, lang, original)
		rb, _ := langRank(kind, b.Language, lang, original)
		if d := ra - rb; d != 0 {
			return d
		}
		return cmp.Compare(b.Score, a.Score)
	})
	return imgs
}

// langRank ranks artwork by language, lowest first. The order is the
// metadata language, English, textless, the work's original language, then
// any other. Backdrops sit behind titles, so textless backdrops come first.
// Logos stand in for the title, so a logo in any other language is never
// used, and ok is false.
func langRank(kind, imgLang, lang, original string) (rank int, ok bool) {
	base, _, _ := strings.Cut(lang, "-")
	textless := imgLang == "" || imgLang == "00"
	switch {
	case kind == artworkBackdrop && textless:
		return 0, true
	case imgLang == base:
		return 1, true
	case imgLang == "en":
		return 2, true
	case textless:
		return 3, true
	case kind == artworkLogo:
		return 0, false
	case imgLang == original:
		return 4, true
	}
	return 5, true
}

// names cleans a provider's names and drops duplicates.
func names(ns []string) []string {
	out := []string{} // non-nil, because an empty list clears the field
	for _, n := range ns {
		out = appendNew(out, clean(n, maxName))
	}
	return out
}

// Credit roles.
const (
	roleActor    = "actor"
	roleDirector = "director"
	roleWriter   = "writer"
)

// credits returns the billed cast, followed by the directors and writers.
func credits(cast, crew []Person) []catalog.Credit {
	out := []catalog.Credit{}
	for _, c := range cast[:min(len(cast), maxCast)] {
		out = append(out, catalog.Credit{Name: clean(c.Name, maxName), Role: roleActor, Character: clean(c.Character, maxName), ExternalIDs: c.IDs})
	}
	for _, c := range crew {
		cr := catalog.Credit{Name: clean(c.Name, maxName), Role: c.Role, ExternalIDs: c.IDs}
		if !slices.ContainsFunc(out, func(o catalog.Credit) bool { return o.Name == cr.Name && o.Role == cr.Role }) {
			out = append(out, cr)
		}
	}
	return out
}

// photosOf returns the provider's photos of a work's top-billed cast and of
// its directors and writers, with each person once.
func (s *Service) photosOf(cast, crew []Person) []remotePhoto {
	var out []remotePhoto
	seen := map[catalog.ExternalID]bool{}
	add := func(p Person) {
		if len(p.IDs) == 0 || p.Photo == "" || seen[p.IDs[0]] {
			return
		}
		seen[p.IDs[0]] = true
		out = append(out, remotePhoto{person: p.IDs[0], image: remoteImage{provider: s.video.Name(), url: p.Photo}})
	}
	for _, c := range cast[:min(len(cast), maxCastPhotos)] {
		add(c)
	}
	for _, c := range crew {
		add(c)
	}
	return out
}

// year returns the year of a YYYY-MM-DD date, or zero.
func year(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

// appendNew appends s to list unless it's empty or listed already.
func appendNew(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// releaseDate returns s if it is a valid YYYY-MM-DD date, and empty
// otherwise.
func releaseDate(s string) string {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return ""
	}
	return s
}

// languageCode returns a provider's language code if it names a language,
// and "" otherwise. TMDb's "xx" names no language.
func languageCode(code string) string {
	if media.BaseLanguage(code) == "" {
		return ""
	}
	return code
}

// clean makes provider or NFO text safe to store. The result is valid UTF-8
// with no control characters except newlines, trimmed, and at most n
// runes.
func clean(s string, n int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > n {
		s = strings.TrimSpace(string([]rune(s)[:n]))
	}
	return s
}
