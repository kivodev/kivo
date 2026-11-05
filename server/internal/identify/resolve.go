package identify

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/identify/parse"
	"github.com/mmcdole/kivo/internal/metadata"
)

// The sources of a decision, in order of trust.
const (
	sourceManual = "manual"
	sourcePath   = "path"
	sourceNFO    = "nfo"
	sourceSearch = "search"
)

// decision records what a unit was decided to be. match is the accepted
// match, or nil when the unit is unmatched.
type decision struct {
	source, query string
	match         *metadata.Match
	score         int
	anime         *metadata.AniDB // set when an AniDB ID decided; says where its episodes sit
}

// decide walks the sources in order of trust and stops at the first one
// that decides. A stored match is reused when its source and query are
// unchanged, so a matched folder is searched once, not once per run or per
// file. An unmatched folder is looked up again each time. A manual decision,
// matched or not, always stands.
func (i *Identifier) decide(ctx context.Context, s *snapshot) (decision, error) {
	u := s.unit
	prev, err := i.storedMatch(ctx, s)
	switch {
	case err != nil:
		return decision{}, err
	case prev.ok && prev.row.Source == sourceManual:
		return fromRow(prev.row), nil
	}
	if d, ok, err := i.exact(ctx, s, sourcePath, u.ids, prev); ok || err != nil {
		return d, err
	}
	var nfo []parse.ExternalID
	for _, name := range u.nfos {
		if b, ok := s.nfos[name]; ok {
			nfo = append(nfo, parseNFO(b)...)
		}
	}
	if d, ok, err := i.exact(ctx, s, sourceNFO, nfo, prev); ok || err != nil {
		return d, err
	}
	if !u.search {
		return decision{}, nil // nothing to search by, so it stays provisional
	}
	query := parse.Key(u.title) + "|" + strconv.Itoa(u.year)
	if prev.matched(sourceSearch, query) {
		return fromRow(prev.row), nil
	}
	alts := slices.DeleteFunc(slices.Clone(u.alts), func(a string) bool { return parse.Key(a) == parse.Key(u.title) })
	c, score, ok, err := i.search(ctx, want{kind: u.kind, title: u.title, year: u.year, country: u.country}, s.lib.MetadataLanguage, alts)
	d := decision{source: sourceSearch, query: query, score: score}
	if ok {
		d.match = &c
	}
	return d, err
}

// stored is the match row a unit had before this decision. ok is false
// when there was none.
type stored struct {
	row dbq.Match
	ok  bool
}

// storedMatch reads the match row of the snapshot's unit.
func (i *Identifier) storedMatch(ctx context.Context, s *snapshot) (stored, error) {
	row, err := i.q.GetMatch(ctx, dbq.GetMatchParams{LibraryID: s.lib.ID, Subject: s.unit.subject})
	if errors.Is(err, sql.ErrNoRows) {
		return stored{}, nil
	}
	return stored{row: row, ok: err == nil}, err
}

// matched reports whether the stored row is a match found through source
// with query.
func (st stored) matched(source, query string) bool {
	return st.ok && st.row.Status == statusMatched && st.row.Source == source && st.row.Query == query
}

// records reports whether the stored row already records d with the given
// ordering.
func (st stored) records(d decision, ordering string) bool {
	return st.ok && fromRow(st.row).same(d) && st.row.Ordering.String == ordering
}

// same reports whether two decisions came from the same lookup and chose
// the same match.
func (d decision) same(o decision) bool {
	if d.source != o.source || d.query != o.query || (d.match == nil) != (o.match == nil) {
		return false
	}
	return d.match == nil || d.match.ID == o.match.ID
}

func fromRow(r dbq.Match) decision {
	d := decision{source: r.Source, query: r.Query, score: int(r.Score.Int64)}
	if r.Status == statusMatched {
		d.match = &metadata.Match{
			ID:    catalog.ExternalID{Provider: r.Provider.String, Kind: r.Kind.String, ID: r.ExtID.String},
			Title: r.Title.String, Year: int(r.Year.Int64),
		}
	}
	return d
}

// exact tries the IDs from one source, preferring TMDb's own. An ID the
// provider doesn't know is skipped. An AniDB ID resolves only through the
// Anime-Lists mapping, and one the mapping can't map leaves the unit
// unmatched.
func (i *Identifier) exact(ctx context.Context, s *snapshot, source string, ids []parse.ExternalID, prev stored) (decision, bool, error) {
	kind, lang := s.unit.kind, s.lib.MetadataLanguage
	order := map[string]int{"tmdb": 0, "imdb": 1, "tvdb": 2, "anidb": 3}
	ids = slices.Clone(ids)
	slices.SortStableFunc(ids, func(a, b parse.ExternalID) int { return order[a.Provider] - order[b.Provider] })
	for _, x := range ids {
		d := decision{source: source, query: x.Provider + ":" + x.ID, score: 100}
		target := catalog.ExternalID{Provider: x.Provider, Kind: string(kind), ID: x.ID}
		if x.Provider == "anidb" {
			a, err := i.deps.Metadata.AniDB(ctx, x.ID)
			if errors.Is(err, metadata.ErrNotFound) {
				return decision{source: source, query: d.query}, true, nil
			}
			if err != nil {
				return decision{}, false, err
			}
			d.anime = &a
			if target = animeTarget(kind, a); target.ID == "" {
				return decision{source: source, query: d.query}, true, nil
			}
		}
		if prev.matched(source, d.query) {
			r := fromRow(prev.row)
			r.anime = d.anime
			return r, true, nil
		}
		c, err := i.deps.Metadata.Resolve(ctx, target, lang)
		switch {
		case errors.Is(err, metadata.ErrNotFound):
			if x.Provider == "anidb" {
				return decision{source: source, query: d.query}, true, nil
			}
			continue
		case err != nil:
			return decision{}, false, err
		}
		d.match = &c
		return d, true, nil
	}
	return decision{}, false, nil
}

// animeTarget returns the provider ID an AniDB entry maps to for a work
// kind.
func animeTarget(kind catalog.WorkKind, a metadata.AniDB) catalog.ExternalID {
	if kind == catalog.KindMovie {
		if a.TMDbMovie != "" {
			return catalog.ExternalID{Provider: "tmdb", Kind: string(kind), ID: a.TMDbMovie}
		}
		return catalog.ExternalID{Provider: "imdb", Kind: string(kind), ID: a.IMDb}
	}
	if a.TMDbSeries != "" {
		return catalog.ExternalID{Provider: "tmdb", Kind: string(kind), ID: a.TMDbSeries}
	}
	return catalog.ExternalID{Provider: "tvdb", Kind: string(kind), ID: a.TVDB}
}

// search looks for w. It searches with w's year, then with any year, then
// by the other titles the names suggest. If no result is accepted, it looks
// up the most promising one, to also compare its alternative titles and
// other release years. lang is the library's metadata language; empty means the
// server's.
func (i *Identifier) search(ctx context.Context, w want, lang string, alts []string) (metadata.Match, int, bool, error) {
	find := i.deps.Metadata.SearchMovies
	if w.kind == catalog.KindSeries {
		find = i.deps.Metadata.SearchSeries
	}
	var tries []metadata.Query
	for _, t := range append([]string{w.title}, alts...) {
		if w.year != 0 {
			tries = append(tries, metadata.Query{Title: t, Year: w.year, Language: lang})
		}
		tries = append(tries, metadata.Query{Title: t, Language: lang})
	}
	var seen []metadata.Match
	inYear := map[catalog.ExternalID]bool{} // found by a search by year
	for _, q := range tries {
		matches, err := find(ctx, q)
		if err != nil {
			return metadata.Match{}, 0, false, err
		}
		if c, sc, ok := accept(want{kind: w.kind, title: q.Title, year: w.year, country: w.country}, matches); ok {
			return c, sc, true, nil
		}
		for _, c := range matches {
			if !slices.ContainsFunc(seen, func(s metadata.Match) bool { return s.ID == c.ID }) {
				seen = append(seen, c)
			}
			inYear[c.ID] = inYear[c.ID] || q.Year != 0
		}
	}
	var full []metadata.Match
	if len(seen) > 0 {
		r, err := i.deps.Metadata.Resolve(ctx, lookupOrder(w, seen, inYear)[0].ID, lang)
		switch {
		case errors.Is(err, metadata.ErrNotFound):
		case err != nil:
			return metadata.Match{}, 0, false, err
		default:
			full = append(full, r)
		}
	}
	c, sc, ok := accept(w, full)
	if !ok { // keep the best score seen with the unmatched decision
		sc = 0
		for _, c := range append(seen, full...) {
			sc = max(sc, score(w, c))
		}
	}
	return c, sc, ok, nil
}

// lookupOrder orders search results for looking up, the most promising
// first. A search result carries only its first release year and none of
// its alternative titles. A search by year finds a work released that year
// anywhere, so the results of one, marked in inYear, go first. Then the
// closest by title, less at most 10 points for the year, as another release
// may be near enough.
func lookupOrder(w want, seen []metadata.Match, inYear map[catalog.ExternalID]bool) []metadata.Match {
	near := func(c metadata.Match) int { return likeness(w.title, c) - min(3*yearDistance(w.year, c), 10) }
	ranked := slices.Clone(seen)
	slices.SortStableFunc(ranked, func(a, b metadata.Match) int {
		if inYear[a.ID] != inYear[b.ID] {
			if inYear[a.ID] {
				return -1
			}
			return 1
		}
		return near(b) - near(a)
	})
	return ranked
}

// placeEpisodes returns the series' ordering and the provider episodes of
// each version that still needs placing. A version needs no episode list if
// it is already on non-provisional episodes of the root, the root is what
// the decision describes, and it was placed in this ordering. So a settled
// folder makes no provider call.
func (i *Identifier) placeEpisodes(ctx context.Context, s *snapshot, ps placements, d decision, root id.ID, es []*entry) (string, map[*entry][]metadata.Episode, error) {
	o, err := i.q.Ordering(ctx, root)
	if err != nil {
		return "", nil, err
	}
	ordering := o.String
	described, err := i.describes(ctx, i.q, root, d.match)
	if err != nil {
		return "", nil, err
	}
	placedIn := ""
	if row, err := i.q.GetMatch(ctx, dbq.GetMatchParams{LibraryID: s.lib.ID, Subject: s.unit.subject}); err == nil {
		placedIn = row.Ordering.String
	}
	var pending []*entry
	for _, e := range es {
		p := ps.of(e)
		settled := p != nil && described && placedIn == ordering &&
			!slices.ContainsFunc(p.works, func(w placedWork) bool { return w.provisional || w.root != root })
		if !settled {
			pending = append(pending, e)
		}
	}
	if len(pending) == 0 {
		return ordering, nil, nil
	}
	eps, err := i.episodes(ctx, d.match.ID, metadata.Ordering(ordering), s.lib.MetadataLanguage, d.anime, pending)
	return ordering, eps, err
}

// refetchAge is the age past which an episode list that is missing a number
// is fetched again, once.
const refetchAge = time.Minute

// episodes finds the provider episodes each version covers. Episodes are
// looked up, never searched for. A name is looked up by season and number
// in the series' ordering. A dated name is looked up by air date only, never
// by number. An absolute number goes through the absolute order or the
// Anime-Lists mapping.
//
// A number missing from the list triggers one refetch. A version still
// unplaced after that is left out of the result and stays unmatched.
func (i *Identifier) episodes(ctx context.Context, series catalog.ExternalID, o metadata.Ordering, lang string, anime *metadata.AniDB, es []*entry) (map[*entry][]metadata.Episode, error) {
	out := map[*entry][]metadata.Episode{}
	var list, abs []metadata.Episode
	needAbs := o != metadata.Absolute && slices.ContainsFunc(es, func(e *entry) bool {
		return len(e.guess.Absolute) > 0 && len(e.guess.Episodes) == 0 && (anime == nil || anime.Absolute)
	})
	fetch := func(maxAge time.Duration) error {
		var err error
		if list, err = i.deps.Metadata.Episodes(ctx, series, o, lang, maxAge); err != nil {
			return err
		}
		if needAbs {
			abs, err = i.deps.Metadata.Episodes(ctx, series, metadata.Absolute, lang, maxAge)
		}
		return err
	}
	if err := fetch(0); err != nil {
		return nil, err
	}
	var missing []*entry
	for _, e := range es {
		if eps := lookup(e.guess, list, abs, o, anime); eps != nil {
			out[e] = eps
		} else if lookable(e.guess) {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	if err := fetch(refetchAge); err != nil {
		return nil, err
	}
	for _, e := range missing {
		if eps := lookup(e.guess, list, abs, o, anime); eps != nil {
			out[e] = eps
		}
	}
	return out, nil
}

// lookable reports whether a name says which episode it is.
func lookable(g parse.Guess) bool {
	return !g.Special && (len(g.Episodes) > 0 || len(g.Absolute) > 0 || !g.AirDate.IsZero())
}

// lookup finds a name's episodes in an episode list, or returns nil. Every
// episode the name covers must be found exactly once.
func lookup(g parse.Guess, list, abs []metadata.Episode, o metadata.Ordering, anime *metadata.AniDB) []metadata.Episode {
	switch {
	case g.Special:
		return nil
	case !g.AirDate.IsZero():
		var on []metadata.Episode
		for _, e := range list {
			if e.AirDate == g.AirDate.String() {
				on = append(on, e)
			}
		}
		if len(on) != 1 {
			return nil
		}
		return on
	case len(g.Episodes) > 0:
		return numbered(list, g.Season, g.Episodes, 0)
	case len(g.Absolute) > 0 && anime != nil && !anime.Absolute:
		// An AniDB entry numbers from 1 within one season of the series.
		return numbered(list, anime.Season, g.Absolute, anime.Offset)
	case len(g.Absolute) > 0 && o == metadata.Absolute:
		return numbered(list, 1, g.Absolute, 0)
	case len(g.Absolute) > 0:
		// Map through the absolute order to the provider's episode ID, then
		// find the same episode in the series' ordering.
		in := numbered(abs, 1, g.Absolute, 0)
		var out []metadata.Episode
		for _, a := range in {
			k := slices.IndexFunc(list, func(e metadata.Episode) bool { return e.ID == a.ID })
			if k < 0 {
				return nil
			}
			out = append(out, list[k])
		}
		return out
	}
	return nil
}

func numbered(list []metadata.Episode, season int, numbers []int, offset int) []metadata.Episode {
	if len(numbers) == 0 {
		return nil
	}
	var out []metadata.Episode
	for _, n := range numbers {
		var found []metadata.Episode
		for _, e := range list {
			if e.Season == season && e.Number == n+offset {
				found = append(found, e)
			}
		}
		if len(found) != 1 {
			return nil // missing, or two episodes share the number
		}
		out = append(out, found[0])
	}
	return out
}
