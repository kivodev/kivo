package metadata

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/tasks"
)

const moviePath = "/3/movie/438631"

func (e *env) strings(query string, args ...any) []string {
	e.t.Helper()
	rows, err := e.b.DB.Reader().Query(query, args...)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// collectionArt lists the artwork of the collection key names, as "kind
// origin" strings.
func (e *env) collectionArt(key string) []string {
	e.t.Helper()
	return e.strings(`SELECT i.kind || ' ' || i.origin FROM image i JOIN collection c ON c.id = i.collection_id
		WHERE c.key = ? ORDER BY i.kind`, key)
}

// due lists the works due a refresh, as the task reads them.
func (e *env) due() []id.ID {
	e.t.Helper()
	works, err := e.s.pendingRefreshes(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	return works
}

func TestRefreshMovie(t *testing.T) {
	e := newEnv(t)
	lib, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	sub := e.s.deps.Events.Subscribe(func(events.Scope) bool { return true })
	defer sub.Close()

	e.mustRefresh(w)

	for col, want := range map[string]string{
		"title": "Dune", "original_title": "Dune", "year": "2021", "content_rating": "PG-13", "content_rating_age": "13", "provisional": "0",
		"community_rating": "7.8", "community_votes": "12345", "original_language": "en",
		"trailer_url": "https://www.youtube.com/watch?v=n9xhJrPXop4",
	} {
		if got := e.column(w, col); got != want {
			t.Errorf("%s = %q, want %q", col, got, want)
		}
	}
	if !strings.HasPrefix(e.column(w, "overview"), "Paul Atreides") || !strings.Contains(e.column(w, "alt_titles"), "Dune: Part One") {
		t.Errorf("overview %q, alt titles %q", e.column(w, "overview"), e.column(w, "alt_titles"))
	}
	if got, want := e.credits(w), []string{
		"actor:Timothée Chalamet", "actor:Rebecca Ferguson", "actor:Oscar Isaac",
		"director:Denis Villeneuve", "writer:Denis Villeneuve", "writer:Jon Spaihts",
	}; !slices.Equal(got, want) {
		t.Errorf("credits %v, want %v", got, want)
	}
	if got := e.strings(`SELECT key FROM provider_cache WHERE key LIKE '/movie/438631?%include_video_language=en&%'`); len(got) != 1 || !strings.Contains(got[0], "videos") {
		t.Errorf("movie requests %v", got)
	}
	// Each credited person keeps the TMDb ID the credit carries. The ID tells
	// apart people with the same name.
	if got := e.strings(`SELECT p.name || ' ' || x.provider || '/' || x.kind || '/' || x.ext_id FROM person_external_id x
		JOIN person p ON p.id = x.person_id ORDER BY p.name`); !slices.Equal(got, []string{
		"Oscar Isaac tmdb/person/25072", "Rebecca Ferguson tmdb/person/933238", "Timothée Chalamet tmdb/person/1190668",
	}) {
		t.Errorf("person IDs %v", got)
	}
	// The Rating sort key is (12345 * 7.8 + 50 * 6.5) / (12345 + 50) * 1000.
	if got := e.strings(`SELECT CAST(rating_sort AS TEXT) FROM work_library WHERE work_id = ?`, w); !slices.Equal(got, []string{"7794"}) {
		t.Errorf("rating key %v", got)
	}
	if got := e.strings(`SELECT g.name FROM work_genre wg JOIN genre g ON g.id = wg.genre_id WHERE wg.work_id = ? ORDER BY g.name`, w); !slices.Equal(got, []string{"Adventure", "Science Fiction"}) {
		t.Errorf("genres %v", got)
	}
	if got := e.strings(`SELECT s.name FROM work_company ws JOIN company s ON s.id = ws.company_id WHERE ws.work_id = ?`, w); !slices.Equal(got, []string{"Legendary Pictures"}) {
		t.Errorf("studios %v", got)
	}
	// Movie collections come from TMDb's collection data, ordered by release.
	if got := e.strings(`SELECT c.key || ' ' || c.name || ' ' || cw.ord FROM collection c
		JOIN collection_work cw ON cw.collection_id = c.id WHERE cw.work_id = ?`, w); !slices.Equal(got, []string{"tmdb:726871 Dune Collection 1"}) {
		t.Errorf("collection %v", got)
	}
	// The collection has its own poster and backdrop from TMDb.
	if got := e.collectionArt("tmdb:726871"); !slices.Equal(got, []string{
		"backdrop " + tmdbImages + "/dune-collection-backdrop.jpg", "poster " + tmdbImages + "/dune-collection-poster.jpg",
	}) {
		t.Errorf("collection artwork %v", got)
	}

	imgs := e.images(w)
	for kind, origin := range map[string]string{
		artworkPoster:   tmdbImages + "/dune-poster-en.jpg", // the metadata language beats a higher vote in French
		artworkBackdrop: tmdbImages + "/dune-backdrop.jpg",  // textless first
		artworkLogo:     tmdbImages + "/dune-logo-en.png",
	} {
		if r := imgs[kind]; r.Origin != origin || r.Source != sourceProvider || len(r.Blurhash) != 28 {
			t.Errorf("%s: %+v, want from %s", kind, r, origin)
		}
	}

	select {
	case ev := <-sub.C():
		if ev.Kind != events.WorkChanged || !slices.Equal(ev.Scope.Libraries, []id.ID{lib}) || ev.Data != w {
			t.Errorf("event %+v", ev)
		}
	default:
		t.Error("no workChanged event")
	}

	// A second refresh finds everything current and encodes nothing.
	encoded := e.img.(*fakeImager).count()
	e.mustRefresh(w)
	if n := e.img.(*fakeImager).count(); n != encoded {
		t.Errorf("a refresh with nothing new encoded %d images", n-encoded)
	}
	// A movie made in English gets its images from its details alone.
	if n := e.fx.count("/3/movie/438631/images"); n != 0 {
		t.Errorf("Dune's images fetched apart %d times", n)
	}
}

// A movie made in another language gets every image TMDb holds, fetched
// once, because its posters may all be in its own language. A logo in that
// language is never used, since a logo stands in for the title.
func TestRefreshOriginalLanguageArtwork(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.b.Work(catalog.WorkInput{
		Kind: catalog.KindMovie, Title: "Aranmanai", Year: 2014,
		ExternalIDs: []catalog.ExternalID{catalogID("tmdb", "movie", "280116")},
	})
	e.b.Version([]id.ID{e.b.File(folder, "Aranmanai (2014)/Aranmanai (2014).mkv")}, w)
	e.mustRefresh(w)

	imgs := e.images(w)
	for kind, origin := range map[string]string{
		artworkPoster:   tmdbImages + "/aranmanai-poster-ta.jpg", // the original language beats a higher vote in Telugu
		artworkBackdrop: tmdbImages + "/aranmanai-backdrop.jpg",
		artworkLogo:     tmdbImages + "/aranmanai-logo.png",
	} {
		if r := imgs[kind]; r.Origin != origin {
			t.Errorf("%s: %+v, want from %s", kind, r, origin)
		}
	}

	want := []string{
		"poster provider ta selected",
		"poster provider ta",
		"poster provider te",
		"backdrop provider selected",
		"backdrop provider ta",
		"logo provider selected", // the Tamil logo isn't offered
	}
	if got := what(e.artwork(w, "")); !slices.Equal(got, want) {
		t.Errorf("listing:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if n := e.fx.count("/3/movie/280116/images"); n != 1 {
		t.Errorf("Aranmanai's images fetched %d times, want 1", n)
	}
}

func TestLangRank(t *testing.T) {
	order := func(kind string, langs ...string) []string {
		var out []string
		for _, l := range langs {
			if _, ok := langRank(kind, l, "de-DE", "ta"); ok {
				out = append(out, l)
			}
		}
		slices.SortStableFunc(out, func(a, b string) int {
			ra, _ := langRank(kind, a, "de-DE", "ta")
			rb, _ := langRank(kind, b, "de-DE", "ta")
			return ra - rb
		})
		return out
	}
	langs := []string{"te", "ta", "", "en", "de"}
	for kind, want := range map[string][]string{
		artworkPoster:   {"de", "en", "", "ta", "te"},
		artworkBackdrop: {"", "de", "en", "ta", "te"},
		artworkLogo:     {"de", "en", ""},
	} {
		if got := order(kind, langs...); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", kind, got, want)
		}
	}
}

// A movie's refresh fetches TMDb's photos of the people it credits, each
// once. A refresh that finds the same photo fetches nothing, and a person
// with no photo on TMDb gets none.
func TestRefreshCastPhotos(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.mustRefresh(w)

	photos := `SELECT p.name || ' ' || i.origin || ' ' || i.width FROM image i JOIN person p ON p.id = i.person_id
		WHERE i.kind = 'photo' ORDER BY p.name`
	want := []string{
		"Rebecca Ferguson " + tmdbProfiles + "/ferguson.jpg 300",
		"Timothée Chalamet " + tmdbProfiles + "/chalamet.jpg 300",
	}
	if got := e.strings(photos); !slices.Equal(got, want) {
		t.Errorf("photos %v, want %v", got, want)
	}
	fetched := e.fx.count("/t/p/h632/chalamet.jpg")
	if fetched != 1 {
		t.Errorf("Chalamet's photo fetched %d times, want 1", fetched)
	}
	e.mustRefresh(w)
	if got := e.fx.count("/t/p/h632/chalamet.jpg"); got != fetched {
		t.Errorf("a second refresh fetched the same photo again")
	}
	if got := e.strings(photos); !slices.Equal(got, want) {
		t.Errorf("after a second refresh, photos %v, want %v", got, want)
	}
}

// The content rating comes from the content rating country. A work with no
// rating there is unrated, and no other country's rating stands in.
func TestRefreshContentRatingCountry(t *testing.T) {
	e := newEnv(t)
	lib, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	for _, tc := range []struct{ country, cert, rank string }{{"DE", "12", "12"}, {"FR", "", ""}} {
		if err := e.cfg.Update(context.Background(), func(s *config.Settings) error {
			s.ContentRatingCountry = tc.country
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		e.mustRefresh(w)
		if c, r := e.column(w, "content_rating"), e.column(w, "content_rating_age"); c != tc.cert || r != tc.rank {
			t.Errorf("%s: content rating %q, rank %q, want %q, %q", tc.country, c, r, tc.cert, tc.rank)
		}
	}

	// A library's own country wins over the server's.
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, _, err := e.b.Catalog.UpdateLibrary(ctx, tx, lib, catalog.LibraryChange{ContentRatingCountry: new("DE")})
		return err
	})
	e.mustRefresh(w)
	if c := e.column(w, "content_rating"); c != "12" {
		t.Errorf("the library's DE over the server's FR: content rating %q, want 12", c)
	}
	if got := e.s.ContentRatings("DE"); len(got) == 0 || got[0].Name != "0" {
		t.Errorf("DE's content ratings %v", got)
	}
}

func TestRefreshSeries(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Shows)
	series := e.b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: "The Expanse",
		ExternalIDs: []catalog.ExternalID{catalogID("tmdb", "series", "63639")}})
	byID := e.b.Episode(folder, series, 1, 1)
	byNumber := e.b.Episode(folder, series, 2, 1)
	ep, err := e.b.Catalog.Work(ctx, byID.Work)
	if err != nil {
		t.Fatal(err)
	}
	season := ep.Parent
	// The file says S01E01, but its match names TMDb episode 1002. The ID
	// wins.
	e.b.Work(catalog.WorkInput{Kind: catalog.KindEpisode, Parent: season, Title: "Episode 1", ParentNumber: 1, Number: 1,
		ExternalIDs: []catalog.ExternalID{catalogID("tmdb", "episode", "1002")}})

	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.s.Refresh(ctx, tx, series) })
	got := e.tasks.take(TaskRefreshMetadata)
	if len(got) != 5 { // the series, two seasons, two episodes
		t.Fatalf("Refresh asked for %v", got)
	}
	for _, w := range got {
		e.mustRefresh(w)
	}

	if e.column(series, "original_language") != "en" || e.column(series, "title") != "The Expanse" || e.column(series, "content_rating") != "TV-14" || e.column(series, "content_rating_age") != "14" || e.column(series, "year") != "2015" {
		t.Errorf("series: %s %s %s %s", e.column(series, "title"), e.column(series, "content_rating"), e.column(series, "content_rating_age"), e.column(series, "year"))
	}
	if got := e.strings(`SELECT s.name FROM work_company ws JOIN company s ON s.id = ws.company_id WHERE ws.work_id = ? ORDER BY s.name`, series); !slices.Equal(got, []string{"Prime Video", "Syfy"}) {
		t.Errorf("networks %v", got)
	}
	if r := e.images(series)[artworkLogo]; r.Origin != tmdbImages+"/expanse-logo.png" {
		t.Errorf("series logo from %s", r.Origin)
	}
	if e.column(season, "title") != "Season 1" || e.images(season)[artworkPoster].Origin != tmdbImages+"/expanse-s1-poster.jpg" {
		t.Errorf("season: %q, poster %+v", e.column(season, "title"), e.images(season))
	}
	if e.column(byID.Work, "title") != "The Big Empty" || e.images(byID.Work)[artworkStill].Origin != tmdbImages+"/expanse-s1e2.jpg" {
		t.Errorf("episode by ID: %q, %+v", e.column(byID.Work, "title"), e.images(byID.Work))
	}
	if e.column(byNumber.Work, "title") != "Safe" || e.column(byNumber.Work, "release_date") != "2017-02-01" {
		t.Errorf("episode by number: %q", e.column(byNumber.Work, "title"))
	}
	// Community ratings, with none for an episode that has no votes.
	for w, want := range map[id.ID]string{series: "8.2/4321", byID.Work: "7.6/40", byNumber.Work: "/"} {
		if got := e.column(w, "community_rating") + "/" + e.column(w, "community_votes"); got != want {
			t.Errorf("%s: community rating %s, want %s", e.column(w, "title"), got, want)
		}
	}
}

func TestRefreshSkipsProvisionalWorks(t *testing.T) {
	e := newEnv(t)
	w := e.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Some File", Provisional: true})
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.s.Refresh(ctx, tx, w) })
	if got := e.tasks.take(TaskRefreshMetadata); len(got) != 0 {
		t.Fatalf("asked for %v for a provisional work", got)
	}
	err := e.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error { return e.s.Refresh(ctx, tx, id.New()) })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unknown work: %v", err)
	}
}

func TestLockedFieldsAndChosenArtworkSurvive(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.mustRefresh(w)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.EditMetadata(ctx, tx, w, catalog.WorkMetadata{Title: "Dune (Admin's Title)"}, catalog.FieldTitle)
	})
	before := e.images(w)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE image SET chosen = 1 WHERE kind = 'poster'`) // an admin chose the poster
		return err
	})

	e.mustRefresh(w)
	if got := e.column(w, "title"); got != "Dune (Admin's Title)" {
		t.Errorf("locked title became %q", got)
	}
	if !strings.HasPrefix(e.column(w, "overview"), "Paul Atreides") {
		t.Error("unlocked fields weren't refreshed")
	}
	if got := e.column(w, "community_rating"); got != "7.8" {
		t.Errorf("community rating %q after an admin's edit and a refresh", got)
	}
	after := e.images(w)
	if after[artworkPoster].ID != before[artworkPoster].ID || !after[artworkPoster].Chosen {
		t.Errorf("the chosen poster changed: %+v → %+v", before[artworkPoster], after[artworkPoster])
	}
	// The provider's backdrop URL is unchanged and its files are there. So
	// it is the same image, kept with its ID and not downloaded again.
	if after[artworkBackdrop].ID != before[artworkBackdrop].ID {
		t.Errorf("an unchanged backdrop was replaced: %+v → %+v", before[artworkBackdrop], after[artworkBackdrop])
	}
}

// Unmatching clears a work's artwork, including chosen artwork and its
// seasons' posters, and removes the files.
func TestClearArtwork(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Shows)
	series := e.b.Work(catalog.WorkInput{Kind: catalog.KindSeries, Title: "The Expanse",
		ExternalIDs: []catalog.ExternalID{catalogID("tmdb", "series", "63639")}})
	ep, err := e.b.Catalog.Work(ctx, e.b.Episode(folder, series, 1, 1).Work)
	if err != nil {
		t.Fatal(err)
	}
	season := ep.Parent
	e.mustRefresh(series)
	e.mustRefresh(season)
	poster := e.images(series)[artworkPoster]
	if poster.ID.IsZero() || len(e.images(season)) == 0 {
		t.Fatalf("artwork before: %+v, %+v", e.images(series), e.images(season))
	}
	path, err := e.s.ImagePath(ctx, poster.ID, 240)
	if err != nil {
		t.Fatal(err)
	}
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE image SET chosen = 1 WHERE id = ?`, poster.ID)
		return err
	})

	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.s.ClearArtwork(ctx, tx, series) })
	if len(e.images(series)) != 0 || len(e.images(season)) != 0 {
		t.Fatalf("artwork left: %+v, %+v", e.images(series), e.images(season))
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the poster's file is still there: %v", err)
	}
}

// A refresh in flight while its root work's match changes writes the old
// match's details. The change asks for the root's refresh again, and the
// task does it once the first is over. The root ends up with the new
// match's details.
func TestRefreshInFlightAcrossAMatchChange(t *testing.T) {
	e := newEnv(t)
	r := tasks.New(e.b.DB, e.cfg, events.NewBus(), tasks.Options{})
	for _, tk := range e.s.Tasks() {
		r.Add(tk)
	}
	e.s.deps.Tasks = r
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	var once sync.Once
	e.fx.hook = func(path string) {
		if path != moviePath {
			return
		}
		once.Do(func() { // Fix match, as the first refresh waits on the provider
			err := e.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
				err := e.b.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error {
					return b.Describe(ctx, w, catalogID("tmdb", "movie", "280116"), "Aranmanai", 2014, true)
				})
				if err != nil {
					return err
				}
				return e.s.Refresh(ctx, tx, w)
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	rctx, cancel := context.WithCancel(ctx)
	ran := make(chan error, 1)
	go func() { ran <- r.Run(rctx) }()
	defer func() {
		cancel()
		<-ran
	}()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.s.Refresh(ctx, tx, w) })

	for deadline := time.Now().Add(10 * time.Second); !strings.HasPrefix(e.column(w, "overview"), "A family"); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the title ended as %q: %q", e.column(w, "title"), e.column(w, "overview"))
		}
	}
	if e.fx.count(moviePath) != 1 || e.fx.count("/3/movie/280116") == 0 {
		t.Errorf("fetched the old match %d times, the new one %d", e.fx.count(moviePath), e.fx.count("/3/movie/280116"))
	}
	if r := e.images(w)[artworkPoster]; r.Origin != tmdbImages+"/aranmanai-poster-ta.jpg" {
		t.Errorf("poster %+v, want the new match's", r)
	}
}

// A refresh in flight while its root work is unmatched writes nothing,
// records nothing and keeps no artwork, because nothing describes the root any
// more.
func TestRefreshInFlightAcrossUnmatch(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.fx.hook = func(path string) {
		if path != moviePath {
			return
		}
		err := e.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			return e.b.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error { return b.Unmatch(ctx, w, "Dune", 2021) })
		})
		if err != nil {
			t.Error(err)
		}
	}
	if err := e.s.refreshWork(ctx, w); err != nil {
		t.Fatal(err)
	}
	if e.column(w, "overview") != "" || len(e.images(w)) != 0 || len(e.credits(w)) != 0 {
		t.Fatalf("an unmatched root work was described: %q, %v, %v", e.column(w, "overview"), e.images(w), e.credits(w))
	}
	if e.column(w, "refreshed_at") != "" || e.column(w, "refresh_error") != "" {
		t.Fatalf("an unmatched root work's refresh recorded %q, %q", e.column(w, "refreshed_at"), e.column(w, "refresh_error"))
	}
	var files []string
	filepath.WalkDir(e.cache, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 0 {
		t.Fatalf("discarded artwork left behind: %v", files)
	}
}

// A work merged or deleted before its refresh has nothing to do.
func TestRefreshWorkGone(t *testing.T) {
	e := newEnv(t)
	if err := e.s.refreshWork(ctx, id.New()); err != nil {
		t.Fatalf("a missing work: %v", err)
	}
}

// A refresh records when the work was refreshed, in the same write as the
// metadata.
func TestRefreshRecordsWhen(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.exec(`UPDATE work SET refreshed_at = 0`)
	before := time.Now().UnixMilli()
	e.mustRefresh(w)
	if at := e.refreshedAt(w); at < before {
		t.Errorf("refreshed_at %d, want from %d", at, before)
	}
}

// The works due are those never refreshed, and those refreshed longer ago
// than their library's refresh days, oldest first. Each work counts on its
// own, so a series, its season and its episode each go by their own date.
// Provisional works and extras are never due. A library set to never only
// refreshes works that were never refreshed.
func TestDueRefreshes(t *testing.T) {
	e := newEnv(t)
	lib, folder := e.b.Library(catalog.Movies)
	shows, showFolder := e.b.Library(catalog.Shows)
	_, neverFolder := e.b.Library(catalog.Movies)
	e.exec(`UPDATE library SET refresh_days = 30 WHERE id IN (?, ?)`, lib, shows)
	old := e.b.Movie(folder, "Old", 2000)
	fresh := e.b.Movie(folder, "New", 2001).Work
	kept := e.b.Movie(neverFolder, "Kept", 2002).Work
	provisional := e.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Some File", Provisional: true})
	e.b.Version([]id.ID{e.b.File(folder, "Some File.mkv")}, provisional)
	extra := e.b.Work(catalog.WorkInput{Kind: catalog.KindExtra, Parent: old.Work, Title: "Trailer", ExtraType: "trailer"})
	e.b.Version([]id.ID{e.b.File(folder, "Old (2000)/Trailer.mkv")}, extra)
	series := e.b.Series("Show")
	ep := e.b.Episode(showFolder, series, 1, 1)
	episode, err := e.b.Catalog.Work(ctx, ep.Work)
	if err != nil {
		t.Fatal(err)
	}
	e.exec(`UPDATE work SET refreshed_at = ?`, daysAgo(40))
	e.exec(`UPDATE work SET refreshed_at = ? WHERE id IN (?, ?)`, daysAgo(10), fresh, series)
	e.exec(`UPDATE work SET refreshed_at = ? WHERE id = ?`, daysAgo(50), old.Work)
	e.exec(`UPDATE work SET refreshed_at = NULL WHERE id IN (?, ?, ?)`, ep.Work, provisional, extra)

	got := e.due()
	if len(got) != 3 || got[0] != ep.Work || got[1] != old.Work || got[2] != episode.Parent {
		t.Fatalf("due %v, want the episode, the old movie and the season, %v %v %v", got, ep.Work, old.Work, episode.Parent)
	}
	e.exec(`UPDATE work SET refreshed_at = NULL WHERE id = ?`, kept)
	if got := e.due(); !slices.Contains(got, kept) {
		t.Errorf("due %v: a work never refreshed in a library set to never", got)
	}
	if ok, err := e.s.q.RefreshWanted(ctx); err != nil || !ok {
		t.Errorf("wanted %v, %v", ok, err)
	}
	e.exec(`UPDATE library SET refresh_days = 0`)
	if ok, err := e.s.q.RefreshWanted(ctx); err != nil || ok {
		t.Errorf("wanted with every library set to never: %v, %v", ok, err)
	}
}

func daysAgo(days int) int64 {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
}

// Refresh makes the works due in the caller's transaction, and asks for them
// only once it commits. It doesn't ask before the commit, or at all if the
// transaction rolls back.
func TestRefreshAsksAfterCommit(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.exec(`UPDATE work SET refreshed_at = 1, refresh_error = 'old'`)

	errRollback := errors.New("rollback")
	err := e.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if err := e.s.Refresh(ctx, tx, w); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
	if got := e.tasks.take(TaskRefreshMetadata); len(got) != 0 {
		t.Fatalf("a rolled back refresh asked for %v", got)
	}
	if e.column(w, "refreshed_at") != "1" {
		t.Fatalf("a rolled back refresh left refreshed_at %q", e.column(w, "refreshed_at"))
	}

	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if err := e.s.Refresh(ctx, tx, w); err != nil {
			return err
		}
		if got := e.tasks.take(TaskRefreshMetadata); len(got) != 0 {
			t.Errorf("asked for %v before the commit", got)
		}
		return nil
	})
	if got := e.tasks.take(TaskRefreshMetadata); !slices.Equal(got, []id.ID{w}) {
		t.Fatalf("asked for %v, want %v", got, w)
	}
	if e.column(w, "refreshed_at") != "" || e.column(w, "refresh_error") != "" {
		t.Fatalf("not due: refreshed_at %q, refresh_error %q", e.column(w, "refreshed_at"), e.column(w, "refresh_error"))
	}
	if got := e.due(); !slices.Equal(got, []id.ID{w}) {
		t.Fatalf("due %v", got)
	}

	// RefreshWorks makes only the work itself due.
	e.exec(`UPDATE work SET refreshed_at = ?`, time.Now().UnixMilli())
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.s.RefreshWorks(ctx, tx, w) })
	if got := e.tasks.take(TaskRefreshMetadata); !slices.Equal(got, []id.ID{w}) || e.column(w, "refreshed_at") != "" {
		t.Fatalf("RefreshWorks asked for %v, refreshed_at %q", got, e.column(w, "refreshed_at"))
	}
}

// A refresh that fails for a reason other than an outage is recorded with
// its reason. The work leaves the due list until its library's refresh days
// pass. A retry makes it due again, and a successful refresh clears the
// reason.
func TestFailedRefresh(t *testing.T) {
	e := newEnv(t)
	lib, folder := e.b.Library(catalog.Movies)
	e.exec(`UPDATE library SET refresh_days = 30 WHERE id = ?`, lib)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.exec(`UPDATE work SET refreshed_at = NULL`)
	e.fx.set(moviePath, http.StatusForbidden)

	before := time.Now().UnixMilli()
	if err := e.s.refreshWork(ctx, w); err != nil {
		t.Fatal(err)
	}
	if reason := e.column(w, "refresh_error"); !strings.Contains(reason, "403") {
		t.Fatalf("refresh_error %q", reason)
	}
	if at := e.refreshedAt(w); at < before {
		t.Fatalf("refreshed_at %d, want from %d", at, before)
	}
	if got := e.due(); len(got) != 0 {
		t.Fatalf("due %v after a failed refresh", got)
	}
	failed, err := e.s.failedRefreshes(ctx)
	if err != nil || len(failed) != 1 || failed[0].Item != w || !strings.Contains(failed[0].Reason, "403") {
		t.Fatalf("failed %+v, %v", failed, err)
	}
	if left, err := e.s.pendingRefreshes(ctx); err != nil || len(left) != 0 {
		t.Fatalf("%d left: %v", left, err)
	}

	// Once its refresh days pass, it is due again, like any refreshed work.
	e.exec(`UPDATE work SET refreshed_at = ?`, daysAgo(40))
	if got := e.due(); !slices.Equal(got, []id.ID{w}) {
		t.Fatalf("due %v once its days passed", got)
	}
	e.exec(`UPDATE work SET refreshed_at = ?`, time.Now().UnixMilli())

	got, err := e.s.retryRefreshes(ctx)
	if err != nil || !slices.Equal(got, []id.ID{w}) {
		t.Fatalf("retry %v, %v", got, err)
	}
	if got := e.due(); !slices.Equal(got, []id.ID{w}) {
		t.Fatalf("due %v after a retry", got)
	}
	if failed, err := e.s.failedRefreshes(ctx); err != nil || len(failed) != 0 {
		t.Fatalf("failed %+v after a retry: %v", failed, err)
	}

	e.fx.set(moviePath, 0)
	if err := e.s.refreshWork(ctx, w); err != nil {
		t.Fatal(err)
	}
	if e.column(w, "refresh_error") != "" || len(e.due()) != 0 {
		t.Fatalf("after a refresh that worked: refresh_error %q, due %v", e.column(w, "refresh_error"), e.due())
	}
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
}

func (e *env) refreshedAt(work id.ID) int64 {
	e.t.Helper()
	var at int64
	if err := e.b.DB.Reader().QueryRow(`SELECT refreshed_at FROM work WHERE id = ?`, work).Scan(&at); err != nil {
		e.t.Fatal(err)
	}
	return at
}

// A refresh asks the provider again for responses cached more than a day
// ago, even if they haven't expired. A refresh within the day uses the
// cache.
func TestRefreshWorkRefetches(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.mustRefresh(w)
	run := func() {
		t.Helper()
		if err := e.s.refreshWork(ctx, w); err != nil {
			t.Fatal(err)
		}
	}
	before := e.fx.count(moviePath)
	run()
	if n := e.fx.count(moviePath); n != before {
		t.Fatalf("fetched the movie %d more times within the day", n-before)
	}
	e.exec(`UPDATE provider_cache SET fetched_at = ?`, time.Now().Add(-2*24*time.Hour).UnixMilli())
	run()
	if n := e.fx.count(moviePath); n != before+1 {
		t.Fatalf("fetched the movie %d more times after two days, want 1", n-before)
	}
}

// A refresh doesn't use an old cached answer while the provider is down. It
// waits out the outage, recording nothing, and tries again once it ends.
func TestRefreshWorkWaitsOutAnOutage(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.mustRefresh(w)
	e.exec(`UPDATE provider_cache SET fetched_at = ?`, time.Now().Add(-2*24*time.Hour).UnixMilli())
	e.exec(`UPDATE work SET refreshed_at = 0`)
	down := func(d time.Duration) { // records a first failure (a 30-second rest), dated so the rest ends d from now
		p := e.s.fetcher.provider(provTMDb)
		p.Recovered()
		p.Failed(time.Now().Add(d-30*time.Second), 0)
	}

	// An outage longer than the task's time limit is waited out until the
	// task's context ends.
	down(time.Hour)
	tctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := e.s.refreshWork(tctx, w); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a refresh during an outage: %v", err)
	}
	if at := e.refreshedAt(w); at != 0 || e.column(w, "refresh_error") != "" {
		t.Fatalf("recorded a refresh during an outage: %d, %q", at, e.column(w, "refresh_error"))
	}

	// An outage that ends is waited out, and then the refresh succeeds.
	hits := e.fx.count(moviePath)
	down(200 * time.Millisecond)
	start := time.Now()
	if err := e.s.refreshWork(ctx, w); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 200*time.Millisecond {
		t.Errorf("refreshed after %v, before the outage ended", waited)
	}
	if e.fx.count(moviePath) != hits+1 || e.refreshedAt(w) < start.UnixMilli() || e.column(w, "refresh_error") != "" {
		t.Fatalf("after the outage: %d fetches, refreshed_at %d, refresh_error %q", e.fx.count(moviePath)-hits, e.refreshedAt(w), e.column(w, "refresh_error"))
	}
}
