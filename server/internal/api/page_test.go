package api

import (
	"testing"

	"github.com/mmcdole/kivo/internal/query"
)

// A request's paging keeps its size within bounds and knows its first
// page. The total is given on the first page only. The cursor is parsed or
// refused.
func TestPaging(t *testing.T) {
	if _, err := pagingOf(nil, ptr(0)); problemOf(err).code != codeInvalidRequest {
		t.Errorf("limit 0: %v", err)
	}
	if p, _ := pagingOf(nil, ptr(201)); p.limit != 201 {
		t.Errorf("a limit for the list to check %+v", p)
	}
	if _, err := ownPagingOf(nil, ptr(201), 50, 200); err == nil {
		t.Error("limit over the most")
	}
	p, err := ownPagingOf(nil, nil, 50, 200)
	if err != nil || p.limit != 50 || !p.first() || p.total(7) == nil || *p.total(7) != 7 {
		t.Errorf("first page %+v %v", p, err)
	}
	p, _ = pagingOf(ptr("12"), ptr(2))
	if n, err := cursor(p, parseSeq, "bad"); err != nil || n != 12 || p.first() || p.total(7) != nil {
		t.Errorf("cursor %d %v", n, err)
	}
	if _, err := cursor(paging{after: "-1"}, parseSeq, "bad"); problemOf(err).detail != "bad" {
		t.Errorf("a refused cursor: %v", err)
	}
}

// A grid request's paging reads its offset and limit, and refuses a limit
// under 1.
func TestGridPaging(t *testing.T) {
	if _, err := gridOptionsOf(nil, ptr(0)); problemOf(err).code != codeInvalidRequest {
		t.Errorf("limit 0: %v", err)
	}
	if o, err := gridOptionsOf(ptr(120), ptr(60)); err != nil || o != (query.GridOptions{Offset: 120, Limit: 60}) {
		t.Errorf("an offset: %+v %v", o, err)
	}
}

func problemOf(err error) problem {
	p, _ := problemFor(err)
	return p
}
