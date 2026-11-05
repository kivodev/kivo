package query

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

// shelf builds RecentlyAdded rows for the bundling tests, with times
// counted back from now.
type shelf struct {
	now   time.Time
	rows  []addition
	names map[id.ID]string
}

func newShelf() *shelf { return &shelf{now: time.Now(), names: map[id.ID]string{}} }

func (s *shelf) work(name string) id.ID {
	w := id.New()
	s.names[w] = name
	return w
}

func (s *shelf) movie(name string, ago time.Duration) id.ID {
	w := s.work(name)
	s.rows = append(s.rows, addition{ID: w, Kind: "movie", RootID: w, AddedAt: s.now.Add(-ago).UnixMilli(), SortTitle: name, SeriesSortTitle: name})
	return w
}

// show is a series and its seasons.
type show struct {
	s       *shelf
	id      id.ID
	seasons map[int]id.ID
}

func (s *shelf) show(name string) *show {
	return &show{s: s, id: s.work(name), seasons: map[int]id.ID{}}
}

func (sh *show) season(n int) id.ID {
	if _, ok := sh.seasons[n]; !ok {
		sh.seasons[n] = sh.s.work(fmt.Sprintf("%s S%d", sh.s.names[sh.id], n))
	}
	return sh.seasons[n]
}

func (sh *show) episode(season, episode int, ago time.Duration) id.ID {
	name := fmt.Sprintf("%s S%dE%d", sh.s.names[sh.id], season, episode)
	w := sh.s.work(name)
	sh.s.rows = append(sh.s.rows, addition{ID: w, Kind: "episode", ParentID: sh.season(season), RootID: sh.id,
		AddedAt: sh.s.now.Add(-ago).UnixMilli(), SortTitle: name, SeasonSortTitle: fmt.Sprintf("Season %d", season),
		SeriesSortTitle: sh.s.names[sh.id]})
	return w
}

// play marks works played by the user.
func (s *shelf) play(works ...id.ID) {
	for i, r := range s.rows {
		if slices.Contains(works, r.ID) {
			s.rows[i].Played = true
		}
	}
}

// got renders bundles as "name" or "name ×N".
func (s *shelf) got() []string {
	var out []string
	for _, b := range bundles(s.rows) {
		name := s.names[b.work]
		if b.episodes > 0 {
			name += fmt.Sprintf(" ×%d", b.episodes)
		}
		out = append(out, name)
	}
	return out
}

func TestBundles(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func(s *shelf)
		want  []string
	}{{
		name: "an import across seasons is the show's summary",
		build: func(s *shelf) {
			sh := s.show("Lost")
			for e := range 6 {
				sh.episode(1+e/3, e%3+1, time.Duration(e)*time.Minute)
			}
		},
		want: []string{"Lost ×6"},
	}, {
		name: "an import of one season is the season's summary",
		build: func(s *shelf) {
			sh := s.show("Lost")
			for e := range 4 {
				sh.episode(2, e+1, time.Duration(e)*time.Minute)
			}
		},
		want: []string{"Lost S2 ×4"},
	}, {
		name: "a lone later episode is its own summary",
		build: func(s *shelf) {
			sh := s.show("Lost")
			sh.episode(1, 4, 0)
			for e := range 3 {
				sh.episode(1, e+1, 3*24*time.Hour+time.Duration(e)*time.Minute)
			}
		},
		want: []string{"Lost S1E4", "Lost S1 ×3"},
	}, {
		name: "a later season dropped in is a season's summary",
		build: func(s *shelf) {
			sh := s.show("Lost")
			for e := range 3 {
				sh.episode(2, e+1, time.Duration(e)*time.Minute)
			}
			for e := range 6 {
				sh.episode(1, e+1, 30*24*time.Hour+time.Duration(e)*time.Minute)
			}
		},
		want: []string{"Lost S2 ×3", "Lost S1 ×6"},
	}, {
		name: "additions more than 24 hours apart are separate runs",
		build: func(s *shelf) {
			sh := s.show("Lost")
			sh.episode(1, 3, 0)
			sh.episode(1, 2, 23*time.Hour)                 // within a day of E3
			sh.episode(1, 1, 46*time.Hour)                 // within a day of E2, so one run
			sh.episode(2, 1, 71*time.Hour)                 // 25 hours after E1
			sh.episode(2, 2, 72*time.Hour)                 // with S2E1
			sh.episode(3, 1, 10*24*time.Hour)              // alone
			sh.episode(3, 2, 10*24*time.Hour+25*time.Hour) // alone too
		},
		want: []string{"Lost S1 ×3", "Lost S2 ×2", "Lost S3E1", "Lost S3E2"},
	}, {
		name: "a show's runs onto the same summary show once, at the newest",
		build: func(s *shelf) {
			sh := s.show("Lost")
			sh.episode(1, 3, 0)
			sh.episode(1, 2, time.Minute)
			sh.episode(1, 1, 3*24*time.Hour)
			sh.episode(1, 5, 3*24*time.Hour+time.Minute)
		},
		want: []string{"Lost S1 ×2"},
	}, {
		name: "shows are bundled apart, interleaved or not",
		build: func(s *shelf) {
			a, b := s.show("Andor"), s.show("Bluey")
			for e := range 4 {
				a.episode(1, e+1, time.Duration(2*e)*time.Minute)
				b.episode(1, e+1, time.Duration(2*e+1)*time.Minute)
			}
		},
		want: []string{"Andor S1 ×4", "Bluey S1 ×4"},
	}, {
		name: "movies are a summary each",
		build: func(s *shelf) {
			s.movie("Dune", 0)
			s.movie("Blade Runner", time.Hour)
			s.movie("Arrival", 2*time.Hour)
		},
		want: []string{"Dune", "Blade Runner", "Arrival"},
	}, {
		name: "a batch added together reads alphabetically",
		build: func(s *shelf) {
			s.movie("Zodiac", 0)
			s.movie("Alien", 0)
			sh := s.show("Mad Men")
			sh.episode(1, 1, 0)
			sh.episode(1, 2, time.Minute)
		},
		want: []string{"Alien", "Mad Men S1 ×2", "Zodiac"}, // the season's summary by its title, "Season 1"
	}, {
		name: "played works are left out, and a bundle only once all of it is",
		build: func(s *shelf) {
			s.play(s.movie("Dune", 0))
			s.movie("Arrival", time.Hour)
			a, b := s.show("Andor"), s.show("Bluey")
			s.play(a.episode(1, 1, time.Minute), a.episode(1, 2, 2*time.Minute))
			s.play(b.episode(1, 1, time.Minute))
			b.episode(1, 2, 2*time.Minute)
		},
		want: []string{"Bluey S1 ×2", "Arrival"},
	}, {
		name: "a show's older run takes the place of a played one on the same summary",
		build: func(s *shelf) {
			sh := s.show("Lost")
			s.play(sh.episode(1, 3, 0), sh.episode(1, 2, time.Minute))
			sh.episode(1, 1, 3*24*time.Hour)
			sh.episode(1, 5, 3*24*time.Hour+time.Minute)
		},
		want: []string{"Lost S1 ×2"},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			s := newShelf()
			tt.build(s)
			if got := s.got(); !slices.Equal(got, tt.want) {
				t.Errorf("bundles = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBundlesBigImport checks that a 300-episode import is one summary, so it
// leaves room in the row for what else was added.
func TestBundlesBigImport(t *testing.T) {
	s := newShelf()
	big := s.show("One Piece")
	for e := range 300 {
		big.episode(1+e/50, e%50+1, time.Duration(e)*time.Second)
	}
	for i := range 25 {
		sh := s.show(fmt.Sprintf("Show %02d", i))
		sh.episode(1, 1, time.Hour+time.Duration(i)*time.Minute)
	}
	got := s.got()
	if len(got) != 26 || got[0] != "One Piece ×300" || got[1] != "Show 00 S1E1" {
		t.Errorf("bundles = %q, want One Piece's 300 episodes as one summary, then the 25 other shows", got)
	}
}
