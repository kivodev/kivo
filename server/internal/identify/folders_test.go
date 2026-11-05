package identify_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// TestFolders runs the folder goldens in testdata/folders. Each golden is a
// tree listing and NFO files, identified against the fake provider's canned
// responses, and the works that result. Files are identified one at a time
// in the order listed, as probes finish. Each golden also lists the searches
// and episode-list fetches made, so a second search for one folder shows
// up.
func TestFolders(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  catalog.LibraryKind
		files []string
		nfos  map[string]string
	}{
		{name: "movies", kind: catalog.Movies, files: []string{
			"Blade Runner (1982)/Blade Runner (1982).mkv",
			"The Matrix 1999 1080p BluRay x264.mkv",
			"Heat.1995.mkv",
			"Heat (1995)/Heat (1995).mkv",
		}},
		{name: "remake by year", kind: catalog.Movies, files: []string{
			"The Thing (1982)/The Thing (1982).mkv",
			"The Thing (2011)/The Thing.mkv",
			"Dune (2020)/Dune.mkv", // a year off: the 2021 film's festival year
		}},
		{name: "multi-version", kind: catalog.Movies, files: []string{
			"Dune (2021)/Dune (2021) - 2160p.mkv",
			"Dune (2021)/Dune (2021) - 1080p.mkv",
			"Dune (2021)/Dune.2021.720p.WEB-DL.mkv",
		}},
		{name: "editions", kind: catalog.Movies, files: []string{
			"Blade Runner (1982)/Blade Runner (1982) {edition-Final Cut}.mkv",
			"Blade Runner (1982)/Blade Runner (1982).mkv",
			"Blade Runner (1982)/Blade Runner (1982) {edition-Director's Cut} 4K.mkv",
		}},
		{name: "folder edition", kind: catalog.Movies, files: []string{
			"Blade Runner (1982) {edition-Final Cut}/Blade Runner (1982) {edition-Final Cut}.mkv",
			"Blade Runner (1982) {edition-Final Cut}/Blade Runner (1982) {edition-Final Cut} 4K.mkv",
			"Blade Runner (1982) {edition-Final Cut}/Blade Runner (1982) {edition-Workprint}.mkv",
			"Blade Runner (1982)/Blade Runner (1982).mkv",
		}},
		{name: "editions only", kind: catalog.Movies, files: []string{
			"Blade Runner (1982)/Blade Runner (1982) {edition-The Final Cut}.mkv",
			"Blade Runner (1982)/Blade Runner (1982) {edition-Director's Cut}.mkv",
			"Blade Runner (1982)/Blade Runner (1982) {edition-Director's Cut} 4K.mkv",
			"Heat.1995.{edition-Extended}.mkv",
		}},
		{name: "parts", kind: catalog.Movies, files: []string{
			"Kill Bill Vol 1 (2003)/Kill Bill Vol 1 (2003) cd1.avi",
			"Kill Bill Vol 1 (2003)/Kill Bill Vol 1 (2003) cd2.avi",
		}},
		{name: "extras", kind: catalog.Movies, files: []string{
			"Dune (2021)/Dune (2021).mkv",
			"Dune (2021)/Trailers/Official Trailer.mkv",
			"Dune (2021)/Dune (2021)-featurette.mkv",
			"Dune (2021)/Behind The Scenes/Making Arrakis.mkv",
		}},
		{name: "disc rips", kind: catalog.Movies, files: []string{
			"The Matrix (1999)/VIDEO_TS/VIDEO_TS.VOB@1000",
			"The Matrix (1999)/VIDEO_TS/VTS_01_0.VOB@1000",
			"The Matrix (1999)/VIDEO_TS/VTS_01_1.VOB@3000",
			"The Matrix (1999)/VIDEO_TS/VTS_01_2.VOB@3000",
			"The Matrix (1999)/VIDEO_TS/VTS_02_1.VOB@2000",
			"Heat (1995)/BDMV/STREAM/00000.m2ts@5000",
			"Heat (1995)/BDMV/STREAM/00001.m2ts@90000",
		}},
		{name: "ignored and hashed", kind: catalog.Movies, files: []string{
			"Heat (1995)/Heat (1995).mkv",
			"Heat (1995)/heat-sample.mkv",
			"Heat (1995)/Sample/heat.mkv",
			"0123456789abcdef0123456789abcdef.mkv",
			"Blade Runner (1982)/a1b2c3d4e5f60718293a4b5c6d7e8f90.mkv",
		}},
		{name: "ids and nfos", kind: catalog.Movies, files: []string{
			"Arrakis {tmdb-438631}/Arrakis.mkv",
			"Some Folder/Some Movie.mkv",
			"Other Folder/Other.mkv",
			"Ghibli/Gake [anidb-5975].mkv",
			"Unknown {anidb-1}/Unknown.mkv",
		}, nfos: map[string]string{
			"Some Folder/Some Movie.nfo": `<movie><title>x</title><uniqueid type="imdb">tt1160419</uniqueid></movie>`,
			"Other Folder/movie.nfo":     "https://www.themoviedb.org/movie/949-heat",
		}},
		{name: "shows", kind: catalog.Shows, files: []string{
			"The Expanse (2015)/Season 01/The Expanse - S01E01 - Dulcinea.mkv",
			"The Expanse (2015)/Season 01/The Expanse - S01E02.mkv",
			"The Expanse (2015)/Season 01/The.Expanse.S01E02.1080p.mkv",
			"The Expanse (2015)/Season 2/S02E01.mkv",
			"The Expanse (2015)/Specials/The Expanse - S00E01.mkv",
			"The Expanse (2015)/Season 01/The Expanse - S01E09.mkv",
			"The Expanse (2015)/Extras/Inside the Expanse.mkv",
			"The Expanse (2015)/Season 01/Featurettes/Season One Recap.mkv",
		}},
		{name: "multi-episode and split", kind: catalog.Shows, files: []string{
			"Firefly/Season 1/Firefly - S01E01-E02.mkv",
			"Firefly/Season 1/Firefly - S01E01.mkv",
			"Firefly/Season 1/Firefly - S01E03a.mkv",
			"Firefly/Season 1/Firefly - S01E03b.mkv",
		}},
		{name: "date episodes", kind: catalog.Shows, files: []string{
			"The Daily Show/The Daily Show 2024-02-12.mkv",
			"The Daily Show/The Daily Show 2024-02-13.mkv",
			"The Daily Show/The Daily Show 2024-02-14.mkv",
		}},
		{name: "anime absolute", kind: catalog.Shows, files: []string{
			"One Piece/[Group] One Piece - 001 [ABCD1234].mkv",
			"One Piece/[Group] One Piece - 004 [ABCD1234].mkv",
			"One Piece/[Group] One Piece - 012.5 [ABCD1234].mkv",
		}},
		{name: "anime openings", kind: catalog.Shows, files: []string{
			"Serial Experiments Lain/Serial Experiments Lain 01 - Weird.mkv",
			"Serial Experiments Lain/Serial Experiments Lain OP [v2].mkv",
			"Serial Experiments Lain/Serial Experiments Lain ED.mkv",
		}},
		{name: "anime lists", kind: catalog.Shows, files: []string{
			"Wan Pisu [anidb-69]/[Group] Wan Pisu - 005.mkv",
		}},
		{name: "show ids", kind: catalog.Shows, files: []string{
			"Serenity Series {tvdb-78874}/S01E04.mkv",
			"Loose.Show.S01E01.mkv",
		}},
		{name: "books", kind: catalog.Audiobooks, files: []string{
			"Andy Weir/Project Hail Mary/Project Hail Mary.m4b",
			"Stephen King/11.22.63/10 - Chapter 9.mp3",
			"Stephen King/11.22.63/02 - Chapter 1.mp3",
			"Stephen King/11.22.63/01 - Opening Credits.mp3",
			"Michael J. Sullivan/Riyria Revelations/Heir of Novron/CD2/01.mp3",
			"Michael J. Sullivan/Riyria Revelations/Heir of Novron/CD1/02.mp3",
			"Michael J. Sullivan/Riyria Revelations/Heir of Novron/CD1/01.mp3",
			"James S. A. Corey/The Expanse/Book 1.5 - The Churn/The Churn.m4b",
			"Sphere.m4b",
		}},
		{name: "other videos", kind: catalog.Videos, files: []string{
			"Birthdays/2019/Cake.mp4",
			"Birthdays/2019/Candles.mp4",
			"Holiday 2020.mov",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			lib := e.library(tc.kind)
			for rel, body := range tc.nfos {
				e.nfo(lib, rel, body)
			}
			// A scan records a folder's files together. Probes, and so
			// identifications, finish one at a time.
			e.run(e.add(lib, tc.files...)...)
			got := "searches:\n"
			for _, s := range e.video.searches {
				got += "  " + s + "\n"
			}
			got += "episode lists:\n"
			for _, s := range e.video.episodes {
				got += "  " + s + "\n"
			}
			got += "works:\n" + e.dump()
			golden := filepath.Join("testdata", "folders", strings.ReplaceAll(tc.name, " ", "-")+".golden")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if got != string(want) {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// One identification covers every root work in its file's folder, since it is
// the only one queued for the folder.
func TestJobIdentifiesWholeFolder(t *testing.T) {
	e := newEnv(t)
	f := e.add(e.library(catalog.Movies), "Movies/Heat (1995).mkv", "Movies/Blade Runner (1982).mkv")
	e.run(f[0])
	for i, want := range []string{"Heat", "Blade Runner"} {
		if w := e.workOf(f[i]); len(w) != 1 || w[0].Provisional || w[0].Title != want {
			t.Fatalf("%s: %+v", want, w)
		}
	}
}

// Running a settled folder again makes no write. The match row stays as it
// was, and nothing is refreshed.
func TestSettledFolderWritesNothing(t *testing.T) {
	e := newEnv(t)
	lib := e.library(catalog.Shows)
	f := e.add(lib, "Firefly/S01E01.mkv", "Firefly/S01E02.mkv")
	e.run(f...)
	updated := func() (at int64) {
		t.Helper()
		if err := e.b.DB.Reader().QueryRow(`SELECT updated_at FROM match`).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	before := updated()
	e.takeRefreshes()
	time.Sleep(2 * time.Millisecond)
	e.again(f...)
	if updated() != before || len(e.takeRefreshes()) != 0 {
		t.Fatal("a settled folder was written again")
	}
	more := e.add(lib, "Firefly/S01E03.mkv")
	e.run(more...)
	if w := e.workOf(more[0])[0]; w.Provisional || w.Title != "Our Mrs. Reynolds" {
		t.Fatalf("a new episode in a settled folder: %+v", w)
	}
}
