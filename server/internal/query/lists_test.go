package query_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
)

func (h *household) lists(v users.Viewer) []query.List {
	h.t.Helper()
	ls, err := h.q.Lists(ctx, v)
	if err != nil {
		h.t.Fatal(err)
	}
	return ls
}

func TestListsMyListExists(t *testing.T) {
	h := newHousehold(t)
	ls := h.lists(h.friend)
	if len(ls) != 1 || !ls[0].BuiltIn || ls[0].Name != "My List" || ls[0].Works != 0 {
		t.Fatalf("a new user's lists: %+v", ls)
	}
	// Each user has their own.
	if h.lists(h.admin)[0].ID == ls[0].ID {
		t.Error("two users share a My List")
	}
}

// putOnList adds a work to a list directly, so a test can say when.
func (h *household) putOnList(list, work id.ID, addedAt int64) {
	h.t.Helper()
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO list_work (list_id, work_id, added_at) VALUES (?, ?, ?)`, list, work, addedAt)
		return err
	})
}

func TestListWorksOrderAndOwnership(t *testing.T) {
	h := newHousehold(t)
	mine := h.lists(h.friend)[0].ID
	h.putOnList(mine, h.dune.Work, 1000)
	h.putOnList(mine, h.toyStory.Work, 2000)
	p, err := h.q.ListWorks(ctx, h.friend, mine, nil, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{h.toyStory.Work, h.dune.Work}; !slices.Equal(ids(p.Items), want) || p.Total != 2 {
		t.Errorf("my list: %v (%d), want most recently added first %v", ids(p.Items), p.Total, want)
	}
	// Adding again keeps its place.
	if err := h.state.AddToList(ctx, h.friend, mine, h.dune.Work); err != nil {
		t.Fatal(err)
	}
	if p2, err := h.q.ListWorks(ctx, h.friend, mine, nil, query.GridOptions{}); err != nil || p2.Total != 2 || p2.Items[0].ID != h.toyStory.Work {
		t.Errorf("after adding again: %+v, %v", p2.Items, err)
	}
	// Playing keeps a work in the list.
	h.play(h.friend, h.dune.Version, 155*time.Minute)
	if p3, err := h.q.ListWorks(ctx, h.friend, mine, nil, query.GridOptions{}); err != nil || p3.Total != 2 {
		t.Errorf("after playing it: total %d, %v", p3.Total, err)
	}

	// It's the Viewer's list: nobody else reads it.
	if _, err := h.q.ListWorks(ctx, h.admin, mine, nil, query.GridOptions{}); !errors.Is(err, query.ErrNotFound) {
		t.Errorf("another user's list: %v", err)
	}
	if err := h.state.AddToList(ctx, h.admin, mine, h.dune.Work); err == nil {
		t.Error("added to another user's list")
	}

	if err := h.state.RemoveFromList(ctx, h.friend, mine, h.dune.Work); err != nil {
		t.Fatal(err)
	}
	if d, err := h.q.Detail(ctx, h.friend, h.toyStory.Work); err != nil || !slices.Equal(d.ListIDs, []id.ID{mine}) {
		t.Errorf("listIds = %v, %v", d.ListIDs, err)
	}
	if d, _ := h.q.Detail(ctx, h.friend, h.dune.Work); len(d.ListIDs) != 0 {
		t.Errorf("listIds after removing: %v", d.ListIDs)
	}
}

// A work the Viewer can't see stays out of the list's page and its count.
func TestListWorksLeaveOutWhatTheViewerCantSee(t *testing.T) {
	h := newHousehold(t)
	mine := h.lists(h.kid)[0].ID
	h.putOnList(mine, h.toyStory.Work, 1000)
	h.putOnList(mine, h.bladeRunner.Work, 2000)
	p, err := h.q.ListWorks(ctx, h.kid, mine, nil, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{h.toyStory.Work}; !slices.Equal(ids(p.Items), want) || p.Total != 1 {
		t.Errorf("the kid's list: %v (%d), want %v", ids(p.Items), p.Total, want)
	}
	if n := h.lists(h.kid)[0].Works; n != 1 {
		t.Errorf("the kid's list counts %d works", n)
	}
}

// A new list stays off an arranged Home, and a reset leaves it off.
func TestListRowsOnAnArrangedHomeAndReset(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	h.arrange(us, h.friend, users.HomeRow{ID: "continueWatching", Shown: true})
	list, err := us.CreateList(ctx, h.friend, "Date Night")
	if err != nil {
		t.Fatal(err)
	}
	row := "list:" + list.String()
	shown := func() bool {
		t.Helper()
		got := h.layout(h.friend)
		i := slices.IndexFunc(got, func(r query.HomeRowInfo) bool { return r.ID == row })
		if i < 0 {
			t.Fatalf("%s isn't in %v", row, rowIDs(got))
		}
		return got[i].Shown
	}
	if shown() {
		t.Error("a new list is on an arranged Home")
	}
	h.arrange(us, h.friend, users.HomeRow{ID: row, Shown: true})
	if !shown() {
		t.Error("the list wasn't turned on")
	}
	if err := us.ResetHomeRows(ctx, h.friend); err != nil {
		t.Fatal(err)
	}
	if shown() {
		t.Error("a reset left the list on Home")
	}
	if got := h.layout(h.friend); got[1].ID != "list:"+h.lists(h.friend)[0].ID.String() || !got[1].Shown {
		t.Errorf("My List after a reset: %+v", got[1])
	}
}

// A stale list's row, or another user's, is ignored when the layout changes.
func TestArrangedHomeRowsIgnoreListsNotTheUsers(t *testing.T) {
	h := newHousehold(t)
	foreign := "list:" + h.lists(h.admin)[0].ID.String()
	stale := "list:" + id.New().String()
	rows, err := h.q.ArrangedHomeRows(ctx, h.friend, []users.HomeRow{{ID: foreign, Shown: true}, {ID: stale, Shown: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == foreign || r.ID == stale {
			t.Errorf("%s is in the layout", r.ID)
		}
	}
}

func TestListWorksOfKinds(t *testing.T) {
	h := newHousehold(t)
	mine := h.lists(h.admin)[0].ID
	h.putOnList(mine, h.dune.Work, 1000)
	h.putOnList(mine, h.hailMary, 2000)
	p, err := h.q.ListWorks(ctx, h.admin, mine, []catalog.WorkKind{catalog.KindMovie}, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{h.dune.Work}; !slices.Equal(ids(p.Items), want) || p.Total != 1 {
		t.Errorf("movies only: %v (%d), want %v", ids(p.Items), p.Total, want)
	}
}

func TestListsCRUDAndHome(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	date, err := us.CreateList(ctx, h.friend, "Date Night")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := us.CreateList(ctx, h.friend, "date night"); !errors.Is(err, users.ErrListNameTaken) {
		t.Errorf("a duplicate name: %v", err)
	}
	if _, err := us.CreateList(ctx, h.friend, " padded"); err == nil {
		t.Error("a padded name was accepted")
	}
	if err := us.RenameList(ctx, h.friend, date, "Road Trip"); err != nil {
		t.Fatal(err)
	}

	got := h.layout(h.friend)
	byID := map[string]query.HomeRowInfo{}
	for _, r := range got {
		byID[r.ID] = r
	}
	mine := "list:" + h.lists(h.friend)[0].ID.String()
	other := "list:" + date.String()
	if !byID[mine].Shown || got[1].ID != mine {
		t.Errorf("My List on Home second, on: %+v", got[:2])
	}
	if byID[other].Shown || got[len(got)-1].ID != other || byID[other].List.Name != "Road Trip" {
		t.Errorf("a new list stays off Home: %+v", byID[other])
	}

	if err := us.DeleteList(ctx, h.friend, h.lists(h.friend)[0].ID); err != users.ErrListBuiltIn {
		t.Errorf("deleting My List: %v", err)
	}
	if err := us.DeleteList(ctx, h.friend, date); err != nil {
		t.Fatal(err)
	}
	// Past the cap a create fails.
	for i := 0; ; i++ {
		_, err := us.CreateList(ctx, h.friend, "list"+string(rune('a'+i%26))+string(rune('a'+i/26)))
		if err != nil {
			if err != users.ErrListLimit || i != users.MaxLists-1 {
				t.Errorf("at the cap: %d, %v", i, err)
			}
			break
		}
	}
}
