package query_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// home reads the Viewer's Home rows.
func (h *household) home(v users.Viewer) []query.HomeRow {
	h.t.Helper()
	home, _, err := h.q.Home(ctx, v)
	if err != nil {
		h.t.Fatal(err)
	}
	return home
}

// versionOf is the version of a work with one version.
func (h *household) versionOf(work id.ID) id.ID {
	h.t.Helper()
	var v id.ID
	if err := h.b.DB.Reader().QueryRow(`SELECT version_id FROM media_version_work WHERE work_id = ?`, work).Scan(&v); err != nil {
		h.t.Fatal(err)
	}
	return v
}

// TestRemoveFromContinue checks that a work removed from Continue Watching
// or Continue Listening keeps its position, and comes back when it's
// played again. A series comes back when any of its episodes is played,
// even briefly.
func TestRemoveFromContinue(t *testing.T) {
	h := newHousehold(t)
	h.play(h.admin, h.dune.Version, 40*time.Minute)
	h.play(h.admin, h.expanse1.Version, 24*time.Minute)
	h.play(h.admin, h.versionOf(h.hailMary), 30*time.Minute)
	remove := func(work id.ID) {
		t.Helper()
		if err := h.state.RemoveFromContinue(ctx, h.admin, work); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // so a play after it is later
	}
	check := func(name string, watching, listening []id.ID) {
		t.Helper()
		home := h.home(h.admin)
		if got := ids(row(home, query.RowContinueWatching)); !slices.Equal(got, watching) {
			t.Errorf("%s: Continue Watching %v, want %v", name, got, watching)
		}
		if got := ids(row(home, query.RowContinueListening)); !slices.Equal(got, listening) {
			t.Errorf("%s: Continue Listening %v, want %v", name, got, listening)
		}
	}
	check("before", []id.ID{h.expanse2.Work, h.dune.Work}, []id.ID{h.hailMary})

	// The card shows the episode up next, which takes its series out.
	remove(h.dune.Work)
	remove(h.expanse2.Work)
	remove(h.hailMary)
	check("removed", nil, nil)
	st, err := h.q.Userstate(ctx, h.admin, h.dune.Work)
	if err != nil {
		t.Fatal(err)
	}
	if st.Position != 40*time.Minute {
		t.Errorf("Dune's position after removing %v, want 40m", st.Position)
	}
	if st, _ := h.q.Userstate(ctx, h.admin, h.expanse1.Work); !st.Played {
		t.Error("the episode watched is no longer played")
	}

	h.play(h.admin, h.dune.Version, 41*time.Minute)
	check("Dune played again", []id.ID{h.dune.Work}, nil)
	h.play(h.admin, h.expanse3.Version, 10*time.Second)
	check("an Expanse episode glimpsed", []id.ID{h.dune.Work, h.expanse2.Work}, nil)
	h.play(h.admin, h.versionOf(h.hailMary), 31*time.Minute)
	check("the book played again", []id.ID{h.dune.Work, h.expanse2.Work}, []id.ID{h.hailMary})

	// Removing is the user's own.
	h.play(h.friend, h.dune.Version, 40*time.Minute)
	remove(h.dune.Work)
	if got := ids(row(h.home(h.friend), query.RowContinueWatching)); !slices.Equal(got, []id.ID{h.dune.Work}) {
		t.Errorf("the friend's Continue Watching %v after the admin removed Dune", got)
	}
}

// TestStaleSeries checks that a series' next episode leaves Continue
// Watching a year after the user last played in the series. An episode
// added since that play keeps it. Movies and books in progress stay however
// old.
func TestStaleSeries(t *testing.T) {
	h := newHousehold(t)
	h.play(h.admin, h.dune.Version, 40*time.Minute)
	h.play(h.admin, h.expanse1.Version, 24*time.Minute)
	h.play(h.admin, h.versionOf(h.hailMary), 30*time.Minute)
	// Everything in the household was added long before the plays.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE work SET added_at = added_at - ?`, (800 * 24 * time.Hour).Milliseconds()); err != nil {
			return err
		}
		return redate(ctx, tx)
	})
	playedAgo := func(days int) {
		h.b.Write(func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE user_state SET last_played_at = ? WHERE user_id = ? AND last_played_at IS NOT NULL`,
				time.Now().Add(-time.Duration(days)*24*time.Hour).UnixMilli(), h.admin.User())
			return err
		})
	}
	check := func(name string, watching, listening []id.ID) {
		t.Helper()
		home := h.home(h.admin)
		for _, r := range []struct {
			kind query.RowKind
			want []id.ID
		}{{query.RowContinueWatching, watching}, {query.RowContinueListening, listening}} {
			if got := ids(row(home, r.kind)); !slices.Equal(got, r.want) {
				t.Errorf("%s: %s %v, want %v", name, r.kind, got, r.want)
			}
		}
	}

	playedAgo(364)
	check("364 days", []id.ID{h.dune.Work, h.expanse2.Work}, []id.ID{h.hailMary})
	playedAgo(366)
	check("366 days", []id.ID{h.dune.Work}, []id.ID{h.hailMary})

	h.b.Episode(h.showsFolder, h.expanse, 2, 1)
	check("added since", []id.ID{h.dune.Work, h.expanse2.Work}, []id.ID{h.hailMary})
}

// TestRecentlyAddedPlayed checks that Recently Added leaves out what the
// user played. A bundle stays until every episode in it is played, and the
// row stays full.
func TestRecentlyAddedPlayed(t *testing.T) {
	h := newHousehold(t)
	h.play(h.admin, h.toyStory.Version, 2*time.Hour)
	h.play(h.admin, h.versionOf(h.leviathan), 2*time.Hour-time.Minute)
	h.play(h.admin, h.expanse1.Version, 25*time.Minute)
	h.play(h.admin, h.expanse2.Version, 25*time.Minute)
	h.play(h.friend, h.expanse3.Version, 25*time.Minute)
	added := func(v users.Viewer, lib id.ID) []id.ID {
		t.Helper()
		for _, r := range recentlyAdded(h.home(v)) {
			if r.Library.ID == lib {
				return ids(r.Items)
			}
		}
		return nil
	}
	if got := added(h.admin, h.movies); slices.Contains(got, h.toyStory.Work) || !slices.Contains(got, h.toyStory2.Work) {
		t.Errorf("Movies %v, want Toy Story 2 without Toy Story", got)
	}
	if got := added(h.admin, h.books); slices.Contains(got, h.leviathan) || !slices.Contains(got, h.churn) {
		t.Errorf("Audiobooks %v, want The Churn without Leviathan Wakes", got)
	}
	if got := added(h.admin, h.shows); !slices.Contains(got, h.expanseS1) {
		t.Errorf("Shows %v, want The Expanse's season with one episode left", got)
	}
	h.play(h.admin, h.expanse3.Version, 25*time.Minute)
	if got := added(h.admin, h.shows); slices.Contains(got, h.expanseS1) {
		t.Errorf("Shows %v, with every episode of the season played", got)
	}
	if got := added(h.friend, h.movies); !slices.Contains(got, h.toyStory.Work) {
		t.Errorf("the friend's Movies %v, want Toy Story", got)
	}

	// A played movie, the newest, leaves room for an older one.
	for i := range 25 {
		h.movie(h.moviesFolder, fmt.Sprintf("Film %02d", i), 2000, rankG)
	}
	latest := h.movie(h.moviesFolder, "Latest", 2000, rankG).Work
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE work SET added_at = ? WHERE id = ?`, time.Now().Add(time.Hour).UnixMilli(), latest); err != nil {
			return err
		}
		return redate(ctx, tx)
	})
	if got := added(h.admin, h.movies); len(got) != 20 || got[0] != latest {
		t.Fatalf("Movies %v, want 20 from the newest", got)
	}
	if err := h.state.Update(context.Background(), h.admin, latest, userstate.Change{Played: new(true)}); err != nil {
		t.Fatal(err)
	}
	if got := added(h.admin, h.movies); len(got) != 20 || slices.Contains(got, latest) {
		t.Errorf("Movies %v, want 20 without the played one", got)
	}
}
