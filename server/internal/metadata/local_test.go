package metadata

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

const duneRel = "Dune (2021)/Dune (2021).mkv"

const duneNFO = `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<movie>
  <title>Dune: Part One</title>
  <plot>The local plot.</plot>
  <genre>Space Opera</genre>
  <mpaa>US:R</mpaa>
  <actor><name>Second Actor</name><role>B</role><order>1</order></actor>
  <actor><name>First Actor</name><role>A</role><order>0</order></actor>
  <uniqueid type="tmdb" default="true">438631</uniqueid>
</movie>
https://www.themoviedb.org/movie/438631
`

// setMtime sets a file's modification time to the given age.
func setMtime(t *testing.T, path string, age time.Duration) {
	t.Helper()
	at := time.Now().Add(-age)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestLocalFilesOverrideProviderAndAdminOverridesBoth(t *testing.T) {
	e := newEnv(t)
	_, folder, dir := e.localLibrary(catalog.Movies)
	writeFile(t, filepath.Join(dir, "Dune (2021)", "movie.nfo"), []byte(duneNFO))
	writeFile(t, filepath.Join(dir, "Dune (2021)", "Poster.JPG"), pngBytes(200, 300))
	writeFile(t, filepath.Join(dir, "Dune (2021)", "Dune (2021)-fanart.png"), pngBytes(320, 180))
	w := e.movie(folder, duneRel)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.EditMetadata(ctx, tx, w, catalog.WorkMetadata{Overview: "The admin's plot."}, catalog.FieldOverview)
	})

	e.mustRefresh(w)
	for col, want := range map[string]string{
		"title":              "Dune: Part One",    // the NFO's, over TMDb's
		"overview":           "The admin's plot.", // the admin's, over both
		"content_rating":     "R",                 // the NFO's, over TMDb's PG-13
		"content_rating_age": "17",                // with its rank
		"year":               "2021",              // TMDb's, which the NFO doesn't set
	} {
		if got := e.column(w, col); got != want {
			t.Errorf("%s = %q, want %q", col, got, want)
		}
	}
	// The NFO's cast replaces TMDb's. The roles it doesn't list stay TMDb's.
	if got, want := e.credits(w), []string{
		"actor:First Actor", "actor:Second Actor", "director:Denis Villeneuve", "writer:Denis Villeneuve", "writer:Jon Spaihts",
	}; !slices.Equal(got, want) {
		t.Errorf("credits %v, want %v", got, want)
	}
	if got := e.strings(`SELECT g.name FROM work_genre wg JOIN genre g ON g.id = wg.genre_id WHERE wg.work_id = ?`, w); !slices.Equal(got, []string{"Space Opera"}) {
		t.Errorf("genres %v", got)
	}
	imgs := e.images(w)
	if r := imgs[artworkPoster]; r.Source != sourceLocal || r.Origin != filepath.Join(dir, "Dune (2021)", "Poster.JPG") {
		t.Errorf("poster %+v", r)
	}
	if r := imgs[artworkBackdrop]; r.Source != sourceLocal || !strings.HasSuffix(r.Origin, "Dune (2021)-fanart.png") {
		t.Errorf("backdrop %+v", r)
	}
	if r := imgs[artworkLogo]; r.Source != sourceProvider {
		t.Errorf("logo %+v: no local logo, so the provider's", r)
	}

	// A changed local file is picked up. An unchanged one isn't re-encoded.
	poster := imgs[artworkPoster].ID
	encoded := e.img.(*fakeImager).count()
	e.mustRefresh(w)
	if e.images(w)[artworkPoster].ID != poster || e.img.(*fakeImager).count() != encoded {
		t.Error("an unchanged local poster was encoded again")
	}
	writeFile(t, filepath.Join(dir, "Dune (2021)", "Poster.JPG"), pngBytes(400, 600))
	setMtime(t, filepath.Join(dir, "Dune (2021)", "Poster.JPG"), -time.Minute)
	e.mustRefresh(w)
	if e.images(w)[artworkPoster].ID == poster {
		t.Error("a changed local poster wasn't picked up")
	}
}

// A root work's folder may hold an NFO beside each version. The newest wins.
func TestNewestNFOWins(t *testing.T) {
	e := newEnv(t)
	_, folder, dir := e.localLibrary(catalog.Movies)
	nfoA := filepath.Join(dir, "Dune (2021)", "Dune (2021).nfo")
	nfoB := filepath.Join(dir, "Dune (2021)", "Dune (2021) - 4K.nfo")
	writeFile(t, nfoA, []byte(`<movie><title>Title From A</title></movie>`))
	writeFile(t, nfoB, []byte(`<movie><title>Title From B</title></movie>`))
	w := e.movie(folder, duneRel)
	e.b.Version([]id.ID{e.b.File(folder, "Dune (2021)/Dune (2021) - 4K.mkv")}, w)

	setMtime(t, nfoA, 2*time.Hour)
	setMtime(t, nfoB, time.Hour)
	e.mustRefresh(w)
	if got := e.column(w, "title"); got != "Title From B" {
		t.Fatalf("title %q, want the newer NFO's", got)
	}
	setMtime(t, nfoA, 0)
	e.mustRefresh(w)
	if got := e.column(w, "title"); got != "Title From A" {
		t.Fatalf("title %q after A changed", got)
	}
}

func TestLocalMetadataOffFallsBackToProviders(t *testing.T) {
	e := newEnv(t)
	lib, folder, dir := e.localLibrary(catalog.Movies)
	writeFile(t, filepath.Join(dir, "Dune (2021)", "movie.nfo"), []byte(duneNFO))
	writeFile(t, filepath.Join(dir, "Dune (2021)", "poster.png"), pngBytes(200, 300))
	w := e.movie(folder, duneRel)
	e.mustRefresh(w)
	local := e.images(w)[artworkPoster]
	if e.column(w, "overview") != "The local plot." || local.Source != sourceLocal {
		t.Fatalf("local metadata wasn't used: %q, %+v", e.column(w, "overview"), local)
	}

	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, _, err := e.b.Catalog.UpdateLibrary(ctx, tx, lib, catalog.LibraryChange{UseLocalMetadata: new(false)})
		return err
	})
	e.mustRefresh(w)
	if got := e.column(w, "title"); got != "Dune" {
		t.Errorf("title %q, want TMDb's", got)
	}
	if !strings.HasPrefix(e.column(w, "overview"), "Paul Atreides") || e.column(w, "content_rating_age") != "13" {
		t.Errorf("overview %q, rating %s: want TMDb's", e.column(w, "overview"), e.column(w, "content_rating_age"))
	}
	if r := e.images(w)[artworkPoster]; r.Source != sourceProvider {
		t.Errorf("poster %+v, want TMDb's", r)
	}
	if _, err := os.Stat(e.s.path(local.ID, 240)); !os.IsNotExist(err) {
		t.Errorf("the replaced local poster's files remain: %v", err)
	}
}

func TestLocalSeriesFiles(t *testing.T) {
	e := newEnv(t)
	_, folder, dir := e.localLibrary(catalog.Shows)
	show := filepath.Join(dir, "The Expanse (2015)")
	writeFile(t, filepath.Join(show, "tvshow.nfo"), []byte(`<tvshow><title>The Expanse (local)</title><studio>Alcon</studio></tvshow>`))
	writeFile(t, filepath.Join(show, "season01-poster.jpg"), pngBytes(200, 300))
	writeFile(t, filepath.Join(show, "Season 01", "The Expanse S01E01-thumb.jpg"), pngBytes(320, 180))
	writeFile(t, filepath.Join(show, "Season 01", "The Expanse S01E01.nfo"), []byte(`<episodedetails><title>Dulcinea (local)</title><aired>2015-12-14</aired></episodedetails>`))

	series := e.b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: "The Expanse",
		ExternalIDs: []catalog.ExternalID{catalogID("tmdb", "series", "63639")}})
	season := e.b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: "Season 1", Number: 1})
	ep := e.b.Work(catalog.WorkInput{Kind: catalog.KindEpisode, Parent: season, Title: "Episode 1", ParentNumber: 1, Number: 1})
	e.b.Version([]id.ID{e.b.File(folder, "The Expanse (2015)/Season 01/The Expanse S01E01.mkv")}, ep)

	for _, w := range []id.ID{series, season, ep} {
		e.mustRefresh(w)
	}
	if got := e.column(series, "title"); got != "The Expanse (local)" {
		t.Errorf("series title %q", got)
	}
	if got := e.strings(`SELECT s.name FROM work_company ws JOIN company s ON s.id = ws.company_id WHERE ws.work_id = ?`, series); !slices.Equal(got, []string{"Alcon"}) {
		t.Errorf("series studios %v", got)
	}
	if r := e.images(season)[artworkPoster]; r.Source != sourceLocal || !strings.HasSuffix(r.Origin, "season01-poster.jpg") {
		t.Errorf("season poster %+v", r)
	}
	if got := e.column(ep, "title"); got != "Dulcinea (local)" {
		t.Errorf("episode title %q", got)
	}
	if r := e.images(ep)[artworkStill]; r.Source != sourceLocal || !strings.HasSuffix(r.Origin, "S01E01-thumb.jpg") {
		t.Errorf("episode still %+v", r)
	}
}

func TestLocalOnlyWork(t *testing.T) {
	// A videos library has no provider, so the NFO is all there is.
	e := newEnv(t)
	_, folder, dir := e.localLibrary(catalog.Videos)
	writeFile(t, filepath.Join(dir, "Holidays", "Beach 2019.nfo"), []byte(`<movie><title>At the Beach</title><year>2019</year></movie>`))
	w := e.b.Work(catalog.WorkInput{Kind: catalog.KindVideo, Title: "Beach 2019"})
	e.b.Version([]id.ID{e.b.File(folder, "Holidays/Beach 2019.mp4")}, w)
	e.mustRefresh(w)
	if e.column(w, "title") != "At the Beach" || e.column(w, "year") != "2019" {
		t.Fatalf("title %q, year %s", e.column(w, "title"), e.column(w, "year"))
	}
	// Folder-wide artwork names don't apply in the library's own folder.
	writeFile(t, filepath.Join(dir, "poster.jpg"), pngBytes(10, 10))
	_, folder2, dir2 := e.localLibrary(catalog.Videos)
	writeFile(t, filepath.Join(dir2, "poster.jpg"), pngBytes(10, 10))
	loose := e.b.Work(catalog.WorkInput{Kind: catalog.KindVideo, Title: "Loose"})
	e.b.Version([]id.ID{e.b.File(folder2, "Loose.mp4")}, loose)
	e.mustRefresh(loose)
	if len(e.images(loose)) != 0 {
		t.Errorf("a loose file took the folder's poster: %v", e.images(loose))
	}
}

func TestNFOEntitiesAreNotExpanded(t *testing.T) {
	for name, doc := range map[string]string{
		"billion laughs": `<?xml version="1.0"?><!DOCTYPE movie [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;"><!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">]><movie><title>&c;</title></movie>`,
		"external":       `<?xml version="1.0"?><!DOCTYPE movie [<!ENTITY x SYSTEM "file:///etc/passwd">]><movie><title>&x;</title></movie>`,
	} {
		if n, err := parseNFO(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: parsed as %+v", name, n)
		}
	}

	// The work still refreshes from its provider.
	e := newEnv(t)
	_, folder, dir := e.localLibrary(catalog.Movies)
	writeFile(t, filepath.Join(dir, "Dune (2021)", "movie.nfo"), []byte(`<!DOCTYPE movie [<!ENTITY x SYSTEM "file:///etc/passwd">]><movie><title>&x;</title></movie>`))
	w := e.movie(folder, duneRel)
	e.mustRefresh(w)
	if got := e.column(w, "title"); got != "Dune" {
		t.Fatalf("title %q", got)
	}
}
