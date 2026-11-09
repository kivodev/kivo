package query_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
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
	// A new user gets My List on creation too.
	if h.lists(h.admin)[0].ID == ls[0].ID {
		t.Error("two users share a My List")
	}
}

func TestListWorksOrderAndVisibility(t *testing.T) {
	h := newHousehold(t)
	mine := h.lists(h.friend)[0].ID
	add := func(w id.ID) {
		t.Helper()
		if err := h.state.AddToList(ctx, h.friend, mine, w); err != nil {
			t.Fatal(err)
		}
	}
	add(h.dune.Work)
	time.Sleep(2 * time.Millisecond)
	add(h.toyStory.Work)
	p, err := h.q.ListWorks(ctx, h.friend, mine, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{h.toyStory.Work, h.dune.Work}; !slices.Equal(ids(p.Items), want) || p.Total != 2 {
		t.Errorf("my list: %v (%d), want most recently added first %v", ids(p.Items), p.Total, want)
	}
	// Adding again keeps its place.
	add(h.dune.Work)
	if p2, err := h.q.ListWorks(ctx, h.friend, mine, query.GridOptions{}); err != nil || p2.Total != 2 || p2.Items[0].ID != h.toyStory.Work {
		t.Errorf("after adding again: %+v, %v", p2.Items, err)
	}
	// Playing keeps a work in the list.
	h.play(h.friend, h.dune.Version, 155*time.Minute)
	if p3, err := h.q.ListWorks(ctx, h.friend, mine, query.GridOptions{}); err != nil || p3.Total != 2 {
		t.Errorf("after playing it: total %d, %v", p3.Total, err)
	}

	// It's the Viewer's list: nobody else reads it.
	if _, err := h.q.ListWorks(ctx, h.admin, mine, query.GridOptions{}); !errors.Is(err, query.ErrNotFound) {
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
