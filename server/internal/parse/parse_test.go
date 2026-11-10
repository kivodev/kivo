package parse

import (
	"reflect"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// today is the date the table tests parse on.
var today = Date{2026, 1, 1}

type parseCase struct {
	rel  string
	kind Kind
	want Guess // Rule is checked only when set
}

func runParse(t *testing.T, cases []parseCase) {
	t.Helper()
	p := Parser{Today: today}
	for _, c := range cases {
		got := p.Parse(c.rel, c.kind)
		if c.want.Rule == "" {
			got.Rule = ""
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q, %s)\n got %+v\nwant %+v", c.rel, c.kind, got, c.want)
		}
	}
}

// Patterns are tried most specific first. Bare numbers count only when
// nothing stronger matches.
func TestOrder(t *testing.T) {
	runParse(t, []parseCase{
		{"Show/Show 12 S01E05.mkv", Shows, Guess{Title: "Show 12", Season: 1, Episodes: []int{5}, Rule: "sxxeyy"}},
		{"Show/Show 12 2x05.mkv", Shows, Guess{Title: "Show 12", Season: 2, Episodes: []int{5}, Rule: "nxnn"}},
		{"Show/Show 2024-03-07 Episode 5.mkv", Shows, Guess{Title: "Show", Season: 1, AirDate: Date{2024, 3, 7}, Rule: "date"}},
		{"Show/Show 2x05 Episode 7.mkv", Shows, Guess{Title: "Show", Season: 2, Episodes: []int{5}, Rule: "nxnn"}},
		{"Show/Show Ep 7 [08].mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{7}, Rule: "episode"}},
		{"Show/Show - 08 [720p].mkv", Shows, Guess{Title: "Show", Season: 1, Absolute: []int{8}, Rule: "absolute"}},
		{"Show/Show 08 [720p].mkv", Shows, Guess{Title: "Show", Season: 1, Absolute: []int{8}, Rule: "bare"}},
		{"Show/Show.S01.1080p.mkv", Shows, Guess{Title: "Show", Season: 1, Rule: "season"}},
		{"Show/Show.mkv", Shows, Guess{Title: "Show", Season: 1, Rule: "title"}},
	})
}

// A year is from 1900 to the current year. Of two years, the bracketed or
// the last one wins. Numeric titles stay titles.
func TestYear(t *testing.T) {
	runParse(t, []parseCase{
		{"Dune (2021)/Dune (2021).mkv", Movies, Guess{Title: "Dune", Year: 2021}},
		{"Dune.2021.2160p.WEB-DL.x265-GRP.mkv", Movies, Guess{Title: "Dune", Year: 2021}},
		{"2001 A Space Odyssey (1968).mkv", Movies, Guess{Title: "2001 A Space Odyssey", Year: 1968}},
		{"300 2001 (2006).mkv", Movies, Guess{Title: "300 2001", Year: 2006}},
		{"Blade.Runner.2049.2017.1080p.mkv", Movies, Guess{Title: "Blade Runner 2049", Year: 2017}},
		{"Wonder.Woman.1984.2020.1080p.mkv", Movies, Guess{Title: "Wonder Woman 1984", Year: 2020}},
		{"1917 (2019).mkv", Movies, Guess{Title: "1917", Year: 2019}},
		{"2012.2009.1080p.mkv", Movies, Guess{Title: "2012", Year: 2009}},
		{"1923.mkv", Movies, Guess{Title: "1923"}},
		{"24.mkv", Movies, Guess{Title: "24"}},
		{"Movie 1899.mkv", Movies, Guess{Title: "Movie 1899"}},
		{"Movie 2027.mkv", Movies, Guess{Title: "Movie 2027"}},
		{"(2000) Le Film.mkv", Movies, Guess{Title: "Le Film", Year: 2000}},
		{"1923/1923 S02E01.mkv", Shows, Guess{Title: "1923", Season: 2, Episodes: []int{1}}},
		{"24/24 - S01E04.mkv", Shows, Guess{Title: "24", Season: 1, Episodes: []int{4}}},
		{"The Flash/The Flash 2014 S01E04.mkv", Shows, Guess{Title: "The Flash", Year: 2014, Season: 1, Episodes: []int{4}}},
	})
}

// Episodes are tried as S01E02, then a date, then 1x02, then Episode 2, then
// absolute, then a bare number. Resolutions aren't episodes, and a range's
// end is checked.
func TestEpisode(t *testing.T) {
	runParse(t, []parseCase{
		{"Show/Show S01E02.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{2}, Rule: "sxxeyy"}},
		{"Show/show.s01e02.720p.mkv", Shows, Guess{Title: "show", Season: 1, Episodes: []int{2}}},
		{"Show/Show - S01 - E02.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{2}}},
		{"Show/Show 1x02.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{2}, Rule: "nxnn"}},
		{"Show/Show Episode 2.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{2}, Rule: "episode"}},
		{"Show/Show.Ep.02.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{2}, Rule: "episode"}},
		{"Show/Show - 02.mkv", Shows, Guess{Title: "Show", Season: 1, Absolute: []int{2}, Rule: "absolute"}},
		{"Show/Show 02.mkv", Shows, Guess{Title: "Show", Season: 1, Absolute: []int{2}, Rule: "bare"}},
		{"Show/Show 1920x1080.mkv", Shows, Guess{Title: "Show", Season: 1}},
		{"Show/Show 1280x720 2x05.mkv", Shows, Guess{Title: "Show", Season: 2, Episodes: []int{5}}},
		{"Show/Show S09E14-1080p.mkv", Shows, Guess{Title: "Show", Season: 9, Episodes: []int{14}}},
		{"Show/Show S01E05-03.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{5}}},
		{"Show/Show S2025E86.mkv", Shows, Guess{Title: "Show", Season: 2025, Episodes: []int{86}}},
		{"Show/Show S1500E01.mkv", Shows, Guess{Title: "Show S1500E01", Season: 1}},
		{"Show/Show S1999E01.mkv", Shows, Guess{Title: "Show", Season: 1999, Episodes: []int{1}}},
		{"Show/Show 1x02-03.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{2, 3}}},
		{"Show/Naruto - 116 - 360 Degrees.mkv", Shows, Guess{Title: "Naruto", Season: 1, Absolute: []int{116}}},
	})
}

// The season comes from the file name, else the season folder, else it is
// 1. Specials and Season 00 are season 0.
func TestSeason(t *testing.T) {
	runParse(t, []parseCase{
		{"Show/Season 03/Show S02E05.mkv", Shows, Guess{Title: "Show", Season: 2, Episodes: []int{5}}},
		{"Show/Season 03/Show - Episode 5.mkv", Shows, Guess{Title: "Show", Season: 3, Episodes: []int{5}}},
		{"Show/Season 03/05 - Title.mkv", Shows, Guess{Season: 3, Episodes: []int{5}, Rule: "leading"}},
		{"Show/Show - Episode 5.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{5}}},
		{"Show/Specials/Show - Episode 2.mkv", Shows, Guess{Title: "Show", Season: 0, Episodes: []int{2}}},
		{"Show/Season 00/Show - Episode 2.mkv", Shows, Guess{Title: "Show", Season: 0, Episodes: []int{2}}},
		{"Show/Show S00E02.mkv", Shows, Guess{Title: "Show", Season: 0, Episodes: []int{2}}},
		{"Show/Series 2/Show Episode 2.mkv", Shows, Guess{Title: "Show", Season: 2, Episodes: []int{2}}},
		{"Anime/[Grp] Show S2 - 05 [ABCD1234].mkv", Shows, Guess{Title: "Show", Season: 2, Episodes: []int{5}, Group: "Grp", Checksum: "ABCD1234"}},
		{"Anime/[Grp] Show 2nd Season - 05.mkv", Shows, Guess{Title: "Show", Season: 2, Episodes: []int{5}, Group: "Grp"}},
		{"Anime/Show 2nd Season 24 (BD 1080p).mkv", Shows, Guess{Title: "Show", Season: 2, Episodes: []int{24}}},
	})
}

// S01E01-E03 is a range of episodes, and S01E01E03 is a list.
func TestMultiEpisode(t *testing.T) {
	runParse(t, []parseCase{
		{"Show/Show S01E01-E03.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{1, 2, 3}}},
		{"Show/Show S01E01-03.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{1, 2, 3}}},
		{"Show/Show S01E01E03.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{1, 3}}},
		{"Show/Show S01E25+E26.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{25, 26}}},
		{"Show/Show S01E01-E02-E03.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{1, 2, 3}}},
		{"Show/Show - 01 ~ 03 [1080p].mkv", Shows, Guess{Title: "Show", Season: 1, Absolute: []int{1, 2, 3}}},
		{"Show/Show S01 E15 - 459 (1080p).mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{15}}},
	})
}

// S01E05a and "S01E05 - part1" are parts of one split episode.
func TestSplitEpisode(t *testing.T) {
	runParse(t, []parseCase{
		{"Show/Show S01E05a.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{5}, SplitPart: "a"}},
		{"Show/Show S01E05b - Title.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{5}, SplitPart: "b"}},
		{"Show/Show S01E05 - part1.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{5}, SplitPart: "1"}},
		{"Show/Show.S01E05.pt2.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{5}, SplitPart: "2"}},
		{"Show/Show S01E05 Abc.mkv", Shows, Guess{Title: "Show", Season: 1, Episodes: []int{5}}},
	})
}

// A date episode is YYYY-MM-DD or DD-MM-YYYY. A month above 12 swaps day and
// month, and dates more than 14 days ahead are rejected.
func TestDateEpisode(t *testing.T) {
	runParse(t, []parseCase{
		{"Show/Show 2024-03-07.mkv", Shows, Guess{Title: "Show", Season: 1, AirDate: Date{2024, 3, 7}, Rule: "date"}},
		{"Show/Show.2024.03.07.720p.mkv", Shows, Guess{Title: "Show", Season: 1, AirDate: Date{2024, 3, 7}}},
		{"Show/Show 07-03-2024.mkv", Shows, Guess{Title: "Show", Season: 1, AirDate: Date{2024, 3, 7}, Rule: "date-dmy"}},
		{"Show/Show 2024-25-03.mkv", Shows, Guess{Title: "Show", Season: 1, AirDate: Date{2024, 3, 25}}},
		{"Show/Show 03-25-2024.mkv", Shows, Guess{Title: "Show", Season: 1, AirDate: Date{2024, 3, 25}}},
		{"Show/Show 2026-01-15.mkv", Shows, Guess{Title: "Show", Season: 1, AirDate: Date{2026, 1, 15}}},
		{"Show/Show 2026-01-16.mkv", Shows, Guess{Title: "Show", Year: 2026, Season: 1}},
		{"Show/Show 2024-02-30.mkv", Shows, Guess{Title: "Show", Year: 2024, Season: 1}},
		{"Show/Show 2024-03.07.mkv", Shows, Guess{Title: "Show", Year: 2024, Season: 1}},
	})
}

// In anime names, the 8-hex-digit token is a checksum, v2 is a revision,
// 12.5 is a special, and the number is absolute.
func TestAnime(t *testing.T) {
	runParse(t, []parseCase{
		{"Frieren/[SubsPlease] Sousou no Frieren - 12 (1080p) [ABCD1234].mkv", Shows,
			Guess{Title: "Sousou no Frieren", Season: 1, Absolute: []int{12}, Group: "SubsPlease", Checksum: "ABCD1234"}},
		{"Frieren/[SubsPlease] Sousou no Frieren - 12v2 [1080p][abcd1234].mkv", Shows,
			Guess{Title: "Sousou no Frieren", Season: 1, Absolute: []int{12}, Group: "SubsPlease", Checksum: "ABCD1234", Revision: 2}},
		{"Frieren/[SubsPlease] Sousou no Frieren - 12.5 [1080p].mkv", Shows,
			Guess{Title: "Sousou no Frieren", Season: 1, Absolute: []int{12}, Special: true, Group: "SubsPlease"}},
		{"One Piece/[Grp]_One_Piece_-_1111_[1080p][2E05E658].mkv", Shows,
			Guess{Title: "One Piece", Season: 1, Absolute: []int{1111}, Group: "Grp", Checksum: "2E05E658"}},
		{"Toradora/[TaigaSubs]_Toradora!_(2008)_-_01v2_-_Tiger_and_Dragon_[1280x720_H.264_FLAC][1234ABCD].mkv", Shows,
			Guess{Title: "Toradora!", Year: 2008, Season: 1, Absolute: []int{1}, Group: "TaigaSubs", Checksum: "1234ABCD", Revision: 2}},
		{"Bleach/[DB]_Bleach_264_[012073FE].avi", Shows,
			Guess{Title: "Bleach", Season: 1, Absolute: []int{264}, Group: "DB", Checksum: "012073FE"}},
		{"Show/[Grp] Show - 05a [ABCD1234].mkv", Shows,
			Guess{Title: "Show", Season: 1, Absolute: []int{5}, SplitPart: "a", Group: "Grp", Checksum: "ABCD1234"}},
	})
}

// Names are normalized to NFC. Matching uses NFKC, so full-width digits and
// brackets parse.
func TestUnicode(t *testing.T) {
	nfd := norm.NFD.String("Amélie (2001)/Amélie (2001).mkv")
	if nfd == norm.NFC.String(nfd) {
		t.Fatal("test input isn't decomposed")
	}
	runParse(t, []parseCase{
		{nfd, Movies, Guess{Title: "Amélie", Year: 2001}},
		{norm.NFD.String("Pokémon/Season 01/Pokémon S01E01.mkv"), Shows, Guess{Title: "Pokémon", Season: 1, Episodes: []int{1}}},
		{"Frieren/［ＳｕｂｓＰｌｅａｓｅ］　Ｆｒｉｅｒｅｎ　－　１２　［１０８０ｐ］.mkv", Shows,
			Guess{Title: "Frieren", Season: 1, Absolute: []int{12}, Group: "SubsPlease"}},
		{"Golden Time/【MMZYSUB】Golden Time - 24 【ABCD1234】.mkv", Shows,
			Guess{Title: "Golden Time", Season: 1, Absolute: []int{24}, Group: "MMZYSUB", Checksum: "ABCD1234"}},
	})
}

// IDs, editions, extras and parts follow the common media naming
// conventions.
func TestNaming(t *testing.T) {
	runParse(t, []parseCase{
		{"Dune (2021) {tmdb-438631}/Dune (2021) {tmdb-438631} {edition-Director's Cut}.mkv", Movies,
			Guess{Title: "Dune", Year: 2021, Edition: "Director's Cut", IDs: []ExternalID{{"tmdb", "438631"}}}},
		{"Show/Show [tvdbid=81189] S01E01.mkv", Shows,
			Guess{Title: "Show", Season: 1, Episodes: []int{1}, IDs: []ExternalID{{"tvdb", "81189"}}}},
		{"Movie [imdbid-tt0111161].mkv", Movies, Guess{Title: "Movie", IDs: []ExternalID{{"imdb", "tt0111161"}}}},
		{"Movie (1999)/Movie (1999)-trailer.mkv", Movies, Guess{Title: "Movie", Year: 1999, Extra: Trailer}},
		{"Movie (1999)/Movie (1999)-behindthescenes.mkv", Movies, Guess{Title: "Movie", Year: 1999, Extra: BehindTheScenes}},
		{"Movie (1999)/Featurettes/Making Of.mkv", Movies, Guess{Title: "Making Of", Extra: Featurette}},
		{"Show/Season 01/Behind The Scenes/On Set.mkv", Shows, Guess{Title: "On Set", Season: 1, Extra: BehindTheScenes}},
		{"Trailers/Trailer.mkv", Movies, Guess{Title: "Trailer"}},
		{"FMA/Fullmetal_Alchemist_Brotherhood_OP1v2_Clean_[BD_1080p][AtsA][0F1CC673].mkv", Shows,
			Guess{Title: "Fullmetal Alchemist Brotherhood OP1v2 Clean", Season: 1, Extra: OtherExtra, Checksum: "0F1CC673", Rule: "title"}},
		{"Show/[Group] Show - NCED2 [BD].mkv", Shows, Guess{Title: "Show - NCED2", Season: 1, Extra: OtherExtra, Group: "Group", Rule: "title"}},
		{"IGLOO/[OZC]MS IGLOO - The Hidden One Year War Episode 1 Clean OP.mkv", Shows,
			Guess{Title: "MS IGLOO - The Hidden One Year War", Season: 1, Episodes: []int{1}, Extra: OtherExtra, Group: "OZC", Rule: "episode"}},
		{"Serial Experiments Lain/Serial Experiments Lain OP [v2].mkv", Shows,
			Guess{Title: "Serial Experiments Lain OP", Season: 1, Extra: OtherExtra, Rule: "title"}},
		{"Serial Experiments Lain/Serial Experiments Lain ED.mkv", Shows,
			Guess{Title: "Serial Experiments Lain ED", Season: 1, Extra: OtherExtra, Rule: "title"}},
		{"Mister Ed/Mister Ed.mkv", Shows, Guess{Title: "Mister Ed", Season: 1, Rule: "title"}},
		{"Ed Edd n Eddy/Ed Edd n Eddy S01E01.mkv", Shows, Guess{Title: "Ed Edd n Eddy", Season: 1, Episodes: []int{1}, Rule: "sxxeyy"}},
		{"Movie (1999)/Movie (1999) - cd1.mkv", Movies, Guess{Title: "Movie", Year: 1999, Part: &Part{Index: 1, Token: "cd1"}}},
		{"Movie (1999)/Movie (1999) - part b.mkv", Movies, Guess{Title: "Movie", Year: 1999, Part: &Part{Index: 2, Token: "part b"}}},
		{"Movie/Movie.disc2.mkv", Movies, Guess{Title: "Movie", Part: &Part{Index: 2, Token: "disc2"}}},
		{"Kill Bill Part 2 (2004).mkv", Movies, Guess{Title: "Kill Bill Part 2", Year: 2004}},
		{"Home/2019-06-12 Birthday_party.mp4", Videos, Guess{Title: "2019-06-12 Birthday party", Rule: "videos"}},
	})
}

func TestDir(t *testing.T) {
	p := Parser{Today: today}
	cases := []struct {
		name string
		want DirGuess
	}{
		{"Dune (2021) {tmdb-438631}", DirGuess{Title: "Dune", Year: 2021, IDs: []ExternalID{{"tmdb", "438631"}}}},
		{"Show (2005) [tvdbid=81189]", DirGuess{Title: "Show", Year: 2005, IDs: []ExternalID{{"tvdb", "81189"}}}},
		{"Cowboy Bebop {anidb-23}", DirGuess{Title: "Cowboy Bebop", IDs: []ExternalID{{"anidb", "23"}}}},
		{"Season 02", DirGuess{IsSeason: true, Season: 2}},
		{"season 1", DirGuess{IsSeason: true, Season: 1}},
		{"S03", DirGuess{IsSeason: true, Season: 3}},
		{"Season 00", DirGuess{IsSeason: true, Season: 0}},
		{"Specials", DirGuess{IsSeason: true, Season: 0}},
		{"Season 2023", DirGuess{IsSeason: true, Season: 2023}},
		{"Trailers", DirGuess{Title: "Trailers", Extra: Trailer}},
		{"The Office (US)", DirGuess{Title: "The Office", Country: "US"}},
		{"Cash Cab (US) (2005)", DirGuess{Title: "Cash Cab", Year: 2005, Country: "US"}},
		{"Love Island (UK)", DirGuess{Title: "Love Island", Country: "UK"}},
		{"Kill It (It)", DirGuess{Title: "Kill It (It)"}},
		{"Behind The Scenes", DirGuess{Title: "Behind The Scenes", Extra: BehindTheScenes}},
		{"2001 A Space Odyssey (1968)", DirGuess{Title: "2001 A Space Odyssey", Year: 1968}},
		{norm.NFD.String("Amélie (2001)"), DirGuess{Title: "Amélie", Year: 2001}},
	}
	for _, c := range cases {
		if got := p.Dir(c.name); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Dir(%q)\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

// Titles compare by Key: decomposed with marks stripped, case-folded, and
// stripped of punctuation. "&" reads as "and", and a, an, the, and, or, of
// are dropped unless first or last.
func TestKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Lord of the Rings", "the lord rings"},
		{"Amélie", "amelie"},
		{norm.NFD.String("Amélie"), "amelie"},
		{"Law & Order", "law order"},
		{"Marvel's Agents of S.H.I.E.L.D.", "marvels agents shield"},
		{"Spider-Man: Into the Spider-Verse", "spider man into spider verse"},
		{"STRASSE", "strasse"},
		{"Straße", "strasse"},
		{"Of Mice and Men", "of mice men"},
		{"Ｆｒｉｅｒｅｎ", "frieren"},
		{"A", "a"},
		{"The The", "the the"},
		{"", ""},
		{"?!", ""},
	}
	for _, c := range cases {
		if got := Key(c.in); got != c.want {
			t.Errorf("Key(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestZeroToday(t *testing.T) {
	if got := (Parser{}).Parse("Movie (2090).mkv", Movies); got.Year != 2090 {
		t.Errorf("zero Today: year %d, want 2090", got.Year)
	}
}
