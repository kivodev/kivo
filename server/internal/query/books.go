package query

import (
	"cmp"
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
)

// BookInfo is what a book's summary carries besides a work's own fields.
type BookInfo struct {
	Authors, Narrators []PersonRef // in credit order; the first author is the primary one
	Series             []SeriesRef // the primary first
}

// PersonRef names a person, for a link to their page.
type PersonRef struct {
	ID   id.ID
	Name string
}

// SeriesRef is a book's place in a series.
type SeriesRef struct {
	ID       id.ID
	Name     string
	Position string // as written: "1", "1.5", "1-3"; empty for none
	Primary  bool
	// num is the number Position orders by, as catalog stores it. It is
	// NULL for a label with no number.
	num sql.NullFloat64
}

// Book holds the fields of a book's detail page that its summary doesn't.
type Book struct {
	Subtitle, Publisher, Language string
	ReleaseDate                   string // YYYY, YYYY-MM or YYYY-MM-DD
	Abridged                      *bool  // nil when unknown
}

// Author is an item of an audiobooks library's "Authors" tab.
type Author struct {
	ID    id.ID
	Name  string
	Books int    // the library's books they wrote that the Viewer sees
	Photo *Image // nil without one
}

// SeriesSummary is an item of an audiobooks library's "Series" tab.
type SeriesSummary struct {
	ID     id.ID
	Name   string
	Books  int    // its books in the library that the Viewer sees
	Images Images // its first book's images
}

// Person is a person's page: an actor's, director's, writer's, author's or
// narrator's.
type Person struct {
	ID       id.ID
	Name     string
	Overview string // their biography
	Photo    *Image
	// Roots are the movies, shows and videos the person is credited on,
	// directly or on an episode, newest first.
	Roots []WorkSummary
	// Series are the series of the books the person wrote, by name. Each
	// lists those books in series order.
	Series []Series
	// Standalone are the books the person wrote that are in no series, by
	// title.
	Standalone []WorkSummary
	Narrated   []WorkSummary // the books the person narrated, by title
}

// Series is a series with books in it, in series order.
type Series struct {
	ID    id.ID
	Name  string
	Books []WorkSummary
}

// bookCreditsOf lists the authors and narrators of a set of books, in
// credit order. Like artwork, they are attached after a page is read.
var bookCreditsOf = stmt("BookCreditsOf", `
SELECT c.work_id, c.role, p.id, p.name
FROM credit c
JOIN work w ON w.id = c.work_id
JOIN person p ON p.id = c.person_id
WHERE c.work_id IN (SELECT unhex(value) FROM json_each(@works))
  AND c.role IN ('author', 'narrator')
  AND `+visible+`
ORDER BY c.work_id, c.role, c.ord`)

// bookSeriesOf lists the series of a set of books, each book's primary
// series first.
var bookSeriesOf = stmt("BookSeriesOf", `
SELECT m.work_id, s.id, s.name, coalesce(m.position_label, ''), m.position_num, m.is_primary
FROM book_series_book m
JOIN work w ON w.id = m.work_id
JOIN book_series s ON s.id = m.series_id
WHERE m.work_id IN (SELECT unhex(value) FROM json_each(@works))
  AND `+visible+`
ORDER BY m.work_id, m.is_primary DESC, s.sort_name, s.id`)

// withBooks attaches each book's authors, narrators and series to its
// summaries. It uses one statement for the credits and one for the series,
// covering every book at once.
func (s *Service) withBooks(ctx context.Context, vw viewer, lists ...[]WorkSummary) error {
	var works []id.ID
	books := map[id.ID]*BookInfo{}
	for _, l := range lists {
		for _, c := range l {
			if c.Kind == catalog.KindBook && books[c.ID] == nil {
				books[c.ID] = &BookInfo{}
				works = append(works, c.ID)
			}
		}
	}
	if len(works) == 0 {
		return nil
	}
	args := vw.args(sql.Named("works", id.Set(works...)))
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		err := each(ctx, tx, bookCreditsOf, args, func(r scanner) error {
			var work id.ID
			var role string
			var p PersonRef
			if err := r.Scan(&work, &role, &p.ID, &p.Name); err != nil {
				return err
			}
			if b := books[work]; role == catalog.RoleAuthor {
				b.Authors = append(b.Authors, p)
			} else {
				b.Narrators = append(b.Narrators, p)
			}
			return nil
		})
		if err != nil {
			return err
		}
		return each(ctx, tx, bookSeriesOf, args, func(r scanner) error {
			var work id.ID
			var sr SeriesRef
			if err := r.Scan(&work, &sr.ID, &sr.Name, &sr.Position, &sr.num, &sr.Primary); err != nil {
				return err
			}
			books[work].Series = append(books[work].Series, sr)
			return nil
		})
	})
	if err != nil {
		return err
	}
	for _, l := range lists {
		for i := range l {
			if b := books[l[i].ID]; b != nil {
				l[i].Book = b
			}
		}
	}
	return nil
}

// An audiobooks library's authors and series come from the books the Viewer
// sees there.

// libraryWorks selects the library's top-level works that the Viewer sees,
// over work_library wl and work w. The Authors and Series lists and the
// facets read from these works.
var libraryWorks = `wl.library_id = @library AND wl.is_root
  AND ` + visible

// authorLetters counts the Authors list's people by the first character of
// their sort name, for the list's letter rail. The character is a code
// point, or 0 for an empty name. The groups come in the list's order, and
// the counts add up to the list's total.
var authorLetters = stmt("AuthorLetters", `
SELECT coalesce(unicode(p.sort_name), 0) AS initial, count(DISTINCT p.id) AS n
FROM work_library wl
JOIN work w ON w.id = wl.work_id
JOIN credit c ON c.work_id = w.id AND c.role = 'author'
JOIN person p ON p.id = c.person_id
WHERE `+libraryWorks+`
GROUP BY 1
ORDER BY 1`)

// authors lists the people credited as an author of a book in the library,
// by sort name. Each comes with how many of the library's books they wrote
// and their photo. A person has at most one photo, so joining it doesn't
// change the count.
var authors = stmt("Authors", `
SELECT p.id, p.name, count(*) AS books, `+photoColumns+`
FROM work_library wl
JOIN work w ON w.id = wl.work_id
JOIN credit c ON c.work_id = w.id AND c.role = 'author'
JOIN person p ON p.id = c.person_id
LEFT JOIN image i ON i.person_id = p.id AND i.kind = 'photo'
WHERE `+libraryWorks+`
GROUP BY p.id
ORDER BY p.sort_name, p.id
LIMIT @limit OFFSET @offset`)

// bookSeriesLetters counts the Series list's series by the first character
// of their sort name, for the list's letter rail. The character is a code
// point, or 0 for an empty name. The groups come in the list's order, and
// the counts add up to the list's total.
var bookSeriesLetters = stmt("BookSeriesLetters", `
SELECT coalesce(unicode(s.sort_name), 0) AS initial, count(DISTINCT s.id) AS n
FROM work_library wl
JOIN work w ON w.id = wl.work_id
JOIN book_series_book m ON m.work_id = w.id
JOIN book_series s ON s.id = m.series_id
WHERE `+libraryWorks+`
GROUP BY 1
ORDER BY 1`)

// bookSeriesList lists the series of the books in the library, by sort
// name. Each comes with how many of its books are in the library, and the
// first of those in series order, whose cover the series shows.
var bookSeriesList = stmt("BookSeriesList", `
SELECT s.id, s.name, count(*) AS books,
  (SELECT f.work_id FROM book_series_book f JOIN work w ON w.id = f.work_id
    WHERE f.series_id = s.id AND EXISTS (SELECT 1 FROM work_library fl WHERE fl.library_id = @library AND fl.work_id = w.id)
      AND `+visible+`
    ORDER BY f.position_num IS NULL, f.position_num, w.sort_title, w.id LIMIT 1) AS first_book
FROM work_library wl
JOIN work w ON w.id = wl.work_id
JOIN book_series_book m ON m.work_id = w.id
JOIN book_series s ON s.id = m.series_id
WHERE `+libraryWorks+`
GROUP BY s.id
ORDER BY s.sort_name, s.id
LIMIT @limit OFFSET @offset`)

// Authors returns a page of the people who wrote the books in an
// audiobooks library that the Viewer sees, by sort name.
func (s *Service) Authors(ctx context.Context, v users.Viewer, lib id.ID, o GridOptions) (GridPage[Author], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[Author]{}, err
	}
	vw := viewerOf(v)
	var items []Author
	var letters []Letter
	total := 0
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := library(ctx, tx, vw, lib); err != nil {
			return notFound(err)
		}
		// The letters' counts add up to the total.
		var err error
		if letters, total, err = rail(ctx, tx, authorLetters, vw.args(sql.Named("library", lib)), o); err != nil {
			return err
		}
		items, err = rowsOf(ctx, tx, authors, vw.args(sql.Named("library", lib),
			sql.Named("offset", o.Offset), sql.Named("limit", limit)),
			func(r scanner, a *Author) error {
				var p photo
				err := r.Scan(append([]any{&a.ID, &a.Name, &a.Books}, p.dest()...)...)
				a.Photo = p.image()
				return err
			})
		return err
	})
	if err != nil {
		return GridPage[Author]{}, err
	}
	return gridPage(items, total, letters), nil
}

// SeriesList returns a page of the series of the books in an
// audiobooks library that the Viewer sees, by sort name, each with its
// first book's cover.
func (s *Service) SeriesList(ctx context.Context, v users.Viewer, lib id.ID, o GridOptions) (GridPage[SeriesSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[SeriesSummary]{}, err
	}
	vw := viewerOf(v)
	var items []SeriesSummary
	var firsts []WorkSummary // each series' first book, for its cover
	var letters []Letter
	total := 0
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := library(ctx, tx, vw, lib); err != nil {
			return notFound(err)
		}
		// The letters' counts add up to the total.
		var err error
		if letters, total, err = rail(ctx, tx, bookSeriesLetters, vw.args(sql.Named("library", lib)), o); err != nil {
			return err
		}
		return each(ctx, tx, bookSeriesList, vw.args(sql.Named("library", lib),
			sql.Named("offset", o.Offset), sql.Named("limit", limit)),
			func(r scanner) error {
				var c SeriesSummary
				var first WorkSummary
				if err := r.Scan(&c.ID, &c.Name, &c.Books, &first.ID); err != nil {
					return err
				}
				items, firsts = append(items, c), append(firsts, first)
				return nil
			})
	})
	if err != nil {
		return GridPage[SeriesSummary]{}, err
	}
	if err := s.withImages(ctx, vw, firsts); err != nil {
		return GridPage[SeriesSummary]{}, err
	}
	for i := range items {
		items[i].Images = firsts[i].Images
	}
	return gridPage(items, total, letters), nil
}

// personOne reads a person credited on a work the Viewer sees, with their
// biography and photo.
var personOne = stmt("Person", `
SELECT p.id, p.name, coalesce(p.overview, ''), `+photoColumns+`
FROM person p
LEFT JOIN image i ON i.person_id = p.id AND i.kind = 'photo'
WHERE p.id = @person
  AND EXISTS (SELECT 1 FROM credit c JOIN work w ON w.id = c.work_id
    WHERE c.person_id = p.id AND w.kind <> 'extra'
    AND `+visible+`)`)

// personBooks lists the books a person is credited on in a role, by title.
var personBooks = stmt("PersonBooks", `
SELECT `+summaryColumns+`
FROM credit pc
JOIN work w ON w.id = pc.work_id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE pc.person_id = @person AND pc.role = @role AND w.kind = 'book'
  AND `+visible+`
ORDER BY w.sort_title, w.id`)

// personRoots lists the movies, shows and videos a person is credited on,
// directly or on an episode, newest first.
var personRoots = stmt("PersonRoots", `
SELECT `+summaryColumns+`
FROM work w
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.id IN (SELECT pw.root_id FROM credit pc JOIN work pw ON pw.id = pc.work_id
    WHERE pc.person_id = @person AND pw.kind <> 'extra')
  AND w.kind IN ('movie', 'series', 'video')
  AND `+visible+`
ORDER BY coalesce(w.year, 0) DESC, w.sort_title, w.id`)

// Person returns a person's page: the root works and books the Viewer sees that
// the person is credited on. A person credited on none of them is
// ErrNotFound.
func (s *Service) Person(ctx context.Context, v users.Viewer, person id.ID) (Person, error) {
	vw := viewerOf(v)
	args := vw.args(sql.Named("person", person))
	var p Person
	var wrote []WorkSummary
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		var ph photo
		if err := rowOf(ctx, tx, personOne, args, append([]any{&p.ID, &p.Name, &p.Overview}, ph.dest()...)...); err != nil {
			return notFound(err)
		}
		p.Photo = ph.image()
		var err error
		if p.Roots, err = summaries(ctx, tx, personRoots, args); err != nil {
			return err
		}
		if wrote, err = summaries(ctx, tx, personBooks, vw.args(sql.Named("person", person), sql.Named("role", catalog.RoleAuthor))); err != nil {
			return err
		}
		p.Narrated, err = summaries(ctx, tx, personBooks, vw.args(sql.Named("person", person), sql.Named("role", catalog.RoleNarrator)))
		return err
	})
	if err != nil {
		return Person{}, err
	}
	if err := s.decorate(ctx, vw, p.Roots, wrote, p.Narrated); err != nil {
		return Person{}, err
	}
	bySeries := map[id.ID]*Series{}
	for _, c := range wrote {
		if c.Book == nil || len(c.Book.Series) == 0 {
			p.Standalone = append(p.Standalone, c)
			continue
		}
		for _, r := range c.Book.Series {
			g := bySeries[r.ID]
			if g == nil {
				g = &Series{ID: r.ID, Name: r.Name}
				bySeries[r.ID] = g
			}
			g.Books = append(g.Books, c)
		}
	}
	for _, g := range bySeries {
		slices.SortStableFunc(g.Books, func(a, b WorkSummary) int {
			return comparePositions(a.Book.position(g.ID), b.Book.position(g.ID))
		})
		p.Series = append(p.Series, *g)
	}
	slices.SortFunc(p.Series, func(a, b Series) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.ID.String(), b.ID.String()))
	})
	return p, nil
}

// position is the number a book's position in a series orders by. It is
// NULL when the book has no number in that series.
func (b *BookInfo) position(series id.ID) sql.NullFloat64 {
	for _, r := range b.Series {
		if r.ID == series {
			return r.num
		}
	}
	return sql.NullFloat64{}
}

// comparePositions orders positions as SeriesBooks does: by number, with
// positions that have no number last. The summaries are already sorted by
// title, so a stable sort breaks ties by title.
func comparePositions(a, b sql.NullFloat64) int {
	switch {
	case a.Valid && b.Valid:
		return cmp.Compare(a.Float64, b.Float64)
	case a.Valid:
		return -1
	case b.Valid:
		return 1
	}
	return 0
}

// bookSeries reads a series, if it has a book the Viewer sees.
var bookSeries = stmt("BookSeries", `
SELECT s.id, s.name FROM book_series s
WHERE s.id = @series
  AND EXISTS (SELECT 1 FROM book_series_book m JOIN work w ON w.id = m.work_id
    WHERE m.series_id = s.id
    AND `+visible+`)`)

// seriesBooks lists a series' books in series order: by position, with
// books that have no position last, then by title.
var seriesBooks = stmt("SeriesBooks", `
SELECT `+summaryColumns+`
FROM book_series_book sm
JOIN work w ON w.id = sm.work_id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE sm.series_id = @series
  AND `+visible+`
ORDER BY sm.position_num IS NULL, sm.position_num, w.sort_title, w.id`)

// Series returns a book series' page: its books the Viewer sees, in series
// order. A series with no such books is ErrNotFound.
func (s *Service) Series(ctx context.Context, v users.Viewer, series id.ID) (Series, error) {
	vw := viewerOf(v)
	args := vw.args(sql.Named("series", series))
	var out Series
	err := s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := rowOf(ctx, tx, bookSeries, args, &out.ID, &out.Name); err != nil {
			return notFound(err)
		}
		var err error
		out.Books, err = summaries(ctx, tx, seriesBooks, args)
		return err
	})
	if err != nil {
		return Series{}, err
	}
	if err := s.decorate(ctx, vw, out.Books); err != nil {
		return Series{}, err
	}
	return out, nil
}

// bookOne reads a book's own fields, with its publisher (a company) and
// release date.
var bookOne = stmt("Book", `
SELECT coalesce(bk.subtitle, ''),
  coalesce((SELECT c.name FROM work_company wc JOIN company c ON c.id = wc.company_id
    WHERE wc.work_id = w.id ORDER BY c.name LIMIT 1), ''),
  coalesce(bk.language, ''), bk.abridged, coalesce(w.release_date, '')
FROM book bk
JOIN work w ON w.id = bk.work_id
WHERE bk.work_id = @work
  AND `+visible)

// book reads a book's own fields for its detail page.
func book(ctx context.Context, q querier, args []any) (*Book, error) {
	var b Book
	var abridged sql.NullBool
	if err := rowOf(ctx, q, bookOne, args, &b.Subtitle, &b.Publisher, &b.Language, &abridged, &b.ReleaseDate); err != nil {
		return nil, notFound(err)
	}
	if abridged.Valid {
		b.Abridged = &abridged.Bool
	}
	return &b, nil
}
