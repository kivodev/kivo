package query

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/mmcdole/kivo/internal/id"
)

// A grid is a listing that a client shows as a grid and can load at any
// position. The grids are a library's works, its collections, an
// audiobooks library's authors and series, and a list's works. A page
// is a run of the listing from an offset. Ties in a grid's order break by
// title and then ID, so every row has one place.

// GridOptions picks a page of a grid.
type GridOptions struct {
	Offset int // how many items to skip from the start
	Limit  int // the page size: 60 when zero, and at most 200
}

// limit checks the options and returns the page size.
func (o GridOptions) limit() (int, error) {
	if o.Offset < 0 {
		return 0, invalid("offset", "outOfRange", fmt.Sprintf("offset %d", o.Offset))
	}
	return pageLimit(o.Limit)
}

// GridPage is a page of a grid.
type GridPage[T any] struct {
	Items []T
	// Total is the grid's size. A library's works, a list's works and a
	// query's works have it on the first page (offset 0) only, and 0 on a
	// later page.
	Total int
	// Letters is where each letter starts. It is set on the first page
	// (offset 0) of a grid sorted by title or by name, and is nil otherwise.
	Letters []Letter
}

// gridPage returns items as a page, never with nil Items.
func gridPage[T any](items []T, total int, letters []Letter) GridPage[T] {
	if items == nil {
		items = []T{}
	}
	return GridPage[T]{Items: items, Total: total, Letters: letters}
}

// summaryPage decorates a grid's summaries and returns them as its page.
func (s *Service) summaryPage(ctx context.Context, vw viewer, items []WorkSummary, total int, letters []Letter) (GridPage[WorkSummary], error) {
	if err := s.decorate(ctx, vw, items); err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return gridPage(items, total, letters), nil
}

// Letter is where a letter starts in a grid sorted by title, in a
// library's "Collections" list, or in an audiobooks library's "Authors" or
// "Series" list. Works file under the first character of their sort title,
// so leading articles are skipped. People, series and collections file
// under the first character of their sort name. The
// letters are A to Z, with # for anything else. Each letter is listed once,
// at its first row in the list's order.
type Letter struct {
	Letter string // "A" to "Z", or "#"
	Offset int
}

// letterCount is a row of CollectionLetters, AuthorLetters or
// BookSeriesLetters, or of a query's letters. It counts how many of a
// list's rows start with one character, given as a code point, or 0 for an
// empty name.
type letterCount struct {
	Initial int64
	N       int64
}

func scanLetterCount(r scanner, c *letterCount) error { return r.Scan(&c.Initial, &c.N) }

// rail reads a list's counts by first character. It returns the list's
// total, and where each letter starts when the page is the first one.
func rail(ctx context.Context, q querier, counts statement, args []any, o GridOptions) ([]Letter, int, error) {
	rows, err := rowsOf(ctx, q, counts, args, scanLetterCount)
	if err != nil {
		return nil, 0, err
	}
	letters, total := railLetters(rows)
	if o.Offset > 0 {
		letters = nil
	}
	return letters, total, nil
}

// nameList reads a page of one of a library's lists by name, from one
// snapshot: it checks that the Viewer sees the library, reads the letters
// and total from counts, and reads the page from list with scan.
func nameList[T any](ctx context.Context, s *Service, vw viewer, lib id.ID, o GridOptions,
	counts, list statement, scan func(scanner, *T) error) (GridPage[T], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[T]{}, err
	}
	var items []T
	var letters []Letter
	total := 0
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := library(ctx, tx, vw, lib); err != nil {
			return notFound(err)
		}
		// The letters' counts add up to the total.
		var err error
		if letters, total, err = rail(ctx, tx, counts, vw.args(sql.Named("library", lib)), o); err != nil {
			return err
		}
		items, err = rowsOf(ctx, tx, list, vw.args(sql.Named("library", lib),
			sql.Named("offset", o.Offset), sql.Named("limit", limit)), scan)
		return err
	})
	if err != nil {
		return GridPage[T]{}, err
	}
	return gridPage(items, total, letters), nil
}

// railLetters turns a list's counts by first character, in the list's
// order, into where each letter starts, and adds up the total.
func railLetters(rows []letterCount) ([]Letter, int) {
	letters := []Letter{}
	at := 0
	for _, r := range rows {
		l := "#"
		if c := rune(r.Initial); c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			l = string(c &^ 0x20) // upper case
		}
		if !slices.ContainsFunc(letters, func(x Letter) bool { return x.Letter == l }) {
			letters = append(letters, Letter{Letter: l, Offset: at})
		}
		at += int(r.N)
	}
	return letters, at
}
