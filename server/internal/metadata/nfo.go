package metadata

import (
	"cmp"
	"encoding/xml"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

// maxNFO bounds how much of an NFO file is read.
const maxNFO = 1 << 20

// nfo is the metadata an NFO file sets, such as movie.nfo, tvshow.nfo or an
// episode's .nfo. The root element's name doesn't matter.
type nfo struct {
	Title         string     `xml:"title"`
	OriginalTitle string     `xml:"originaltitle"`
	SortTitle     string     `xml:"sorttitle"`
	Year          string     `xml:"year"`
	Plot          string     `xml:"plot"`
	Outline       string     `xml:"outline"`
	MPAA          string     `xml:"mpaa"`
	Premiered     string     `xml:"premiered"`
	Aired         string     `xml:"aired"`
	Genres        []string   `xml:"genre"`
	Tags          []string   `xml:"tag"`
	Studios       []string   `xml:"studio"`
	Directors     []string   `xml:"director"`
	Writers       []string   `xml:"credits"`
	Actors        []nfoActor `xml:"actor"`

	mtime time.Time // decides which file wins when several describe one work
}

type nfoActor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role"`
	Order string `xml:"order"`
}

// parseNFO reads an NFO file. encoding/xml never expands entities a DTD
// declares, so entity bombs and external entities fail to parse instead of
// expanding.
func parseNFO(r io.Reader) (*nfo, error) {
	d := xml.NewDecoder(io.LimitReader(r, maxNFO))
	var n nfo
	if err := d.Decode(&n); err != nil {
		return nil, err
	}
	return &n, nil
}

// apply overlays what the NFO sets onto m, because local files win over
// providers.
func (n *nfo) apply(m *catalog.WorkMetadata, country string) {
	set := func(dst *string, v string, limit int) {
		if v = clean(v, limit); v != "" {
			*dst = v
		}
	}
	set(&m.Title, n.Title, maxTitle)
	set(&m.OriginalTitle, n.OriginalTitle, maxTitle)
	set(&m.SortTitle, n.SortTitle, maxTitle)
	set(&m.Overview, cmp.Or(strings.TrimSpace(n.Plot), n.Outline), maxOverview)
	date := cmp.Or(strings.TrimSpace(n.Aired), strings.TrimSpace(n.Premiered))
	if _, err := time.Parse(time.DateOnly, date); err == nil {
		m.ReleaseDate = date
		m.Year = year(date)
	}
	if y, err := strconv.Atoi(strings.TrimSpace(n.Year)); err == nil && y > 1800 {
		m.Year = y
	}
	if c, r := contentRating(country, n.MPAA); r != nil {
		m.ContentRating, m.ContentRatingAge = c, r
	}
	for _, l := range []struct {
		dst *[]string
		src []string
	}{{&m.Genres, n.Genres}, {&m.Tags, n.Tags}, {&m.Companies, n.Studios}} {
		var vs []string
		for _, v := range l.src {
			vs = appendNew(vs, clean(v, maxName))
		}
		if len(vs) > 0 {
			*l.dst = vs
		}
	}

	// Each role the NFO lists replaces the provider's people in that role.
	actors := slices.Clone(n.Actors)
	slices.SortStableFunc(actors, func(a, b nfoActor) int {
		x, errA := strconv.Atoi(strings.TrimSpace(a.Order))
		y, errB := strconv.Atoi(strings.TrimSpace(b.Order))
		if errA != nil || errB != nil {
			return 0
		}
		return x - y
	})
	var local []catalog.Credit
	for _, a := range actors {
		local = append(local, catalog.Credit{Name: clean(a.Name, maxName), Role: roleActor, Character: clean(a.Role, maxName)})
	}
	for _, d := range n.Directors {
		local = append(local, catalog.Credit{Name: clean(d, maxName), Role: roleDirector})
	}
	for _, w := range n.Writers {
		local = append(local, catalog.Credit{Name: clean(w, maxName), Role: roleWriter})
	}
	local = slices.DeleteFunc(local, func(c catalog.Credit) bool { return c.Name == "" })
	if len(local) == 0 {
		return
	}
	credits := slices.DeleteFunc(slices.Clone(m.Credits), func(c catalog.Credit) bool {
		return slices.ContainsFunc(local, func(l catalog.Credit) bool { return l.Role == c.Role })
	})
	m.Credits = append(local, credits...)
}
