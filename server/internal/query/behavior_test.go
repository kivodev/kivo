package query_test

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

func ids(cs []query.WorkSummary) []id.ID {
	out := make([]id.ID, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

// row returns the summaries of Home's row of kind, or none when Home left
// the row out.
func row(home []query.HomeRow, kind query.RowKind) []query.WorkSummary {
	for _, r := range home {
		if r.Kind == kind {
			return r.Items
		}
	}
	return nil
}

// recentlyAdded returns Home's Recently Added rows.
func recentlyAdded(home []query.HomeRow) []query.HomeRow {
	return slices.DeleteFunc(slices.Clone(home), func(r query.HomeRow) bool { return r.Kind != query.RowRecentlyAdded })
}

func find(cs []query.WorkSummary, work id.ID) (query.WorkSummary, bool) {
	i := slices.IndexFunc(cs, func(c query.WorkSummary) bool { return c.ID == work })
	if i < 0 {
		return query.WorkSummary{}, false
	}
	return cs[i], true
}

// TestProvisionalWorks checks that a provisional work shows in Recently
// Added and in grids, marked unmatched. It never shows in Continue Watching
// or in collections.
func TestProvisionalWorks(t *testing.T) {
	h := newHousehold(t)
	h.play(h.admin, h.unmatched.Version, 40*time.Minute)      // would be in Continue Watching
	h.play(h.admin, h.unmatchedShow1.Version, 25*time.Minute) // would put episode 2 there
	h.b.Write(func(ctx context.Context, tx *store.Tx) error { // as if a collection had taken it in
		_, err := tx.ExecContext(ctx, `INSERT INTO collection_work (collection_id, work_id, ord) VALUES (?, ?, 9)`,
			h.toyStoryCollection, h.unmatched.Work)
		return err
	})

	home, _, err := h.q.Home(ctx, h.admin)
	if err != nil {
		t.Fatal(err)
	}
	if cw := row(home, query.RowContinueWatching); len(cw) != 0 {
		t.Errorf("Continue Watching %v: want no provisional work", ids(cw))
	}
	for _, row := range recentlyAdded(home) {
		want := map[id.ID]id.ID{h.movies: h.unmatched.Work, h.shows: h.parent(h.unmatchedShow1.Work)}[row.Library.ID]
		if want.IsZero() {
			continue
		}
		if c, ok := find(row.Items, want); !ok || c.MatchState != query.Unmatched {
			t.Errorf("Recently Added in %s: %s missing or not unmatched (%+v)", h.names[row.Library.ID], h.names[want], c)
		}
	}
	p, err := h.browse(h.admin, h.movies, grid{Filter: filter{Unmatched: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(p.Items), []id.ID{h.unmatched.Work}) || p.Items[0].MatchState != query.Unmatched || p.Total != 1 {
		t.Errorf("unmatched filter = %v (total %d)", ids(p.Items), p.Total)
	}
	c, err := h.q.Collection(ctx, h.admin, h.toyStoryCollection)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := find(c.Items, h.unmatched.Work); ok || len(c.Items) != 2 || c.Collection.Items != 2 {
		t.Errorf("collection holds %v", ids(c.Items))
	}
	d, err := h.q.Detail(ctx, h.admin, h.unmatched.Work)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Collections) != 0 {
		t.Errorf("provisional work's detail lists collections %+v", d.Collections)
	}
}

// TestBrowsePaging reads every sort two items at a time by offset. It
// checks that the pages join up into the first page of the whole listing,
// with no repeats, and that the first page has the total and a later one
// none.
func TestBrowsePaging(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	for _, sort := range []string{"title", "addedAt", "releaseDate", "rating", "myRating", "runtimeMs", "lastPlayedAt", "lastEpisodeAddedAt"} {
		t.Run(string(sort), func(t *testing.T) {
			all, err := h.browse(h.admin, h.movies, grid{Sort: sort})
			if err != nil {
				t.Fatal(err)
			}
			if all.Total != len(all.Items) || len(all.Items) != 6 {
				t.Fatalf("one page: %d items, total %d", len(all.Items), all.Total)
			}
			var walked []id.ID
			for offset := 0; offset < all.Total; offset += 2 {
				p, err := h.browse(h.admin, h.movies, grid{Sort: sort, GridOptions: query.GridOptions{Offset: offset, Limit: 2}})
				if err != nil {
					t.Fatal(err)
				}
				walked = append(walked, ids(p.Items)...)
			}
			if !slices.Equal(walked, ids(all.Items)) {
				t.Errorf("pages give %v, want %v", walked, ids(all.Items))
			}
			for offset := range len(all.Items) + 1 {
				p, err := h.browse(h.admin, h.movies, grid{Sort: sort, GridOptions: query.GridOptions{Offset: offset, Limit: 2}})
				if err != nil {
					t.Fatal(err)
				}
				if want := ids(all.Items[offset:min(offset+2, len(all.Items))]); !slices.Equal(ids(p.Items), want) {
					t.Errorf("offset %d: %v, want %v", offset, ids(p.Items), want)
				}
				want := 0
				if offset == 0 {
					want = all.Total
				}
				if p.Total != want {
					t.Errorf("offset %d: total %d, want %d", offset, p.Total, want)
				}
			}
		})
	}
	for _, o := range []query.GridOptions{{Offset: -1}, {Limit: -1}, {Limit: 1000}} {
		if _, err := h.browse(h.admin, h.movies, grid{GridOptions: o}); !errors.Is(err, query.ErrInvalid) {
			t.Errorf("Browse(%+v): %v, want ErrInvalid", o, err)
		}
	}
	for _, o := range []grid{{Sort: "popularity"}, {Filter: filter{Played: "maybe"}}} {
		var invalid *query.InvalidQueryError
		if _, err := h.browse(h.admin, h.movies, o); !errors.As(err, &invalid) {
			t.Errorf("Browse(%+v): %v, want an invalid query", o, err)
		}
	}
}

func TestBrowseOrders(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	order := func(sort string) []string {
		p, err := h.browse(h.admin, h.movies, grid{Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range p.Items {
			out = append(out, h.names[c.ID])
		}
		return out
	}
	// Works sort by sort title, bytewise, so capitals sort first. The Final
	// Cut is grouped under Blade Runner, and Lost Film's file is gone.
	tests := map[string][]string{
		"title":       {"movie:Blade Runner", "movie:Dune", "movie:Home Movie", "movie:Toy Story", "movie:Toy Story 2", "movie:some.film.2023"},
		"releaseDate": {"movie:Dune", "movie:Home Movie", "movie:Toy Story 2", "movie:Toy Story", "movie:Blade Runner", "movie:some.film.2023"},
		// Blade Runner's 9 from 12 votes weighs less than Toy Story 2's 7.6
		// from 14,000. Home Movie and the unmatched film have no rating.
		"rating": {"movie:Dune", "movie:Toy Story", "movie:Toy Story 2", "movie:Blade Runner", "movie:Home Movie", "movie:some.film.2023"},
		// The admin rated only Toy Story.
		"myRating": {"movie:Toy Story", "movie:Blade Runner", "movie:Dune", "movie:Home Movie", "movie:Toy Story 2", "movie:some.film.2023"},
		// Dune's file runs 2h35m, the others 2h.
		"runtimeMs":    {"movie:Blade Runner", "movie:Home Movie", "movie:Toy Story", "movie:Toy Story 2", "movie:some.film.2023", "movie:Dune"},
		"lastPlayedAt": {"movie:Dune", "movie:Blade Runner", "movie:Home Movie", "movie:Toy Story", "movie:Toy Story 2", "movie:some.film.2023"},
	}
	for sort, want := range tests {
		if got := order(sort); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", sort, got, want)
		}
	}
	// Works made within a millisecond tie on their added time. So the added
	// sort is checked for its order, newest first and then by title, rather
	// than against a list.
	p, err := h.browse(h.admin, h.movies, grid{Sort: "addedAt"})
	if err != nil {
		t.Fatal(err)
	}
	byAdded := func(a, b query.WorkSummary) int {
		return cmp.Or(b.Added.Compare(a.Added), strings.Compare(a.SortTitle, b.SortTitle))
	}
	if !slices.IsSortedFunc(p.Items, byAdded) || len(p.Items) != 6 {
		t.Errorf("added: not newest first, then by title: %v", ids(p.Items))
	}
}

// TestBrowseByLastEpisodeAdded checks that a series sorts by its newest
// episode in the library, which moves as episodes are added and removed.
func TestBrowseByLastEpisodeAdded(t *testing.T) {
	h := newHousehold(t)
	order := func() []string {
		t.Helper()
		p, err := h.browse(h.admin, h.shows, grid{Sort: "lastEpisodeAddedAt"})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range p.Items {
			out = append(out, h.names[c.ID])
		}
		return out
	}
	// Each step waits a moment so its episode is the newest by the clock.
	add := func(series id.ID, episode int) catalogtest.Item {
		time.Sleep(2 * time.Millisecond)
		return h.episode(series, 1, episode)
	}
	add(h.expanse, 4)
	if got, want := order(), []string{"series:The Expanse", "series:unknown.show", "series:Bluey"}; !slices.Equal(got, want) {
		t.Errorf("after an Expanse episode: %v, want %v", got, want)
	}
	gone := add(h.bluey, 4)
	if got, want := order(), []string{"series:Bluey", "series:The Expanse", "series:unknown.show"}; !slices.Equal(got, want) {
		t.Errorf("after a Bluey episode: %v, want %v", got, want)
	}
	h.b.Write(func(ctx context.Context, tx *store.Tx) error { return h.b.MarkGone(ctx, tx, gone.File) })
	if got, want := order(), []string{"series:The Expanse", "series:unknown.show", "series:Bluey"}; !slices.Equal(got, want) {
		t.Errorf("after the Bluey episode's file is gone: %v, want %v", got, want)
	}
}

// TestCollectionsSortByName checks that collections sort as works do,
// without a leading article: "A Christmas Story Collection" under C. The
// first page says where each letter starts, by the sort name, and later
// pages don't.
func TestCollectionsSortByName(t *testing.T) {
	h := newHousehold(t)
	christmas := h.movie(h.moviesFolder, "A Christmas Story", 1983, rankPG)
	h.metadata(christmas.Work, catalog.WorkMetadata{Collection: &catalog.CollectionRef{Key: "tmdb:1", Name: "A Christmas Story Collection"}})
	h.metadata(h.bladeRunner.Work, catalog.WorkMetadata{Collection: &catalog.CollectionRef{Key: "tmdb:2", Name: "Blade Runner Collection"}})
	p, err := h.q.Collections(ctx, h.admin, h.movies, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range p.Items {
		got = append(got, c.Name)
	}
	want := []string{"Blade Runner Collection", "A Christmas Story Collection", "Toy Story Collection"}
	if !slices.Equal(got, want) {
		t.Errorf("collections %v, want %v", got, want)
	}
	if got, want := starts(p.Letters), "B0 C1 T2"; got != want || p.Total != 3 {
		t.Errorf("letters %s of %d, want %s of 3", got, p.Total, want)
	}
	p, err = h.q.Collections(ctx, h.admin, h.movies, query.GridOptions{Offset: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || p.Items[0].Name != want[1] || p.Total != 3 || p.Letters != nil {
		t.Errorf("offset 1, limit 1: %+v", p)
	}
}

func TestBrowseFilters(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	tests := []struct {
		name string
		lib  id.ID
		f    filter
		want []id.ID
	}{
		{"played", h.movies, filter{Played: "played"}, nil},
		{"in progress", h.movies, filter{Played: "inProgress"}, []id.ID{h.dune.Work}},
		{"series in progress", h.shows, filter{Played: "inProgress"}, []id.ID{h.bluey, h.expanse}},
		{"series unplayed", h.shows, filter{Played: "unplayed"}, []id.ID{h.unmatchedShow}},
		{"genre", h.movies, filter{Genres: []string{"Adventure", "Western"}}, []id.ID{h.dune.Work}},
		{"years", h.movies, filter{YearFrom: 1995, YearTo: 1999}, []id.ID{h.toyStory.Work, h.toyStory2.Work}},
		{"years to", h.movies, filter{YearTo: 1995}, []id.ID{h.bladeRunner.Work, h.toyStory.Work}},
		{"ratings", h.movies, filter{ContentRatings: []string{"R", "PG-13"}}, []id.ID{h.bladeRunner.Work, h.dune.Work}},
		{"1080p", h.movies, filter{Resolutions: []query.Resolution{query.Resolution1080p}}, []id.ID{h.dune.Work}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := h.browse(h.admin, tt.lib, grid{Filter: tt.f})
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(p.Items); !slices.Equal(got, tt.want) || p.Total != len(tt.want) {
				t.Errorf("got %v (total %d), want %v", got, p.Total, tt.want)
			}
		})
	}
	// The Final Cut's 4K HDR copy is its own root work in the 4K library.
	p, err := h.browse(h.admin, h.uhd, grid{Filter: filter{
		Resolutions: []query.Resolution{query.Resolution4K}, Ranges: []query.Range{query.DolbyVision},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(p.Items), []id.ID{h.finalCut4K.Work}) {
		t.Errorf("4K Dolby Vision = %v, want the Final Cut's 4K copy", ids(p.Items))
	}
}

func TestChildrenCombineMultiEpisodeVersions(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work SET overview = 'Bingo gets her own bed.' WHERE id = ?`, h.bluey3.Work)
		return err
	})
	eps, err := h.q.Children(ctx, h.admin, h.blueyS1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(eps), []id.ID{h.bluey1, h.bluey3.Work}) {
		t.Fatalf("season 1 = %v, want E1-E2 and E3", ids(eps))
	}
	c := eps[0]
	if c.Number != 1 || c.NumberEnd != 2 || c.Title != "Magic Xylophone / Hospital" || !c.Userstate.Played {
		t.Errorf("combined summary = %+v", c)
	}
	if eps[0].Overview != "" || eps[1].Overview != "Bingo gets her own bed." {
		t.Errorf("overviews %q and %q, want none and E3's", eps[0].Overview, eps[1].Overview)
	}
	seasons, err := h.q.Children(ctx, h.admin, h.bluey)
	if err != nil {
		t.Fatal(err)
	}
	var numbers []int
	for _, s := range seasons {
		numbers = append(numbers, s.Number)
	}
	if !slices.Equal(numbers, []int{1, 0}) {
		t.Errorf("seasons %v, want specials last", numbers)
	}
	if seasons[0].Unplayed != 1 {
		t.Errorf("season 1 unplayed = %d, want 1 (E3)", seasons[0].Unplayed)
	}
	if s1, specials := seasons[0].Episodes, seasons[1].Episodes; s1 != 2 || specials != 1 {
		t.Errorf("seasons list %d and %d episodes, want 2 (E1-E2 and E3) and 1", s1, specials)
	}
}

func TestHomeRows(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	home, _, err := h.q.Home(ctx, h.friend)
	if err != nil {
		t.Fatal(err)
	}
	// The row is most recently played first. Bluey's up next is E3, after
	// the double episode, not the special. The Expanse's episode in
	// progress follows.
	if want := []id.ID{h.bluey3.Work, h.expanse2.Work, h.dune.Work}; !slices.Equal(ids(row(home, query.RowContinueWatching)), want) {
		t.Errorf("Continue Watching = %v, want %v", ids(row(home, query.RowContinueWatching)), want)
	}
	if want := []id.ID{h.toyStory.Work}; !slices.Equal(ids(row(home, query.RowList)), want) {
		t.Errorf("My List = %v, want %v", ids(row(home, query.RowList)), want)
	}
	var libs []id.ID
	for _, row := range recentlyAdded(home) {
		libs = append(libs, row.Library.ID)
	}
	if !slices.Equal(libs, []id.ID{h.movies, h.shows}) {
		t.Errorf("Recently Added libraries = %v, want the friend's two", libs)
	}
	shows := recentlyAdded(home)[1].Items
	// Each show's episodes were added together. A show added within one
	// season gets the season's summary. Bluey's special and season 1 span
	// two seasons, so they get the series' summary.
	if want := []id.ID{h.parent(h.unmatchedShow1.Work), h.bluey, h.expanseS1}; !slices.Equal(ids(shows), want) {
		t.Errorf("Recently Added in Shows = %v, want %v", ids(shows), want)
	}
	for work, want := range map[id.ID]int{h.bluey: 4, h.expanseS1: 3} {
		if c, _ := find(shows, work); c.Bundled != want {
			t.Errorf("%s bundles %d episodes, want %d", h.names[work], c.Bundled, want)
		}
	}

	// An unreachable folder leaves a work in Continue Watching, unavailable.
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE library_folder SET reachable = 0 WHERE id = ?`, h.moviesFolder)
		return err
	})
	home, _, err = h.q.Home(ctx, h.friend)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := find(row(home, query.RowContinueWatching), h.dune.Work); !ok || !c.Unavailable {
		t.Errorf("Dune in Continue Watching with its folder unreachable: %+v, %v", c, ok)
	}
}

// TestPinnedLibraries checks a user's own library order. Every library is
// pinned, A to Z by name, until the user chooses otherwise. A
// library the user hasn't placed, or gets back after losing it, is pinned
// at the end. Home has rows for pinned libraries only, and search still
// finds the rest.
func TestPinnedLibraries(t *testing.T) {
	h := newHousehold(t)
	us := users.New(h.b.DB, events.NewBus())
	type entry struct {
		lib    id.ID
		pinned bool
	}
	check := func(name string, v users.Viewer, want ...entry) {
		t.Helper()
		libs, err := h.q.Libraries(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]entry, len(libs))
		for i, l := range libs {
			got[i] = entry{l.ID, l.Pinned}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: Libraries = %v, want %v", name, got, want)
		}
	}
	check("default", h.friend, entry{h.movies, true}, entry{h.shows, true})

	err := us.EditPreferences(ctx, h.friend, users.PreferencesChange{Libraries: &[]users.LibraryChoice{{Library: h.shows, Pinned: true}, {Library: h.movies}}})
	if err != nil {
		t.Fatal(err)
	}
	check("chosen", h.friend, entry{h.shows, true}, entry{h.movies, false})
	home, _, err := h.q.Home(ctx, h.friend)
	if err != nil {
		t.Fatal(err)
	}
	if ra := recentlyAdded(home); len(ra) != 2 || ra[0].Library.ID != h.movies || ra[1].Library.ID != h.shows {
		t.Errorf("Recently Added rows = %+v, want Movies then Shows: the server's order, whatever the user's order and pins", ra)
	}
	if got, _ := h.q.Search(ctx, h.friend, "blade runner", 0); !slices.Equal(ids(got.Works), []id.ID{h.bladeRunner.Work}) {
		t.Errorf("Search finds %v in a library that isn't pinned", ids(got.Works))
	}

	// Losing Movies forgets its place. When it's given back, it's pinned
	// again.
	for _, libs := range [][]id.ID{{h.shows}, {h.movies, h.shows}} {
		if err := us.Update(ctx, id.ID{}, h.friend.User(), users.Change{Grants: &users.Grants{Libraries: libs}}); err != nil {
			t.Fatal(err)
		}
	}
	check("granted back", h.friend, entry{h.shows, true}, entry{h.movies, true})

	// Libraries the user didn't place follow, A to Z by name.
	if err := us.EditPreferences(ctx, h.admin, users.PreferencesChange{Libraries: &[]users.LibraryChoice{{Library: h.books, Pinned: true}, {Library: h.shows}}}); err != nil {
		t.Fatal(err)
	}
	check("admin", h.admin, entry{h.books, true}, entry{h.shows, false},
		entry{h.movies, true}, entry{h.uhd, true}, entry{h.private, true})

	for name, libs := range map[string][]users.LibraryChoice{
		"not granted": {{Library: h.private, Pinned: true}},
		"twice":       {{Library: h.shows}, {Library: h.shows}},
		"missing":     {{Library: id.New()}},
	} {
		v := h.friend
		if name == "missing" {
			v = h.admin // sees every library, so only the database knows
		}
		if err := us.EditPreferences(ctx, v, users.PreferencesChange{Libraries: &libs}); !errors.Is(err, users.ErrUnknownLibrary) {
			t.Errorf("%s: err = %v, want ErrUnknownLibrary", name, err)
		}
	}

	// A change whose playback part is refused keeps the libraries too.
	err = us.EditPreferences(ctx, h.friend, users.PreferencesChange{
		Libraries: &[]users.LibraryChoice{{Library: h.movies, Pinned: true}},
		Playback:  &users.PlaybackChange{BookSpeed: new(9.0)},
	})
	if !errors.Is(err, users.ErrInvalidPlayback) {
		t.Fatalf("err = %v, want ErrInvalidPlayback", err)
	}
	check("refused", h.friend, entry{h.shows, true}, entry{h.movies, true})
}

// TestBookRows checks Continue Listening, and that books in progress are
// kept out of Continue Watching.
func TestBookRows(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	version := func(work id.ID) id.ID {
		var v id.ID
		if err := h.b.DB.Reader().QueryRow(`SELECT version_id FROM media_version_work WHERE work_id = ?`, work).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	rows := func(v users.Viewer) []query.HomeRow {
		home, _, err := h.q.Home(ctx, v)
		if err != nil {
			t.Fatal(err)
		}
		return home
	}
	h.play(h.admin, version(h.hailMary), 30*time.Minute)
	h.play(h.admin, version(h.leviathan), 2*time.Hour-time.Minute) // finished: a minute left
	home := rows(h.admin)
	if want := []id.ID{h.hailMary}; !slices.Equal(ids(row(home, query.RowContinueListening)), want) {
		t.Errorf("Continue Listening = %v, want %v", ids(row(home, query.RowContinueListening)), want)
	}
	if _, ok := find(row(home, query.RowContinueWatching), h.hailMary); ok {
		t.Error("a book in Continue Watching")
	}

	// A book joins Continue Listening once it's started.
	h.play(h.admin, version(h.churn), 10*time.Minute)
	home = rows(h.admin)
	if want := []id.ID{h.churn, h.hailMary}; !slices.Equal(ids(row(home, query.RowContinueListening)), want) {
		t.Errorf("Continue Listening = %v, want %v", ids(row(home, query.RowContinueListening)), want)
	}
	// The friend has no audiobooks library.
	if home := rows(h.friend); len(row(home, query.RowContinueListening)) != 0 {
		t.Errorf("the friend's Continue Listening: %v", ids(row(home, query.RowContinueListening)))
	}
	// Pins don't hide Continue Listening.
	if err := users.New(h.b.DB, events.NewBus()).EditPreferences(ctx, h.admin, users.PreferencesChange{Libraries: &[]users.LibraryChoice{{Library: h.books}}}); err != nil {
		t.Fatal(err)
	}
	if home := rows(h.admin); len(row(home, query.RowContinueListening)) == 0 {
		t.Error("Continue Listening is off for an audiobooks library that isn't pinned")
	}
}

// TestRecentlyAddedBundles checks that a big import is one summary, which
// leaves the rest of the row in place. It also checks that an episode added
// days after its season gets a summary of its own.
func TestRecentlyAddedBundles(t *testing.T) {
	h := newHousehold(t)
	big := h.series("One Piece", rankG)
	for e := range 300 {
		h.b.Episode(h.showsFolder, big, 1+e/100, e%100+1)
	}
	expanse4 := h.b.Episode(h.showsFolder, h.expanse, 1, 4).Work
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		// The household was added a day ago, One Piece just now, and The
		// Expanse's E4 two days after the rest of its season.
		now := time.Now().UnixMilli()
		_, err := tx.ExecContext(ctx, `UPDATE work SET added_at = ? - rid WHERE root_id = ?`, now, big)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE work SET added_at = (SELECT max(added_at) FROM work) + 48 * 3600000 WHERE id = ?`, expanse4)
		}
		if err == nil {
			err = redate(ctx, tx)
		}
		return err
	})
	home, _, err := h.q.Home(ctx, h.admin)
	if err != nil {
		t.Fatal(err)
	}
	shows := recentlyAdded(home)[1].Items
	if want := []id.ID{expanse4, big, h.parent(h.unmatchedShow1.Work), h.bluey, h.expanseS1}; !slices.Equal(ids(shows), want) {
		t.Errorf("Recently Added in Shows = %v, want %v", ids(shows), want)
	}
	for work, want := range map[id.ID]int{expanse4: 0, big: 300, h.expanseS1: 3} {
		if c, _ := find(shows, work); c.Bundled != want {
			t.Errorf("%s bundles %d episodes, want %d", h.names[work], c.Bundled, want)
		}
	}
}

// TestDetailVersions checks that a root work's versions list each cut's
// versions together, untagged first, with their edition labels, the taller
// picture and then the larger size first within a cut, in the same order
// the catalog gives them. It also checks that tracks and paths come
// through. It also checks that a copy in another
// library is a root work of its own, which a Viewer without that library
// doesn't see.
func TestDetailVersions(t *testing.T) {
	h := newHousehold(t)
	versions := func(d query.WorkDetail) []id.ID {
		var out []id.ID
		for _, v := range d.Versions {
			out = append(out, v.ID)
		}
		return out
	}
	later := h.b.File(h.moviesFolder, "Blade Runner (1982)/Blade Runner (1982) 720p.mkv")
	plain2 := h.b.Version([]id.ID{later}, h.bladeRunner.Work)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE media_file SET size = size * 2 WHERE id = ?`, later)
		return err
	})
	uhdFile := h.b.File(h.moviesFolder, "Blade Runner (1982)/Blade Runner (1982) 2160p.mkv")
	uhd2 := h.b.Version([]id.ID{uhdFile}, h.bladeRunner.Work)
	h.tracks(uhdFile, 117*time.Minute, []catalog.Track{
		{Index: 0, Type: "video", Codec: "hevc", Default: true, Info: info(map[string]any{"width": 3840, "height": 2160, "range": "sdr"})},
	})
	order := []id.ID{uhd2, plain2, h.bladeRunner.Version, h.finalCut.Version}
	admin, err := h.q.Detail(ctx, h.admin, h.bladeRunner.Work)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(versions(admin), order) {
		t.Fatalf("admin's versions = %v, want %v", versions(admin), order)
	}
	if a, b := admin.Versions[0].Edition, admin.Versions[3].Edition; a != "" || b != "Final Cut" {
		t.Errorf("edition labels %q and %q, want none and Final Cut", a, b)
	}
	inCatalog, err := h.b.Catalog.VersionsOf(ctx, h.bladeRunner.Work)
	if err != nil {
		t.Fatal(err)
	}
	var catalogOrder []id.ID
	for _, v := range inCatalog {
		catalogOrder = append(catalogOrder, v.ID)
	}
	if !slices.Equal(catalogOrder, order) {
		t.Errorf("the catalog's versions = %v, want %v", catalogOrder, order)
	}
	copy4K, err := h.q.Detail(ctx, h.admin, h.finalCut4K.Work)
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{h.finalCut4K.Version}; !slices.Equal(versions(copy4K), want) {
		t.Fatalf("the 4K copy's versions = %v, want %v", versions(copy4K), want)
	}
	uhd := copy4K.Versions[0]
	if !uhd.Unavailable || uhd.Library != h.uhd || uhd.Parts[0].Path == "" || uhd.Tracks[0].Width != 3840 || uhd.Tracks[0].DolbyVisionProfile != 8 {
		t.Errorf("4K version = %+v", uhd)
	}

	friend, err := h.q.Detail(ctx, h.friend, h.bladeRunner.Work)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(versions(friend), order) {
		t.Errorf("friend's versions = %v, want %v", versions(friend), order)
	}
	if friend.Versions[0].Parts[0].Path != "" {
		t.Error("a non-admin sees file paths")
	}
	if _, err := h.q.Detail(ctx, h.friend, h.finalCut4K.Work); !errors.Is(err, query.ErrNotFound) {
		t.Errorf("friend sees the 4K copy: %v", err)
	}
}

// TestGoneVersionHidden checks that a version whose file is gone leaves the
// root work's page, the catalog's versions and the grid's resolution filter
// at once, while the root stays with its other version, and that it comes
// back with its tracks when its file returns.
func TestGoneVersionHidden(t *testing.T) {
	h := newHousehold(t)
	const rel = "Dune (2021)/Dune (2021) 2160p.mkv"
	f := h.b.File(h.moviesFolder, rel)
	uhd := h.b.Version([]id.ID{f}, h.dune.Work)
	h.tracks(f, 155*time.Minute, []catalog.Track{
		{Index: 0, Type: "video", Codec: "hevc", Default: true, Info: info(map[string]any{"width": 3840, "height": 2160, "range": "hdr10"})},
	})
	check := func(when string, want []id.ID, in4K bool) {
		t.Helper()
		d, err := h.q.Detail(ctx, h.admin, h.dune.Work)
		if err != nil {
			t.Fatal(err)
		}
		var listed []id.ID
		for _, v := range d.Versions {
			listed = append(listed, v.ID)
			if v.Unavailable || len(v.Tracks) == 0 {
				t.Errorf("%s: version %+v", when, v)
			}
		}
		if !slices.Equal(listed, want) || d.Summary.Unavailable {
			t.Errorf("%s: versions %v (unavailable %v), want %v", when, listed, d.Summary.Unavailable, want)
		}
		inCatalog, err := h.b.Catalog.VersionsOf(ctx, h.dune.Work)
		if err != nil {
			t.Fatal(err)
		}
		var catalogOrder []id.ID
		for _, v := range inCatalog {
			catalogOrder = append(catalogOrder, v.ID)
		}
		if !slices.Equal(catalogOrder, want) {
			t.Errorf("%s: the catalog's versions = %v, want %v", when, catalogOrder, want)
		}
		p, err := h.browse(h.admin, h.movies, grid{Filter: filter{Resolutions: []query.Resolution{query.Resolution4K}}})
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(ids(p.Items), h.dune.Work); got != in4K {
			t.Errorf("%s: Dune in the 4K filter %v, want %v", when, got, in4K)
		}
	}
	check("with both files", []id.ID{uhd, h.dune.Version}, true)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error { return h.b.MarkGone(ctx, tx, f) })
	check("with the 4K file gone", []id.ID{h.dune.Version}, false)
	if back := h.b.File(h.moviesFolder, rel); back != f {
		t.Fatalf("the returning file is %v, want %v", back, f)
	}
	check("with the 4K file back", []id.ID{uhd, h.dune.Version}, true)
}

// TestLabelledRootIsItsOwn checks that a movie labelled by its folder's
// edition tag is a root work of its own: it shows in its library's grid and
// in search with its label, beside the film's other folder.
func TestLabelledRootIsItsOwn(t *testing.T) {
	h := newHousehold(t)
	labelled := h.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Blade Runner", Year: 1982, Edition: "Final Cut"})
	h.b.Version([]id.ID{h.b.File(h.moviesFolder, "Blade Runner (1982) {edition-Final Cut}/Blade Runner (1982).mkv")}, labelled)
	h.metadata(labelled, catalog.WorkMetadata{Year: 1982, ContentRating: certs[rankR], ContentRatingAge: ptr(rankR)})

	got, err := h.q.Search(ctx, h.friend, "blade runner", 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []id.ID{h.bladeRunner.Work, labelled}; !slices.Equal(ids(got.Works), want) {
		t.Fatalf("search = %v, want %v", ids(got.Works), want)
	}
	if c := got.Works[1]; c.Edition != "Final Cut" {
		t.Errorf("the labelled root's summary has label %q", c.Edition)
	}
	p, err := h.browse(h.friend, h.movies, grid{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := find(p.Items, labelled); !ok {
		t.Error("the labelled title is missing from its grid")
	}
}

// TestDetailIdentity checks what a detail page reads about a work's
// identity and children. That is its external IDs, its locked fields (for
// admins only), the works each version covers, and its seasons or episodes
// as Children lists them.
func TestDetailIdentity(t *testing.T) {
	h := newHousehold(t)
	arrival := h.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Arrival", Year: 2016, ExternalIDs: []catalog.ExternalID{
		{Provider: "tmdb", Kind: "movie", ID: "329865"}, {Provider: "imdb", Kind: "title", ID: "tt2543164"},
	}})
	h.b.Version([]id.ID{h.b.File(h.moviesFolder, "Arrival (2016)/Arrival (2016).mkv")}, arrival)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return h.b.Catalog.EditMetadata(ctx, tx, arrival, catalog.WorkMetadata{Title: "Arrival", Overview: "Heptapods."}, catalog.FieldOverview, catalog.FieldTitle)
	})
	identity, err := h.b.Catalog.Work(ctx, arrival)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := h.q.Detail(ctx, h.admin, arrival)
	if err != nil {
		t.Fatal(err)
	}
	var xs []catalog.ExternalID
	for _, x := range admin.ExternalIDs {
		xs = append(xs, catalog.ExternalID{Provider: x.Provider, Kind: x.Kind, ID: x.ID})
	}
	if !slices.Equal(xs, identity.ExternalIDs) || len(xs) != 2 {
		t.Errorf("external IDs = %v, want catalog's %v", xs, identity.ExternalIDs)
	}
	if !slices.Equal(admin.LockedFields, identity.LockedFields) || len(admin.LockedFields) != 2 {
		t.Errorf("admin's locked fields = %v, want catalog's %v", admin.LockedFields, identity.LockedFields)
	}
	friend, err := h.q.Detail(ctx, h.friend, arrival)
	if err != nil {
		t.Fatal(err)
	}
	if friend.LockedFields != nil || len(friend.ExternalIDs) != 2 {
		t.Errorf("friend's locked fields = %v and external IDs %v, want none and two", friend.LockedFields, friend.ExternalIDs)
	}

	episode, err := h.q.Detail(ctx, h.admin, h.bluey2)
	if err != nil {
		t.Fatal(err)
	}
	if v := episode.Versions[0]; v.ID != h.blueyDouble || !slices.Equal(v.Works, []id.ID{h.bluey1, h.bluey2}) {
		t.Errorf("double episode's version covers %v, want E1 and E2", v.Works)
	}
	for _, w := range []id.ID{h.bluey, h.blueyS1, h.expanse, h.expanseS1, h.bluey1, h.dune.Work} {
		for _, v := range []users.Viewer{h.admin, h.kid, h.friend} {
			d, err := h.q.Detail(ctx, v, w)
			if errors.Is(err, query.ErrNotFound) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if d.Summary.Kind == "series" || d.Summary.Kind == "season" {
				children, err := h.q.Children(ctx, v, w)
				if err != nil {
					t.Fatal(err)
				}
				want = len(children)
			}
			if d.Children != want {
				t.Errorf("%s's children = %d, want %d", h.names[w], d.Children, want)
			}
		}
	}
}

// TestAdjacentEpisodes checks an episode's previous and next episodes. They
// cross a season boundary and put specials last. They skip the second
// episode of a double-episode version, which shows in the first episode's
// row. They come only from what the Viewer sees.
func TestAdjacentEpisodes(t *testing.T) {
	h := newHousehold(t)
	s2e1 := h.episode(h.bluey, 2, 1).Work
	adjacent := func(v users.Viewer, work id.ID) (prev, next id.ID) {
		t.Helper()
		d, err := h.q.Detail(ctx, v, work)
		if err != nil {
			t.Fatal(err)
		}
		return d.PreviousEpisode, d.NextEpisode
	}
	for _, tt := range []struct {
		name             string
		work, prev, next id.ID
	}{
		{"S1E1-E2", h.bluey1, id.ID{}, h.bluey3.Work},
		{"S1E3", h.bluey3.Work, h.bluey1, s2e1},
		{"S2E1", s2e1, h.bluey3.Work, h.blueySpecial.Work},
		{"the special", h.blueySpecial.Work, s2e1, id.ID{}},
	} {
		if prev, next := adjacent(h.admin, tt.work); prev != tt.prev || next != tt.next {
			t.Errorf("%s: previous %s, next %s; want %s, %s", tt.name, h.names[prev], h.names[next], h.names[tt.prev], h.names[tt.next])
		}
	}

	// Above the kid's ceiling, S2E1 is stepped over.
	h.metadata(s2e1, catalog.WorkMetadata{ContentRatingAge: ptr(rankR)})
	if prev, next := adjacent(h.kid, h.bluey3.Work); prev != h.bluey1 || next != h.blueySpecial.Work {
		t.Errorf("kid at S1E3: previous %s, next %s; want %s, %s", h.names[prev], h.names[next], h.names[h.bluey1], h.names[h.blueySpecial.Work])
	}
	if prev, next := adjacent(h.admin, h.dune.Work); !prev.IsZero() || !next.IsZero() {
		t.Errorf("a movie's previous %s, next %s; want none", h.names[prev], h.names[next])
	}
}

// TestSearch checks that user input never reaches FTS5 as syntax. It also
// checks what matches: whole words without case or diacritics, with the
// last word a prefix, in titles, alternative titles and people, and only
// what the Viewer sees.
func TestSearch(t *testing.T) {
	h := newHousehold(t)
	for _, q := range []string{`"`, `blade"`, `NEAR(blade runner)`, `blade OR dune`, `title:dune`, `*`, `^dune`, `du-ne`, `a b`, `AND`, `{title}`} {
		if _, err := h.q.Search(ctx, h.admin, q, 0); err != nil {
			t.Errorf("Search(%q): %v", q, err)
		}
	}
	tests := []struct {
		q    string
		v    string
		want []id.ID
	}{
		{"blade runner", "admin", []id.ID{h.bladeRunner.Work, h.finalCut4K.Work}}, // the title equal to the query first
		{"runner blade", "admin", []id.ID{h.bladeRunner.Work, h.finalCut4K.Work}},
		{"DUNE", "admin", []id.ID{h.dune.Work}},
		{"villeneuve", "admin", []id.ID{h.dune.Work}},
		{"timothee", "admin", []id.ID{h.dune.Work}}, // Timothée, through diacritics removal
		{"secret", "friend", nil},
		{"secret", "admin", []id.ID{h.secret.Work}},
		{"to", "admin", []id.ID{h.toyStory.Work, h.toyStory2.Work}}, // the most voted first
		{"toy story 2", "admin", []id.ID{h.toyStory2.Work}},
		{"oy", "admin", nil}, // words, not fragments
		{"EX", "admin", []id.ID{h.expanse}},
		{"ex", "kid", nil},
		{"the expanse", "admin", []id.ID{h.expanse}},
		{"!!", "admin", nil},
		{" ", "admin", nil},
	}
	for _, tt := range tests {
		v := map[string]users.Viewer{"admin": h.admin, "friend": h.friend, "kid": h.kid}[tt.v]
		got, err := h.q.Search(ctx, v, tt.q, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ids(got.Works), tt.want) {
			t.Errorf("Search(%s, %q) = %v, want %v", tt.v, tt.q, ids(got.Works), tt.want)
		}
	}
	if got, _ := h.q.Search(ctx, h.admin, "toy", 1); len(got.Works) != 1 {
		t.Errorf("limit 1 gave %d", len(got.Works))
	}
	if _, err := h.q.Search(ctx, h.admin, "toy", 1000); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("limit 1000: %v", err)
	}
}

// TestSearchRanks checks the order of works. First come titles that are the
// query, ignoring a leading article, then titles that start with it, hold
// it as a phrase, or hold its words. Alternative titles come next, then
// matches on people only. The most voted come first within each rank.
func TestSearchRanks(t *testing.T) {
	h := newHousehold(t)
	movie := func(title string, votes int, m catalog.WorkMetadata) id.ID {
		w := h.movie(h.moviesFolder, title, 2000, rankG).Work
		m.Title, m.Community = title, &catalog.CommunityRating{Rating: 7, Votes: votes}
		h.metadata(w, m)
		return w
	}
	name := movie("Name", 1, catalog.WorkMetadata{})
	names := movie("Names of Things", 500, catalog.WorkMetadata{})
	rose := movie("The Name of the Rose", 100, catalog.WorkMetadata{})
	father := movie("In the Name of the Father", 1000, catalog.WorkMetadata{})
	lastName := movie("A Rose by Any Other Name", 2000, catalog.WorkMetadata{})
	alt := movie("Der Name der Rose", 50, catalog.WorkMetadata{AltTitles: []string{"Name of the Rose"}})
	garden := movie("Garden", 9000, catalog.WorkMetadata{Credits: []catalog.Credit{{Name: "Rose Name", Role: "actor"}}})
	movie("The Tournament", 5000, catalog.WorkMetadata{})
	spider := movie("Spider-Man", 10, catalog.WorkMetadata{})
	titles := func(ws []id.ID) []string {
		out := make([]string, len(ws))
		for i, w := range ws {
			out[i] = h.names[w]
		}
		return out
	}

	for _, tt := range []struct {
		q    string
		want []id.ID
	}{
		{"name", []id.ID{name, names, rose, lastName, father, alt, garden}},
		{"Name of the", []id.ID{rose, father, alt}},
		{"the name of the rose", []id.ID{rose, alt}},
		{"rose", []id.ID{lastName, rose, alt, garden}},
		{"spider man", []id.ID{spider}},
		{"spiderman", []id.ID{spider}},
		{"spider-m", []id.ID{spider}},
	} {
		got, err := h.q.Search(ctx, h.admin, tt.q, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ids(got.Works), tt.want) {
			t.Errorf("Search(%q) = %v, want %v", tt.q, titles(ids(got.Works)), titles(tt.want))
		}
	}
}

// TestSearchPeople checks which people search finds. A person matches by
// the words of their name and must be credited on something the Viewer
// sees. Each comes with their role there and how many root works have it.
func TestSearchPeople(t *testing.T) {
	h := newHousehold(t)
	type match struct {
		name, role string
		roots      int
	}
	for _, tt := range []struct {
		q    string
		v    string
		want []match
	}{
		{"zendaya", "admin", []match{{"Zendaya", "actor", 1}}},
		{"denis v", "admin", []match{{"Denis Villeneuve", "director", 1}}},
		{"timothee", "admin", []match{{"Timothée Chalamet", "actor", 1}}},
		{"zendaya", "kid", nil}, // Dune is above the kid's ceiling
		{"ray porter", "admin", []match{{"Ray Porter", "narrator", 2}}},
		{"j", "admin", []match{{"James S. A. Corey", "author", 2}, {"Jefferson Mays", "narrator", 1}}},
		{"chani", "admin", nil}, // a character isn't a person
	} {
		v := map[string]users.Viewer{"admin": h.admin, "kid": h.kid}[tt.v]
		got, err := h.q.Search(ctx, v, tt.q, 0)
		if err != nil {
			t.Fatal(err)
		}
		var ms []match
		for _, p := range got.People {
			ms = append(ms, match{p.Name, p.Role, p.Roots})
		}
		if !slices.Equal(ms, tt.want) {
			t.Errorf("Search(%s, %q) people = %v, want %v", tt.v, tt.q, ms, tt.want)
		}
	}
}

// TestSearchCollections checks that collections match by name as works
// match by title, and are found only when the Viewer sees one of their works.
func TestSearchCollections(t *testing.T) {
	h := newHousehold(t)
	h.metadata(h.secret.Work, catalog.WorkMetadata{Collection: &catalog.CollectionRef{Key: "tmdb:1", Name: "Pokémon Collection"}})
	h.metadata(h.bladeRunner.Work, catalog.WorkMetadata{Collection: &catalog.CollectionRef{Key: "tmdb:2", Name: "Blade Runner Collection"}})
	for _, tt := range []struct {
		q    string
		v    string
		want []string
	}{
		{"toy story", "admin", []string{"Toy Story Collection"}},
		{"TOY ST", "admin", []string{"Toy Story Collection"}}, // the last word a prefix
		{"story toy", "admin", []string{"Toy Story Collection"}},
		{"oy", "admin", nil}, // words, not fragments
		{"pokemon", "admin", []string{"Pokémon Collection"}},
		{"pokemon", "friend", nil}, // its one movie is in a library the friend doesn't see
		{"collection", "admin", []string{"Blade Runner Collection", "Pokémon Collection", "Toy Story Collection"}},
		{"collection", "kid", []string{"Toy Story Collection"}}, // Blade Runner is above the kid's ceiling
	} {
		v := map[string]users.Viewer{"admin": h.admin, "friend": h.friend, "kid": h.kid}[tt.v]
		got, err := h.q.Search(ctx, v, tt.q, 0)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, c := range got.Collections {
			names = append(names, c.Name)
		}
		if !slices.Equal(names, tt.want) {
			t.Errorf("Search(%s, %q) collections = %v, want %v", tt.v, tt.q, names, tt.want)
		}
	}
	if got, _ := h.q.Search(ctx, h.admin, "collection", 1); len(got.Collections) != 1 {
		t.Errorf("limit 1 gave %d", len(got.Collections))
	}
}

// TestCollectionsCarryArtwork checks a collection's poster and backdrop.
// The poster is the collection's own, or else the poster of its first work
// that the Viewer sees and that has one. The backdrop is its own.
func TestCollectionsCarryArtwork(t *testing.T) {
	h := newHousehold(t)
	img := func(owner string, of id.ID, kind string) id.ID {
		i := id.New()
		h.b.Write(func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO image (id, `+owner+`, kind, source, origin, width, height, blurhash, fetched_at)
				VALUES (?, ?, ?, 'provider', 'x', 480, 720, 'LEHV6nWB2yk8', 0)`, i, of, kind)
			return err
		})
		return i
	}
	poster := func(v users.Viewer, coll id.ID) id.ID {
		t.Helper()
		p, err := h.q.Collection(ctx, v, coll)
		if err != nil {
			t.Fatal(err)
		}
		if p.Collection.Images.Poster == nil {
			return id.ID{}
		}
		return p.Collection.Images.Poster.ID
	}

	second := img("work_id", h.toyStory2.Work, "poster")
	if got := poster(h.admin, h.toyStoryCollection); got != second {
		t.Errorf("with only Toy Story 2's poster: %s", got)
	}
	first := img("work_id", h.toyStory.Work, "poster")
	if got := poster(h.admin, h.toyStoryCollection); got != first {
		t.Errorf("with both posters: %s, want Toy Story's", got)
	}
	own := img("collection_id", h.toyStoryCollection, "poster")
	backdrop := img("collection_id", h.toyStoryCollection, "backdrop")
	p, err := h.q.Collection(ctx, h.admin, h.toyStoryCollection)
	if err != nil {
		t.Fatal(err)
	}
	if im := p.Collection.Images; im.Poster == nil || im.Poster.ID != own || im.Backdrop == nil || im.Backdrop.ID != backdrop || im.Poster.Blurhash == "" {
		t.Errorf("with its own: %+v", im)
	}
	g, err := h.q.Collections(ctx, h.admin, h.movies, query.GridOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Items) != 1 || g.Items[0].Images.Poster == nil || g.Items[0].Images.Poster.ID != own {
		t.Errorf("the library's collections: %+v", g.Items)
	}

	// A poster of a work the Viewer doesn't see is never shown.
	h.metadata(h.secret.Work, catalog.WorkMetadata{Year: 2001, Collection: &catalog.CollectionRef{Key: "folder:x/Mixed", Name: "Mixed"}})
	h.metadata(h.dune.Work, catalog.WorkMetadata{Year: 2021, Collection: &catalog.CollectionRef{Key: "folder:x/Mixed", Name: "Mixed"}})
	mixed := h.collectionOf(h.secret.Work)
	secret := img("work_id", h.secret.Work, "poster")
	dune := img("work_id", h.dune.Work, "poster")
	if got := poster(h.admin, mixed); got != secret {
		t.Errorf("the admin's Mixed: %s, want Secret's", got)
	}
	if got := poster(h.friend, mixed); got != dune {
		t.Errorf("the friend's Mixed: %s, want Dune's", got)
	}
}

func TestSummariesCarryArtwork(t *testing.T) {
	h := newHousehold(t)
	img := func(work id.ID, kind string) id.ID {
		i := id.New()
		h.b.Write(func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO image (id, work_id, kind, source, origin, width, height, blurhash, fetched_at)
				VALUES (?, ?, ?, 'provider', 'x', 480, 720, 'LEHV6nWB2yk8', 0)`, i, work, kind)
			return err
		})
		return i
	}
	poster := img(h.expanse, "poster")
	backdrop := img(h.expanse, "backdrop")
	still := img(h.expanse1.Work, "still")
	dune := img(h.dune.Work, "poster")

	d, err := h.q.Detail(ctx, h.admin, h.expanse1.Work)
	if err != nil {
		t.Fatal(err)
	}
	im := d.Summary.Images
	if im.Still == nil || im.Still.ID != still || im.Poster == nil || im.Poster.ID != poster || im.Backdrop.ID != backdrop || im.Poster.Blurhash == "" {
		t.Fatalf("an episode's art: %+v; want its still and the series' poster and backdrop", im)
	}
	p, err := h.browse(h.admin, h.movies, grid{})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := find(p.Items, h.dune.Work)
	if c.Images.Poster == nil || c.Images.Poster.ID != dune || c.Images.Backdrop != nil {
		t.Fatalf("Dune's summary: %+v", c.Images)
	}
}

func TestWatchHistory(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	works := func(p query.Page[query.PlayEvent]) []id.ID {
		var out []id.ID
		for _, play := range p.Items {
			out = append(out, play.Work.ID)
		}
		return out
	}

	// The friend's own plays come newest first. The double episode counts
	// as a play of each of its episodes.
	p, err := h.q.History(ctx, h.friend, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	got := works(p)
	if len(got) != 5 || !slices.Contains(got[:2], h.bluey1) || !slices.Contains(got[:2], h.bluey2) ||
		!slices.Equal(got[2:], []id.ID{h.expanse2.Work, h.expanse1.Work, h.dune.Work}) {
		t.Fatalf("the friend's history = %v", got)
	}
	if first := p.Items[4]; first.Username != "friend" || first.Position != 40*time.Minute || first.Work.Title != "Dune" {
		t.Fatalf("a play: %+v", first)
	}

	// The server-wide history, as the admin sees it, has everyone's plays.
	if p, err = h.q.ServerHistory(ctx, h.admin, "", 0); err != nil || len(p.Items) != 10 || p.Next != "" {
		t.Fatalf("server-wide: %d plays, next %q, %v", len(p.Items), p.Next, err)
	}
	// Plays of works a Viewer can't see are left out, so the kid sees only
	// Bluey.
	if p, err = h.q.ServerHistory(ctx, h.kid, "", 0); err != nil || len(p.Items) != 4 {
		t.Fatalf("the kid's view of the server-wide history: %v, %v", works(p), err)
	}
	if p, err = h.q.History(ctx, h.kid, "", 0); err != nil || len(p.Items) != 0 {
		t.Fatalf("the kid's own history: %v, %v", works(p), err)
	}
	if _, err := h.q.History(ctx, h.friend, "bogus", 0); !errors.Is(err, query.ErrInvalid) {
		t.Fatalf("a bad cursor: %v", err)
	}
}

// TestLastPlayedIsAWorksOwn checks whose plays a work's last play is: a
// film's are its own, so playing one of its extras neither moves it up the
// grid by last played nor sets the date for a rule, and a series' are its
// episodes'.
func TestLastPlayedIsAWorksOwn(t *testing.T) {
	h := newHousehold(t)
	h.watch()
	trailer := h.b.Work(catalog.WorkInput{Kind: catalog.KindExtra, Parent: h.toyStory.Work, Title: "Trailer", ExtraType: "trailer"})
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO user_state (user_id, work_id, played, play_count, last_played_at, updated_at) VALUES (?, ?, 1, 1, ?, ?)`,
			h.admin.User(), trailer, time.Now().Add(time.Hour).UnixMilli(), time.Now().UnixMilli())
		return err
	})
	played := where(query.Rule{Field: "lastPlayedAt", Op: "isSet"})
	for _, tt := range []struct {
		name  string
		lib   id.ID
		kind  catalog.WorkKind
		first []id.ID // the played works, most recent first
	}{
		{"movies", h.movies, catalog.KindMovie, []id.ID{h.dune.Work}},
		{"shows", h.shows, catalog.KindSeries, []id.ID{h.bluey, h.expanse}},
	} {
		p, err := h.browse(h.admin, tt.lib, grid{Sort: "lastPlayedAt"})
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(p.Items); len(got) <= len(tt.first) || !slices.Equal(got[:len(tt.first)], tt.first) {
			t.Errorf("the %s by last played start %v, want %v", tt.name, got, tt.first)
		}
		// The rest follow by title.
		rest := p.Items[len(tt.first):]
		if !slices.IsSortedFunc(rest, func(a, b query.WorkSummary) int { return cmp.Compare(a.SortTitle, b.SortTitle) }) {
			t.Errorf("the %s never played aren't by title: %v", tt.name, ids(rest))
		}
		q := query.WorkQuery{Kinds: []catalog.WorkKind{tt.kind}, Where: played, Sort: []query.SortKey{{Field: "lastPlayedAt"}}}
		if got := ids(h.runQuery(h.admin, q).Items); !slices.Equal(got, tt.first) {
			t.Errorf("the %s with a last play are %v, want %v", tt.name, got, tt.first)
		}
	}
}

// TestMovieTrailers checks that a movie's page lists its trailers in order,
// each with its link and the thumbnail it owns, that a series' page lists
// its own, and that no other page has any.
func TestMovieTrailers(t *testing.T) {
	h := newHousehold(t)
	thumb := id.New()
	h.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO image (id, trailer_id, kind, source, origin, width, height, blurhash, fetched_at)
			SELECT ?, id, 'thumb', 'provider', 'https://i.ytimg.com/vi/8g18jFHCLXk/sddefault.jpg', 640, 480, 'LEHV6nWB2yk8pyo0adR*.7kCMdnj', 1
			FROM trailer WHERE key = '8g18jFHCLXk'`, thumb)
		return err
	})
	for name, v := range map[string]users.Viewer{"admin": h.admin, "friend": h.friend} {
		d, err := h.q.Detail(ctx, v, h.dune.Work)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Trailers) != 2 {
			t.Fatalf("%s: trailers %+v", name, d.Trailers)
		}
		first, second := d.Trailers[0], d.Trailers[1]
		if first.ID.IsZero() || first.Title != "Official Main Trailer" || first.URL != "https://www.youtube.com/watch?v=n9xhJrPXop4" || first.Image != nil {
			t.Errorf("%s: the first trailer %+v", name, first)
		}
		if second.Title != "Official Trailer" || second.URL != "https://www.youtube.com/watch?v=8g18jFHCLXk" ||
			second.Image == nil || *second.Image != (query.Image{ID: thumb, Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj", Width: 640, Height: 480}) {
			t.Errorf("%s: the second trailer %+v, image %+v", name, second, second.Image)
		}
	}
	d, err := h.q.Detail(ctx, h.friend, h.expanse)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Trailers) != 1 || d.Trailers[0].Title != "Official Trailer" || d.Trailers[0].URL != "https://www.youtube.com/watch?v=kQuTJdE3j2Q" {
		t.Errorf("the series' trailers %+v", d.Trailers)
	}
	for name, w := range map[string]id.ID{"a movie without trailers": h.bladeRunner.Work, "a season": h.expanseS1, "an episode": h.expanse1.Work} {
		d, err := h.q.Detail(ctx, h.admin, w)
		if err != nil || len(d.Trailers) != 0 {
			t.Errorf("%s: trailers %+v, %v", name, d.Trailers, err)
		}
	}
}
