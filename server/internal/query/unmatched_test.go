package query_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

// TestUnmatched checks that the Unmatched list holds every library's
// unmatched works, library by library, each with what it was parsed from.
// It also checks paging by cursor and filtering by the Viewer.
func TestUnmatched(t *testing.T) {
	ctx := context.Background()
	h := newHousehold(t)

	p, err := h.q.Unmatched(ctx, h.admin, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []id.ID{h.unmatched.Work, h.unmatchedShow}
	if !slices.Equal(ids(p.Items), want) || p.Next != "" {
		t.Fatalf("Unmatched = %v (next %q), want %v", ids(p.Items), p.Next, want)
	}
	paths := []string{"/media/Movies/some.film.2023.mkv", fmt.Sprintf("/media/Shows/Show %v", h.unmatchedShow)}
	for i, c := range p.Items {
		if c.Path != paths[i] || c.MatchState != query.Unmatched || c.Library.IsZero() {
			t.Errorf("%s: path %q, match %s, library %v; want path %q", h.names[c.ID], c.Path, c.MatchState, c.Library, paths[i])
		}
	}

	// With one work per page, the cursor walks from Movies to Shows.
	var got []id.ID
	var after query.Cursor
	for range 3 {
		p, err := h.q.Unmatched(ctx, h.admin, after, 1)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ids(p.Items)...)
		if after = p.Next; after == "" {
			break
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("paged one at a time = %v, want %v", got, want)
	}

	for _, c := range []struct {
		name string
		v    users.Viewer
		want []id.ID
	}{
		{"friend", h.friend, want},
		{"kid", h.kid, nil}, // blocks unrated works, as unmatched ones are
		{"nobody", h.nobody, nil},
	} {
		p, err := h.q.Unmatched(ctx, c.v, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(p.Items); !slices.Equal(got, c.want) {
			t.Errorf("%s: Unmatched = %v, want %v", c.name, got, c.want)
		}
	}

	if _, err := h.q.Unmatched(ctx, h.admin, "bogus", 0); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("a bogus cursor: %v, want ErrInvalid", err)
	}
	history, err := json.Marshal(map[string]any{"l": "history", "n": []int64{1}, "i": h.dune.Work})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.q.Unmatched(ctx, h.admin, query.Cursor(base64.RawURLEncoding.EncodeToString(history)), 0); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("another list's cursor: %v, want ErrInvalid", err)
	}
}
