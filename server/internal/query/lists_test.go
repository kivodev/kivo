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
	list, err := us.CreateList(ctx, h.friend, "Date Night", nil)
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

func movies(rules ...query.Rule) query.WorkQuery {
	return query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie}, Where: where(rules...)}
}

func (h *household) smartList(us *users.Service, v users.Viewer, name string, q query.WorkQuery) id.ID {
	h.t.Helper()
	saved, err := q.Saved()
	if err != nil {
		h.t.Fatal(err)
	}
	list, err := us.CreateList(ctx, v, name, &saved)
	if err != nil {
		h.t.Fatal(err)
	}
	return list
}

func TestSmartLists(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	toyStory := movies(query.Rule{Field: "title", Op: "contains", Values: []string{"toy"}})
	list := h.smartList(us, h.admin, "Toys", toyStory)

	got, err := h.q.List(ctx, h.admin, list)
	if err != nil {
		t.Fatal(err)
	}
	if got.Smart == nil || len(got.Smart.Errors) != 0 || got.Works != 2 || got.Smart.Query.Version != 1 {
		t.Errorf("a saved smart list: %+v, %+v", got, got.Smart)
	}
	p, err := h.q.ListWorks(ctx, h.admin, list, nil, query.GridOptions{})
	if err != nil || p.Total != 2 || !slices.Contains(ids(p.Items), h.toyStory.Work) {
		t.Errorf("its works: %v (%d), %v", ids(p.Items), p.Total, err)
	}
	if p, err := h.q.ListWorks(ctx, h.admin, list, []catalog.WorkKind{catalog.KindBook}, query.GridOptions{}); err != nil || p.Total != 0 {
		t.Errorf("kinds it doesn't select: %d, %v", p.Total, err)
	}

	// Membership is refused on a smart list; a manual list refuses rules.
	if err := h.state.AddToList(ctx, h.admin, list, h.dune.Work); !errors.Is(err, users.ErrListKind) {
		t.Errorf("adding to a smart list: %v", err)
	}
	if err := h.state.RemoveFromList(ctx, h.admin, list, h.toyStory.Work); !errors.Is(err, users.ErrListKind) {
		t.Errorf("removing from a smart list: %v", err)
	}
	mine := h.lists(h.admin)[0].ID
	if err := us.SetListQuery(ctx, h.admin, mine, `{}`); !errors.Is(err, users.ErrListBuiltIn) {
		t.Errorf("My List's rules: %v", err)
	}
	manual, _ := us.CreateList(ctx, h.admin, "Manual", nil)
	if err := us.SetListQuery(ctx, h.admin, manual, `{}`); !errors.Is(err, users.ErrListKind) {
		t.Errorf("rules on a manual list: %v", err)
	}

	// A new smart list is off Home, and on when enabled, honouring maxWorks.
	row := "list:" + list.String()
	if slices.ContainsFunc(h.layout(h.admin), func(r query.HomeRowInfo) bool { return r.ID == row && r.Shown }) {
		t.Error("a new smart list is on Home")
	}
	one := 1
	capped := toyStory
	capped.MaxWorks = &one
	saved, _ := capped.Saved()
	if err := us.SetListQuery(ctx, h.admin, list, saved); err != nil {
		t.Fatal(err)
	}
	h.arrange(us, h.admin, users.HomeRow{ID: row, Shown: true})
	home := h.home(h.admin)
	i := slices.IndexFunc(home, func(r query.HomeRow) bool { return r.ID == row })
	if i < 0 || len(home[i].Items) != 1 {
		t.Fatalf("the smart list on Home: %+v", home)
	}

	// Rules naming a person follow a merge, and rules that no longer run
	// keep the list, empty and off Home.
	byPerson := h.smartList(us, h.admin, "By person", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindBook},
		Where: where(query.Rule{Field: "author", Op: "anyOf", Values: []string{h.weir.String()}})})
	into, err := h.b.Catalog.RenamePerson(ctx, h.weir, "James S. A. Corey")
	if err != nil || into != h.corey {
		t.Fatalf("merging: %v, %v", into, err)
	}
	moved, _ := h.q.List(ctx, h.admin, byPerson)
	if !slices.Equal(moved.Smart.Query.Where.Rules[0].Values, []string{h.corey.String()}) {
		t.Errorf("the rule still names the old person: %+v", moved.Smart.Query.Where.Rules[0])
	}
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE list SET query = '{"kinds":["movie"],"where":{"match":"all","rules":[{"field":"retired","op":"isSet"}]}}' WHERE id = ?`, list)
		return err
	})
	broken, err := h.q.List(ctx, h.admin, list)
	if err != nil || len(broken.Smart.Errors) == 0 || broken.Works != 0 {
		t.Errorf("a query that no longer runs: %+v, %v", broken.Smart, err)
	}
	if p, err := h.q.ListWorks(ctx, h.admin, list, nil, query.GridOptions{}); err != nil || p.Total != 0 {
		t.Errorf("its works: %d, %v", p.Total, err)
	}
	home = h.home(h.admin)
	if slices.ContainsFunc(home, func(r query.HomeRow) bool { return r.ID == row }) {
		t.Error("a broken smart list is on Home")
	}
}

func TestSmartListRandomIsSeededByList(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	random := query.WorkQuery{Kinds: queryAllKinds, Sort: []query.SortKey{{Field: "random"}}}
	a := h.smartList(us, h.admin, "A", random)
	b := h.smartList(us, h.admin, "B", random)
	read := func(list id.ID) []id.ID {
		p, err := h.q.ListWorks(ctx, h.admin, list, nil, query.GridOptions{Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		return ids(p.Items)
	}
	first := read(a)
	if !slices.Equal(first, read(a)) {
		t.Error("a list's random order moved")
	}
	if slices.Equal(first, read(b)) {
		t.Error("two lists share a random order")
	}
}

func TestListsCRUDAndHome(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	date, err := us.CreateList(ctx, h.friend, "Date Night", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := us.CreateList(ctx, h.friend, "date night", nil); !errors.Is(err, users.ErrListNameTaken) {
		t.Errorf("a duplicate name: %v", err)
	}
	if _, err := us.CreateList(ctx, h.friend, " padded", nil); err == nil {
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
		_, err := us.CreateList(ctx, h.friend, "list"+string(rune('a'+i%26))+string(rune('a'+i/26)), nil)
		if err != nil {
			if err != users.ErrListLimit || i != users.MaxLists-1 {
				t.Errorf("at the cap: %d, %v", i, err)
			}
			break
		}
	}
}
