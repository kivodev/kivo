package query_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

func rowIDs(rows []query.HomeRowInfo) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func (h *household) layout(v users.Viewer) []query.HomeRowInfo {
	h.t.Helper()
	rows, err := h.q.HomeRows(ctx, v)
	if err != nil {
		h.t.Fatal(err)
	}
	return rows
}

func (h *household) arrange(us *users.Service, v users.Viewer, changes ...users.HomeRow) []query.HomeRowInfo {
	h.t.Helper()
	rows, err := h.q.ArrangedHomeRows(ctx, v, changes)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := us.SetHomeRows(ctx, v, rows); err != nil {
		h.t.Fatal(err)
	}
	return h.layout(v)
}

// shownIDs lists the IDs of the rows shown.
func shownIDs(rows []query.HomeRowInfo) []string {
	var out []string
	for _, r := range rows {
		if r.Shown {
			out = append(out, r.ID)
		}
	}
	return out
}

func TestHomeRowsDefaultOrder(t *testing.T) {
	h := newHousehold(t)
	got := rowIDs(h.layout(h.admin))
	movies, shows, books := "recentlyAdded:"+h.movies.String(), "recentlyAdded:"+h.shows.String(), "recentlyAdded:"+h.books.String()
	wantFirst := []string{"continueWatching", "list:" + h.lists(h.admin)[0].ID.String(), movies, shows}
	if !slices.Equal(got[:4], wantFirst) {
		t.Errorf("default order %v, want first %v", got, wantFirst)
	}
	cl, b := slices.Index(got, "continueListening"), slices.Index(got, books)
	if cl < 4 || cl+1 != b {
		t.Errorf("Continue Listening and the audiobooks row: %v", got)
	}
	if shown := shownIDs(h.layout(h.admin)); len(shown) != len(got) {
		t.Errorf("every row shown by default: %v of %v", shown, got)
	}
}

func TestContinueListeningNeedsAnAudiobooksLibrary(t *testing.T) {
	h := newHousehold(t)
	if !slices.Contains(rowIDs(h.layout(h.admin)), "continueListening") {
		t.Error("the admin, who sees audiobooks, has no Continue Listening row")
	}
	if got := rowIDs(h.layout(h.friend)); slices.Contains(got, "continueListening") {
		t.Errorf("the friend, who sees no audiobooks library, has one: %v", got)
	}
}

func TestHomeRowsArrange(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	movies, shows := "recentlyAdded:"+h.movies.String(), "recentlyAdded:"+h.shows.String()

	// Listed rows come first in order, and the rest follow, keeping shown.
	got := h.arrange(us, h.admin, users.HomeRow{ID: shows, Shown: true}, users.HomeRow{ID: movies, Shown: false})
	ids := rowIDs(got)
	if !slices.Equal(ids[:2], []string{shows, movies}) || got[1].Shown || !got[2].Shown {
		t.Errorf("arranged %v", got)
	}

	// A hidden row with works stays off Home.
	if home := h.home(h.admin); len(recentlyAdded(home)) == 0 || recentlyAdded(home)[0].ID != shows {
		t.Errorf("Home after hiding Movies: %v", ids)
	} else if slices.ContainsFunc(home, func(r query.HomeRow) bool { return r.ID == movies }) {
		t.Error("the hidden Movies row is on Home")
	}

	// A ?kind= read returns it anyway.
	hidden, err := h.q.Home(ctx, h.admin, query.RowRecentlyAdded)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(hidden, func(r query.HomeRow) bool { return r.ID == movies }) {
		t.Errorf("the kind read leaves out the hidden row: %v", hidden)
	}

	// A new library joins the end, shown.
	newLib, _ := h.library("New Movies", "movies")
	got = h.layout(h.admin)
	last := got[len(got)-1]
	if last.ID != "recentlyAdded:"+newLib.String() || !last.Shown {
		t.Errorf("new library row: %v", last)
	}

	// A row of a kind added later joins an arranged Home off: with its
	// stored state gone, it comes last and hidden.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM user_home_row WHERE user_id = ? AND row_id = 'continueListening'`, h.admin.User())
		return err
	})
	got = h.layout(h.admin)
	if last := got[len(got)-1]; last.ID != "continueListening" || last.Shown {
		t.Errorf("a new built-in row on an arranged Home: %+v", last)
	}

	if err := us.ResetHomeRows(ctx, h.admin); err != nil {
		t.Fatal(err)
	}
	got = h.layout(h.admin)
	if got[0].ID != "continueWatching" || len(shownIDs(got)) != len(got) {
		t.Errorf("after reset: %v", got)
	}
}

func TestHomeRowsUnarrangedHidden(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	lib := "recentlyAdded:" + h.movies.String()
	// A row stored hidden with no position leaves the Home unarranged, so
	// nothing else moves and the library order still decides.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO user_home_row (user_id, row_id, position, shown) VALUES (?, ?, NULL, 0)`, h.admin.User(), lib)
		return err
	})
	got := h.layout(h.admin)
	if got[0].ID != "continueWatching" || got[2].ID != lib || got[2].Shown {
		t.Errorf("hidden row on an unarranged Home: %+v", got[:3])
	}
	// Changing the library order still reorders Recently Added.
	if err := us.EditPreferences(ctx, h.admin, users.PreferencesChange{Libraries: &[]users.LibraryChoice{{Library: h.shows, Pinned: true}, {Library: h.movies, Pinned: true}}}); err != nil {
		t.Fatal(err)
	}
	got = h.layout(h.admin)
	if got[2].ID != "recentlyAdded:"+h.shows.String() || got[3].ID != lib {
		t.Errorf("library order on an unarranged Home: %+v", got[:4])
	}
	// Pins don't touch Home at all: an unpinned library's row is still there.
	if err := us.EditPreferences(ctx, h.admin, users.PreferencesChange{Libraries: &[]users.LibraryChoice{{Library: h.shows, Pinned: false}}}); err != nil {
		t.Fatal(err)
	}
	home := h.home(h.admin)
	if !slices.ContainsFunc(home, func(r query.HomeRow) bool { return r.ID == "recentlyAdded:"+h.shows.String() }) {
		t.Error("unpinning a library hid its row")
	}
}

func TestHomeRowsEmptyRowKeptInEditor(t *testing.T) {
	h := newHousehold(t)
	// Nothing played, so Continue Watching has no works: off Home, in the editor.
	if home := h.home(h.admin); slices.ContainsFunc(home, func(r query.HomeRow) bool { return r.Kind == query.RowContinueWatching }) {
		t.Error("an empty Continue Watching is on Home")
	}
	if got := rowIDs(h.layout(h.admin)); !slices.Contains(got, "continueWatching") {
		t.Errorf("the editor is missing Continue Watching: %v", got)
	}
	if got := query.ValidRowID("recentlyAdded:nope"); got {
		t.Error("a malformed recentlyAdded id passed")
	}
	if !query.ValidRowID("recentlyAdded:"+h.movies.String()) || !query.ValidRowID("continueListening") {
		t.Error("a well-formed id was refused")
	}
}
