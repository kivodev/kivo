package parse

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// seasonDirRe matches season folder names, such as "Season 02",
// "Series 2", "Staffel 1" or "S03".
var seasonDirRe = regexp.MustCompile(`(?i)^(?:(?:season|series|saison|staffel|temporada)[ ._-]*(\d{1,4})(?:$|[ ._-])|s(\d{1,4})$)`)

// specialsDirRe matches the specials folder, which is season 0.
var specialsDirRe = regexp.MustCompile(`(?i)^specials?$`)

// seasonDir reads a season folder name.
func seasonDir(name string) (int, bool) {
	name = strings.TrimSpace(name)
	if specialsDirRe.MatchString(name) {
		return 0, true
	}
	m := seasonDirRe.FindStringSubmatchIndex(name)
	if m == nil {
		return 0, false
	}
	for i := 2; i < len(m); i += 2 {
		if m[i] >= 0 {
			n := atoi(name[m[i]:m[i+1]])
			return n, validSeason(n)
		}
	}
	return 0, false
}

// Dir reads a folder name, such as "Dune (2021) {tmdb-438631}",
// "Season 02", "Specials" or "Trailers". A season folder has no title.
func (p Parser) Dir(name string) DirGuess {
	name = norm.NFC.String(name)
	var d DirGuess
	d.Extra = extraFolders[strings.ToLower(strings.TrimSpace(name))]
	if d.Season, d.IsSeason = seasonDir(name); d.IsSeason {
		return d
	}
	s := matchForm(name)
	s, d.IDs = takeIDs(s)
	s, d.Edition = takeEdition(s)
	d.Title, d.Year, _ = titleYear(s, p.maxYear())
	d.Title, d.Country = takeCountry(d.Title)
	return d
}

// countryRe matches a trailing two-letter country code in parentheses. It
// must be in capitals, so a title ending in "(It)" doesn't match.
var countryRe = regexp.MustCompile(`^(.+?)\s*\(([A-Z]{2})\)$`)

// takeCountry splits a trailing country code off a title.
func takeCountry(t string) (string, string) {
	if m := countryRe.FindStringSubmatch(t); m != nil {
		return m[1], m[2]
	}
	return t, ""
}
