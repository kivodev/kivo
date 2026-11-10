package query_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
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
	if len(ls) != 1 || !ls[0].BuiltIn || ls[0].Name != "Watchlist" || ls[0].Works != 0 {
		t.Fatalf("a new user's lists: %+v", ls)
	}
	// Each user has their own.
	if h.lists(h.admin)[0].ID == ls[0].ID {
		t.Error("two users share a My List")
	}
}

// putOnList adds a work to a list directly, so a test can say when. As a
// work added does, it goes before those added earlier in the custom order.
func (h *household) putOnList(list, work id.ID, addedAt int64) {
	h.t.Helper()
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO list_work (list_id, work_id, added_at, position) VALUES (?, ?, ?, ?)`, list, work, addedAt, -addedAt)
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
	// The list itself isn't counted: the first page of its works is.
	if got.Smart == nil || len(got.Smart.Errors) != 0 || got.Works != 0 || got.Smart.Query.Version != 1 {
		t.Errorf("a saved smart list: %+v, %+v", got, got.Smart)
	}
	p, err := h.q.ListWorks(ctx, h.admin, list, nil, query.GridOptions{})
	if err != nil || p.Total != 2 || !slices.Contains(ids(p.Items), h.toyStory.Work) {
		t.Errorf("its works: %v (%d), %v", ids(p.Items), p.Total, err)
	}
	if later, err := h.q.ListWorks(ctx, h.admin, list, nil, query.GridOptions{Offset: 1}); err != nil || later.Total != 0 || !slices.Equal(ids(later.Items), ids(p.Items)[1:]) {
		t.Errorf("a later page: %v (%d), %v, want %v and no total", ids(later.Items), later.Total, err, ids(p.Items)[1:])
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
	if err := us.UpdateList(ctx, h.admin, mine, nil, new(`{}`), nil); !errors.Is(err, users.ErrListBuiltIn) {
		t.Errorf("My List's rules: %v", err)
	}
	manual, _ := us.CreateList(ctx, h.admin, "Manual", nil)
	if err := us.UpdateList(ctx, h.admin, manual, nil, new(`{}`), nil); !errors.Is(err, users.ErrListKind) {
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
	if err := us.UpdateList(ctx, h.admin, list, nil, &saved, nil); err != nil {
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

// The kind filter narrows the capped set; it doesn't reach past the cap.
func TestBrokenSmartLists(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	list := h.smartList(us, h.admin, "Broken", movies(query.Rule{Field: "title", Op: "contains", Values: []string{"secret"}}))
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE list SET query = '{"version":1,"kinds":["movie"],"where":{"match":"all","rules":[{"field":"mood","op":"anyOf","values":["secret"]}]}}' WHERE id = ?`, list)
		return err
	})
	var logged bytes.Buffer
	defer slog.SetDefault(slog.Default())
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	saved, err := us.SavedListQueries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	query.LogBrokenSmartLists(ctx, saved)
	if line := logged.String(); !strings.Contains(line, list.String()) || !strings.Contains(line, "invalid") || strings.Contains(line, "secret") {
		t.Errorf("logged %q", line)
	}
	rows, err := h.q.HomeRows(ctx, h.admin)
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(rows, func(r query.HomeRowInfo) bool { return r.List.ID == list })
	if at < 0 || !rows[at].Broken() {
		t.Errorf("the layout doesn't say the list is broken: %+v", rows)
	}
}

func TestHomeSkipsARowThatFails(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	list := h.smartList(us, h.admin, "Fails", movies(query.Rule{Field: "collection", Op: "anyOf", Values: []string{id.New().String()}}))
	if err := us.SetHomeRows(ctx, h.admin, []users.HomeRow{{ID: "list:" + list.String(), Shown: true}}); err != nil {
		t.Fatal(err)
	}
	rename := func(from, to string) {
		h.b.Write(func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `ALTER TABLE `+from+` RENAME TO `+to)
			return err
		})
	}
	rename("collection_work", "collection_work_away")
	defer rename("collection_work_away", "collection_work")
	var logged bytes.Buffer
	defer slog.SetDefault(slog.Default())
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	if _, _, err := h.q.Home(ctx, h.admin); err != nil {
		t.Fatalf("a failing row failed Home: %v", err)
	}
	if line := logged.String(); !strings.Contains(line, "list:"+list.String()) {
		t.Errorf("logged %q", line)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := h.q.Home(cancelled, h.admin); err == nil {
		t.Error("a cancelled Home read succeeded")
	}
}

func TestSmartListsCountOnlyWhenAsked(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	list := h.smartList(us, h.admin, "Toys", movies(query.Rule{Field: "title", Op: "contains", Values: []string{"toy"}}))
	works := func(read func(context.Context, users.Viewer) ([]query.List, error)) int {
		ls, err := read(ctx, h.admin)
		if err != nil {
			t.Fatal(err)
		}
		return ls[slices.IndexFunc(ls, func(l query.List) bool { return l.ID == list })].Works
	}
	if got := works(h.q.Lists); got != 0 {
		t.Errorf("Lists counted a smart list: %d", got)
	}
	if got := works(h.q.CountedLists); got != 2 {
		t.Errorf("CountedLists: %d, want 2", got)
	}
}

// A query with no cap reads as a capped one does, in the same order and
// with the same kind filter.
func TestPlainQueryMatchesACappedOne(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	us := users.New(h.b.DB, events.NewBus())
	lots := 100000
	kinds := []catalog.WorkKind{catalog.KindMovie, catalog.KindSeries, catalog.KindEpisode, catalog.KindBook}
	for _, field := range []string{"title", "addedAt", "year", "rating", "lastPlayedAt", "releaseDate", "random"} {
		q := query.WorkQuery{Kinds: kinds, Sort: []query.SortKey{{Field: field}}}
		capped := q
		capped.MaxWorks = &lots
		plain, err := h.q.QueryWorks(ctx, h.admin, q, query.Scope{}, query.GridOptions{})
		if err != nil {
			t.Fatal(err)
		}
		both, err := h.q.QueryWorks(ctx, h.admin, capped, query.Scope{}, query.GridOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if field != "random" && (plain.Total != both.Total || !slices.Equal(ids(plain.Items), ids(both.Items))) {
			t.Errorf("%s: plain %d %v, capped %d %v", field, plain.Total, ids(plain.Items), both.Total, ids(both.Items))
		}
		a := h.smartList(us, h.admin, "plain "+field, q)
		for _, only := range []catalog.WorkKind{catalog.KindMovie, catalog.KindEpisode} {
			p, err := h.q.ListWorks(ctx, h.admin, a, []catalog.WorkKind{only}, query.GridOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range p.Items {
				if w.Kind != only {
					t.Errorf("%s: a %s under the %s filter", field, w.Kind, only)
				}
			}
		}
	}
}

func TestSmartListKindFilterKeepsTheCap(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	cap := 1
	q := query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindMovie, catalog.KindBook}, Sort: []query.SortKey{{Field: "title"}}, MaxWorks: &cap}
	list := h.smartList(us, h.admin, "One", q)
	all, err := h.q.ListWorks(ctx, h.admin, list, nil, query.GridOptions{})
	if err != nil || all.Total != 1 {
		t.Fatalf("capped: %d, %v", all.Total, err)
	}
	other := catalog.KindBook
	if all.Items[0].Kind == catalog.KindBook {
		other = catalog.KindMovie
	}
	p, err := h.q.ListWorks(ctx, h.admin, list, []catalog.WorkKind{other}, query.GridOptions{})
	if err != nil || p.Total != 0 {
		t.Errorf("the other kind reached past the cap: %d, %v", p.Total, err)
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
	if err := us.UpdateList(ctx, h.friend, date, new("Road Trip"), nil, nil); err != nil {
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

// A list's items are the first works its page lists for the Viewer, with
// their artwork, so a work the Viewer can't see isn't among them.
func TestCountedListsItems(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	poster := id.New()
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO image (id, work_id, kind, source, origin, width, height, blurhash, fetched_at)
			VALUES (?, ?, 'poster', 'provider', 'x', 480, 720, '', 0)`, poster, h.toyStory.Work)
		return err
	})
	list := func(v users.Viewer, list id.ID) query.List {
		t.Helper()
		ls, err := h.q.CountedLists(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		return ls[slices.IndexFunc(ls, func(l query.List) bool { return l.ID == list })]
	}
	works := func(l query.List) []id.ID {
		out := []id.ID{}
		for _, w := range l.Items {
			out = append(out, w.ID)
		}
		return out
	}

	mine := h.lists(h.admin)[0].ID
	h.putOnList(mine, h.dune.Work, 1000)
	h.putOnList(mine, h.toyStory.Work, 2000)
	h.putOnList(mine, h.bladeRunner.Work, 3000)
	if got, want := works(list(h.admin, mine)), []id.ID{h.bladeRunner.Work, h.toyStory.Work, h.dune.Work}; !slices.Equal(got, want) {
		t.Errorf("a manual list's items: %v, want the most recently added first %v", got, want)
	}
	kids := h.lists(h.kid)[0].ID
	h.putOnList(kids, h.toyStory.Work, 1000)
	h.putOnList(kids, h.bladeRunner.Work, 2000)
	l := list(h.kid, kids)
	if got, want := works(l), []id.ID{h.toyStory.Work}; !slices.Equal(got, want) {
		t.Fatalf("the kid's list's items: %v, want %v", got, want)
	}
	if p := l.Items[0].Images.Poster; p == nil || p.ID != poster {
		t.Errorf("an item's poster: %v, want %v", p, poster)
	}
	empty, err := us.CreateList(ctx, h.kid, "Empty", nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := list(h.kid, empty); l.Items == nil || len(l.Items) != 0 {
		t.Errorf("an empty list's items: %v, want none and not nil", l.Items)
	}

	// A smart list's are the first its rules select, in their order.
	smart := h.smartList(us, h.kid, "R", movies(query.Rule{Field: "title", Op: "contains", Values: []string{"r"}}))
	p, err := h.q.ListWorks(ctx, h.kid, smart, nil, query.GridOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var want []id.ID
	for _, w := range p.Items {
		want = append(want, w.ID)
	}
	l = list(h.kid, smart)
	if got := works(l); len(got) == 0 || !slices.Equal(got, want) || slices.Contains(got, h.bladeRunner.Work) || l.Works != len(got) {
		t.Errorf("the kid's smart list's items: %v counted %d, want %v and never Blade Runner", got, l.Works, want)
	}
}

// A manual list is shown in its sort wherever it is listed: on its page, on
// its shelf and in its Home row. A work added goes first in the custom
// order, a move puts works after another or first and sets the list to the
// custom order, and a smart list has neither.
func TestManualListOrder(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	mine := h.lists(h.admin)[0].ID
	dune, toys, blade := h.dune.Work, h.toyStory.Work, h.bladeRunner.Work
	for _, w := range []id.ID{dune, toys, blade} {
		if err := h.state.AddToList(ctx, h.admin, mine, w); err != nil {
			t.Fatal(err)
		}
	}
	// shown checks the list's order on its page, its shelf and its Home row.
	shown := func(what string, sort users.ListSort, want ...id.ID) {
		t.Helper()
		p, err := h.q.ListWorks(ctx, h.admin, mine, nil, query.GridOptions{})
		if err != nil {
			t.Fatal(err)
		}
		lists, err := h.q.CountedLists(ctx, h.admin)
		if err != nil {
			t.Fatal(err)
		}
		rows, _, err := h.q.Home(ctx, h.admin, query.RowList)
		if err != nil || len(rows) != 1 {
			t.Fatalf("%s: Home's list rows %v, %v", what, rows, err)
		}
		if got := ids(p.Items); !slices.Equal(got, want) || p.Total != len(want) {
			t.Errorf("%s: the page lists %v (%d), want %v", what, got, p.Total, want)
		}
		if got := ids(lists[0].Items); !slices.Equal(got, want) || lists[0].Sort != sort {
			t.Errorf("%s: the shelf lists %v by %q, want %v by %q", what, got, lists[0].Sort, want, sort)
		}
		if got := ids(rows[0].Items); !slices.Equal(got, want) {
			t.Errorf("%s: Home lists %v, want %v", what, got, want)
		}
	}
	shown("as added", users.ListCustom, blade, toys, dune)

	move := func(after id.ID, works ...id.ID) {
		t.Helper()
		if err := h.state.MoveInList(ctx, h.admin, mine, works, after); err != nil {
			t.Fatal(err)
		}
	}
	move(blade, dune)
	shown("Dune after Blade Runner", users.ListCustom, blade, dune, toys)
	move(blade, dune)
	shown("the same move again", users.ListCustom, blade, dune, toys)
	move(id.ID{}, toys, dune)
	shown("two moved to the top", users.ListCustom, toys, dune, blade)
	move(blade, toys, h.hailMary)
	shown("one moved last, with a work not in the list", users.ListCustom, dune, blade, toys)
	// Works put between the same two until no place is left between them.
	for range 40 {
		move(dune, toys)
		move(dune, blade)
	}
	shown("after 80 moves into one gap", users.ListCustom, dune, blade, toys)

	sort := func(s users.ListSort) {
		t.Helper()
		if err := us.UpdateList(ctx, h.admin, mine, nil, nil, &s); err != nil {
			t.Fatal(err)
		}
	}
	sort(users.ListTitle)
	shown("by title", users.ListTitle, blade, dune, toys)
	sort(users.ListReleaseDate)
	shown("by release date", users.ListReleaseDate, dune, toys, blade)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE list_work SET added_at = CASE work_id WHEN ? THEN 3 WHEN ? THEN 2 ELSE 1 END`, toys, blade)
		return err
	})
	sort(users.ListDateAdded)
	shown("by date added", users.ListDateAdded, toys, blade, dune)
	sort(users.ListRating)
	shown("by rating", users.ListRating, dune, toys, blade)
	// A work added while the list is sorted is first in the custom order,
	// and a move goes back to it.
	if err := h.state.AddToList(ctx, h.admin, mine, h.hailMary); err != nil {
		t.Fatal(err)
	}
	move(dune, blade)
	shown("moved while sorted", users.ListCustom, h.hailMary, dune, blade, toys)

	if err := h.state.MoveInList(ctx, h.admin, mine, []id.ID{toys}, h.leviathan); !errors.Is(err, userstate.ErrListAnchor) {
		t.Errorf("after a work that isn't in the list: %v", err)
	}
	if err := h.state.MoveInList(ctx, h.admin, mine, []id.ID{toys}, toys); !errors.Is(err, userstate.ErrListAnchor) {
		t.Errorf("after itself: %v", err)
	}
	if err := h.state.MoveInList(ctx, h.friend, mine, []id.ID{toys}, id.ID{}); !errors.Is(err, users.ErrListNotFound) {
		t.Errorf("a move in another user's list: %v", err)
	}
	if err := us.UpdateList(ctx, h.friend, mine, nil, nil, new(users.ListTitle)); !errors.Is(err, users.ErrListNotFound) {
		t.Errorf("a sort of another user's list: %v", err)
	}
	if err := us.UpdateList(ctx, h.admin, mine, nil, nil, new(users.ListSort("shuffled"))); !errors.Is(err, users.ErrListSort) {
		t.Errorf("an unknown sort: %v", err)
	}
	smart := h.smartList(us, h.admin, "Smart", movies(query.Rule{Field: "title", Op: "contains", Values: []string{"toy"}}))
	if err := h.state.MoveInList(ctx, h.admin, smart, []id.ID{toys}, id.ID{}); !errors.Is(err, users.ErrListKind) {
		t.Errorf("a move in a smart list: %v", err)
	}
	if err := us.UpdateList(ctx, h.admin, smart, nil, nil, new(users.ListTitle)); !errors.Is(err, users.ErrListKind) {
		t.Errorf("a sort of a smart list: %v", err)
	}
	// The list's own orders are no part of a query.
	q := movies(query.Rule{Field: "title", Op: "contains", Values: []string{"toy"}})
	q.Sort = []query.SortKey{{Field: "listPosition"}}
	if problems := q.Validate(); len(problems) != 1 || problems[0].Pointer != "/sort/0/field" {
		t.Errorf("a query sorted by a list's order: %v", problems)
	}
}

// An extra can be added to a list, and is listed there.
func TestListWorksHoldAnyKind(t *testing.T) {
	h := newHousehold(t)
	mine := h.lists(h.admin)[0].ID
	extra := h.b.Work(catalog.WorkInput{Kind: catalog.KindExtra, Parent: h.dune.Work, Title: "Making Of", ExtraType: "featurette"})
	h.b.Version([]id.ID{h.b.File(h.moviesFolder, "Dune (2021)/Featurettes/Making Of.mkv")}, extra)
	if err := h.state.AddToList(ctx, h.admin, mine, extra); err != nil {
		t.Fatal(err)
	}
	p, err := h.q.ListWorks(ctx, h.admin, mine, nil, query.GridOptions{})
	if err != nil || !slices.Equal(ids(p.Items), []id.ID{extra}) {
		t.Errorf("a list with an extra: %v, %v", ids(p.Items), err)
	}
}
