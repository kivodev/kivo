package parse

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Parser parses names. Today bounds years and air dates. It is passed in so
// that parsing has no clock of its own. A zero Today accepts years up to
// 2099.
type Parser struct {
	Today Date
}

func (p Parser) maxYear() int {
	if p.Today.IsZero() {
		return 2099
	}
	return p.Today.Year
}

// Parse reads a '/'-separated, library-relative path in a library of the
// given kind. Names are normalized to NFC first, so NFD paths from macOS and
// SMB parse the same.
//
// The title, year, episode, IDs and edition come from the file name. Folders
// add the season and the extra type, but the file name's season wins.
// Reading folder titles and IDs is Dir's job, since which folder names a
// work depends on the tree around it.
func (p Parser) Parse(rel string, kind Kind) Guess {
	rel = norm.NFC.String(rel)
	var dirs []string
	base := rel
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		dirs, base = strings.Split(rel[:i], "/"), rel[i+1:]
	}
	s := matchForm(stripExt(base))
	var g Guess
	if kind == Videos {
		g.Title, g.Rule = cleanTitle(s), "videos"
		return g
	}
	if g.Extra = folderExtra(dirs); g.Extra == "" {
		s, g.Extra = suffixExtra(s)
	}
	if g.Extra == "" && (creditlessRe.MatchString(s) || bareCreditsRe.MatchString(s)) {
		g.Extra = OtherExtra
	}
	s, g.IDs = takeIDs(s)
	s, g.Edition = takeEdition(s)
	s, g.Checksum = takeChecksum(s)
	s = siteRe.ReplaceAllString(s, "")
	s, g.Group = takeGroup(s)
	if kind == Shows {
		p.parseShow(s, dirs, &g)
	} else {
		p.parseMovie(s, &g)
	}
	return g
}

func (p Parser) parseMovie(s string, g *Guess) {
	start, end, part := findPart(s)
	if part != nil && !hasWord(s[end:]) {
		s = s[:start] // in "Movie - cd1", the token ends the name
	}
	title, year, titleEnd := titleYear(s, p.maxYear())
	if part != nil && start < titleEnd {
		part = nil // in "Kill Bill Part 2 (2004)", the token is part of the title
	}
	g.Title, g.Year, g.Part, g.Rule = title, year, part, "title"
}

func (p Parser) parseShow(s string, dirs []string, g *Guess) {
	folderSeason, inSeason := seasonFolder(dirs)
	sc := scan{p: p, s: s, tag: firstTag(s), letter: strings.IndexFunc(s, isLetter), inSeason: inSeason}
	g.Season = -1
	g.Rule = "title"
	area := s
rules:
	for _, r := range episodeRules {
		for _, m := range r.re.FindAllStringSubmatchIndex(s, -1) {
			if r.before != nil && !r.before(s, m[0]) || !r.extract(&sc, m, g) {
				continue
			}
			g.Rule, area = r.name, s[:m[0]]
			if r.titleSeason {
				area = p.titleSeason(area, g)
			}
			break rules
		}
	}
	g.Title, g.Year, _ = titleYear(area, p.maxYear())
	if g.Season < 0 {
		g.Season = 1
		if inSeason {
			g.Season = folderSeason
		}
	}
}

// titleSeason reads a season marker at the end of the title area into g,
// and returns the area without it. A season turns absolute numbers into
// plain episode numbers, so "[Group] Show S2 - 05" is episode 5 of
// season 2.
func (p Parser) titleSeason(area string, g *Guess) string {
	m := trailingSeason.FindStringSubmatchIndex(area)
	if m == nil || !hasWord(area[:m[0]]) {
		return area
	}
	for i := 2; i < len(m); i += 2 {
		if m[i] >= 0 {
			g.Season = atoi(area[m[i]:m[i+1]])
		}
	}
	if g.Absolute != nil {
		g.Episodes, g.Absolute = g.Absolute, nil
	}
	return area[:m[0]]
}

// idRe finds provider IDs in the {tmdb-123} and [tmdbid=123] forms.
var idRe = regexp.MustCompile(`(?i)[\[{(]\s*(tmdb|tvdb|imdb|anidb)(?:id)?\s*[-=]\s*(tt\d{1,10}|\d{1,10})\s*[\]})]`)

func takeIDs(s string) (string, []ExternalID) {
	ms := idRe.FindAllStringSubmatchIndex(s, -1)
	if ms == nil {
		return s, nil
	}
	var ids []ExternalID
	var b strings.Builder
	last := 0
	for _, m := range ms {
		ids = append(ids, ExternalID{strings.ToLower(s[m[2]:m[3]]), strings.ToLower(s[m[4]:m[5]])})
		b.WriteString(s[last:m[0]])
		b.WriteByte(' ')
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String(), ids
}

// editionRe finds an edition tag, such as {edition-Director's Cut}.
var editionRe = regexp.MustCompile(`(?i)\{edition-([^{}]{1,100})\}`)

func takeEdition(s string) (string, string) {
	m := editionRe.FindStringSubmatchIndex(s)
	if m == nil {
		return s, ""
	}
	return blank(s, m[0], m[1]), strings.TrimSpace(s[m[2]:m[3]])
}

// checksumRe finds an anime release's bracketed 8-hex-digit CRC32.
var checksumRe = regexp.MustCompile(`[\[(]([0-9A-Fa-f]{8})[\])]`)

func takeChecksum(s string) (string, string) {
	ms := checksumRe.FindAllStringSubmatchIndex(s, -1)
	if ms == nil {
		return s, ""
	}
	m := ms[len(ms)-1]
	return blank(s, m[0], m[1]), strings.ToUpper(s[m[2]:m[3]])
}

// siteRe finds a leading download-site name, such as "www.example.org - ".
var siteRe = regexp.MustCompile(`(?i)^\s*ww?w[.,][^ ]+\s+-\s+`)

// groupKeepRe matches leading bracket contents that are part of the name
// rather than a release group: an episode, a decimal or a year.
var groupKeepRe = regexp.MustCompile(`(?i)\d+x\d+|s\d+e\d+|^\d+\.\d+$|(?:^|[^0-9])(?:19|20)\d\d(?:$|[^0-9])`)

// siteWordRe matches bracket contents naming a download site.
var siteWordRe = regexp.MustCompile(`(?i)www|\.(?:com|org|net|to|cd|me|tv|info|nu|pics|world|party|sbs|my)\b`)

// takeGroup reads an anime release's leading [Group]. Brackets holding
// release tags or a site name are dropped without naming a group. Brackets
// holding the title or episode, or followed by no title, are kept.
func takeGroup(s string) (string, string) {
	if !strings.HasPrefix(s, "[") {
		return s, ""
	}
	j := strings.IndexByte(s, ']')
	if j < 0 {
		return s, ""
	}
	content, rest := strings.Trim(s[1:j], "[ "), s[j+1:]
	switch {
	case !hasLetter(rest) || groupKeepRe.MatchString(content):
		return s, ""
	case siteWordRe.MatchString(content) || startsWithTag(content):
		return rest, ""
	}
	return rest, content
}

// partRe finds a multi-file part token: cd, dvd, part, pt, disc or disk
// with a number or a to d.
var partRe = regexp.MustCompile(`(?i)(?:^|[ ._-])((?:cd|dvd|part|pt|disc|disk)[ ._-]?(\d{1,2}|[a-d]))`)

// findPart returns the last part token in s and where it starts and ends.
func findPart(s string) (start, end int, part *Part) {
	for _, m := range partRe.FindAllStringSubmatchIndex(s, -1) {
		if wordAt(s, m[3]) {
			continue
		}
		n := s[m[4]:m[5]]
		i := atoi(n)
		if c := lower(n[0]); c >= 'a' && c <= 'd' {
			i = int(c-'a') + 1
		}
		start, end, part = m[2], m[3], &Part{Index: i, Token: s[m[2]:m[3]]}
	}
	return start, end, part
}

// extraSuffixes lists the extras suffixes, as in "Movie (1999)-trailer.mkv".
var extraSuffixes = []struct {
	suffix string
	t      ExtraType
}{
	{"-trailer", Trailer}, {"-featurette", Featurette}, {"-behindthescenes", BehindTheScenes},
	{"-deleted", DeletedScene}, {"-interview", Interview}, {"-scene", Scene}, {"-short", Short},
	{"-other", OtherExtra},
}

func suffixExtra(s string) (string, ExtraType) {
	for _, x := range extraSuffixes {
		if len(s) > len(x.suffix) && strings.EqualFold(s[len(s)-len(x.suffix):], x.suffix) {
			return s[:len(s)-len(x.suffix)], x.t
		}
	}
	return s, ""
}

// creditlessRe finds an anime release's creditless opening or ending, such
// as NCOP1, NCED, OP4a, ED2v2, "Clean OP" or "Creditless Ending". A bare OP
// or ED needs a number, so "Ed" in a title doesn't match.
var creditlessRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:` +
	`nc(?:op|ed)\d*[a-z]?|(?:op|ed)\d+[a-z]?|(?:clean|creditless)[ _.-]+(?:op|ed|opening|ending)` +
	`)(?:v\d+)?(?:[^a-z0-9]|$)`)

// bareCreditsRe finds a series' opening or ending in a file of its own. The
// name ends with a capital OP or ED after the title, followed only by tags,
// as in "Serial Experiments Lain OP [v2]" or "Show - ED". It matches
// capitals only, so "Mister Ed" doesn't match.
var bareCreditsRe = regexp.MustCompile(`[A-Za-z0-9].*[^A-Za-z0-9](?:OP|ED)(?:v\d+)?(?:[ _.-]*(?:\[[^\]]*\]|\([^)]*\)))*[ _.-]*$`)

// extraFolders maps extras folder names to their extra types.
var extraFolders = map[string]ExtraType{
	"trailers": Trailer, "featurettes": Featurette, "behind the scenes": BehindTheScenes,
	"deleted scenes": DeletedScene, "interviews": Interview, "scenes": Scene, "shorts": Short,
	"other": OtherExtra, "extras": OtherExtra,
}

// folderExtra returns the extra type of the nearest extras folder in dirs.
// It skips the first folder, because an extras folder at the top of a
// library folder is a root work.
func folderExtra(dirs []string) ExtraType {
	for i := len(dirs) - 1; i >= 1; i-- {
		if t := extraFolders[strings.ToLower(strings.TrimSpace(dirs[i]))]; t != "" {
			return t
		}
	}
	return ""
}

// seasonFolder returns the season of the nearest season folder in dirs.
func seasonFolder(dirs []string) (int, bool) {
	for i := len(dirs) - 1; i >= 0; i-- {
		if n, ok := seasonDir(dirs[i]); ok {
			return n, true
		}
	}
	return 0, false
}
