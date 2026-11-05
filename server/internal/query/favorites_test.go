package query_test

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/userstate"
)

// TestFavorites checks the Favorites lists. They hold every work the user
// marked and still sees, most recently marked first, whatever changed
// since. They can list one kind at a time and are paged by offset. Home's
// row follows the same rules.
func TestFavorites(t *testing.T) {
	h := newHousehold(t)
	mark := func(work id.ID, at int64) {
		t.Helper()
		if err := h.state.Update(ctx, h.friend, work, userstate.Change{Favorite: new(true)}); err != nil {
			t.Fatal(err)
		}
		h.b.Write(func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE user_state SET favorited_at = ? WHERE user_id = ? AND work_id = ?`, at, h.friend.User(), work)
			return err
		})
	}
	mark(h.toyStory.Work, 1000)
	mark(h.expanse2.Work, 2000)
	mark(h.dune.Work, 3000)
	// This favorite is in a library the friend isn't granted. It stands for
	// one marked before a grant was taken away, because userstate marks
	// only works the user sees.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO user_state (user_id, work_id, favorite, favorited_at, updated_at) VALUES (?, ?, 1, 4000, 4000)`,
			h.friend.User(), h.secret.Work)
		return err
	})
	mark(h.bluey, 5000)
	// Playing Toy Story and marking it again changes its state but not its
	// place.
	h.play(h.friend, h.toyStory.Version, 10*time.Minute)
	mark(h.toyStory.Work, 1000)
	if err := h.state.Update(ctx, h.friend, h.toyStory.Work, userstate.Change{Favorite: new(true)}); err != nil {
		t.Fatal(err)
	}

	all := []id.ID{h.bluey, h.dune.Work, h.expanse2.Work, h.toyStory.Work}
	p, err := h.q.Favorites(ctx, h.friend, "", query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(p.Items), all) || p.Total != 4 {
		t.Errorf("favorites = %v, total %d; want %v", ids(p.Items), p.Total, all)
	}
	home, err := h.q.Home(ctx, h.friend)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(row(home, query.RowFavorites)), all) {
		t.Errorf("Home's Favorites = %v, want %v", ids(row(home, query.RowFavorites)), all)
	}

	eps, err := h.q.Favorites(ctx, h.friend, "episode", query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(eps.Items) != 1 || eps.Total != 1 {
		t.Fatalf("episodes = %v", ids(eps.Items))
	}
	if e := eps.Items[0]; e.ID != h.expanse2.Work || e.SeriesTitle == "" || e.Number != 2 || e.ParentNumber != 1 {
		t.Errorf("episode summary %+v, want its series' title and S1 E2", e)
	}

	// Pages by offset, each with the total.
	for _, o := range []query.GridOptions{{Limit: 2}, {Offset: 2, Limit: 2}, {Offset: 1, Limit: 1}, {Offset: 4}} {
		p, err := h.q.Favorites(ctx, h.friend, "", o)
		if err != nil {
			t.Fatal(err)
		}
		end := min(o.Offset+cmp.Or(o.Limit, 60), len(all))
		if !slices.Equal(ids(p.Items), all[o.Offset:end]) || p.Total != 4 {
			t.Errorf("%+v: %v, total %d; want %v", o, ids(p.Items), p.Total, all[o.Offset:end])
		}
	}
	if _, err := h.q.Favorites(ctx, h.friend, "", query.GridOptions{Offset: -1}); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("a negative offset: %v", err)
	}

	// Unmarking takes a work off. Marking it again puts it first.
	if err := h.state.Update(ctx, h.friend, h.toyStory.Work, userstate.Change{Favorite: new(false)}); err != nil {
		t.Fatal(err)
	}
	if err := h.state.Update(ctx, h.friend, h.toyStory.Work, userstate.Change{Favorite: new(true)}); err != nil {
		t.Fatal(err)
	}
	movies, err := h.q.Favorites(ctx, h.friend, "movie", query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{h.toyStory.Work, h.dune.Work}; !slices.Equal(ids(movies.Items), want) {
		t.Errorf("movies = %v, want %v", ids(movies.Items), want)
	}
}
