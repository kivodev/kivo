package identify

import (
	"testing"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/metadata"
)

func match(kind, id, title string, year int, alts ...string) metadata.Match {
	return metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: kind, ID: id}, Title: title, Year: year, AltTitles: alts}
}

func office(id, country string, year int) metadata.Match {
	return metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: id}, Title: "The Office", Year: year, Countries: []string{country}}
}

// Matches are in the provider's order, most popular first.
func TestAccept(t *testing.T) {
	thing2011 := match("movie", "60935", "The Thing", 2011)
	thing1982 := match("movie", "1091", "The Thing", 1982)
	for _, tc := range []struct {
		name    string
		want    want
		matches []metadata.Match
		id      string // "" for nothing accepted
		score   int
	}{
		{"exact", want{"movie", "Heat", 1995, ""}, []metadata.Match{match("movie", "949", "Heat", 1995)}, "949", 100},
		{"remake by year", want{"movie", "The Thing", 1982, ""}, []metadata.Match{thing2011, thing1982}, "1091", 100},
		{"remake, no year: popularity", want{"movie", "The Thing", 0, ""}, []metadata.Match{thing2011, thing1982}, "60935", 100},
		{"near-tie goes to the closer year", want{"movie", "The Thing", 2010, ""},
			[]metadata.Match{thing1982, match("movie", "x", "The Thing!", 2012), thing2011}, "60935", 97},
		{"year off by one", want{"movie", "Dune", 2020, ""}, []metadata.Match{match("movie", "438631", "Dune", 2021), match("movie", "841", "Dune", 1984)}, "438631", 97},
		{"other release year counts", want{"movie", "Dune", 2020, ""},
			[]metadata.Match{{ID: catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "438631"}, Title: "Dune", Year: 2021, OtherYears: []int{2020}}}, "438631", 100},
		{"two years off", want{"movie", "Dune", 2019, ""}, []metadata.Match{match("movie", "438631", "Dune", 2021)}, "438631", 94},
		{"three years off", want{"movie", "Dune", 2018, ""}, []metadata.Match{match("movie", "438631", "Dune", 2021)}, "", 0},
		{"far year", want{"movie", "Dune", 1990, ""}, []metadata.Match{match("movie", "438631", "Dune", 2021)}, "", 0},
		{"far year, no year wanted", want{"movie", "Dune", 0, ""}, []metadata.Match{match("movie", "841", "Dune", 1984)}, "841", 100},
		{"far year, other release year", want{"movie", "Hamilton", 2020, ""},
			[]metadata.Match{{ID: catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "556574"}, Title: "Hamilton", Year: 2025, OtherYears: []int{2020, 2021}}}, "556574", 100},
		{"within a year beats a better title further off", want{"movie", "The Thing", 2010, ""},
			[]metadata.Match{match("movie", "x", "The Thing", 2012), match("movie", "y", "The Thingy", 2011)}, "y", 87},
		{"two years off, nothing closer", want{"movie", "The Thing", 2010, ""},
			[]metadata.Match{match("movie", "x", "The Thing", 2012), match("movie", "y", "Something", 2010)}, "x", 94},
		{"within a year only counts when plausible", want{"movie", "The Thing", 2010, ""},
			[]metadata.Match{match("movie", "x", "The Thing", 2012), match("movie", "y", "The Things We Do", 2010)}, "x", 94},
		{"different sequel number", want{"movie", "Toy Story 3", 2010, ""},
			[]metadata.Match{match("movie", "863", "Toy Story 2", 2010), match("movie", "10193", "Toy Story 3", 2010)}, "10193", 100},
		{"different sequel number, nothing else", want{"movie", "Toy Story 3", 2010, ""}, []metadata.Match{match("movie", "863", "Toy Story 2", 2010)}, "", 0},
		{"sequel by alias", want{"movie", "Rocky 2", 1979, ""}, []metadata.Match{match("movie", "1367", "Rocky II", 1979)}, "1367", 100},
		{"sequel number in one alias only", want{"movie", "Dragon 3", 2019, ""},
			[]metadata.Match{match("movie", "1", "Dragon: The Hidden World", 2019, "Dragon 2 3D", "Dragon 3")}, "1", 100},
		{"premiere year", want{"series", "Doctor Who", 2005, ""},
			[]metadata.Match{match("series", "121", "Doctor Who", 1963), match("series", "57243", "Doctor Who", 2005)}, "57243", 100},
		{"no year anywhere", want{"series", "Firefly", 0, ""}, []metadata.Match{match("series", "1437", "Firefly", 0)}, "1437", 100},
		{"undated namesake loses to a year off", want{"movie", "Badlands", 1974, ""},
			[]metadata.Match{match("movie", "1480504", "Badlands", 0), match("movie", "3133", "Badlands", 1973)}, "3133", 97},
		{"undated, year wanted", want{"movie", "Hey Jude", 2021, ""}, []metadata.Match{match("movie", "1691053", "Hey Jude", 0)}, "", 0},
		{"kind mismatch", want{"series", "Dune", 2021, ""}, []metadata.Match{match("movie", "438631", "Dune", 2021)}, "", 0},
		{"below threshold", want{"movie", "Dune Messiah", 0, ""}, []metadata.Match{match("movie", "438631", "Dune", 2021)}, "", 0},
		{"alias", want{"movie", "Dune Part One", 2021, ""}, []metadata.Match{match("movie", "438631", "Dune", 2021, "Dune: Part One")}, "438631", 100},
		{"alias collision: closer year wins", want{"movie", "Solaris", 1972, ""},
			[]metadata.Match{match("movie", "2103", "Solaris", 2002), match("movie", "593", "Solyaris", 1972, "Solaris")}, "593", 100},
		{"alias collision, no year: popularity", want{"movie", "Solaris", 0, ""},
			[]metadata.Match{match("movie", "2103", "Solaris", 2002), match("movie", "593", "Solyaris", 1972, "Solaris")}, "2103", 100},
		{"original title", want{"movie", "Le fabuleux destin d'Amelie Poulain", 2001, ""},
			[]metadata.Match{{ID: catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "194"}, Title: "Amélie", OriginalTitle: "Le Fabuleux Destin d'Amélie Poulain", Year: 2001}}, "194", 100},
		{"words reordered", want{"series", "Undead Unwanted Adventurer", 0, ""},
			[]metadata.Match{match("series", "197848", "The Unwanted Undead Adventurer", 2024)}, "197848", 86},
		{"same words, different title", want{"series", "Love Death Robots", 0, ""},
			[]metadata.Match{match("series", "x", "Robots", 2019)}, "", 0},
		{"country breaks a tie", want{"series", "The Office", 0, "US"},
			[]metadata.Match{office("2996", "GB", 2001), office("2316", "US", 2005)}, "2316", 100},
		{"country, no other result", want{"series", "Cash Cab", 0, "US"},
			[]metadata.Match{{ID: catalog.ExternalID{Provider: "tmdb", Kind: "series", ID: "7"}, Title: "Cash Cab", Year: 2005}}, "7", 100},
		{"UK is GB", want{"series", "The Office", 0, "UK"},
			[]metadata.Match{office("2316", "US", 2005), office("2996", "GB", 2001)}, "2996", 100},
		{"no country: popularity", want{"series", "The Office", 0, ""},
			[]metadata.Match{office("2996", "GB", 2001), office("2316", "US", 2005)}, "2996", 100},
		{"nothing", want{"movie", "Heat", 1995, ""}, nil, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, sc, ok := accept(tc.want, tc.matches)
			switch {
			case tc.id == "" && ok:
				t.Fatalf("accepted %+v (%d), want nothing", c, sc)
			case tc.id != "" && (!ok || c.ID.ID != tc.id || sc != tc.score):
				t.Fatalf("got %s at %d (accepted %v), want %s at %d", c.ID.ID, sc, ok, tc.id, tc.score)
			}
		})
	}
}

// movieIn is a movie with its other release years.
func movieIn(id, title string, year int, other []int, alts ...string) metadata.Match {
	m := match("movie", id, title, year, alts...)
	m.OtherYears = other
	return m
}

// Folders a library once matched wrongly, with the matches the provider
// offers once looked up, in its order.
func TestAcceptLibraryCases(t *testing.T) {
	for _, tc := range []struct {
		want    want
		matches []metadata.Match
		id      string // "" for nothing accepted
		score   int
	}{
		{want{"movie", "101 Dalmations", 1961, ""}, []metadata.Match{
			movieIn("10481", "102 Dalmatians", 2000, []int{2001, 2002}, "102 Dalmations"),
			movieIn("11674", "101 Dalmatians", 1996, []int{1997}),
			movieIn("12230", "One Hundred and One Dalmatians", 1961, []int{1962, 1969}, "101 Dalmatians"),
		}, "12230", 92},
		{want{"movie", "Hamilton", 2020, ""}, []metadata.Match{
			movieIn("556574", "Hamilton", 2025, []int{2020, 2021, 2022, 2023, 2026}, "Hamilton: An American Musical"),
			movieIn("17629", "Hamilton", 1998, []int{2001}),
		}, "556574", 100},
		{want{"movie", "Lilo and Stitch 2", 2005, ""}, []metadata.Match{
			movieIn("1504349", "Lilo & Stitch 2", 2028, nil, "Lilo and Stitch 2"),
			movieIn("20760", "Lilo & Stitch 2: Stitch Has a Glitch", 2005, []int{2009, 2021}, "Lilo & Stitch 2", "Lilo and Stitch 2"),
		}, "20760", 100},
		{want{"movie", "How to Train Your Dragon 3", 2019, ""}, []metadata.Match{
			movieIn("82702", "How to Train Your Dragon 2", 2014, []int{2015, 2019}, "How to Train Your Dragon 2 3D"),
			movieIn("166428", "How to Train Your Dragon: The Hidden World", 2019, []int{2021}, "Dragons 3", "How to Train Your Dragon 3"),
			movieIn("10191", "How to Train Your Dragon", 2010, []int{2011, 2020}),
		}, "166428", 100},
		{want{"movie", "The Lion King II", 1998, ""}, []metadata.Match{
			movieIn("9732", "The Lion King II: Simba's Pride", 1998, []int{1999, 2000}, "Simba's Pride", "The Lion King 2: Simba's Pride"),
			movieIn("11430", "The Lion King 1½", 2004, []int{2011, 2012}, "The Lion King III", "The Lion King 3"),
		}, "", 0},
		{want{"movie", "Above and Beyond", 1996, ""}, []metadata.Match{ // from "Michael Jordan, Above and Beyond"
			movieIn("1034474", "Above and Beyond", 2014, nil),
		}, "", 0},
		{want{"movie", "Stalingrad", 2003, ""}, []metadata.Match{
			movieIn("11101", "Stalingrad", 1993, []int{1994, 1995, 1997}, "Stalingrado - A Batalha Final"),
			movieIn("289484", "Stalingrad", 1943, []int{1945}),
			movieIn("326297", "Stalingrad", 1963, nil),
		}, "", 0},
		{want{"movie", "The Great Father", 2021, ""}, []metadata.Match{
			movieIn("428658", "The Great Father", 2017, nil, "Great Father"),
			movieIn("1108163", "Great Are the Eyes of a Dead Father", 2023, nil),
		}, "", 0},
		{want{"movie", "Galatta kalyanam", 2021, ""}, []metadata.Match{
			movieIn("261694", "Galatta Kalyanam", 1968, nil),
			movieIn("618911", "Atrangi Re", 2021, nil, "Galatta Kalyanam", "Atarangi Re"),
		}, "618911", 100},
		{want{"movie", "Kuberan", 2021, ""}, []metadata.Match{
			movieIn("78469", "Kuberan", 2000, nil),
			movieIn("351421", "Kuberan", 2002, nil),
		}, "", 0},
		{want{"movie", "Uyarndha Manithan", 2020, ""}, []metadata.Match{
			movieIn("261701", "Uyarndha Manidhan", 1968, nil, "Uyarndha Manithan"),
		}, "", 0},
		{want{"movie", "Ragalai", 2021, ""}, []metadata.Match{
			movieIn("1445611", "Sema Ragalai", 2004, nil),
			movieIn("120956", "Rachcha", 2012, nil, "Ragalai"),
		}, "", 0},
	} {
		t.Run(tc.want.title, func(t *testing.T) {
			c, sc, ok := accept(tc.want, tc.matches)
			switch {
			case tc.id == "" && ok:
				t.Fatalf("accepted %+v (%d), want nothing", c, sc)
			case tc.id != "" && (!ok || c.ID.ID != tc.id || sc != tc.score):
				t.Fatalf("got %s at %d (accepted %v), want %s at %d", c.ID.ID, sc, ok, tc.id, tc.score)
			}
		})
	}
}

func TestSimilarity(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"heat", "heat", 100},
		{"", "", 0},
		{"kill bill vol 1", "kill bill vol 1", 100},
		{"dune", "dunes", 80},
		{"matrix", "matrx", 83},
	} {
		if got := similarity(tc.a, tc.b); got != tc.want {
			t.Errorf("similarity(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
