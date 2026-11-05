package metadata

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mmcdole/kivo/internal/store"

	"golang.org/x/text/unicode/norm"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// asinPattern matches an ASIN: ten capital letters and digits.
const asinPattern = `[A-Z0-9]{10}`

var asin = regexp.MustCompile(`^` + asinPattern + `$`)

// IsASIN reports whether s is an ASIN.
func IsASIN(s string) bool { return asin.MatchString(s) }

// SearchBooks returns the book provider's matches for a "Fix match" search,
// in the provider's order. It searches the provider's store for the content
// rating country of the work's library. It returns ErrUnavailable when there
// is no book provider.
func (s *Service) SearchBooks(ctx context.Context, work id.ID, q BookQuery) ([]Match, error) {
	if s.books == nil {
		return nil, fmt.Errorf("%w: no book provider", ErrUnavailable)
	}
	_, country, err := s.locale(ctx, work)
	if err != nil {
		return nil, err
	}
	books, err := s.books.Search(ctx, country, q)
	if err != nil {
		return nil, err
	}
	out := make([]Match, len(books))
	for i, b := range books {
		out[i] = bookMatch(b)
	}
	return out, nil
}

// LookupBook returns the match for the book an ASIN names, from the
// provider's store for the content rating country of the work's library.
func (s *Service) LookupBook(ctx context.Context, work id.ID, asin string) (Match, error) {
	if s.books == nil {
		return Match{}, fmt.Errorf("%w: no book provider", ErrUnavailable)
	}
	_, country, err := s.locale(ctx, work)
	if err != nil {
		return Match{}, err
	}
	b, err := s.books.Book(ctx, country, asin)
	if err != nil {
		return Match{}, err
	}
	return bookMatch(*b), nil
}

// bookMatch converts a provider's book to a match.
func bookMatch(b Book) Match {
	m := Match{
		ID:    catalog.ExternalID{Provider: catalog.ASIN.Provider, Kind: catalog.ASIN.Kind, ID: b.ID},
		Title: b.Title, Subtitle: b.Subtitle, Overview: b.Description,
		Authors: make([]string, len(b.Authors)), Narrators: b.Narrators,
	}
	for i, a := range b.Authors {
		m.Authors[i] = a.Name
	}
	if b.ReleaseDate != nil {
		m.Year, _ = strconv.Atoi((*b.ReleaseDate)[:4]) // YYYY-MM-DD
	}
	if b.Duration != nil {
		m.Duration = *b.Duration
	}
	return m
}

// MatchBook is "Fix match" for a book. In one write, it sets and locks the
// book's ASIN, as an admin's edit does, and asks for the book's refresh.
func (s *Service) MatchBook(ctx context.Context, work id.ID, asin string) error {
	if !IsASIN(asin) {
		return fmt.Errorf("%w: %q is no ASIN", catalog.ErrInvalid, asin)
	}
	w, err := s.deps.Catalog.Work(ctx, work)
	if err != nil {
		return err
	}
	if w.Kind != catalog.KindBook {
		return fmt.Errorf("%w: a %s isn't a book", catalog.ErrInvalid, w.Kind)
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if err := s.deps.Catalog.SetASIN(ctx, tx, work, asin); err != nil {
			return err
		}
		return s.due(ctx, tx, []id.ID{work})
	})
}

// acceptBook accepts a search result whose running time is within the
// larger of durationSlack and durationShare of the files' running time. The
// slack exists because Audible gives running times in whole minutes.
const (
	durationSlack = 3 * time.Minute
	durationShare = 0.02
)

// acceptBook picks the search result that is the book. A candidate must
// have the same title, with or without its subtitle. If the book names any
// authors, the candidate must share one of them. Among those, it picks the
// one whose running time is closest to d and within the slack. ok is false
// when none qualifies.
func acceptBook(title string, authors []string, d time.Duration, results []Book) (best Book, ok bool) {
	want := titleKey(title)
	if want == "" || d <= 0 {
		return Book{}, false
	}
	slack := max(durationSlack, time.Duration(float64(d)*durationShare))
	var off time.Duration
	for _, r := range results {
		if r.Duration == nil || !sameTitle(want, r) || !sharesAuthor(authors, r.Authors) {
			continue
		}
		diff := (*r.Duration - d).Abs()
		if diff <= slack && (!ok || diff < off) {
			best, off, ok = r, diff, true
		}
	}
	return best, ok
}

func sameTitle(key string, r Book) bool {
	if titleKey(r.Title) == key {
		return true
	}
	return r.Subtitle != nil && titleKey(r.Title+" "+*r.Subtitle) == key
}

func sharesAuthor(names []string, authors []BookAuthor) bool {
	if len(names) == 0 {
		return true
	}
	for _, n := range names {
		if slices.ContainsFunc(authors, func(a BookAuthor) bool { return nameKey(a.Name) == nameKey(n) }) {
			return true
		}
	}
	return false
}

// bracketed matches text in brackets or parentheses, such as
// "(Unabridged)".
var bracketed = regexp.MustCompile(`[(\[][^)\]]*[)\]]`)

// titleKey is the key titles are compared by. It is nameKey with bracketed
// asides removed.
func titleKey(s string) string { return nameKey(bracketed.ReplaceAllString(s, " ")) }

// nameKey is the key names are compared by. It keeps only letters and
// digits, lowercased and without accents, so "James S.A. Corey" equals
// "James S. A. Corey".
func nameKey(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

var (
	breakTag = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`)
	anyTag   = regexp.MustCompile(`<[^>]*>`)
	blank    = regexp.MustCompile(`[ \t]*\n[ \t\n]*\n[ \t\n]*`)
)

// plainText turns a provider's HTML into plain text. Paragraphs and line
// breaks become new lines, other tags are removed, and entities are
// decoded.
func plainText(s string) string {
	s = breakTag.ReplaceAllString(s, "\n\n")
	s = html.UnescapeString(anyTag.ReplaceAllString(s, ""))
	return strings.TrimSpace(blank.ReplaceAllString(s, "\n\n"))
}

// bookDescription is what a book's refresh writes. It holds the
// description from the book's files with the provider's filling the gaps,
// the embedded cover picture, the provider's cover, and the authors the
// provider describes.
type bookDescription struct {
	meta    *catalog.WorkMetadata
	cover   *embeddedCover
	found   *found // holds the provider's cover
	authors []*Author
	// later is the provider's outage, if any. The refresh then writes what
	// the files say, keeps what the provider gave before, and runs again.
	later error
}

// describeBook describes a book from its files, then fills the gaps from
// the book provider. The provider's book is the one the book's ASIN names,
// using a locked ASIN first and then the files' ASIN. Without an ASIN, it is
// the search result acceptBook picks, and that result's ASIN becomes the
// book's.
func (s *Service) describeBook(ctx context.Context, w catalog.Work, country string) (bookDescription, error) {
	m, cover, d, err := s.fromBook(ctx, w)
	out := bookDescription{meta: m, cover: cover}
	if err != nil || m == nil || s.books == nil {
		return out, err
	}
	b, authors, err := s.bookFromProvider(ctx, w, m, country, d)
	var o *outage
	if errors.As(err, &o) {
		out.later = err
		return out, s.keepBook(ctx, w, &out)
	}
	if err != nil {
		return out, err
	}
	fillBook(m, b, s.books.Name())
	if b != nil && b.Cover != nil {
		out.found = &found{artwork: map[string][]remoteImage{artworkPoster: {{provider: s.books.Name(), url: *b.Cover}}}}
	}
	for _, a := range authors {
		for _, ids := range m.Book.AuthorIDs {
			if slices.ContainsFunc(ids, func(x catalog.ExternalID) bool { return x.ID == a.ID }) {
				out.authors = append(out.authors, a) // only authors credited on the book
				break
			}
		}
	}
	return out, nil
}

// bookFromProvider finds the provider's book for the book m describes, and
// sets m's ASIN to the one it was found by. It also returns the authors the
// provider's book names with IDs. Finding no match is not an error.
func (s *Service) bookFromProvider(ctx context.Context, w catalog.Work, m *catalog.WorkMetadata, country string, d time.Duration) (*Book, []*Author, error) {
	asin := m.Book.ASIN
	if slices.Contains(w.LockedFields, catalog.FieldASIN) {
		asin = currentASIN(w)
	} else if asin == "" {
		author := ""
		if len(m.Book.Authors) > 0 {
			author = m.Book.Authors[0]
		}
		results, err := s.books.Search(ctx, country, BookQuery{Title: searchTitle(m.Title), Author: author})
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, nil, err
		}
		if r, ok := acceptBook(m.Title, m.Book.Authors, d, results); ok {
			asin = r.ID
		}
		m.Book.ASIN = asin
	}
	if asin == "" {
		return nil, nil, nil
	}
	b, err := s.books.Book(ctx, country, asin)
	if errors.Is(err, ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var authors []*Author
	for _, a := range b.Authors {
		if a.ID == "" {
			continue
		}
		au, err := s.books.Author(ctx, country, a.ID)
		switch {
		case errors.Is(err, ErrNotFound):
			continue
		case err != nil:
			return nil, nil, err
		}
		au.Name = a.Name // the name as the book credits them
		authors = append(authors, au)
	}
	return b, authors, nil
}

// searchTitle returns title without its bracketed asides, for searching.
func searchTitle(title string) string {
	return strings.Join(strings.Fields(bracketed.ReplaceAllString(title, " ")), " ")
}

// currentASIN returns the ASIN a book has now, or "".
func currentASIN(w catalog.Work) string {
	for _, x := range w.ExternalIDs {
		if x.Provider == catalog.ASIN.Provider && x.Kind == catalog.ASIN.Kind {
			return x.ID
		}
	}
	return ""
}

// fillBook fills the gaps in the description from a book's files with the
// provider's book b. What the files say wins.
//
// The provider's description, publisher and release date fill fields the
// files leave empty. Its series apply only to a book in no series, and its
// authors and narrators only to a book with none. Authors whose names match
// the provider's gain the provider's IDs for them. Genres come only from
// providers, so with no book they are cleared.
func fillBook(m *catalog.WorkMetadata, b *Book, provider string) {
	m.Genres = []string{}
	if b == nil {
		return
	}
	bk := m.Book
	if m.Overview == "" && b.Description != nil {
		m.Overview = *b.Description
	}
	if b.Genres != nil {
		m.Genres = b.Genres
	}
	if len(m.Companies) == 0 && b.Publisher != nil {
		m.Companies = []string{*b.Publisher}
	}
	if m.ReleaseDate == "" && b.ReleaseDate != nil {
		m.ReleaseDate, m.Year = *b.ReleaseDate, year(*b.ReleaseDate)
	}
	if len(bk.Authors) == 0 {
		for _, a := range b.Authors {
			bk.Authors = append(bk.Authors, a.Name)
		}
	}
	if len(bk.Narrators) == 0 {
		bk.Narrators = b.Narrators
	}
	if len(bk.Series) == 0 && len(b.Series) > 0 {
		author := ""
		if len(bk.Authors) > 0 {
			author = bk.Authors[0]
		}
		bk.Series = []catalog.SeriesRef{}
		for i, sr := range b.Series {
			bk.Series = append(bk.Series, catalog.SeriesRef{
				Key: catalog.LocalSeriesKey(author, sr.Name), Name: sr.Name, Position: sr.Position, Primary: i == 0,
			})
		}
	}
	for _, n := range bk.Authors {
		i := slices.IndexFunc(b.Authors, func(a BookAuthor) bool { return a.ID != "" && nameKey(a.Name) == nameKey(n) })
		if i < 0 {
			continue
		}
		if bk.AuthorIDs == nil {
			bk.AuthorIDs = map[string][]catalog.ExternalID{}
		}
		bk.AuthorIDs[n] = []catalog.ExternalID{{Provider: provider, Kind: kindAuthor, ID: b.Authors[i].ID}}
	}
}

// kindAuthor is the kind of an author's external ID.
const kindAuthor = "author"

// keepBook builds a book's description while its provider is unavailable.
// What the files say wins. Everything else keeps its current value,
// including the provider's earlier description and cover.
func (s *Service) keepBook(ctx context.Context, w catalog.Work, out *bookDescription) error {
	m, bk := out.meta, out.meta.Book
	cur, err := s.q.BookText(ctx, w.ID)
	if err != nil {
		return err
	}
	if m.Overview == "" {
		m.Overview = cur.Overview.String
	}
	if m.ReleaseDate == "" {
		m.ReleaseDate, m.Year = cur.ReleaseDate.String, year(cur.ReleaseDate.String)
	}
	if len(m.Companies) == 0 {
		m.Companies = nil // nil leaves them unchanged
	}
	if len(bk.Series) == 0 {
		bk.Series = nil
	}
	if len(bk.Narrators) == 0 {
		bk.Narrators = nil
	}
	bk.ASIN = cmp.Or(bk.ASIN, currentASIN(w))
	rows, err := s.q.WorkImages(ctx, w.ID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Kind == artworkPoster && r.Source == sourceProvider {
			out.found = &found{artwork: map[string][]remoteImage{artworkPoster: {{provider: s.books.Name(), url: r.Origin}}}}
		}
	}
	return nil
}
