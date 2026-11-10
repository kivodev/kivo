package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

// Fields of a book that an admin can edit. An edited field locks against
// automatic updates. A book's title, description, genres, publisher and
// release date use the general work fields, with FieldOverview for the
// description and FieldCompanies for the publisher.
const (
	FieldAuthors   = "authors"
	FieldNarrators = "narrators"
	FieldSeries    = "series"
	FieldSubtitle  = "subtitle"
	FieldASIN      = "asin"
)

// Fields of a book that only metadata sources write. No admin edit names
// them, so they never lock.
const (
	FieldLanguage = "language"
	FieldAbridged = "abridged"
)

// A book's credit roles. The first author is the book's primary author.
const (
	RoleAuthor   = "author"
	RoleNarrator = "narrator"
)

// ASIN holds the provider and kind that a book's ASIN is stored under in
// work_external_id. The ASIN itself goes in ID.
var ASIN = ExternalID{Provider: "audible", Kind: "book"}

// BookMetadata holds the fields only a book has. UpdateMetadata and
// EditMetadata write it field by field, like a work's other fields. A
// book's publisher is in WorkMetadata.Companies and its release date is in
// WorkMetadata.ReleaseDate.
type BookMetadata struct {
	Subtitle, Language string
	Abridged           *bool // nil when unknown
	// Authors and Narrators are ordered names, used as written.
	Authors, Narrators []string
	// AuthorIDs maps names in Authors to a provider's IDs for them. The IDs
	// tell an author apart from other people with the same name.
	AuthorIDs map[string][]ExternalID
	// Series lists the book's series, with its primary series marked. nil
	// leaves the series alone, and empty takes the book out of every series.
	Series []SeriesRef
	ASIN   string
}

// SeriesRef is a book's place in a series.
type SeriesRef struct {
	// Key says where the series comes from. A series named by a folder or
	// tag uses LocalSeriesKey. A series an admin names has no key. It
	// resolves to the oldest series with Name, or else a new series keyed
	// by the book's first author.
	Key, Name string
	Position  string // as written, such as "1", "1.5" or "1-3", or empty for none
	Primary   bool
}

// LocalSeriesKey returns the key of a series that a folder or tag names.
func LocalSeriesKey(author, series string) string { return "local:" + author + "/" + series }

// writeBook writes the book fields that take allows. It runs in the same
// transaction as the work's other metadata.
func writeBook(ctx context.Context, q *dbq.Queries, work id.ID, b *BookMetadata, take func(string) bool) error {
	cur, err := q.GetBook(ctx, work)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	row := dbq.PutBookParams{WorkID: work, Subtitle: cur.Subtitle, Language: cur.Language, Abridged: cur.Abridged}
	if take(FieldSubtitle) {
		row.Subtitle = nullString(strings.TrimSpace(b.Subtitle))
	}
	if take(FieldLanguage) {
		row.Language = nullString(strings.TrimSpace(b.Language))
	}
	if take(FieldAbridged) {
		row.Abridged = sql.NullBool{}
		if b.Abridged != nil {
			row.Abridged = sql.NullBool{Bool: *b.Abridged, Valid: true}
		}
	}
	if err := q.PutBook(ctx, row); err != nil {
		return err
	}
	if take(FieldAuthors) && b.Authors != nil {
		if err := setRole(ctx, q, work, RoleAuthor, b.Authors, b.AuthorIDs); err != nil {
			return err
		}
	}
	if take(FieldNarrators) && b.Narrators != nil {
		if err := setRole(ctx, q, work, RoleNarrator, b.Narrators, nil); err != nil {
			return err
		}
	}
	if take(FieldASIN) {
		if err := setASIN(ctx, q, work, strings.TrimSpace(b.ASIN)); err != nil {
			return err
		}
	}
	if take(FieldSeries) && b.Series != nil {
		if err := setSeries(ctx, q, work, b.Series); err != nil {
			return err
		}
	}
	return nil
}

// setASIN replaces a book's ASIN. Two books may share an ASIN.
func setASIN(ctx context.Context, q *dbq.Queries, work id.ID, asin string) error {
	err := q.DeleteBookExternalIDs(ctx, dbq.DeleteBookExternalIDsParams{WorkID: work, Provider: ASIN.Provider, Kind: ASIN.Kind})
	if err != nil || asin == "" {
		return err
	}
	return q.InsertExternalID(ctx, dbq.InsertExternalIDParams{Provider: ASIN.Provider, Kind: ASIN.Kind, ExtID: asin, WorkID: work})
}

// releaseYear returns the year a release date starts with, or zero.
func releaseYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

// setRole replaces a work's credits of one role with names, in order. ids
// gives each name's external IDs, which tell people of the same name apart.
func setRole(ctx context.Context, q *dbq.Queries, work id.ID, role string, names []string, ids map[string][]ExternalID) error {
	if err := q.DeleteRoleCredits(ctx, dbq.DeleteRoleCreditsParams{WorkID: work, Role: role}); err != nil {
		return err
	}
	var seen []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n == "" || slices.Contains(seen, n) {
			continue
		}
		seen = append(seen, n)
		person, err := resolvePerson(ctx, q, n, ids[n])
		if err != nil {
			return err
		}
		err = q.InsertCredit(ctx, dbq.InsertCreditParams{WorkID: work, PersonID: person, Role: role, Ord: int64(len(seen))})
		if err != nil {
			return err
		}
	}
	return nil
}

// setSeries replaces a book's series with refs. A series left with no book
// is deleted.
func setSeries(ctx context.Context, q *dbq.Queries, work id.ID, refs []SeriesRef) error {
	old, err := q.WorkSeries(ctx, work)
	if err != nil {
		return err
	}
	if err := q.LeaveSeries(ctx, work); err != nil {
		return err
	}
	refs = slices.DeleteFunc(slices.Clone(refs), func(r SeriesRef) bool { return strings.TrimSpace(r.Name) == "" })
	primary := max(slices.IndexFunc(refs, func(r SeriesRef) bool { return r.Primary }), 0)
	for i, r := range refs {
		s, err := seriesFor(ctx, q, work, r.Key, strings.TrimSpace(r.Name))
		if err != nil {
			return err
		}
		label := strings.TrimSpace(r.Position)
		err = q.JoinSeries(ctx, dbq.JoinSeriesParams{
			SeriesID: s, WorkID: work, PositionLabel: nullString(label), PositionNum: positionNum(label), IsPrimary: i == primary,
		})
		if err != nil {
			return err
		}
	}
	ids := make([]id.ID, len(old))
	for i, o := range old {
		ids[i] = o.SeriesID
	}
	if len(ids) == 0 {
		return nil
	}
	return q.DeleteEmptySeries(ctx, id.Set(ids...))
}

// FirstAuthor returns a book's first author's name, or "" when it has
// none.
func (c *Catalog) FirstAuthor(ctx context.Context, book id.ID) (string, error) {
	name, err := c.q.FirstAuthor(ctx, book)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return name, nil
}

// seriesFor returns the series a reference names, creating it if it's new.
func seriesFor(ctx context.Context, q *dbq.Queries, work id.ID, key, name string) (id.ID, error) {
	if key == "" {
		s, err := q.SeriesByName(ctx, dbq.SeriesByNameParams{Name: name})
		if !errors.Is(err, sql.ErrNoRows) {
			return s, err
		}
		author, err := q.FirstAuthor(ctx, work)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return id.ID{}, err
		}
		key = LocalSeriesKey(author, name)
	}
	return q.UpsertSeries(ctx, dbq.UpsertSeriesParams{ID: id.New(), Key: key, Name: name, SortName: SortTitle(name), CreatedAt: time.Now().UnixMilli()})
}

var positionNumber = regexp.MustCompile(`\d+(?:\.\d+)?`)

// positionNum returns the number a position label sorts by. It is 1.5 for
// "1.5", 1 for the range "1-3", and none for "Part II".
func positionNum(label string) sql.NullFloat64 {
	n, err := strconv.ParseFloat(positionNumber.FindString(label), 64)
	return sql.NullFloat64{Float64: n, Valid: err == nil}
}

// RenameSeries renames a book series and returns the resulting series.
//
// Renaming to the name of another series merges the two. Every book moves
// to that series, and the renamed series is deleted. A book that was in
// both keeps its position in the other series. A plain rename keeps the
// series' key, so the folder it came from still finds it.
//
// Either way, each of its books counts as edited, so their series field is
// locked. Books are written a batch at a time, as RenamePerson does.
func (c *Catalog) RenameSeries(ctx context.Context, series id.ID, name string) (id.ID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return id.ID{}, invalidField("/name", "required", "a name is required")
	}
	s, err := c.q.GetSeries(ctx, series)
	if err != nil {
		return id.ID{}, notFound(err)
	}
	if s.Name == name {
		return series, nil
	}
	into, err := c.q.SeriesByName(ctx, dbq.SeriesByNameParams{Name: name, Other: series})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		into = series
	case err != nil:
		return id.ID{}, err
	}
	books, err := c.q.SeriesMemberships(ctx, series)
	if err != nil {
		return id.ID{}, err
	}
	rename := func(ctx context.Context, q *dbq.Queries) error {
		if into != series {
			return nil
		}
		return q.RenameSeries(ctx, dbq.RenameSeriesParams{ID: series, Name: name, SortName: SortTitle(name)})
	}
	err = store.Batches(ctx, c.db, books, func(ctx context.Context, tx *store.Tx, batch []dbq.SeriesMembershipsRow) error {
		q := dbq.New(tx)
		if err := rename(ctx, q); err != nil {
			return err
		}
		for _, b := range batch {
			if into != series {
				if err := moveMembership(ctx, q, b, series, into); err != nil {
					return err
				}
			}
			if err := lockField(ctx, q, b.WorkID, FieldSeries); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return id.ID{}, err
	}
	err = c.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		if err := rename(ctx, q); err != nil { // renames a series that has no books
			return err
		}
		if into == series {
			return nil
		}
		return q.DeleteEmptySeries(ctx, id.Set(series))
	})
	if err != nil {
		return id.ID{}, err
	}
	return into, nil
}

// moveMembership moves a book from one series to another. If the book is in
// the other series already, it keeps its place there.
func moveMembership(ctx context.Context, q *dbq.Queries, m dbq.SeriesMembershipsRow, from, into id.ID) error {
	has, err := q.InSeries(ctx, dbq.InSeriesParams{SeriesID: into, WorkID: m.WorkID})
	if err != nil {
		return err
	}
	if has == 0 {
		err := q.JoinSeries(ctx, dbq.JoinSeriesParams{
			SeriesID: into, WorkID: m.WorkID, PositionLabel: m.PositionLabel, PositionNum: m.PositionNum, IsPrimary: m.IsPrimary,
		})
		if err != nil {
			return err
		}
	}
	return q.LeaveOneSeries(ctx, dbq.LeaveOneSeriesParams{SeriesID: from, WorkID: m.WorkID})
}

// BookEdit is a batch edit of books. Each part that is set is written to
// every book and locked, as an edit of each book would.
type BookEdit struct {
	Authors, Narrators, Genres []string // nil leaves them alone
	Series                     *SeriesEdit
}

// SeriesEdit puts books in one series, as their only series.
type SeriesEdit struct {
	Name string // empty takes the books out of every series
	// KeepPosition keeps each book's position from its primary series.
	// Otherwise the books have no position.
	KeepPosition bool
}

// EditBooks is an admin's batch edit of books, in one write. Every work
// must be one the Viewer can see, or the batch fails with ErrNotFound. Then
// every work must be a book, or it fails with ErrNotABook about /workIds.
func (c *Catalog) EditBooks(ctx context.Context, v users.Viewer, works []id.ID, e BookEdit) error {
	for _, w := range works {
		if err := c.Visible(ctx, v, w); err != nil {
			return err
		}
	}
	err := c.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		for _, w := range works {
			cur, err := q.GetWork(ctx, w)
			if err != nil {
				return notFound(err)
			}
			if WorkKind(cur.Kind) != KindBook {
				return fault.About(fmt.Errorf("%w: %s", ErrNotABook, w), fault.Field{Name: "/workIds", Code: "notABook"})
			}
			m := WorkMetadata{Genres: e.Genres, Book: &BookMetadata{Authors: e.Authors, Narrators: e.Narrators}}
			var fields []string
			for f, set := range map[string]bool{FieldAuthors: e.Authors != nil, FieldNarrators: e.Narrators != nil, FieldGenres: e.Genres != nil} {
				if set {
					fields = append(fields, f)
				}
			}
			if s := e.Series; s != nil {
				fields = append(fields, FieldSeries)
				m.Book.Series = []SeriesRef{}
				if s.Name != "" {
					ref := SeriesRef{Name: s.Name, Primary: true}
					if s.KeepPosition {
						if ref.Position, err = primaryPosition(ctx, q, w); err != nil {
							return err
						}
					}
					m.Book.Series = append(m.Book.Series, ref)
				}
			}
			if err := c.edit(ctx, tx, cur, m, fields); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// primaryPosition returns a book's position in its primary series, or "".
func primaryPosition(ctx context.Context, q *dbq.Queries, work id.ID) (string, error) {
	rows, err := q.WorkSeries(ctx, work)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rows[0].PositionLabel.String, nil // the primary sorts first
}
