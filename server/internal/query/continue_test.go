package query_test

import (
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/users"
)

// TestSeriesUpNext checks that a series' summary in Continue Watching and
// its page's primary button follow one rule, based on the episode played
// last. That episode is up next while it's in progress. Otherwise the first
// unwatched episode after it is up next.
func TestSeriesUpNext(t *testing.T) {
	for _, c := range []struct {
		name  string
		plays func(h *household) // the admin's plays of The Expanse
		home  func(h *household) []id.ID
		page  func(h *household) id.ID
	}{
		{
			name: "after the episode watched last",
			plays: func(h *household) {
				h.play(h.admin, h.expanse1.Version, 24*time.Minute)
				h.play(h.admin, h.expanse2.Version, 24*time.Minute)
			},
			home: func(h *household) []id.ID { return []id.ID{h.expanse3.Work} },
			page: func(h *household) id.ID { return h.expanse3.Work },
		},
		{
			name: "the episode in progress",
			plays: func(h *household) {
				h.play(h.admin, h.expanse1.Version, 24*time.Minute)
				h.play(h.admin, h.expanse2.Version, 10*time.Minute)
			},
			home: func(h *household) []id.ID { return []id.ID{h.expanse2.Work} },
			page: func(h *household) id.ID { return h.expanse2.Work },
		},
		{
			name: "an older episode left in progress doesn't hold the series back",
			plays: func(h *household) {
				h.play(h.admin, h.expanse1.Version, 10*time.Minute)
				h.play(h.admin, h.expanse2.Version, 24*time.Minute)
			},
			home: func(h *household) []id.ID { return []id.ID{h.expanse3.Work} },
			page: func(h *household) id.ID { return h.expanse3.Work },
		},
		{
			name: "two episodes in progress show once, by the later played",
			plays: func(h *household) {
				h.play(h.admin, h.expanse2.Version, 10*time.Minute)
				h.play(h.admin, h.expanse1.Version, 10*time.Minute)
			},
			home: func(h *household) []id.ID { return []id.ID{h.expanse1.Work} },
			page: func(h *household) id.ID { return h.expanse1.Work },
		},
		{
			name: "an episode watched again",
			plays: func(h *household) {
				h.play(h.admin, h.expanse1.Version, 24*time.Minute)
				h.play(h.admin, h.expanse2.Version, 24*time.Minute)
				h.play(h.admin, h.expanse1.Version, 10*time.Minute)
			},
			home: func(h *household) []id.ID { return []id.ID{h.expanse1.Work} },
			page: func(h *household) id.ID { return h.expanse1.Work },
		},
		{
			name: "an episode only glimpsed since isn't watched",
			plays: func(h *household) {
				h.play(h.admin, h.expanse1.Version, 24*time.Minute)
				h.play(h.admin, h.expanse3.Version, 10*time.Second)
			},
			home: func(h *household) []id.ID { return []id.ID{h.expanse2.Work} },
			page: func(h *household) id.ID { return h.expanse2.Work },
		},
		{
			name: "caught up, with an older episode left in progress",
			plays: func(h *household) {
				h.play(h.admin, h.expanse2.Version, 10*time.Minute)
				h.play(h.admin, h.expanse1.Version, 24*time.Minute)
				h.play(h.admin, h.expanse3.Version, 24*time.Minute)
			},
			home: func(*household) []id.ID { return nil },
			page: func(h *household) id.ID { return h.expanse1.Work },
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHousehold(t)
			c.plays(h)
			home, _, err := h.q.Home(ctx, h.admin)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(row(home, query.RowContinueWatching)); !slices.Equal(got, c.home(h)) {
				t.Errorf("Continue Watching %v, want %v", got, c.home(h))
			}
			d, err := h.q.Detail(ctx, h.admin, h.expanse)
			if err != nil {
				t.Fatal(err)
			}
			if d.NextUp == nil || d.NextUp.ID != c.page(h) {
				t.Errorf("the series page's next episode %+v, want %v", d.NextUp, c.page(h))
			}
		})
	}
}

// TestContinueWatching checks the row as a whole. It holds the movies in
// progress and each series' episode up next, most recently played first.
// Each item has its own state, and only works the Viewer may see appear.
func TestContinueWatching(t *testing.T) {
	h := newHousehold(t)
	for _, v := range []users.Viewer{h.admin, h.kid, h.friend} {
		h.play(v, h.secret.Version, 40*time.Minute)
		h.play(v, h.dune.Version, 40*time.Minute)
		h.play(v, h.toyStory.Version, 2*time.Hour) // finished
		h.play(v, h.blueyDouble, 5*time.Minute)    // both episodes in progress
		h.play(v, h.expanse1.Version, 24*time.Minute)
	}
	for _, c := range []struct {
		name   string
		viewer users.Viewer
		want   []id.ID
	}{
		{"most recently played first", h.admin, []id.ID{h.expanse2.Work, h.bluey1, h.dune.Work, h.secret.Work}},
		{"in the Viewer's libraries", h.friend, []id.ID{h.expanse2.Work, h.bluey1, h.dune.Work}},
		{"within the rating ceiling", h.kid, []id.ID{h.bluey1, h.secret.Work}},
	} {
		t.Run(c.name, func(t *testing.T) {
			home, _, err := h.q.Home(ctx, c.viewer)
			if err != nil {
				t.Fatal(err)
			}
			cw := row(home, query.RowContinueWatching)
			if got := ids(cw); !slices.Equal(got, c.want) {
				t.Fatalf("Continue Watching %v, want %v", got, c.want)
			}
			if b, _ := find(cw, h.bluey1); b.Userstate.Position != 5*time.Minute || b.NumberEnd != 2 {
				t.Errorf("the double episode in progress %+v, want its position and both numbers", b)
			}
			if e, ok := find(cw, h.expanse2.Work); ok && (e.Userstate.Position != 0 || e.Userstate.Played) {
				t.Errorf("the next episode %+v, want it unplayed", e)
			}
		})
	}
}
