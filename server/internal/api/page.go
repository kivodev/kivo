package api

import "github.com/mmcdole/kivo/internal/query"

// The default and maximum page sizes of the lists that api pages itself.
// The lists in query have their own sizes.
const (
	defaultLimit = 50
	maxLimit     = 200
)

// paging is the paging of a list request read in order.
//   - after is the cursor the previous page ended at, or "" for the first
//     page.
//   - limit is the page size, or 0 when the request gives none.
type paging struct {
	after string
	limit int
}

// pagingOf reads a request's paging. A limit under 1 is invalid. The list
// that owns the page size checks the maximum.
func pagingOf(after *string, limit *int) (paging, error) {
	if limit != nil && *limit < 1 {
		return paging{}, errLimit
	}
	return paging{after: deref(after), limit: deref(limit)}, nil
}

// ownPagingOf reads the paging of a list that api pages itself. def is the
// page size when the request gives none. A limit over max is invalid.
func ownPagingOf(after *string, limit *int, def, max int) (paging, error) {
	p, err := pagingOf(after, limit)
	switch {
	case err != nil:
		return paging{}, err
	case p.limit == 0:
		p.limit = def
	case p.limit > max:
		return paging{}, errLimit
	}
	return p, nil
}

var errLimit = invalid("limit is out of range", field("limit", "outOfRange"))

// first reports whether the request is for a list's first page.
func (p paging) first() bool { return p.after == "" }

// total returns a list's total. Only the first page carries it.
func (p paging) total(n int) *int {
	if !p.first() {
		return nil
	}
	return &n
}

// gridOptionsOf reads a grid request's offset and limit. A limit under 1
// is invalid. Query checks the maximum.
func gridOptionsOf(offset, limit *int) (query.GridOptions, error) {
	if limit != nil && *limit < 1 {
		return query.GridOptions{}, errLimit
	}
	return query.GridOptions{Offset: deref(offset), Limit: deref(limit)}, nil
}

// cursor reads the cursor with parse, which refuses a malformed cursor.
// The zero T is the first page's cursor. detail says what's wrong with a
// refused cursor.
func cursor[T any](p paging, parse func(string) (T, error), detail string) (T, error) {
	var zero T
	if p.after == "" {
		return zero, nil
	}
	c, err := parse(p.after)
	if err != nil {
		return zero, invalid(detail, field("after", "invalid"))
	}
	return c, nil
}
