package identify

import (
	"cmp"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/parse"
)

// classify turns a snapshot's files into units. It handles ignored files
// first, then disc rips, then extras, then parts. That order means an extra
// is never grouped as a part, and a part is never taken for a version.
func classify(p parse.Parser, kind catalog.LibraryKind, folder string, files []file) []*unit {
	switch kind {
	case catalog.Shows:
		return classifyShows(p, folder, files)
	case catalog.Videos:
		return classifyVideos(p, files)
	case catalog.Audiobooks:
		return classifyBooks(folder, files)
	}
	return classifyMovies(p, folder, files)
}

// classifyMovies splits a movie folder's files into units.
//
// A file is a version of the folder's movie if its name starts with the
// folder's name and differs only after a separator, or if it parses to the
// folder's title. A file alone in its folder is also a version, and takes
// the folder's name and year. Other loose movies sharing the folder are
// unrelated, and each is a unit of its own. Each movie then takes its
// folder's edition label (see label).
func classifyMovies(p parse.Parser, folder string, files []file) []*unit {
	var main, extras []*entry
	discs := map[string][]file{} // disc folder → its files
	for _, f := range files {
		if ignored(f.Rel) || folderOf(catalog.Movies, f.Rel) != folder {
			continue
		}
		if d, ok := discFolder(f.Rel); ok {
			discs[d] = append(discs[d], f)
			continue
		}
		g := p.Parse(f.Rel, parse.Movies)
		e := &entry{parts: []file{f}, guess: g, extra: g.Extra}
		if g.Extra != "" {
			e.title = cmp.Or(g.Title, stem(f.Rel))
			extras = append(extras, e)
			continue
		}
		e.edition = g.Edition
		main = append(main, e)
	}
	for _, d := range slices.Sorted(maps.Keys(discs)) {
		if parts := discParts(discs[d]); len(parts) > 0 {
			g := p.Parse(d+"/"+path.Base(d)+".iso", parse.Movies)
			main = append(main, &entry{parts: parts, guess: g, edition: g.Edition})
		}
	}
	main = groupParts(main)

	if folder == "" {
		// A file directly in the library folder stands alone. It is the only
		// file a loose snapshot holds.
		var out []*unit
		for _, e := range main {
			u := looseMovie("", e)
			u.label("")
			out = append(out, u)
		}
		return out
	}
	dg := p.Dir(path.Base(folder))
	var mine, loose []*entry
	for _, e := range main {
		if len(main) == 1 || namedAfter(stem(e.parts[0].Rel), path.Base(folder)) ||
			parse.Key(e.guess.Title) == parse.Key(dg.Title) && (e.guess.Year == 0 || e.guess.Year == dg.Year) {
			mine = append(mine, e)
		} else {
			loose = append(loose, e)
		}
	}
	var out []*unit
	if len(mine) > 0 {
		u := &unit{subject: folder, kind: catalog.KindMovie, title: dg.Title, year: dg.Year, country: dg.Country, search: dg.Title != ""}
		for _, e := range mine {
			u.ids = append(u.ids, e.guess.IDs...)
			u.nfos = append(u.nfos, nfoBeside(e.parts[0].Rel))
			if t := e.guess.Title; t != "" && parse.Key(t) != parse.Key(u.title) && !slices.Contains(u.alts, t) && !hashed(e.parts[0].Rel) {
				u.alts = append(u.alts, t)
			}
		}
		if u.title == "" { // a folder named only by an ID
			u.title, u.year, u.search = mine[0].guess.Title, mine[0].guess.Year, !hashed(mine[0].parts[0].Rel)
		}
		u.ids = append(u.ids, folderIDs(p, folder)...)
		u.nfos = append(u.nfos, path.Join(folder, "movie.nfo"))
		u.versions = mine
		u.label(dg.Edition)
		out = append(out, u)
	}
	bySubject := map[string]*unit{}
	for _, e := range loose {
		u := looseMovie(folder, e)
		if prev := bySubject[u.subject]; prev != nil {
			prev.versions = append(prev.versions, e)
			continue
		}
		bySubject[u.subject] = u
		out = append(out, u)
	}
	for _, u := range out {
		if u.subject != folder { // a loose movie, which the folder's tag doesn't name
			u.label("")
		}
	}
	if len(out) > 0 {
		// Extras belong to the folder's movie, or to the first movie when the
		// folder holds only loose movies.
		out[0].extras = extras
	}
	return out
}

// label gives a movie its folder's edition label. A folder's edition tag,
// as in "Blade Runner (1982) {edition-Final Cut}", labels the folder's
// movie. Its files with that tag carry no label of their own, as the movie's
// label names their cut, and a file with another tag is a version of that
// edition.
func (u *unit) label(folder string) {
	u.edition = folder
	for _, e := range u.versions {
		if e.edition == folder {
			e.edition = ""
		}
	}
}

func looseMovie(folder string, e *entry) *unit {
	g := e.guess
	u := &unit{
		subject: folder + "|" + parse.Key(g.Title) + "|" + strconv.Itoa(g.Year), kind: catalog.KindMovie,
		title: g.Title, year: g.Year, ids: g.IDs, search: g.Title != "" && !hashed(e.parts[0].Rel),
		nfos: []string{nfoBeside(e.parts[0].Rel)}, versions: []*entry{e},
	}
	if !u.search {
		u.subject = folder + "|" + e.parts[0].Rel // nothing to search by, so it is its own unmatched item
		u.title = cmp.Or(g.Title, stem(e.parts[0].Rel))
	}
	return u
}

// namedAfter reports whether a file stem is its folder's name, optionally
// followed by a version label after -, ., _ or [. Examples are
// "Dune (2021) - 4K" and "Dune (2021).1080p".
func namedAfter(stem, folder string) bool {
	if len(stem) < len(folder) || !strings.EqualFold(stem[:len(folder)], folder) {
		return false
	}
	rest := strings.TrimLeft(stem[len(folder):], " ")
	return rest == "" || strings.ContainsRune("-._[", rune(rest[0]))
}

// discFolder returns the folder a disc rip is named by. That is the parent
// of its VIDEO_TS or BDMV folder.
func discFolder(rel string) (string, bool) {
	dirs := strings.Split(rel, "/")
	for i, d := range dirs[:len(dirs)-1] {
		if isDiscDir(d) && i > 0 {
			return strings.Join(dirs[:i], "/"), true
		}
	}
	return "", false
}

// discParts picks a disc rip's main title as one version's parts. For a
// Blu-ray, that is the largest stream. For a DVD, it is the title set with
// the most data, with its VOBs in order (VTS_nn_0 is the menu, and is
// skipped). The rest of the disc isn't catalogued.
func discParts(files []file) []file {
	var best file
	sets := map[string][]file{}
	sizes := map[string]int64{}
	for _, f := range files {
		base := path.Base(f.Rel)
		if strings.EqualFold(path.Ext(base), ".m2ts") && f.Size > best.Size {
			best = f
		}
		if m := vobRe.FindStringSubmatch(base); m != nil && m[2] != "0" {
			sets[m[1]] = append(sets[m[1]], f)
			sizes[m[1]] += f.Size
		}
	}
	if !best.ID.IsZero() {
		return []file{best}
	}
	var pick string
	for _, s := range slices.Sorted(maps.Keys(sets)) {
		if pick == "" || sizes[s] > sizes[pick] {
			pick = s
		}
	}
	vobs := sets[pick]
	slices.SortFunc(vobs, func(a, b file) int { return strings.Compare(a.Rel, b.Rel) })
	return vobs
}

// groupParts groups 2 to 8 files in one folder that differ only in a part
// token (cd1, part b) into one version, ordered by part.
func groupParts(es []*entry) []*entry {
	groups := map[string][]*entry{}
	var out []*entry
	for _, e := range es {
		pt := e.guess.Part
		if pt == nil || len(e.parts) > 1 {
			out = append(out, e)
			continue
		}
		rel := e.parts[0].Rel
		key := dirOf(rel) + "/" + strings.ToLower(strings.Replace(stem(rel), pt.Token, "\x00", 1)) + path.Ext(rel)
		groups[key] = append(groups[key], e)
	}
	for _, k := range slices.Sorted(maps.Keys(groups)) {
		g := groups[k]
		slices.SortFunc(g, func(a, b *entry) int { return a.guess.Part.Index - b.guess.Part.Index })
		if len(g) < 2 || len(g) > 8 || distinctParts(g) != len(g) {
			out = append(out, g...)
			continue
		}
		first := g[0]
		for _, e := range g[1:] {
			first.parts = append(first.parts, e.parts...)
		}
		out = append(out, first)
	}
	slices.SortStableFunc(out, func(a, b *entry) int { return strings.Compare(a.parts[0].Rel, b.parts[0].Rel) })
	return out
}

func distinctParts(g []*entry) int {
	seen := map[int]bool{}
	for _, e := range g {
		seen[e.guess.Part.Index] = true
	}
	return len(seen)
}

// classifyShows makes a series folder one unit. Each file is a version of
// the episodes it covers. Split episodes in one folder, such as S01E05a and
// S01E05b, are parts of one version. Extras attach to the nearest season
// folder, or else to the series.
func classifyShows(p parse.Parser, folder string, files []file) []*unit {
	u := &unit{subject: folder, kind: catalog.KindSeries, search: true}
	if folder != "" {
		dg := p.Dir(folder)
		u.title, u.year, u.country = dg.Title, dg.Year, dg.Country
		u.ids = folderIDs(p, folder)
		u.nfos = []string{path.Join(folder, "tvshow.nfo")}
	}
	splits := map[string][]*entry{}
	for _, f := range files {
		if ignored(f.Rel) {
			continue
		}
		if _, disc := discFolder(f.Rel); disc {
			continue // disc rips of shows aren't catalogued
		}
		g := p.Parse(f.Rel, parse.Shows)
		e := &entry{parts: []file{f}, guess: g, extra: g.Extra}
		if g.Extra != "" {
			e.title, e.season = cmp.Or(g.Title, stem(f.Rel)), extraSeason(p, f.Rel)
			u.extras = append(u.extras, e)
			continue
		}
		if g.SplitPart != "" && len(g.Episodes) > 0 {
			k := splitKey(f.Rel, g)
			splits[k] = append(splits[k], e)
			continue
		}
		u.versions = append(u.versions, e)
	}
	for _, k := range slices.Sorted(maps.Keys(splits)) {
		g := splits[k]
		slices.SortFunc(g, func(a, b *entry) int { return cmp.Compare(a.guess.SplitPart, b.guess.SplitPart) })
		first := g[0]
		for _, e := range g[1:] {
			first.parts = append(first.parts, e.parts...)
		}
		u.versions = append(u.versions, first)
	}
	slices.SortStableFunc(u.versions, func(a, b *entry) int { return strings.Compare(a.parts[0].Rel, b.parts[0].Rel) })
	if len(u.versions) == 0 && len(u.extras) == 0 {
		return nil
	}
	if folder == "" || u.title == "" {
		// For a loose episode, or a folder named only by an ID, the series
		// title comes from the file name.
		first := firstVersion(u)
		if first == nil {
			first = u.extras[0]
		}
		if u.title == "" {
			u.title, u.year = first.guess.Title, first.guess.Year
		}
		if folder == "" {
			u.subject = "|" + parse.Key(u.title) + "|" + strconv.Itoa(u.year)
			if u.title == "" || hashed(first.parts[0].Rel) {
				u.subject, u.title, u.search = "|"+first.parts[0].Rel, stem(first.parts[0].Rel), false
			}
		}
	}
	return []*unit{u}
}

func firstVersion(u *unit) *entry {
	if len(u.versions) == 0 {
		return nil
	}
	return u.versions[0]
}

// splitKey returns the key that split parts share: their folder, season and
// episodes.
func splitKey(rel string, g parse.Guess) string {
	var b strings.Builder
	b.WriteString(dirOf(rel))
	b.WriteString("|" + strconv.Itoa(g.Season))
	for _, n := range g.Episodes {
		b.WriteString("," + strconv.Itoa(n))
	}
	return b.String()
}

// extraSeason returns the season folder an extra sits in, or -1.
func extraSeason(p parse.Parser, rel string) int {
	dirs := strings.Split(rel, "/")
	for i := len(dirs) - 2; i >= 1; i-- {
		if d := p.Dir(dirs[i]); d.IsSeason {
			return d.Season
		}
	}
	return -1
}

func classifyVideos(p parse.Parser, files []file) []*unit {
	var out []*unit
	for _, f := range files {
		if ignored(f.Rel) {
			continue
		}
		g := p.Parse(f.Rel, parse.Videos)
		e := &entry{parts: []file{f}, guess: g, title: cmp.Or(g.Title, stem(f.Rel)), dir: dirOf(f.Rel)}
		out = append(out, &unit{subject: f.Rel, kind: catalog.KindVideo, title: e.title, versions: []*entry{e}})
	}
	return out
}

// folderOf returns the folder a file is identified with. One job gathers
// all of that folder's files. For a series, it is the series' top-level
// folder in the library folder. For a movie, it is the folder the file sits
// in, above any extras and disc folders. For a book, it is the book folder.
// A file directly in the library folder, and any other video, stands alone
// and returns "".
func folderOf(kind catalog.LibraryKind, rel string) string {
	dirs := strings.Split(rel, "/")
	dirs = dirs[:len(dirs)-1]
	if len(dirs) == 0 {
		return ""
	}
	switch kind {
	case catalog.Shows:
		return dirs[0]
	case catalog.Videos:
		return "" // each other video stands alone
	case catalog.Audiobooks:
		return parse.BookFolder(rel)
	}
	return movieDir(dirs)
}

// movieDir returns the folder of a movie whose file sits in dirs. It goes
// above a VIDEO_TS or BDMV folder, and above the nearest extras folder that
// isn't at the top of the library folder.
func movieDir(dirs []string) string {
	for i, d := range dirs {
		if isDiscDir(d) {
			return strings.Join(dirs[:i], "/")
		}
	}
	for i := len(dirs) - 1; i >= 1; i-- {
		if extraFolder(dirs[i]) {
			return strings.Join(dirs[:i], "/")
		}
	}
	return strings.Join(dirs, "/")
}

func isDiscDir(d string) bool {
	return strings.EqualFold(d, "VIDEO_TS") || strings.EqualFold(d, "BDMV")
}

func extraFolder(d string) bool { return (parse.Parser{}).Dir(d).Extra != "" }

var (
	// sampleRe finds "sample" as a word, since release samples aren't
	// catalogued.
	sampleRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])samples?(?:$|[^a-z0-9])`)
	// hashedRe matches obfuscated names, as download tools leave them. Those
	// are 24 or more hex digits, or a long run of letters and digits with no
	// separators.
	hashedRe = regexp.MustCompile(`^(?:[0-9a-fA-F]{24,}|[A-Za-z0-9]{32,})$`)
	// vobRe parses a DVD title set's VOB name. VTS_01_1.VOB is title set 1,
	// part 1.
	vobRe = regexp.MustCompile(`(?i)^VTS_(\d\d)_(\d)\.VOB$`)
)

func ignored(rel string) bool {
	dirs := strings.Split(rel, "/")
	for _, d := range dirs[:len(dirs)-1] {
		if strings.EqualFold(d, "@eaDir") || strings.HasPrefix(d, ".") || strings.EqualFold(d, "sample") || strings.EqualFold(d, "samples") {
			return true
		}
	}
	return sampleRe.MatchString(stem(rel))
}

func hashed(rel string) bool { return hashedRe.MatchString(stem(rel)) }

func stem(rel string) string {
	base := path.Base(rel)
	return strings.TrimSuffix(base, path.Ext(base))
}

func dirOf(rel string) string {
	if d := path.Dir(rel); d != "." {
		return d
	}
	return ""
}

// folderIDs returns the IDs in the names of a folder and its parents,
// nearest first. A series folder's ID names the series.
func folderIDs(p parse.Parser, folder string) []parse.ExternalID {
	dirs := strings.Split(folder, "/")
	var out []parse.ExternalID
	for i := len(dirs) - 1; i >= 0; i-- {
		out = append(out, p.Dir(dirs[i]).IDs...)
	}
	return out
}

// nfoBeside returns the path of the NFO named after a media file.
func nfoBeside(rel string) string { return strings.TrimSuffix(rel, path.Ext(rel)) + ".nfo" }
