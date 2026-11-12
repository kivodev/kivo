package metadata

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// book adds a one-file book at rel in an audiobooks library and returns its
// work. The file is probed with tags and a running time of d.
func (e *env) book(rel string, tags map[string]string, d time.Duration) id.ID {
	e.t.Helper()
	var folder id.ID
	if err := e.b.DB.Reader().QueryRow(`SELECT r.id FROM library_folder r JOIN library l ON l.id = r.library_id WHERE l.kind = 'audiobooks'`).Scan(&folder); errors.Is(err, sql.ErrNoRows) {
		_, folder = e.b.Library(catalog.Audiobooks)
	} else if err != nil {
		e.t.Fatal(err)
	}
	f := e.b.File(folder, rel)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.SetProbe(ctx, tx, f, 1, catalog.Probe{Summary: catalog.ProbeSummary{Container: "mov,mp4,m4a", Duration: d, Tags: tags}})
	})
	w := e.b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: BookTitle(rel, tags)})
	e.b.Version([]id.ID{f}, w)
	return w
}

func (e *env) genres(work id.ID) []string {
	return e.strings(`SELECT g.name FROM work_genre wg JOIN genre g ON g.id = wg.genre_id WHERE wg.work_id = ? ORDER BY g.name`, work)
}

func (e *env) tags(work id.ID) []string {
	return e.strings(`SELECT t.name FROM work_tag wt JOIN tag t ON t.id = wt.tag_id WHERE wt.work_id = ? ORDER BY t.name`, work)
}

func (e *env) asin(work id.ID) string {
	return strings.Join(e.strings(`SELECT ext_id FROM work_external_id WHERE work_id = ? AND provider = 'audible' AND kind = 'book'`, work), ",")
}

func (e *env) publisher(work id.ID) string {
	return strings.Join(e.strings(`SELECT c.name FROM work_company wc JOIN company c ON c.id = wc.company_id WHERE wc.work_id = ?`, work), ",")
}

// author returns a book's first author: their ID, name, biography, Audible
// author IDs, and how many photos they have.
func (e *env) author(work id.ID) (person id.ID, name, bio string, ids []string, photos int) {
	e.t.Helper()
	var overview sql.NullString
	err := e.b.DB.Reader().QueryRow(`SELECT p.id, p.name, p.overview FROM credit c JOIN person p ON p.id = c.person_id
		WHERE c.work_id = ? AND c.role = 'author' ORDER BY c.ord LIMIT 1`, work).Scan(&person, &name, &overview)
	if err != nil {
		e.t.Fatal(err)
	}
	ids = e.strings(`SELECT ext_id FROM person_external_id WHERE person_id = ? AND provider = 'audible' AND kind = 'author'`, person)
	if err := e.b.DB.Reader().QueryRow(`SELECT count(*) FROM image WHERE person_id = ? AND kind = 'photo'`, person).Scan(&photos); err != nil {
		e.t.Fatal(err)
	}
	return person, name, overview.String, ids, photos
}

const audibleSearch = "/1.0/catalog/products"

// A book whose folder names its ASIN is looked up by that ASIN, with no
// search. Audnexus fills its description, genres, publisher, release date
// and cover, and its author's biography and photo.
func TestBookByASIN(t *testing.T) {
	e := newEnv(t)
	w := e.book("Andy Weir/Project Hail Mary [B08G9PRS1K]/Project Hail Mary.m4b", map[string]string{"composer": "Ray Porter"}, 970*time.Minute)
	e.mustRefresh(w)

	if e.fx.count(audibleSearch) != 0 {
		t.Error("a book with an ASIN was searched for")
	}
	wantOverview := "Ryland Grace is the sole survivor on a desperate, last-chance mission—and if he fails, humanity and the earth itself will perish.\n\nExcept that right now, he doesn't know that."
	if got := e.column(w, "overview"); got != wantOverview {
		t.Errorf("overview %q", got)
	}
	if got := e.genres(w); !reflect.DeepEqual(got, []string{"Science Fiction & Fantasy"}) {
		t.Errorf("genres %q", got) // Audnexus's tags aren't genres
	}
	if got := e.tags(w); !reflect.DeepEqual(got, []string{"Hard Science Fiction", "Science Fiction"}) {
		t.Errorf("tags %q", got) // Audible's finer categories
	}
	if e.publisher(w) != "Audible Studios" || e.column(w, "release_date") != "2021-05-04" || e.column(w, "year") != "2021" {
		t.Errorf("publisher %q, release %q, year %q", e.publisher(w), e.column(w, "release_date"), e.column(w, "year"))
	}
	if p := e.images(w)[artworkPoster]; p.Source != sourceProvider || p.Origin != "https://m.media-amazon.com/images/I/91vS2L5YfEL.jpg" {
		t.Errorf("cover %+v", p)
	}
	if got := e.credits(w); !reflect.DeepEqual(got, []string{"author:Andy Weir", "narrator:Ray Porter"}) {
		t.Errorf("credits %q", got)
	}
	_, name, bio, ids, photos := e.author(w)
	if name != "Andy Weir" || bio != "ANDY WEIR was first hired as a programmer for a national laboratory at age fifteen.\n\nHe lives in California." ||
		!reflect.DeepEqual(ids, []string{"B00G0WYW92"}) || photos != 1 {
		t.Errorf("author %q %q %q, %d photos", name, bio, ids, photos)
	}
}

// A book without an ASIN is searched for by title and author. Among results
// with the same title and author, the one whose running time is closest to
// the files' is accepted, and its ASIN becomes the book's.
func TestBookByTitleAuthorAndDuration(t *testing.T) {
	e := newEnv(t)
	w := e.book("Andy Weir/The Martian/The Martian.m4b", nil, 653*time.Minute+20*time.Second)
	e.mustRefresh(w)
	if e.asin(w) != "B00B5HZGUG" {
		t.Fatalf("ASIN %q", e.asin(w))
	}
	if got := e.column(w, "overview"); got != "Six days ago, astronaut Mark Watney became one of the first people to walk on Mars." {
		t.Errorf("overview %q", got)
	}
	// The book has no narrator of its own, so it takes Audible's.
	if got := e.credits(w); !reflect.DeepEqual(got, []string{"author:Andy Weir", "narrator:R. C. Bray"}) {
		t.Errorf("credits %q", got)
	}
	if e.publisher(w) != "Podium Audio" {
		t.Errorf("publisher %q", e.publisher(w))
	}
}

// A search result with the book's title and author but a running time an
// hour off is a different recording. Nothing is accepted, and the book stays
// as its files describe it.
func TestBookNearMissRejected(t *testing.T) {
	e := newEnv(t)
	w := e.book("Andy Weir/The Martian/The Martian.m4b", map[string]string{"description": "From the tags."}, 10*time.Hour)
	e.mustRefresh(w)
	if e.fx.count(audibleSearch) != 1 {
		t.Errorf("searched %d times", e.fx.count(audibleSearch))
	}
	if e.asin(w) != "" || e.fx.count("/books/B00B5HZGUG") != 0 {
		t.Errorf("matched %q", e.asin(w))
	}
	if e.column(w, "overview") != "From the tags." || len(e.genres(w)) != 0 || e.publisher(w) != "" {
		t.Errorf("described from the provider: %q %q %q", e.column(w, "overview"), e.genres(w), e.publisher(w))
	}
}

func TestAcceptBook(t *testing.T) {
	d := func(m int) *time.Duration { x := time.Duration(m) * time.Minute; return &x }
	sub := "Classroom Edition"
	weir := []BookAuthor{{ID: "B00G0WYW92", Name: "Andy Weir"}}
	results := []Book{
		{ID: "wheaton", Title: "The Martian", Subtitle: &sub, Authors: weir, Duration: d(664)},
		{ID: "bradbury", Title: "The Martian Chronicles", Authors: []BookAuthor{{Name: "Ray Bradbury"}}, Duration: d(654)},
		{ID: "bray", Title: "The Martian", Authors: weir, Duration: d(653)},
		{ID: "unknown", Title: "The Martian", Authors: weir},
	}
	for _, tc := range []struct {
		name    string
		title   string
		authors []string
		minutes int
		want    string
	}{
		{"closest", "The Martian", []string{"Andy Weir"}, 653, "bray"},
		{"closest of two in the slack", "The Martian", []string{"Andy Weir"}, 662, "wheaton"},
		{"the slack is 2% of a long book", "The Martian", []string{"Andy Weir"}, 641, "bray"},
		{"too far from either", "The Martian", []string{"Andy Weir"}, 600, ""},
		{"another author", "The Martian", []string{"Ray Bradbury"}, 653, ""},
		{"with its subtitle", "The Martian: Classroom Edition", []string{"Andy Weir"}, 664, "wheaton"},
		{"bracketed asides", "The Martian (Unabridged)", []string{"Andy Weir"}, 653, "bray"},
		{"no author to check", "The Martian", nil, 653, "bray"},
		{"spelling of an author", "The Martian", []string{"andy  weir"}, 653, "bray"},
		{"another title", "Artemis", []string{"Andy Weir"}, 653, ""},
	} {
		got, ok := acceptBook(tc.title, tc.authors, time.Duration(tc.minutes)*time.Minute, results)
		if ok != (tc.want != "") || got.ID != tc.want {
			t.Errorf("%s: got %q %v, want %q", tc.name, got.ID, ok, tc.want)
		}
	}
	// A short book's slack is three minutes.
	short := []Book{{ID: "s", Title: "Short", Duration: d(60)}}
	if _, ok := acceptBook("Short", nil, 63*time.Minute, short); !ok {
		t.Error("3 minutes off a short book")
	}
	if _, ok := acceptBook("Short", nil, 64*time.Minute, short); ok {
		t.Error("4 minutes off a short book")
	}
}

// Folder and tag values win where present: the description, publisher,
// release date, narrator and series. The provider fills the rest, which is
// the genres and the cover the book lacks.
func TestBookLocalBeatsProvider(t *testing.T) {
	e := newEnv(t)
	w := e.book("James S. A. Corey/The Expanse/Book 1 - Leviathan Wakes [B073H9PF2D]/Leviathan Wakes.m4b", map[string]string{
		"description": "From the tags.", "publisher": "Orbit", "date": "2011", "composer": "Erik Davies",
	}, 1206*time.Minute)
	e.mustRefresh(w)
	if e.column(w, "overview") != "From the tags." || e.publisher(w) != "Orbit" || e.column(w, "release_date") != "2011" {
		t.Errorf("local values lost: %q %q %q", e.column(w, "overview"), e.publisher(w), e.column(w, "release_date"))
	}
	if got := e.credits(w); !reflect.DeepEqual(got, []string{"author:James S. A. Corey", "narrator:Erik Davies"}) {
		t.Errorf("credits %q", got)
	}
	series := e.strings(`SELECT s.key || ' ' || b.position_label FROM book_series_book b JOIN book_series s ON s.id = b.series_id WHERE b.work_id = ?`, w)
	if !reflect.DeepEqual(series, []string{"local:James S. A. Corey/The Expanse 1"}) {
		t.Errorf("series %q", series)
	}
	if !reflect.DeepEqual(e.genres(w), []string{"Science Fiction & Fantasy"}) || e.images(w)[artworkPoster].Source != sourceProvider {
		t.Errorf("gaps not filled: %q %+v", e.genres(w), e.images(w))
	}

	// A book in no series takes the provider's series. Its key is the one a
	// series folder of that name under the author would have.
	c := e.book("James S. A. Corey/Caliban's War [B07B4LLQ5W]/Caliban's War.m4b", nil, 1263*time.Minute)
	e.mustRefresh(c)
	series = e.strings(`SELECT s.key || ' ' || b.position_label FROM book_series_book b JOIN book_series s ON s.id = b.series_id WHERE b.work_id = ?`, c)
	if !reflect.DeepEqual(series, []string{"local:James S. A. Corey/The Expanse 2"}) {
		t.Errorf("provider series %q", series)
	}
}

// An admin's edits are never overwritten. Match locks the ASIN it sets, so
// that ASIN beats the one in the book's folder.
func TestBookLockedFields(t *testing.T) {
	e := newEnv(t)
	w := e.book("Andy Weir/Project Hail Mary [B08G9PRS1K]/Project Hail Mary.m4b", nil, 970*time.Minute)
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.EditMetadata(ctx, tx, w, catalog.WorkMetadata{Overview: "Mine."}, catalog.FieldOverview)
	})
	e.mustRefresh(w)
	if e.column(w, "overview") != "Mine." || e.publisher(w) != "Audible Studios" {
		t.Fatalf("overview %q, publisher %q", e.column(w, "overview"), e.publisher(w))
	}

	if err := e.s.MatchBook(ctx, w, "B00B5HZGUG"); err != nil {
		t.Fatal(err)
	}
	if got := e.tasks.take(TaskRefreshMetadata); !slices.Equal(got, []id.ID{w}) || e.column(w, "refreshed_at") != "" {
		t.Fatalf("Match asked for %v, refreshed_at %q", got, e.column(w, "refreshed_at"))
	}
	e.mustRefresh(w)
	if e.asin(w) != "B00B5HZGUG" || e.publisher(w) != "Podium Audio" || e.column(w, "overview") != "Mine." {
		t.Errorf("after Match: ASIN %q, publisher %q, overview %q", e.asin(w), e.publisher(w), e.column(w, "overview"))
	}
	if err := e.s.MatchBook(ctx, w, "not-an-asin"); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("a bad ASIN: %v", err)
	}
}

// An author is the person their Audible ID names, whatever the folder calls
// them. A second book's "James S.A. Corey" is the same person as "James S.
// A. Corey", whom the first book's refresh gave the ID. That person has one
// biography and one photo.
func TestBookAuthorByID(t *testing.T) {
	e := newEnv(t)
	first := e.book("James S. A. Corey/The Expanse/Book 1 - Leviathan Wakes [B073H9PF2D]/Leviathan Wakes.m4b", nil, 1206*time.Minute)
	e.mustRefresh(first)
	second := e.book("James S.A. Corey/Caliban's War [B07B4LLQ5W]/Caliban's War.m4b", nil, 1263*time.Minute)
	e.mustRefresh(second)

	p1, _, _, _, _ := e.author(first)
	p2, name, bio, ids, photos := e.author(second)
	if p1 != p2 || name != "James S. A. Corey" {
		t.Errorf("the second book's author is %s %q, not %s", p2, name, p1)
	}
	if bio != "James S. A. Corey is the pen name of Daniel Abraham and Ty Franck." || !reflect.DeepEqual(ids, []string{"B004AQ8ZQ8"}) || photos != 1 {
		t.Errorf("author %q %q, %d photos", bio, ids, photos)
	}
	if n := e.strings(`SELECT id FROM person WHERE name LIKE 'James S%'`); len(n) != 1 {
		t.Errorf("%d people for one author", len(n))
	}
}

// While Audible is down, a refresh writes what the files say, keeps what
// Audible gave before, and is run again later.
func TestBookProviderOutage(t *testing.T) {
	e := newEnv(t)
	w := e.book("Andy Weir/Project Hail Mary [B08G9PRS1K]/Project Hail Mary.m4b", nil, 970*time.Minute)
	e.mustRefresh(w)
	before := e.images(w)[artworkPoster]

	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM provider_cache`)
		return err
	})
	e.fx.set("/books/B08G9PRS1K", 503)
	if err := e.refresh(w); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("refresh: %v", err)
	}
	if !strings.HasPrefix(e.column(w, "overview"), "Ryland Grace") || e.publisher(w) != "Audible Studios" ||
		!reflect.DeepEqual(e.genres(w), []string{"Science Fiction & Fantasy"}) || len(e.tags(w)) != 2 || e.asin(w) != "B08G9PRS1K" {
		t.Errorf("Audible's description lost: %q %q %q %q %q", e.column(w, "overview"), e.publisher(w), e.genres(w), e.tags(w), e.asin(w))
	}
	if after := e.images(w)[artworkPoster]; after.ID != before.ID {
		t.Errorf("cover replaced: %+v", after)
	}

	// A new book is described from its files at once.
	n := e.book("Becky Chambers/A Psalm for the Wild-Built/A Psalm for the Wild-Built.m4b", map[string]string{"composer": "Em Grosland"}, 4*time.Hour)
	e.fx.set(audibleSearch, 503)
	if err := e.refresh(n); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("refresh: %v", err)
	}
	if got := e.credits(n); !reflect.DeepEqual(got, []string{"author:Becky Chambers", "narrator:Em Grosland"}) {
		t.Errorf("credits %q", got)
	}
}
