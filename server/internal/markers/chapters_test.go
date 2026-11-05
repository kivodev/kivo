package markers

import (
	"reflect"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

func TestTimelineDropsChaptersThatDontFitTheirFile(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	file := func(d time.Duration, chapters ...catalog.Chapter) catalog.File {
		return catalog.File{RelPath: "Show/Show 01.mkv", Probe: catalog.ProbeSummary{Duration: d}, Chapters: chapters}
	}
	// Ordered chapters each play a piece of another file from that file's
	// start: the opening's file, the episode's own file and the ending's
	// file.
	lain := file(ms(1_380_000),
		catalog.Chapter{Start: 0, End: ms(119_620), Title: "Opening"},
		catalog.Chapter{Start: 0, End: ms(1_380_000), Title: "Episode"},
		catalog.Chapter{Start: 0, End: ms(98_599), Title: "Ending"},
	)
	if got, _ := chapters(catalog.KindEpisode, []catalog.File{lain}); got != nil {
		t.Errorf("ordered chapters: %+v", got)
	}
	for name, f := range map[string]catalog.File{
		"out of order": file(ms(600_000),
			catalog.Chapter{Start: ms(300_000), End: ms(600_000), Title: "Two"},
			catalog.Chapter{Start: 0, End: ms(300_000), Title: "One"}),
		"overlapping": file(ms(600_000),
			catalog.Chapter{Start: 0, End: ms(400_000), Title: "One"},
			catalog.Chapter{Start: ms(300_000), End: ms(600_000), Title: "Two"}),
		"past the end": file(ms(600_000),
			catalog.Chapter{Start: 0, End: ms(300_000), Title: "One"},
			catalog.Chapter{Start: ms(300_000), End: ms(900_000), Title: "Two"}),
	} {
		if got := timeline([]catalog.File{f}); got != nil {
			t.Errorf("%s: %+v", name, got)
		}
	}
	// A last chapter ending a moment past the file's duration still fits.
	if got := timeline([]catalog.File{file(ms(600_000), catalog.Chapter{Start: 0, End: ms(600_400), Title: "One"})}); len(got) != 1 {
		t.Errorf("rounded end: %+v", got)
	}
}

func TestTitledIsTheWholeName(t *testing.T) {
	for name, want := range map[string]MarkerKind{
		"Intro":                    IntroMarker,
		"Introduction":             IntroMarker,
		"Opening":                  IntroMarker,
		"Opening Credits":          IntroMarker,
		"2. Opening Credits":       IntroMarker,
		"Chapter 4 - Intro":        IntroMarker,
		"07 Opening":               IntroMarker,
		"[Opening]":                IntroMarker,
		"Title Sequence":           IntroMarker,
		"Main Titles":              IntroMarker,
		"OP":                       IntroMarker,
		"OP1":                      IntroMarker,
		"Intro start":              IntroMarker,
		"Credits":                  CreditsMarker,
		"Credit start":             CreditsMarker,
		"Credits start":            CreditsMarker,
		"End Credits":              CreditsMarker,
		"Ending Credits":           CreditsMarker,
		"Closing Credits":          CreditsMarker,
		"End Titles":               CreditsMarker,
		"Ending":                   CreditsMarker,
		"Closing":                  CreditsMarker,
		"Outro":                    CreditsMarker,
		"ED":                       CreditsMarker,
		"ED2":                      CreditsMarker,
		"ED: Song Name":            CreditsMarker,
		"Credits / End Credits":    CreditsMarker,
		"End Credits (Cheer Down)": CreditsMarker,
		"End Credits [1:12]":       CreditsMarker,
		`End Credits ("Wake Up…")`: CreditsMarker,
		`39. End Titles ("Why Not Tonight?" – Reba)`:  CreditsMarker,
		"Here Comes the Sun (End Credits)":            CreditsMarker,
		"Cheer Down (End Titles)":                     CreditsMarker,
		"Song Name [Credits]":                         CreditsMarker,
		"ED: Song":                                    CreditsMarker,
		"Opening – Song Title":                        IntroMarker,
		"Opening - Song Title":                        IntroMarker,
		"Previously On: The Show":                     RecapMarker,
		"Song Name (Credits Scene)":                   NoMarker,
		"Song Name (Intro, Credits)":                  NoMarker,
		"Opening-Night":                               NoMarker,
		"Ron Makes Changes to the Barbecue - Opening": NoMarker,
		"Recap":                RecapMarker,
		"Previously":           RecapMarker,
		"Previously On":        RecapMarker,
		"Last Time On":         RecapMarker,
		"Recap start":          RecapMarker,
		"Preview":              previewMarker,
		"Next Episode Preview": previewMarker,
		"Next Time":            previewMarker,
		"op":                   NoMarker,
		"Mister Ed":            NoMarker,
		"Ed":                   NoMarker,
		"Intro end":            NoMarker,
		"End of Intro":         NoMarker,
		"Post Intro":           NoMarker,
		"Post-Credits":         NoMarker,
		"Mid-Credits":          NoMarker,
		"After Credits":        NoMarker,
		"Credits Scene":        NoMarker,
		"Prologue":             NoMarker,
		"Avant":                NoMarker,
		"Cold Open":            NoMarker,
		"Teaser":               NoMarker,
		"Stinger":              NoMarker,
		"Skip Intro":           NoMarker,
		"Intro & Recap":        NoMarker,
		"Spending Money":       NoMarker,
		"Chapter 3":            NoMarker,
		"The Open Road":        NoMarker,
		"Ron Makes Changes to the Barbecue, Opening Credits": NoMarker,
		"Andy Gives April a Tour, End Credits":               NoMarker,
		"The Ending and the Beginning":                       NoMarker,
	} {
		if got := titled(name).kind; got != want {
			t.Errorf("titled(%q) = %q, want %q", name, got, want)
		}
	}
}

// marked returns a single-file version's markers.
func marked(work catalog.WorkKind, total time.Duration, cs ...catalog.Chapter) []Marker {
	_, ms := chapters(work, []catalog.File{{RelPath: "a.mkv", Probe: catalog.ProbeSummary{Duration: total}, Chapters: cs}})
	return ms
}

// ch returns a chapter with its times given in seconds.
func ch(title string, start, end int) catalog.Chapter {
	return catalog.Chapter{Start: time.Duration(start) * time.Second, End: time.Duration(end) * time.Second, Title: title}
}

func TestChapterMarkers(t *testing.T) {
	s := time.Second
	mins := func(n int) time.Duration { return time.Duration(n) * time.Minute }
	for _, tc := range []struct {
		name  string
		work  catalog.WorkKind
		total time.Duration
		cs    []catalog.Chapter
		want  []Marker
	}{{
		name: "Parks and Recreation S05E01", work: catalog.KindEpisode, total: mins(21),
		cs: []catalog.Chapter{ch("Cold Open", 0, 62), ch("Opening Credits", 62, 83),
			ch("Ron Makes Changes to the Barbecue, Opening Credits", 83, 600),
			ch("Andy Gives April a Tour, End Credits", 600, 1260)},
		want: []Marker{{Kind: IntroMarker, Start: 62 * s, End: 83 * s, Source: ChapterSource}},
	}, {
		name: "Fullmetal Alchemist: Brotherhood, a cold open named Intro", work: catalog.KindEpisode, total: mins(24),
		cs: []catalog.Chapter{ch("Intro", 0, 31), ch("Opening", 31, 121), ch("Episode", 121, 1320),
			ch("Ending", 1320, 1410), ch("Preview", 1410, 1440)},
		want: []Marker{
			{Kind: IntroMarker, Start: 31 * s, End: 121 * s, Source: ChapterSource},
			{Kind: CreditsMarker, Start: 1320 * s, End: 1410 * s, Source: ChapterSource, Final: true},
		},
	}, {
		name: "Dark Winds S01E03, its opening in two chapters", work: catalog.KindEpisode, total: 2744 * s,
		cs: []catalog.Chapter{ch("Previously On", 7, 69), ch("Scene 3", 69, 191), ch("Scene 4", 191, 210),
			ch("Opening Credits", 210, 253), ch("Title Sequence", 253, 260), ch("Scene 7", 260, 2744)},
		want: []Marker{
			{Kind: RecapMarker, Start: 7 * s, End: 69 * s, Source: ChapterSource},
			{Kind: IntroMarker, Start: 210 * s, End: 260 * s, Source: ChapterSource},
		},
	}, {
		name: "Billions S05E02, the other way round", work: catalog.KindEpisode, total: 3563 * s,
		cs: []catalog.Chapter{ch("Studio Logo", 0, 4), ch("Previously On", 4, 48), ch("Title Sequence", 48, 61),
			ch("Opening Credits", 61, 66), ch("Scene 6", 66, 3563)},
		want: []Marker{
			{Kind: RecapMarker, Start: 4 * s, End: 48 * s, Source: ChapterSource},
			{Kind: IntroMarker, Start: 48 * s, End: 66 * s, Source: ChapterSource},
		},
	}, {
		name: "Animal Kingdom S05E08, a recap in two chapters", work: catalog.KindEpisode, total: 2753 * s,
		cs:   []catalog.Chapter{ch("Previously On", 0, 69), ch("Previously On", 69, 130), ch("Scene 3", 130, 2753)},
		want: []Marker{{Kind: RecapMarker, Start: 0, End: 130 * s, Source: ChapterSource}},
	}, {
		name: "Saint Seiya: The Hades Chapter S01E01, Intro after the opening", work: catalog.KindEpisode, total: 1504 * s,
		cs:   []catalog.Chapter{ch("OP", 0, 117), ch("Intro", 117, 255), ch("Part A", 255, 800), ch("Part B", 800, 1504)},
		want: []Marker{{Kind: IntroMarker, Start: 0, End: 117 * s, Source: ChapterSource}},
	}, {
		name: "Planetes S01E02, a cold open named Intro", work: catalog.KindEpisode, total: 1502 * s,
		cs:   []catalog.Chapter{ch("Intro", 0, 98), ch("Opening", 98, 188), ch("Part A", 188, 742), ch("Part B", 742, 1502)},
		want: []Marker{{Kind: IntroMarker, Start: 98 * s, End: 188 * s, Source: ChapterSource}},
	}, {
		name: "Psycho-Pass S01E02, a cold open named Intro", work: catalog.KindEpisode, total: 1373 * s,
		cs: []catalog.Chapter{ch("Intro", 0, 58), ch("OP", 58, 148), ch("Episode", 148, 1281), ch("ED", 1281, 1373)},
		want: []Marker{
			{Kind: IntroMarker, Start: 58 * s, End: 148 * s, Source: ChapterSource},
			{Kind: CreditsMarker, Start: 1281 * s, End: 1373 * s, Source: ChapterSource, Final: true},
		},
	}, {
		name: "an Intro between two openings", work: catalog.KindEpisode, total: mins(24),
		cs: []catalog.Chapter{ch("Opening", 0, 60), ch("Intro", 60, 90), ch("Title Sequence", 90, 100), ch("Episode", 100, 1440)},
		want: []Marker{
			{Kind: IntroMarker, Start: 0, End: 60 * s, Source: ChapterSource},
			{Kind: IntroMarker, Start: 90 * s, End: 100 * s, Source: ChapterSource},
		},
	}, {
		name: "adjacent openings too long together give none", work: catalog.KindEpisode, total: mins(24),
		cs: []catalog.Chapter{ch("Opening", 0, 100), ch("OP", 100, 200), ch("Episode", 200, 1440)},
	}, {
		name: "adjacent credits too long together give none", work: catalog.KindEpisode, total: mins(45),
		cs: []catalog.Chapter{ch("Story", 0, 2100), ch("Credits", 2100, 2400), ch("End Credits", 2400, 2700)},
	}, {
		name: "chapters apart are separate spans", work: catalog.KindEpisode, total: mins(24),
		cs: []catalog.Chapter{ch("Recap", 0, 30), ch("Recap", 40, 70), ch("Episode", 70, 1440)},
		want: []Marker{
			{Kind: RecapMarker, Start: 0, End: 30 * s, Source: ChapterSource},
			{Kind: RecapMarker, Start: 40 * s, End: 70 * s, Source: ChapterSource},
		},
	}, {
		name: "Post Intro, Intro end and Closing", work: catalog.KindEpisode, total: mins(30),
		cs: []catalog.Chapter{ch("Previously On", 0, 40), ch("Intro end", 40, 60), ch("Post Intro", 60, 1650),
			ch("Closing", 1650, 1800)},
		want: []Marker{
			{Kind: RecapMarker, Start: 0, End: 40 * s, Source: ChapterSource},
			{Kind: CreditsMarker, Start: 1650 * s, End: 1800 * s, Source: ChapterSource, Final: true},
		},
	}, {
		name: "a long Closing is the story's", work: catalog.KindEpisode, total: mins(45),
		cs: []catalog.Chapter{ch("Story", 0, 2100), ch("Closing", 2100, 2700)},
	}, {
		name: "numbered, song-named, and adjacent credits merge", work: catalog.KindEpisode, total: mins(24),
		cs: []catalog.Chapter{ch("2. Opening Credits", 0, 90), ch("Part A", 90, 1300), ch("ED: Song Name", 1300, 1380),
			ch("Credits", 1380, 1400), ch("End Credits", 1400, 1420), ch("Scene", 1420, 1440)},
		want: []Marker{
			{Kind: IntroMarker, Start: 0, End: 90 * s, Source: ChapterSource},
			{Kind: CreditsMarker, Start: 1300 * s, End: 1420 * s, Source: ChapterSource},
		},
	}, {
		name: "too short", work: catalog.KindEpisode, total: mins(40),
		cs: []catalog.Chapter{ch("Intro", 0, 3), ch("Story", 3, 1800), ch("Credits", 1800, 1805), ch("More", 1805, 2400)},
	}, {
		name: "too long", work: catalog.KindEpisode, total: mins(60),
		cs: []catalog.Chapter{ch("Recap", 0, 200), ch("Story", 200, 3000), ch("Credits", 3000, 3600)},
	}, {
		name: "a long preview is the episode's", work: catalog.KindEpisode, total: mins(24),
		cs:   []catalog.Chapter{ch("Episode", 0, 1200), ch("Credits", 1200, 1290), ch("Preview", 1290, 1440)},
		want: []Marker{{Kind: CreditsMarker, Start: 1200 * s, End: 1290 * s, Source: ChapterSource}},
	}, {
		name: "misplaced", work: catalog.KindEpisode, total: mins(24),
		cs: []catalog.Chapter{ch("Credits", 0, 60), ch("Episode", 60, 1380), ch("Intro", 1380, 1440)},
	}, {
		name: "a movie: credits only", work: catalog.KindMovie, total: mins(120),
		cs: []catalog.Chapter{ch("Opening Credits", 0, 180), ch("Film", 180, 6300), ch("End Credits", 6300, 7140),
			ch("Post-Credits", 7140, 7200)},
		want: []Marker{{Kind: CreditsMarker, Start: 6300 * s, End: 7140 * s, Source: ChapterSource}},
	}, {
		name: "a movie's credits named with their song", work: catalog.KindMovie, total: mins(120),
		cs:   []catalog.Chapter{ch("Film", 0, 6600), ch(`39. End Titles ("Why Not Tonight?" – Reba)`, 6600, 7200)},
		want: []Marker{{Kind: CreditsMarker, Start: 6600 * s, End: 7200 * s, Source: ChapterSource, Final: true}},
	}, {
		name: "a movie's song with its credits", work: catalog.KindMovie, total: mins(100),
		cs:   []catalog.Chapter{ch("Film", 0, 5600), ch("Cheer Down (End Credits)", 5600, 5880), ch("Logos", 5880, 6000)},
		want: []Marker{{Kind: CreditsMarker, Start: 5600 * s, End: 5880 * s, Source: ChapterSource}},
	}, {
		name: "a movie's Ending at the middle", work: catalog.KindMovie, total: mins(120),
		cs: []catalog.Chapter{ch("Beginning", 0, 3600), ch("Ending", 3600, 6000), ch("Film", 6000, 7200)},
	}, {
		name: "other videos: none", work: catalog.KindVideo, total: mins(24),
		cs: []catalog.Chapter{ch("Intro", 0, 60), ch("Episode", 60, 1380), ch("Credits", 1380, 1440)},
	}} {
		if got := marked(tc.work, tc.total, tc.cs...); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestMarkersAcrossParts(t *testing.T) {
	part := func(cs ...catalog.Chapter) catalog.File {
		return catalog.File{RelPath: "a.mkv", Probe: catalog.ProbeSummary{Duration: 12 * time.Minute}, Chapters: cs}
	}
	got := versionMarkers(catalog.KindEpisode, []catalog.File{
		part(ch("Opening", 0, 90), ch("Part A", 90, 720)),
		part(ch("Part B", 0, 630), ch("End Credits", 630, 720)),
	})
	want := []Marker{
		{Kind: IntroMarker, Start: 0, End: 90 * time.Second, Source: ChapterSource},
		{Kind: CreditsMarker, Start: 1350 * time.Second, End: 1440 * time.Second, Source: ChapterSource, Final: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

// versionMarkers returns the markers a version's chapters give it, for a
// work of the given kind.
func versionMarkers(work catalog.WorkKind, parts []catalog.File) []Marker {
	_, ms := chapters(work, parts)
	return ms
}
