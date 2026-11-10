package parse

import (
	"reflect"
	"slices"
	"testing"
	"unicode/utf8"
)

// FuzzParse checks that parsing is total and deterministic and keeps its
// invariants. It never panics. Episode lists are strictly increasing, so
// every range ends at or after its start. A shows library always gets a
// season.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"Show/Season 01/Show S01E01-E03.mkv",
		"Show/Show S09E14-1080p.mkv",
		"Show/Show 1920x1080 2x05-07.mkv",
		"Show/Show 2024-25-03.mkv",
		"Anime/[Grp] Show S2 - 01v2 ~ 12.5 [ABCD1234].mkv",
		"Dune (2021) {tmdb-438631}/Dune (2021) {edition-Cut} - cd1.mkv",
		"300 2001 (2006).mkv",
		"[[(((x)]] - 9999-99-99 S99E9999-E1.mkv",
		"Ｆｒｉｅｒｅｎ　－　１２.mkv",
	} {
		f.Add(s, uint8(0))
		f.Add(s, uint8(1))
	}
	kinds := []Kind{Movies, Shows, Videos}
	p := Parser{Today: today}
	f.Fuzz(func(t *testing.T, rel string, k uint8) {
		kind := kinds[int(k)%len(kinds)]
		g := p.Parse(rel, kind)
		if !reflect.DeepEqual(g, p.Parse(rel, kind)) {
			t.Fatalf("Parse(%q) isn't deterministic", rel)
		}
		for _, eps := range [][]int{g.Episodes, g.Absolute} {
			for i := 1; i < len(eps); i++ {
				if eps[i] <= eps[i-1] {
					t.Fatalf("Parse(%q): episodes %v not increasing", rel, eps)
				}
			}
			if slices.ContainsFunc(eps, func(n int) bool { return n < 0 }) {
				t.Fatalf("Parse(%q): negative episode in %v", rel, eps)
			}
		}
		if kind == Shows && g.Season < 0 {
			t.Fatalf("Parse(%q): season %d", rel, g.Season)
		}
		if utf8.ValidString(rel) && !utf8.ValidString(g.Title) {
			t.Fatalf("Parse(%q): title %q isn't UTF-8", rel, g.Title)
		}
		p.Dir(rel)
		Key(rel)
	})
}

// BenchmarkParse parses the vendored fixture names. ns/op is per path.
func BenchmarkParse(b *testing.B) {
	var names []string
	for _, src := range []string{"ptt", "thcolin", "go-ptn", "ptn"} {
		for _, x := range loadGuessit(b, src) {
			names = append(names, x.name)
		}
	}
	for _, x := range loadAnitomy(b) {
		names = append(names, x.name)
	}
	p := Parser{Today: today}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		p.Parse(names[i%len(names)], Shows)
	}
}
