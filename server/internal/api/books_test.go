package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata"
)

func TestBooks(t *testing.T) {
	e := newEnv(t, serving{})
	lib, folder := e.b.Library(catalog.Audiobooks)
	book := func(rel string, m catalog.WorkMetadata) id.ID {
		w := e.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: m.Title})
		e.b.Version([]id.ID{e.b.File(folder, rel)}, w)
		e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.b.Catalog.UpdateMetadata(ctx, tx, w, m) })
		return w
	}
	expanse := func(pos string) []catalog.SeriesRef {
		return []catalog.SeriesRef{{Key: catalog.LocalSeriesKey("James S. A. Corey", "Expanse"), Name: "Expanse", Position: pos, Primary: true}}
	}
	churn := book("James S. A. Corey/Expanse/Book 1.5 - The Churn/a.m4b", catalog.WorkMetadata{Title: "The Churn",
		Book: &catalog.BookMetadata{Authors: []string{"James S. A. Corey"}, Narrators: []string{"Ray Porter"}, Series: expanse("1.5")}})
	lev := book("James S. A. Corey/Expanse/Book 1 - Leviathan Wakes/a.m4b", catalog.WorkMetadata{Title: "Leviathan Wakes",
		Book: &catalog.BookMetadata{Authors: []string{"James S. A. Corey"}, Narrators: []string{"Jefferson Mays"}, Series: expanse("1")}})

	// The library's tabs.
	tabs := "/api/v1/libraries/" + lib.String()
	authors := decode[list](t, e.do("GET", tabs+"/authors", e.adminToken, nil))
	if len(authors.Items) != 1 || authors.Items[0]["role"] != "author" || authors.Items[0]["name"] != "James S. A. Corey" ||
		authors.Items[0]["workCount"] != 2.0 || *authors.Total != 1 || len(authors.Letters) != 1 {
		t.Fatalf("authors %+v", authors)
	}
	series := decode[list](t, e.do("GET", tabs+"/book-series", e.adminToken, nil))
	if len(series.Items) != 1 || series.Items[0]["name"] != "Expanse" || series.Items[0]["bookCount"] != 2.0 {
		t.Fatalf("series %v", series.Items)
	}
	seriesID := series.Items[0]["id"].(string)
	books := decode[list](t, e.grid(e.adminToken, lib, "book", map[string]any{"sort": sortBy("author")}))
	if len(books.Items) != 2 || books.Items[0]["book"].(map[string]any)["authors"].([]any)[0].(map[string]any)["name"] != "James S. A. Corey" {
		t.Fatalf("books %v", books.Items)
	}

	// A series' page, renamed to another series' name, merges into it.
	page := decode[map[string]any](t, e.do("GET", "/api/v1/book-series/"+seriesID, e.adminToken, nil))
	if items := page["items"].([]any); len(items) != 2 || items[0].(map[string]any)["title"] != "Leviathan Wakes" {
		t.Fatalf("series page %v", page)
	}
	w := e.do("PATCH", "/api/v1/book-series/"+seriesID, e.adminToken, map[string]string{"name": "The Expanse"})
	if renamed := decode[map[string]any](t, w); w.Code != http.StatusOK || renamed["name"] != "The Expanse" || renamed["id"] != seriesID {
		t.Fatalf("rename series: %d %v", w.Code, renamed)
	}

	// A batch edit, after the rename has already locked the series. Then
	// an author's page groups the books by series.
	w = e.do("PATCH", "/api/v1/works", e.adminToken, map[string]any{
		"workIds": []string{churn.String(), lev.String()}, "narrators": []string{"Jefferson Mays"}, "genres": []string{"Science Fiction"},
	})
	if edited := decode[list](t, w); w.Code != http.StatusOK || len(edited.Items) != 2 {
		t.Fatalf("batch edit: %d %v", w.Code, edited)
	}
	detail := decode[map[string]any](t, e.do("GET", "/api/v1/works/"+churn.String(), e.adminToken, nil))
	b := detail["book"].(map[string]any)
	if b["narrators"].([]any)[0].(map[string]any)["name"] != "Jefferson Mays" || len(detail["lockedFields"].([]any)) != 3 {
		t.Errorf("after the batch edit %v %v", b, detail["lockedFields"])
	}
	personID := b["authors"].([]any)[0].(map[string]any)["id"].(string)
	person := decode[map[string]any](t, e.do("GET", "/api/v1/people/"+personID, e.adminToken, nil))
	if groups := person["bookSeries"].([]any); len(groups) != 1 || len(groups[0].(map[string]any)["items"].([]any)) != 2 {
		t.Errorf("author page %v", person)
	}

	// The edit of a single book, and a batch that is refused because it
	// includes a movie.
	w = e.do("PATCH", "/api/v1/works/"+lev.String(), e.adminToken, map[string]any{"book": map[string]any{
		"subtitle": "Expanse 1", "series": []map[string]any{{"name": "The Expanse", "position": "1"}}, "asin": "B0055VXLQS",
	}})
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["book"].(map[string]any)["subtitle"] != "Expanse 1" {
		t.Errorf("edit a book: %d %v", w.Code, got)
	}
	w = e.do("PATCH", "/api/v1/works", e.adminToken, map[string]any{"workIds": []string{e.heat.Work.String()}, "genres": []string{"x"}})
	if w.Code != http.StatusBadRequest {
		t.Errorf("a movie in a batch edit: %d", w.Code)
	}
	many := make([]string, 501)
	for i := range many {
		many[i] = churn.String()
	}
	w = e.do("PATCH", "/api/v1/works", e.adminToken, map[string]any{"workIds": many, "genres": []string{"x"}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "tooLarge") {
		t.Errorf("a batch edit of 501 works: %d %s", w.Code, w.Body.String())
	}
	// Viewers without the library see none of it.
	for _, p := range []string{"/api/v1/people/" + personID, "/api/v1/book-series/" + seriesID} {
		if w := e.do("GET", p, e.nobodyToken, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s for nobody: %d", p, w.Code)
		}
	}
}

// Match on a book stores the chosen ASIN as locked and queues a refresh of
// the book. Only an Audible book can be chosen. An author's page shows the
// biography and photo the provider gave.
func TestBookMatchAndAuthorPage(t *testing.T) {
	e := newEnv(t, serving{})
	_, folder := e.b.Library(catalog.Audiobooks)
	w := e.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: "Project Hail Mary"})
	e.b.Version([]id.ID{e.b.File(folder, "Andy Weir/Project Hail Mary/a.m4b")}, w)
	weir := catalog.ExternalID{Provider: "audible", Kind: "author", ID: "B00G0WYW92"}
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.UpdateMetadata(ctx, tx, w, catalog.WorkMetadata{Title: "Project Hail Mary", Book: &catalog.BookMetadata{
			Authors: []string{"Andy Weir"}, AuthorIDs: map[string][]catalog.ExternalID{"Andy Weir": {weir}},
		}})
	})

	path := "/api/v1/works/" + w.String() + "/match"
	if r := e.do("POST", path, e.adminToken, map[string]string{"matchId": "tmdb:movie:438631"}); r.Code != http.StatusBadRequest {
		t.Errorf("a movie for a book: %d", r.Code)
	}
	r := e.do("POST", path, e.adminToken, map[string]string{"matchId": "audible:book:B08G9PRS1K"})
	got := decode[map[string]any](t, r)
	if ids := got["externalIds"].([]any); r.Code != http.StatusOK || len(ids) != 1 || ids[0].(map[string]any)["id"] != "B08G9PRS1K" {
		t.Fatalf("match: %d %v", r.Code, got)
	}
	if locked := got["lockedFields"].([]any); len(locked) != 1 || locked[0] != "book.asin" {
		t.Errorf("locked %v", locked)
	}
	if asked := e.asks.of(metadata.TaskRefreshMetadata); !slices.Equal(asked, []id.ID{w}) {
		t.Errorf("refreshes asked for: %v", asked)
	}
	// The tests run without a book provider, so Match's search is
	// unavailable.
	if r := e.do("GET", "/api/v1/works/"+w.String()+"/matches", e.adminToken, nil); r.Code != http.StatusServiceUnavailable {
		t.Errorf("search: %d", r.Code)
	}

	photo := id.New()
	var person id.ID
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		if person, err = e.b.Catalog.DescribePerson(ctx, tx, weir, "He lives in California."); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO image (id, person_id, kind, source, origin, width, height, blurhash, fetched_at)
			VALUES (?, ?, 'photo', 'provider', 'https://m.media-amazon.com/a.jpg', 500, 500, 'LKO2', 1)`, photo, person)
		return err
	})
	page := decode[map[string]any](t, e.do("GET", "/api/v1/people/"+person.String(), e.adminToken, nil))
	if page["overview"] != "He lives in California." || page["photo"].(map[string]any)["id"] != photo.String() {
		t.Errorf("author page %v", page)
	}
}
