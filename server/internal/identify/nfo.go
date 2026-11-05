package identify

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"

	"github.com/mmcdole/kivo/internal/identify/parse"
)

// maxNFO bounds how much of an NFO file is read.
const maxNFO = 1 << 20

// nfoXML is the part of an NFO file that names a movie or series: the
// <uniqueid> elements and the older single-provider tags.
type nfoXML struct {
	UniqueIDs []struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"uniqueid"`
	TMDb string `xml:"tmdbid"`
	IMDb string `xml:"imdbid"`
	TVDB string `xml:"tvdbid"`
	ID   string `xml:"id"`
}

// nfoURLRe finds provider links in an NFO that holds only a URL.
var nfoURLRe = regexp.MustCompile(`themoviedb\.org/(?:movie|tv)/(\d+)|imdb\.com/title/(tt\d+)`)

// parseNFO reads the IDs in an NFO file. The metadata package reads the
// rest. encoding/xml expands no entities a DTD declares, so entity tricks
// fail to parse instead of expanding.
func parseNFO(b []byte) []parse.ExternalID {
	var ids []parse.ExternalID
	var n nfoXML
	if err := xml.Unmarshal(b, &n); err == nil {
		for _, u := range n.UniqueIDs {
			ids = appendID(ids, strings.ToLower(strings.TrimSpace(u.Type)), u.Value)
		}
		ids = appendID(ids, "tmdb", n.TMDb)
		ids = appendID(ids, "imdb", n.IMDb)
		ids = appendID(ids, "tvdb", n.TVDB)
		if id := strings.TrimSpace(n.ID); strings.HasPrefix(id, "tt") {
			ids = appendID(ids, "imdb", id)
		}
		return ids
	}
	for _, m := range nfoURLRe.FindAllSubmatch(b, -1) {
		if len(m[1]) > 0 {
			ids = appendID(ids, "tmdb", string(m[1]))
		} else {
			ids = appendID(ids, "imdb", string(m[2]))
		}
	}
	return ids
}

// appendID adds a provider ID if Kivo can look it up and it is well formed.
// Well formed means digits, or tt and digits for IMDb. The ID later shapes a
// provider URL, so nothing else gets through.
func appendID(ids []parse.ExternalID, provider, v string) []parse.ExternalID {
	v = strings.TrimSpace(v)
	digits := v
	switch provider {
	case "tmdb", "tvdb", "anidb":
	case "imdb":
		if !strings.HasPrefix(v, "tt") {
			return ids
		}
		digits = v[2:]
	default:
		return ids
	}
	if n, err := strconv.ParseUint(digits, 10, 64); err != nil || n == 0 || len(digits) > 12 {
		return ids
	}
	for _, x := range ids {
		if x.Provider == provider && x.ID == v {
			return ids
		}
	}
	return append(ids, parse.ExternalID{Provider: provider, ID: v})
}
