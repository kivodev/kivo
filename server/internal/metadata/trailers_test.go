package metadata

import (
	"context"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
)

func TestTrailersOf(t *testing.T) {
	trailer := func(key, lang string, official bool, published string) tmdbVideo {
		return tmdbVideo{Key: key, Name: "Trailer " + key, Site: "YouTube", Type: "Trailer", Official: official, Lang: lang, Published: published}
	}
	const older, newer = "2020-01-01T00:00:00.000Z", "2021-01-01T00:00:00.000Z"
	for _, tc := range []struct {
		name, lang string
		videos     []tmdbVideo
		want       []string
	}{
		{"none", "en-US", nil, nil},
		{"official over language", "de-DE", []tmdbVideo{trailer("de", "de", false, newer), trailer("en", "en", true, older)}, []string{"en", "de"}},
		{"metadata language over English", "de-DE", []tmdbVideo{trailer("en", "en", true, newer), trailer("de", "de", true, older)}, []string{"de", "en"}},
		{"English when the language has none", "fr", []tmdbVideo{trailer("es", "es", true, newer), trailer("en", "en", true, older)}, []string{"en", "es"}},
		{"newest within a language", "en-US", []tmdbVideo{trailer("old", "en", true, older), trailer("new", "en", true, newer)}, []string{"new", "old"}},
		{"another language when nothing else", "fr", []tmdbVideo{trailer("es", "es", true, older)}, []string{"es"}},
		{"no teasers, clips or featurettes", "en", []tmdbVideo{
			{Key: "teaser", Site: "YouTube", Type: "Teaser", Official: true, Lang: "en", Published: newer},
			{Key: "clip", Site: "YouTube", Type: "Clip", Official: true, Lang: "en", Published: newer},
			{Key: "featurette", Site: "YouTube", Type: "Featurette", Official: true, Lang: "en", Published: newer},
			trailer("only", "en", false, older),
		}, []string{"only"}},
		{"YouTube only", "en", []tmdbVideo{{Key: "vimeo", Site: "Vimeo", Type: "Trailer", Official: true, Lang: "en", Published: newer}}, nil},
		{"a video listed twice once", "en", []tmdbVideo{trailer("a", "en", true, newer), trailer("a", "en", true, older)}, []string{"a"}},
		{"the best five", "en", []tmdbVideo{
			trailer("u1", "en", false, newer), trailer("o1", "en", true, "2021-01-01T00:00:00.000Z"), trailer("o2", "en", true, "2021-02-01T00:00:00.000Z"),
			trailer("o3", "en", true, "2021-03-01T00:00:00.000Z"), trailer("o4", "en", true, "2021-04-01T00:00:00.000Z"),
			trailer("o5", "fr", true, "2021-05-01T00:00:00.000Z"), trailer("o6", "fr", true, "2021-06-01T00:00:00.000Z"),
		}, []string{"o4", "o3", "o2", "o1", "o6"}},
	} {
		var got []string
		for _, tr := range trailersOf(tc.videos, tc.lang) {
			if tr.Site != catalog.TrailerYouTube || tr.Title != "Trailer "+tr.Key {
				t.Errorf("%s: trailer %+v", tc.name, tr)
			}
			got = append(got, tr.Key)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := trailersOf(nil, "en"); got == nil {
		t.Error("no trailers is nil, which wouldn't clear a movie's trailers")
	}
}

// trailers lists a work's trailers in order, as "key title thumbnail's
// origin".
func (e *env) trailers(work id.ID) []string {
	e.t.Helper()
	return e.strings(`SELECT t.key || ' ' || t.title || ' ' || coalesce(i.origin, '-') FROM trailer t
		LEFT JOIN image i ON i.trailer_id = t.id AND i.kind = 'thumb' WHERE t.work_id = ? ORDER BY t.ord`, work)
}

// thumbFiles returns the cached files of each of a work's trailer
// thumbnails, by the trailer's key.
func (e *env) thumbFiles(work id.ID) map[string][]string {
	e.t.Helper()
	out := map[string][]string{}
	for _, key := range e.strings(`SELECT key FROM trailer WHERE work_id = ?`, work) {
		r, err := e.s.q.TrailerThumb(ctx, dbq.TrailerThumbParams{WorkID: work, Site: catalog.TrailerYouTube, Key: key})
		if err != nil {
			continue
		}
		for _, w := range imageWidths[artworkThumb] {
			out[key] = append(out[key], e.s.path(r.ID, w))
		}
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const ytimg = "https://i.ytimg.com/vi/"

func TestRefreshTrailers(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	// The older trailer has no HD thumbnail.
	e.fx.set("/vi/8g18jFHCLXk/maxresdefault.jpg", http.StatusNotFound)

	e.mustRefresh(w)

	// The newest official trailer first, and no teaser.
	want := []string{
		"n9xhJrPXop4 Official Main Trailer " + ytimg + "n9xhJrPXop4/maxresdefault.jpg",
		"8g18jFHCLXk Official Trailer " + ytimg + "8g18jFHCLXk/sddefault.jpg",
	}
	if got := e.trailers(w); !slices.Equal(got, want) {
		t.Fatalf("trailers %v, want %v", got, want)
	}
	files := e.thumbFiles(w)
	for key, ps := range files {
		for _, p := range ps {
			if !exists(p) {
				t.Errorf("%s: no thumbnail file %s", key, p)
			}
		}
	}
	if len(files) != 2 {
		t.Fatalf("thumbnail files %v", files)
	}
	if got := e.strings(`SELECT i.kind || ' ' || i.source || ' ' || i.width || 'x' || i.height || ' ' || length(i.blurhash) FROM image i
		WHERE i.trailer_id IS NOT NULL`); !slices.Equal(got, []string{"thumb provider 320x180 28", "thumb provider 320x180 28"}) {
		t.Errorf("thumbnail rows %v", got)
	}

	// A second refresh keeps each trailer's row and thumbnail, and fetches
	// nothing.
	ids := e.strings(`SELECT hex(t.id) || hex(i.id) FROM trailer t JOIN image i ON i.trailer_id = t.id WHERE t.work_id = ? ORDER BY t.ord`, w)
	hits := e.fx.count("/vi/n9xhJrPXop4/maxresdefault.jpg")
	e.mustRefresh(w)
	if got := e.strings(`SELECT hex(t.id) || hex(i.id) FROM trailer t JOIN image i ON i.trailer_id = t.id WHERE t.work_id = ? ORDER BY t.ord`, w); !slices.Equal(got, ids) {
		t.Errorf("a second refresh changed the trailers: %v, were %v", got, ids)
	}
	if got := e.fx.count("/vi/n9xhJrPXop4/maxresdefault.jpg"); got != hits {
		t.Errorf("a second refresh fetched the thumbnail again: %d requests, were %d", got, hits)
	}

	// A trailer the provider no longer lists goes, with its thumbnail.
	gone := id.New()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.UpdateMetadata(ctx, tx, w, catalog.WorkMetadata{Trailers: []catalog.Trailer{
			{Site: catalog.TrailerYouTube, Key: "n9xhJrPXop4", Title: "Official Main Trailer"},
			{Site: catalog.TrailerYouTube, Key: "8g18jFHCLXk", Title: "Official Trailer"},
			{Site: catalog.TrailerYouTube, Key: "withdrawn", Title: "Withdrawn"},
		}})
	})
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO image (id, trailer_id, kind, source, origin, width, height, blurhash, fetched_at)
			SELECT ?, id, 'thumb', 'provider', ?, 320, 180, 'x', 1 FROM trailer WHERE key = 'withdrawn'`, gone, ytimg+"withdrawn/maxresdefault.jpg")
		return err
	})
	for _, width := range imageWidths[artworkThumb] {
		writeFile(t, e.s.path(gone, width), []byte("webp"))
	}
	e.mustRefresh(w)
	if got := e.trailers(w); !slices.Equal(got, want) {
		t.Errorf("trailers after one was withdrawn %v, want %v", got, want)
	}
	for _, width := range imageWidths[artworkThumb] {
		if exists(e.s.path(gone, width)) {
			t.Errorf("the withdrawn trailer's thumbnail file is still there at %d", width)
		}
	}

	// Unmatching clears the artwork and then the work, which leaves no
	// trailer, no thumbnail and no file.
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error {
			if err := e.s.ClearArtwork(ctx, tx, w); err != nil {
				return err
			}
			return b.Unmatch(ctx, w, "Dune", 2021)
		})
	})
	if got := e.strings(`SELECT key FROM trailer UNION ALL SELECT hex(id) FROM image WHERE trailer_id IS NOT NULL`); len(got) != 0 {
		t.Errorf("after unmatching: %v", got)
	}
	for key, ps := range files {
		for _, p := range ps {
			if exists(p) {
				t.Errorf("%s: thumbnail file %s is still there after unmatching", key, p)
			}
		}
	}
}

// A thumbnail that can't be fetched leaves the trailer without one, and the
// refresh succeeds.
func TestRefreshTrailerWithoutThumbnail(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	for _, name := range []string{"maxresdefault", "sddefault", "hqdefault"} {
		e.fx.set("/vi/n9xhJrPXop4/"+name+".jpg", http.StatusNotFound)
	}
	e.fx.set("/vi/8g18jFHCLXk/maxresdefault.jpg", http.StatusForbidden)

	e.mustRefresh(w)

	want := []string{"n9xhJrPXop4 Official Main Trailer -", "8g18jFHCLXk Official Trailer -"}
	if got := e.trailers(w); !slices.Equal(got, want) {
		t.Fatalf("trailers %v, want %v", got, want)
	}
	if e.column(w, "refresh_error") != "" || e.column(w, "title") != "Dune" {
		t.Errorf("the refresh: error %q, title %q", e.column(w, "refresh_error"), e.column(w, "title"))
	}

	// The next refresh fetches what the last couldn't.
	e.fx.set("/vi/8g18jFHCLXk/maxresdefault.jpg", 0)
	e.mustRefresh(w)
	want[1] = "8g18jFHCLXk Official Trailer " + ytimg + "8g18jFHCLXk/maxresdefault.jpg"
	if got := e.trailers(w); !slices.Equal(got, want) {
		t.Errorf("trailers after a second refresh %v, want %v", got, want)
	}
}

// A trailer's thumbnail is served like any image, and the prune keeps its
// files.
func TestTrailerThumbnailIsAnImage(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.mustRefresh(w)

	r, err := e.s.q.TrailerThumb(ctx, dbq.TrailerThumbParams{WorkID: w, Site: catalog.TrailerYouTube, Key: "n9xhJrPXop4"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.s.ImagePath(ctx, r.ID, 400)
	if err != nil || p != e.s.path(r.ID, 640) {
		t.Fatalf("ImagePath = %s, %v, want the 640 file", p, err)
	}
	old := time.Now().Add(-2 * pruneAge)
	for _, width := range imageWidths[artworkThumb] {
		if err := os.Chtimes(e.s.path(r.ID, width), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.s.pruneImages(ctx); err != nil {
		t.Fatal(err)
	}
	if !exists(p) {
		t.Error("the prune removed a trailer's thumbnail")
	}

	// Trailers deleted with their movie take their image rows with them.
	// The prune then removes the files.
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM trailer WHERE work_id = ?`, w)
		return err
	})
	if got := e.strings(`SELECT hex(id) FROM image WHERE trailer_id IS NOT NULL`); len(got) != 0 {
		t.Errorf("after deleting the trailers: %v", got)
	}
	if _, err := e.s.pruneImages(ctx); err != nil {
		t.Fatal(err)
	}
	if exists(p) {
		t.Error("the prune kept a deleted trailer's thumbnail")
	}
}
