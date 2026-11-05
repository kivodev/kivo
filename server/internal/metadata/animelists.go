package metadata

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	animeListsURL = "https://raw.githubusercontent.com/Anime-Lists/anime-lists/master/anime-list-master.xml"
	ttlAnimeLists = 7 * day // the project updates the file every few days
)

// AniDB is what the Anime-Lists mapping knows of one AniDB entry: the TMDb
// or TVDB series it belongs to, and where its episodes sit in that series.
// AniDB numbers each cour or season as its own entry, starting from
// episode 1. So an entry's episode n is episode n+Offset of Season in TVDB's
// (and TMDb's) aired order.
type AniDB struct {
	TMDbMovie, TMDbSeries, TVDB, IMDb string // empty when the mapping has none
	// Season is the aired season the entry's episodes are in. Absolute means
	// they are numbered across the series instead, and Season is unused.
	Season   int
	Absolute bool
	Offset   int
}

// animeLists holds the parsed mapping file in memory and reloads it once it
// is a week old. Parsing several megabytes of XML per lookup would be
// wasteful.
type animeLists struct {
	f *fetcher

	mu     sync.Mutex
	byID   map[string]AniDB
	loaded time.Time
}

func newAnimeLists(f *fetcher) *animeLists { return &animeLists{f: f} }

// AniDB looks up an AniDB ID in the Anime-Lists mapping. This is the only
// way an {anidb-…} path ID resolves. ErrNotFound means the mapping has no
// usable entry for it.
func (s *Service) AniDB(ctx context.Context, anidb string) (AniDB, error) {
	return s.anime.AniDB(ctx, anidb)
}

// AniDB returns what the mapping knows of an AniDB ID. It returns
// ErrNotFound when the mapping has no usable entry for it.
func (a *animeLists) AniDB(ctx context.Context, anidb string) (AniDB, error) {
	m, err := a.get(ctx)
	if err != nil {
		return AniDB{}, err
	}
	x, ok := m[anidb]
	if !ok {
		return AniDB{}, fmt.Errorf("%w: anidb %s", ErrNotFound, anidb)
	}
	return x, nil
}

func (a *animeLists) get(ctx context.Context) (map[string]AniDB, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.byID != nil && time.Since(a.loaded) < ttlAnimeLists {
		return a.byID, nil
	}
	body, err := a.f.fetch(ctx, request{provider: provAnimeLists, url: animeListsURL, key: "anime-list-master.xml", ttl: fixed(ttlAnimeLists)})
	if err != nil {
		if a.byID != nil {
			return a.byID, nil // an old mapping beats none
		}
		return nil, err
	}
	m, err := parseAnimeLists(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	a.byID, a.loaded = m, time.Now()
	return m, nil
}

// parseAnimeLists reads anime-list-master.xml. It leaves out entries mapped
// to nothing a provider can look up. Those have a tvdbid of "movie", "OVA",
// "unknown" or similar, and no TMDb or IMDb ID.
func parseAnimeLists(r io.Reader) (map[string]AniDB, error) {
	type entry struct {
		AniDB      string `xml:"anidbid,attr"`
		TVDB       string `xml:"tvdbid,attr"`
		Season     string `xml:"defaulttvdbseason,attr"`
		Offset     string `xml:"episodeoffset,attr"`
		TMDbMovie  string `xml:"tmdbid,attr"`
		TMDbSeries string `xml:"tmdbtv,attr"`
		IMDb       string `xml:"imdbid,attr"`
	}
	d := xml.NewDecoder(r)
	out := map[string]AniDB{}
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "anime" {
			continue
		}
		var e entry
		if err := d.DecodeElement(&e, &se); err != nil {
			return nil, err
		}
		a := AniDB{TMDbMovie: numeric(e.TMDbMovie), TMDbSeries: numeric(e.TMDbSeries), TVDB: numeric(e.TVDB)}
		if imdb, _, _ := strings.Cut(e.IMDb, ","); strings.HasPrefix(imdb, "tt") {
			a.IMDb = imdb
		}
		a.Absolute = e.Season == "a"
		a.Season, _ = strconv.Atoi(e.Season)
		a.Offset, _ = strconv.Atoi(e.Offset)
		if e.AniDB != "" && (a.TMDbMovie != "" || a.TMDbSeries != "" || a.TVDB != "" || a.IMDb != "") {
			out[e.AniDB] = a
		}
	}
}

// numeric returns s if it is a numeric ID, and "" otherwise. The file uses
// words such as "movie" and "unknown" in ID attributes.
func numeric(s string) string {
	s, _, _ = strings.Cut(s, ",")
	if _, err := strconv.Atoi(s); err != nil || s == "0" {
		return ""
	}
	return s
}
