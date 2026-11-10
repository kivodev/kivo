package catalog_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// newBook adds a book of one file in an audiobooks library.
func newBook(t *testing.T, b *catalogtest.Builder, folder id.ID, rel string) id.ID {
	t.Helper()
	w := b.Work(catalog.WorkInput{Kind: catalog.KindBook, Title: rel})
	b.Version([]id.ID{b.File(folder, rel)}, w)
	return w
}

// scanned writes a book's metadata the way a refresh from its folders and
// tags does.
func scanned(t *testing.T, b *catalogtest.Builder, w id.ID, m catalog.WorkMetadata) {
	t.Helper()
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.UpdateMetadata(ctx, tx, w, m)
	})
}

func local(title string, authors []string, series, pos string) catalog.WorkMetadata {
	m := catalog.WorkMetadata{Title: title, Book: &catalog.BookMetadata{Authors: authors, Narrators: []string{"Ray Porter"}, Series: []catalog.SeriesRef{}}}
	if series != "" {
		m.Book.Series = append(m.Book.Series, catalog.SeriesRef{Key: catalog.LocalSeriesKey(authors[0], series), Name: series, Position: pos, Primary: true})
	}
	return m
}

// credits returns the names in a work's credits of one role, in order.
func credits(t *testing.T, b *catalogtest.Builder, w id.ID, role string) []string {
	t.Helper()
	return strings(t, b, `SELECT p.name FROM credit c JOIN person p ON p.id = c.person_id WHERE c.work_id = ? AND c.role = ? ORDER BY c.ord`, w, role)
}

// memberships returns a work's series as "name position primary".
func memberships(t *testing.T, b *catalogtest.Builder, w id.ID) []string {
	t.Helper()
	return strings(t, b, `SELECT s.name || ' ' || coalesce(m.position_label, '-') || ' ' || m.is_primary
		FROM book_series_book m JOIN book_series s ON s.id = m.series_id WHERE m.work_id = ? ORDER BY m.is_primary DESC, s.name`, w)
}

func strings(t *testing.T, b *catalogtest.Builder, q string, args ...any) []string {
	t.Helper()
	rows, err := b.DB.Reader().Query(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestBookMetadataKeepsEditsThroughRescans(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Audiobooks)
	w := newBook(t, b, folder, "James S. A. Corey/The Expanse/Book 1.5 - The Churn/The Churn.m4b")
	m := local("The Churn", []string{"James S. A. Corey"}, "The Expanse", "1.5")
	m.Year, m.ReleaseDate, m.Companies, m.Book.ASIN = 2014, "2014-04-29", []string{"Hachette Audio"}, "B00JQ5QYLQ"
	scanned(t, b, w, m)

	if got := credits(t, b, w, catalog.RoleAuthor); !reflect.DeepEqual(got, []string{"James S. A. Corey"}) {
		t.Errorf("authors %v", got)
	}
	if got := memberships(t, b, w); !reflect.DeepEqual(got, []string{"The Expanse 1.5 1"}) {
		t.Errorf("series %v", got)
	}
	var num float64
	if err := b.DB.Reader().QueryRow(`SELECT position_num FROM book_series_book WHERE work_id = ?`, w).Scan(&num); err != nil || num != 1.5 {
		t.Errorf("position number %v, %v", num, err)
	}
	if got := strings(t, b, `SELECT creator_sort FROM work_library WHERE work_id = ?`, w); !reflect.DeepEqual(got, []string{"James S. A. Corey"}) {
		t.Errorf("author sort key %v", got)
	}
	cw, _ := b.Catalog.Work(ctx, w)
	if cw.Year != 2014 || len(cw.ExternalIDs) != 1 || cw.ExternalIDs[0] != (catalog.ExternalID{Provider: "audible", Kind: "book", ID: "B00JQ5QYLQ"}) {
		t.Errorf("year and ASIN: %+v", cw)
	}

	// An admin's edit locks the fields it changes. A rescan keeps those
	// fields and rewrites the rest.
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.EditMetadata(ctx, tx, w, catalog.WorkMetadata{Book: &catalog.BookMetadata{
			Authors: []string{"Daniel Abraham", "Ty Franck"}, Subtitle: "An Expanse Story",
			Series: []catalog.SeriesRef{{Name: "The Expanse", Position: "1.5"}, {Name: "Expanse Novellas", Position: "1", Primary: true}},
		}}, catalog.FieldAuthors, catalog.FieldSubtitle, catalog.FieldSeries)
	})
	m.Companies = []string{"Orbit"}
	scanned(t, b, w, m)
	if got := credits(t, b, w, catalog.RoleAuthor); !reflect.DeepEqual(got, []string{"Daniel Abraham", "Ty Franck"}) {
		t.Errorf("edited authors after a rescan %v", got)
	}
	if got := memberships(t, b, w); !reflect.DeepEqual(got, []string{"Expanse Novellas 1 1", "The Expanse 1.5 0"}) {
		t.Errorf("edited series after a rescan %v", got)
	}
	var subtitle, publisher string
	if err := b.DB.Reader().QueryRow(`SELECT b.subtitle, c.name FROM book b JOIN work_company wc ON wc.work_id = b.work_id
		JOIN company c ON c.id = wc.company_id WHERE b.work_id = ?`, w).Scan(&subtitle, &publisher); err != nil {
		t.Fatal(err)
	}
	if subtitle != "An Expanse Story" || publisher != "Orbit" {
		t.Errorf("subtitle %q, publisher %q", subtitle, publisher)
	}
	// The series the admin named "The Expanse" is found by name, so it is
	// the series the folder made.
	if got := strings(t, b, `SELECT key FROM book_series ORDER BY key`); !reflect.DeepEqual(got, []string{"local:Daniel Abraham/Expanse Novellas", "local:James S. A. Corey/The Expanse"}) {
		t.Errorf("series keys %v", got)
	}

	// A book's fields are for books only.
	_, films := b.Library(catalog.Movies)
	movie := b.Movie(films, "Dune", 2021)
	err := b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.EditMetadata(ctx, tx, movie.Work, catalog.WorkMetadata{Book: &catalog.BookMetadata{}}, catalog.FieldSubtitle)
	})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("book fields on a movie: %v", err)
	}
}

func TestRenamePersonAndMerge(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Audiobooks)
	hobbit := newBook(t, b, folder, "J.R.R. Tolkien/The Hobbit/The Hobbit.m4b")
	scanned(t, b, hobbit, local("The Hobbit", []string{"J.R.R. Tolkien"}, "", ""))
	silm := newBook(t, b, folder, "J. R. R. Tolkien/The Silmarillion/The Silmarillion.m4b")
	scanned(t, b, silm, local("The Silmarillion", []string{"J. R. R. Tolkien", "Christopher Tolkien"}, "", ""))
	var old, kept id.ID
	b.DB.Reader().QueryRow(`SELECT id FROM person WHERE name = 'J.R.R. Tolkien'`).Scan(&old)
	b.DB.Reader().QueryRow(`SELECT id FROM person WHERE name = 'J. R. R. Tolkien'`).Scan(&kept)

	if got := searchPeople(t, b, "christopher"); len(got) != 1 {
		t.Errorf("a book's new author isn't in the people index: %v", got)
	}

	// A plain rename keeps the person, and search finds the new name.
	got, err := b.Catalog.RenamePerson(ctx, old, "JRR Tolkien")
	if err != nil || got != old {
		t.Fatalf("rename: %v, %v", got, err)
	}
	if got := searchPeople(t, b, "jrr"); !reflect.DeepEqual(got, []id.ID{old}) {
		t.Errorf("people named jrr after the rename: %v", got)
	}
	if got := search(t, b, "jrr"); !reflect.DeepEqual(got, []id.ID{hobbit}) {
		t.Errorf("works by jrr after the rename: %v", got)
	}
	// Renaming to someone else's name merges the two, so the books move to
	// the other person.
	if got, err = b.Catalog.RenamePerson(ctx, old, "J. R. R. Tolkien"); err != nil || got != kept {
		t.Fatalf("merge: %v, %v", got, err)
	}
	if got := credits(t, b, hobbit, catalog.RoleAuthor); !reflect.DeepEqual(got, []string{"J. R. R. Tolkien"}) {
		t.Errorf("merged authors %v", got)
	}
	if n := strings(t, b, `SELECT name FROM person WHERE id = ?`, old); len(n) != 0 {
		t.Errorf("the merged person is still there: %v", n)
	}
	if got := searchPeople(t, b, "jrr"); got != nil {
		t.Errorf("the merged person is still in the people index: %v", got)
	}
	// The books are locked, so their folders' names don't bring the old
	// person back.
	scanned(t, b, hobbit, local("The Hobbit", []string{"J.R.R. Tolkien"}, "", ""))
	if got := credits(t, b, hobbit, catalog.RoleAuthor); !reflect.DeepEqual(got, []string{"J. R. R. Tolkien"}) {
		t.Errorf("authors after a rescan %v", got)
	}
	if got := strings(t, b, `SELECT creator_sort FROM work_library WHERE work_id = ?`, hobbit); !reflect.DeepEqual(got, []string{"J. R. R. Tolkien"}) {
		t.Errorf("author sort key %v", got)
	}
	if _, err := b.Catalog.RenamePerson(ctx, kept, " "); !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("an empty name: %v", err)
	}
}

// People are told apart by their external IDs first, then by name. A credit
// with a known ID is that person, whatever the name. A credit whose IDs
// don't conflict is the oldest person with that name, who gains the IDs. A
// credit with an ID that the person of that name lacks is someone else.
// Renaming to an existing name merges into the oldest person with that
// name, including the IDs.
func TestPeopleAreToldApartByIDThenName(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	tmdb := func(n string) []catalog.ExternalID {
		return []catalog.ExternalID{{Provider: "tmdb", Kind: "person", ID: n}}
	}
	movie := func(title, name string, ids []catalog.ExternalID) id.ID {
		w := b.Movie(folder, title, 2000).Work
		scanned(t, b, w, catalog.WorkMetadata{Credits: []catalog.Credit{{Name: name, Role: "actor", ExternalIDs: ids}}})
		return w
	}
	person := func(w id.ID) id.ID {
		t.Helper()
		var p id.ID
		if err := b.DB.Reader().QueryRow(`SELECT person_id FROM credit WHERE work_id = ?`, w).Scan(&p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	nfo := movie("A", "John Smith", nil)          // a name alone makes a new person
	first := movie("B", "John Smith", tmdb("1"))  // no conflict, so the same person, who gains the ID
	second := movie("C", "John Smith", tmdb("2")) // the oldest John Smith is tmdb 1, so this is someone else
	byID := movie("D", "Jon Smith", tmdb("2"))    // a known ID wins over the name
	again := movie("E", "John Smith", nil)        // a name alone finds the oldest person with that name
	if p := person(nfo); person(first) != p || person(again) != p {
		t.Errorf("one John Smith: %v %v %v", p, person(first), person(again))
	}
	if p := person(second); person(byID) != p || p == person(first) {
		t.Errorf("the other John Smith: %v %v, first %v", p, person(byID), person(first))
	}
	if n := strings(t, b, `SELECT ext_id FROM person_external_id ORDER BY ext_id`); !reflect.DeepEqual(n, []string{"1", "2"}) {
		t.Errorf("IDs %v", n)
	}

	// Renaming to a name two people share merges into the oldest of them,
	// who takes the renamed person's IDs.
	jane := person(movie("F", "Jane Doe", tmdb("3")))
	got, err := b.Catalog.RenamePerson(ctx, jane, "John Smith")
	if err != nil || got != person(first) {
		t.Fatalf("merge: %v, %v, want %v", got, err, person(first))
	}
	if p := person(movie("G", "Jane Doe", tmdb("3"))); p != got {
		t.Errorf("Jane Doe's ID after the merge names %v, want %v", p, got)
	}
}

func TestRenameSeriesAndMerge(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Audiobooks)
	one := newBook(t, b, folder, "Joe Abercrombie/The First Law/Book 1 - The Blade Itself/a.m4b")
	scanned(t, b, one, local("The Blade Itself", []string{"Joe Abercrombie"}, "The First Law", "1"))
	two := newBook(t, b, folder, "Joe Abercrombie/The First Law Trilogy/Book 2 - Before They Are Hanged/a.m4b")
	scanned(t, b, two, local("Before They Are Hanged", []string{"Joe Abercrombie"}, "The First Law Trilogy", "2"))
	var first, trilogy id.ID
	b.DB.Reader().QueryRow(`SELECT id FROM book_series WHERE name = 'The First Law'`).Scan(&first)
	b.DB.Reader().QueryRow(`SELECT id FROM book_series WHERE name = 'The First Law Trilogy'`).Scan(&trilogy)

	got, err := b.Catalog.RenameSeries(ctx, trilogy, "The First Law")
	if err != nil || got != first {
		t.Fatalf("merge: %v, %v", got, err)
	}
	if got := memberships(t, b, two); !reflect.DeepEqual(got, []string{"The First Law 2 1"}) {
		t.Errorf("merged book %v", got)
	}
	if n := strings(t, b, `SELECT name FROM book_series ORDER BY name`); !reflect.DeepEqual(n, []string{"The First Law"}) {
		t.Errorf("series %v", n)
	}
	scanned(t, b, two, local("Before They Are Hanged", []string{"Joe Abercrombie"}, "The First Law Trilogy", "2"))
	if got := memberships(t, b, two); !reflect.DeepEqual(got, []string{"The First Law 2 1"}) {
		t.Errorf("after a rescan %v", got)
	}

	// A plain rename keeps the key, so the folder still finds the series.
	if got, err = b.Catalog.RenameSeries(ctx, first, "First Law"); err != nil || got != first {
		t.Fatalf("rename: %v, %v", got, err)
	}
	three := newBook(t, b, folder, "Joe Abercrombie/The First Law/Book 3 - Last Argument of Kings/a.m4b")
	scanned(t, b, three, local("Last Argument of Kings", []string{"Joe Abercrombie"}, "The First Law", "3"))
	if got := memberships(t, b, three); !reflect.DeepEqual(got, []string{"First Law 3 1"}) {
		t.Errorf("a new book of the renamed series' folder %v", got)
	}
}

func TestEditBooks(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Audiobooks)
	one := newBook(t, b, folder, "A/S/Book 1 - One/a.m4b")
	scanned(t, b, one, local("One", []string{"A"}, "S", "1"))
	two := newBook(t, b, folder, "A/Two/a.m4b")
	scanned(t, b, two, local("Two", []string{"A"}, "", ""))

	v := admin(t, b)
	err := b.Catalog.EditBooks(ctx, v, []id.ID{one, two}, catalog.BookEdit{
		Narrators: []string{"Kevin R. Free"}, Genres: []string{"Fantasy"},
		Series: &catalog.SeriesEdit{Name: "Saga", KeepPosition: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []id.ID{one, two} {
		if got := credits(t, b, w, catalog.RoleNarrator); !reflect.DeepEqual(got, []string{"Kevin R. Free"}) {
			t.Errorf("narrators %v", got)
		}
		cw, _ := b.Catalog.Work(ctx, w)
		if !reflect.DeepEqual(cw.LockedFields, []string{catalog.FieldGenres, catalog.FieldNarrators, catalog.FieldSeries}) {
			t.Errorf("locked %v", cw.LockedFields)
		}
	}
	if got := memberships(t, b, one); !reflect.DeepEqual(got, []string{"Saga 1 1"}) {
		t.Errorf("kept position %v", got)
	}
	if got := memberships(t, b, two); !reflect.DeepEqual(got, []string{"Saga - 1"}) {
		t.Errorf("no position %v", got)
	}
	// "S" lost its only book.
	if n := strings(t, b, `SELECT name FROM book_series ORDER BY name`); !reflect.DeepEqual(n, []string{"Saga"}) {
		t.Errorf("series %v", n)
	}
	if err := b.Catalog.EditBooks(ctx, v, []id.ID{one}, catalog.BookEdit{Series: &catalog.SeriesEdit{}}); err != nil {
		t.Fatal(err)
	}
	if got := memberships(t, b, one); len(got) != 0 {
		t.Errorf("cleared series %v", got)
	}
}

// TestEditBooksRefuses checks that a batch edit is refused whole when it
// holds a work the Viewer can't see, which comes before holding a work
// that isn't a book.
func TestEditBooksRefuses(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Audiobooks)
	book := newBook(t, b, folder, "A/One/a.m4b")
	_, films := b.Library(catalog.Movies)
	movie := b.Movie(films, "Dune", 2021).Work
	v := admin(t, b)
	edit := catalog.BookEdit{Genres: []string{"x"}}

	err := b.Catalog.EditBooks(ctx, v, []id.ID{book, movie}, edit)
	if fe, ok := fault.FieldOf(err); !errors.Is(err, catalog.ErrNotABook) || !ok || fe != (fault.Field{Name: "/workIds", Code: "notABook"}) {
		t.Errorf("a movie in a batch edit: %v %v", err, fe)
	}
	for name, tc := range map[string]struct {
		v     users.Viewer
		works []id.ID
	}{
		"a work the viewer can't see": {users.Viewer{}, []id.ID{book}},
		"an unknown work":             {v, []id.ID{book, id.New()}},
		"an unknown work and a movie": {v, []id.ID{movie, id.New()}},
	} {
		if err := b.Catalog.EditBooks(ctx, tc.v, tc.works, edit); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("%s: %v, want ErrNotFound", name, err)
		}
	}
	if got := strings(t, b, `SELECT g.name FROM work_genre wg JOIN genre g ON g.id = wg.genre_id`); len(got) != 0 {
		t.Errorf("a refused batch wrote genres %v", got)
	}
}

// admin returns the Viewer of a new admin who sees every library.
func admin(t *testing.T, b *catalogtest.Builder) users.Viewer {
	t.Helper()
	svc := users.New(b.DB, events.NewBus())
	var u users.User
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		u, err = svc.CreateUser(ctx, tx, users.NewUser{Username: "admin-" + id.New().String(), Admin: true, Grants: users.Grants{All: true}})
		return err
	})
	v, err := svc.Viewer(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDeleteLeavesNoEmptySeries(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Audiobooks)
	one := newBook(t, b, folder, "A/S/1/a.m4b")
	scanned(t, b, one, local("One", []string{"A"}, "S", "1"))
	two := newBook(t, b, folder, "A/T/1/a.m4b")
	scanned(t, b, two, local("One again", []string{"A"}, "T", "1"))

	// A deleted book's series is deleted once it has no book left. Here the
	// book's version is deleted, as deleting its files would do, and then
	// deleting empty works deletes the book.
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE media_file SET version_id = NULL, part = NULL`)
		if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM media_version`)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM work_library`)
		}
		return err
	})
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := b.Catalog.DeleteEmptyWorks(ctx, tx)
		return err
	})
	if n := strings(t, b, `SELECT name FROM book_series`); len(n) != 0 {
		t.Errorf("series left %v", n)
	}
}
