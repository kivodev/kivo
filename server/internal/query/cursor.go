package query

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"

	"github.com/mmcdole/kivo/internal/id"
)

// Page is one page of a listing that is read in order by cursor: History,
// Missing files and Unmatched. A grid is read by position instead (see
// GridPage).
type Page[T any] struct {
	Items []T
	Next  Cursor // empty on the last page
	Total int    // the listing's size, set only on the first page (no cursor)
}

// Cursor marks where the next page of a listing starts. It is opaque, so
// pass back what Page.Next returned. The empty Cursor asks for the first
// page.
type Cursor string

// Page sizes: the default when a request asks for none, and the most a
// request may ask for.
const (
	defaultLimit = 60
	maxLimit     = 200
)

// pageLimit checks a requested page size. Zero asks for the default.
func pageLimit(n int) (int, error) {
	switch {
	case n == 0:
		return defaultLimit, nil
	case n < 0 || n > maxLimit:
		return 0, invalid("limit", "outOfRange", fmt.Sprintf("limit %d", n))
	}
	return n, nil
}

// position is the sort key of the last item a page returned. The page's
// Cursor carries it, so the next page starts after that item (keyset
// paging). No page costs more for being deep, and rows added or removed in
// the meantime don't shift what comes next. A position names its listing,
// so a cursor from one sort can't be replayed against another.
type position struct {
	List    string  `json:"l"`
	Library id.ID   `json:"b,omitzero"`  // the Unmatched list's first key
	N       []int64 `json:"n,omitempty"` // numeric keys, in sort order
	Title   string  `json:"t,omitempty"`
	ID      id.ID   `json:"i"`
}

// cursor encodes p as JSON in base64.
func (p position) cursor() Cursor {
	b, _ := json.Marshal(p)
	return Cursor(base64.RawURLEncoding.EncodeToString(b))
}

// decode reads c, which must belong to list and carry n numeric keys. The
// empty cursor asks for the first page, and decode reports it with ok
// false.
func (c Cursor) decode(list string, n int) (p position, ok bool, err error) {
	if c == "" {
		return position{}, false, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(string(c))
	if err != nil || json.Unmarshal(b, &p) != nil || p.List != list || len(p.N) != n {
		return position{}, false, invalid("after", "invalid", "cursor")
	}
	return p, true, nil
}

// page trims the one extra row each listing fetches to learn whether there
// is a next page. It makes the cursor from the last item kept.
func page[T any](items []T, limit int, total int, key func(T) position) Page[T] {
	p := Page[T]{Items: items, Total: total}
	if len(items) > limit {
		p.Items = items[:limit]
		p.Next = key(p.Items[limit-1]).cursor()
	}
	if p.Items == nil {
		p.Items = []T{}
	}
	return p
}

// startDesc is the key at the start of a listing by a descending numeric
// key, before every real key.
const startDesc = math.MaxInt64
