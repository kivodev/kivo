package query_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/query"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
	"github.com/mmcdole/kivo/internal/users"
)

// The read budgets, at 50k works while a scan is writing. scrollBudget is
// for a later page of a grid or a list of top-level works on a household's
// server, which a client reads as it scrolls.
const (
	browseBudget = 50 * time.Millisecond
	scrollBudget = 20 * time.Millisecond
	homeBudget   = 100 * time.Millisecond
	searchBudget = 100 * time.Millisecond
)

// TestBudgets times each read against 50k generated works (40,000 movies and
// 400 series of 24 episodes). Meanwhile another goroutine writes batches of
// 500 files, shaped like a scan's. The test fails when a read's p95 is over
// its budget. It then times the smart lists there, and the smart lists, the
// Lists screen, Home and the grids on a household's server, and fails when
// a held one's median is over its budget: a p95 taken beside the writer
// says more of the machine than of the read.
//
// It runs only when KIVO_BUDGET=1, which make budget sets, and runs alone
// without the race detector. Timings taken beside other packages' tests
// mean nothing, and skipping it keeps a plain go test ./... deterministic.
func TestBudgets(t *testing.T) {
	if os.Getenv("KIVO_BUDGET") != "1" {
		t.Skip("times reads; make budget runs it (KIVO_BUDGET=1)")
	}
	db := storetest.New(t)
	movies, shows, moviesFolder := generate(t, db, fiftyK)
	v := budgetViewer(t, db, "admin")
	q := query.New(db)
	sv, smart := smartViewer(t, db, q)

	stop := scanWriter(t, db, moviesFolder)

	tests := append(gridCases(q, v, movies, shows, 20000, false), // deep is halfway through the movies
		budgetCase{"home", homeBudget, true, func() error { _, _, err := q.Home(ctx, v); return err }},
		budgetCase{"my list", browseBudget, true, func() error {
			lists, err := q.Lists(ctx, v)
			if err != nil {
				return err
			}
			_, err = q.ListWorks(ctx, v, lists[0].ID, nil, query.GridOptions{})
			return err
		}},
		budgetCase{"search", searchBudget, true, func() error { _, err := q.Search(ctx, v, "movie 123", 0); return err }},
		budgetCase{"search, a person", searchBudget, true, func() error { _, err := q.Search(ctx, v, "mary actor 123", 0); return err }},
		// Every movie and every person has "the" and a word starting with
		// "m", so this query has the most matches a query can have.
		budgetCase{"search, a word in every title", searchBudget, true, func() error { _, err := q.Search(ctx, v, "the", 0); return err }},
		budgetCase{"search, one letter", searchBudget, true, func() error { _, err := q.Search(ctx, v, "m", 0); return err }},
	)
	timeBudgets(t, tests, false)
	// A smart list's first page reads every work of its kinds in the
	// Viewer's libraries, and so does a later page that no index holds in
	// order or that few works match. So a list is held to the budget where
	// those works are few, where a cap keeps its order to the first works,
	// or where its rules are light: not the other lists of 40,000 movies.
	timeBudgets(t, smartCases(q, sv, smart, browseBudget, "movies, title and rating, by title", "series, by last episode added",
		"episodes, aired this year", "movies, capped at 100"), true)
	stop()

	// The smart lists again on a household's server, as a user granted one
	// of its two movies libraries and one of its two shows libraries. Every
	// list is held, and Home and the Lists screen.
	db = storetest.New(t)
	movies, shows, moviesFolder = generate(t, db, householdShape)
	q = query.New(db)
	sv, smart = smartViewer(t, db, q, movies, shows)
	stop = scanWriter(t, db, moviesFolder)
	defer stop()
	t.Log("a household's server: 5,000 movies and 1,500 series of 105,000 episodes, in four libraries, two of them granted")
	timeBudgets(t, smartCases(q, sv, smart, scrollBudget, "movies, title and rating, by title", "movies, one actor, by added",
		"movies, unplayed, by last played", "series, by last episode added", "episodes, aired this year", "movies, capped at 100",
		smartHome, listsScreen), true)
	// Its grids, as that user: 4,000 movies and 1,200 series.
	timeBudgets(t, gridCases(q, sv, movies, shows, 2000, true), true)
}

// gridCases are the reads of a movies and a shows library's grids: each
// sort's first page, which counts the grid, and a page a few screens down,
// which doesn't; the movies at deep, by an order of each kind; and the
// filtered grids.
//
// On a household's server every read is held, and a later page to
// scrollBudget. On any other, a page reads every row its offset skips, so
// the deep pages are timed only, and so is the filter on per-user state and
// genres, which is checked on every work to count the matches.
func gridCases(q *query.Service, v users.Viewer, movies, shows id.ID, deep int, household bool) []budgetCase {
	var tests []budgetCase
	add := func(name string, enforce bool, lib id.ID, o query.BrowseOptions) {
		budget := browseBudget
		if household && o.Offset > 0 && o.Offset != deep {
			budget = scrollBudget
		}
		tests = append(tests, budgetCase{name, budget, enforce, browse(q, v, lib, o)})
	}
	sorts := []struct {
		name  string
		sort  query.Sort
		shows bool
	}{{"title", query.ByTitle, true}, {"added", query.ByAdded, false}, {"release", query.ByRelease, false},
		{"rating", query.ByCommunityRating, true}, {"my rating", query.ByMyRating, true}, {"runtime", query.ByRuntime, false},
		{"last played", query.ByLastPlayed, true}, {"last episode added", query.ByLastEpisodeAdded, true}}
	later := query.GridOptions{Offset: 600}
	for _, s := range sorts {
		if s.sort != query.ByLastEpisodeAdded {
			add("browse movies by "+s.name, true, movies, query.BrowseOptions{Sort: s.sort})
			add("browse movies by "+s.name+", later page", true, movies, query.BrowseOptions{Sort: s.sort, GridOptions: later})
		}
		if s.shows {
			add("browse shows by "+s.name, true, shows, query.BrowseOptions{Sort: s.sort})
			add("browse shows by "+s.name+", later page", true, shows, query.BrowseOptions{Sort: s.sort, GridOptions: later})
		}
	}
	for _, s := range []int{0, 3, 4, 6} {
		add("browse movies by "+sorts[s].name+", deep", household, movies, query.BrowseOptions{Sort: sorts[s].sort, GridOptions: query.GridOptions{Offset: deep}})
	}
	unplayedDrama := query.Filter{Genres: []string{"Drama"}, Played: query.Unplayed}
	uhd := query.Filter{Resolutions: []query.Resolution{query.Resolution4K}}
	add("browse movies, genre and unplayed", household, movies, query.BrowseOptions{Filter: unplayedDrama})
	add("browse movies, genre and unplayed, later page", household, movies, query.BrowseOptions{Filter: unplayedDrama, GridOptions: later})
	add("browse movies, 4K", true, movies, query.BrowseOptions{Filter: uhd})
	add("browse movies, 4K, by added, later page", true, movies, query.BrowseOptions{Sort: query.ByAdded, Filter: uhd, GridOptions: later})
	add("browse movies, HDR", true, movies, query.BrowseOptions{Filter: query.Filter{Ranges: []query.Range{query.HDR}}})
	add("browse movies, 1990s, by rating", true, movies, query.BrowseOptions{Sort: query.ByCommunityRating, Filter: query.Filter{YearFrom: 1990, YearTo: 1999}})
	add("browse shows, in progress", true, shows, query.BrowseOptions{Filter: query.Filter{Played: query.InProgress}})
	return tests
}

// smartHome is the read of a Home that shows five smart lists, and
// listsScreen that of the Lists screen.
const (
	smartHome   = "home, 5 smart rows"
	listsScreen = "lists screen, 10 lists"
)

// budgetCase is a read to time, held to budget when enforce is set.
type budgetCase struct {
	name    string
	budget  time.Duration
	enforce bool
	read    func() error
}

// smartCases are the reads of the smart lists, of the Lists screen, and of
// a Home that shows five of the lists. A list's first page reads its works
// and counts them, a later page reads its works alone, and its header reads
// the list without running its rules. held names the lists whose reads are
// held to browseBudget, a later page of top-level works to later; with
// smartHome it holds Home, and with listsScreen the Lists screen, which
// runs every smart list's rules as Home runs five, to homeBudget.
func smartCases(q *query.Service, v users.Viewer, smart []smartList, later time.Duration, held ...string) []budgetCase {
	var tests []budgetCase
	for _, l := range smart {
		enforce := slices.Contains(held, l.name)
		page := func(o query.GridOptions) func() error {
			return func() error { _, err := q.ListWorks(ctx, v, l.id, nil, o); return err }
		}
		scroll := later
		if l.nested {
			scroll = browseBudget
		}
		tests = append(tests,
			budgetCase{"smart " + l.name, browseBudget, enforce, page(query.GridOptions{})},
			budgetCase{"smart " + l.name + ", later page", scroll, enforce, page(query.GridOptions{Offset: 60})},
			budgetCase{"smart " + l.name + ", header", browseBudget, enforce, func() error { _, err := q.List(ctx, v, l.id); return err }})
	}
	return append(tests,
		budgetCase{smartHome, homeBudget, slices.Contains(held, smartHome), func() error { _, _, err := q.Home(ctx, v); return err }},
		budgetCase{listsScreen, homeBudget, slices.Contains(held, listsScreen), func() error { _, err := q.CountedLists(ctx, v); return err }})
}

// timeBudgets times each read and fails for an enforced one whose p95 is
// over its budget, or whose median is when median is set.
func timeBudgets(t *testing.T, tests []budgetCase, median bool) {
	t.Helper()
	// The budgets are for a real server. A shared CI runner is several
	// times slower, so CI sets KIVO_BUDGET_SCALE and the check there guards
	// against regressions rather than proving the budget.
	scale := 1.0
	if f, err := strconv.ParseFloat(os.Getenv("KIVO_BUDGET_SCALE"), 64); err == nil && f > 0 {
		scale = f
	}
	for _, tt := range tests {
		tt.budget = time.Duration(float64(tt.budget) * scale)
		p50, p95, err := percentiles(tt.read)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		held, name, timed := "", "p95", p95
		if median {
			held, name, timed = " on p50", "p50", p50
		}
		if !tt.enforce {
			held = ", not held"
		}
		t.Logf("%-50s p50 %6.1f ms  p95 %6.1f ms (budget %v%s)", tt.name, ms(p50), ms(p95), tt.budget, held)
		if tt.enforce && timed > tt.budget {
			t.Errorf("%s: %s %v over its budget of %v", tt.name, name, timed, tt.budget)
		}
	}
}

func browse(q *query.Service, v users.Viewer, lib id.ID, o query.BrowseOptions) func() error {
	return func() error { _, err := q.Browse(ctx, v, lib, o); return err }
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// percentiles times 60 reads after 5 to warm up, and returns their median
// and 95th percentile.
func percentiles(read func() error) (p50, p95 time.Duration, err error) {
	const runs = 60
	var ds []time.Duration
	for i := range runs + 5 {
		start := time.Now()
		if err := read(); err != nil {
			return 0, 0, err
		}
		if i >= 5 { // warm-up
			ds = append(ds, time.Since(start))
		}
	}
	slices.Sort(ds)
	return ds[runs/2], ds[runs*95/100], nil
}

// scanWriter writes batches of 500 new files, as a scan of a large folder
// does, until stopped.
func scanWriter(t *testing.T, db *store.DB, folder id.ID) (stop func()) {
	cat := catalog.New(db, events.NewBus())
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		for batch := 0; ctx.Err() == nil; batch++ {
			obs := make([]catalog.FileObservation, 500)
			for i := range obs {
				obs[i] = catalog.FileObservation{RelPath: fmt.Sprintf("scan/%d/%d.mkv", batch, i), Size: 1 << 30, MTime: time.UnixMilli(1)}
			}
			err := db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
				_, err := cat.UpsertFiles(ctx, tx, folder, obs)
				return err
			})
			if err != nil && ctx.Err() == nil {
				t.Error(err)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
	return func() { cancel(); wg.Wait() }
}

// shape is how many works generate makes: movies, and series of seasons
// seasons of episodes episodes each, an episode airing every airEvery
// seconds. The last moviesElsewhere movies and seriesElsewhere series are
// in a second movies and a second shows library.
type shape struct {
	movies, series, seasons, episodes, airEvery int
	moviesElsewhere, seriesElsewhere            int
}

var (
	// fiftyK is 40,000 movies and 400 series of 2 seasons of 12 episodes,
	// in two libraries, the episodes aired one every 9 hours over the last
	// ten years.
	fiftyK = shape{movies: 40000, series: 400, seasons: 2, episodes: 12, airEvery: 32400}
	// householdShape is a household's server: 5,000 movies and 1,500 series
	// of 5 seasons of 14 episodes, 105,000 episodes aired over the last ten
	// years, in four libraries.
	householdShape = shape{movies: 5000, series: 1500, seasons: 5, episodes: 14, airEvery: 3000, moviesElsewhere: 1000, seriesElsewhere: 300}
)

// generate fills db with s's movies and series. Each movie and episode has
// a file, a version and a video track. It adds 20,000 people credited on
// the movies, and indexes everything for search. It writes the tables
// directly, which only a test may do, because building 50k works through
// catalog one at a time would take minutes.
func generate(t *testing.T, db *store.DB, s shape) (movies, shows, moviesFolder id.ID) {
	t.Helper()
	cat := catalog.New(db, events.NewBus())
	var showsFolder id.ID
	// Where the works elsewhere go: the first libraries when there are none.
	var movies2, shows2, moviesFolder2, showsFolder2 id.ID
	err := db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		m, err := cat.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Movies", Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/m")}})
		if err != nil {
			return err
		}
		sh, err := cat.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Shows", Kind: catalog.Shows, Folders: []string{catalogtest.Abs("/s")}})
		if err != nil {
			return err
		}
		movies, moviesFolder, shows, showsFolder = m.ID, m.Folders[0].ID, sh.ID, sh.Folders[0].ID
		movies2, moviesFolder2, shows2, showsFolder2 = movies, moviesFolder, shows, showsFolder
		if s.moviesElsewhere > 0 || s.seriesElsewhere > 0 {
			m, err := cat.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Movies 2", Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/m2")}})
			if err != nil {
				return err
			}
			sh, err := cat.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Shows 2", Kind: catalog.Shows, Folders: []string{catalogtest.Abs("/s2")}})
			if err != nil {
				return err
			}
			movies2, moviesFolder2, shows2, showsFolder2 = m.ID, m.Folders[0].ID, sh.ID, sh.Folders[0].ID
		}
		now := time.Now().UnixMilli()
		for _, stmt := range []string{
			// The movies. Each ID is a letter and a number, padded to 16
			// bytes.
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < @n_movies)
			 INSERT INTO work (id, kind, root_id, title, sort_title, year, content_rating_age, community_rating, community_votes, added_at)
			 SELECT CAST(printf('m%015d', i) AS BLOB), 'movie', CAST(printf('m%015d', i) AS BLOB),
			   printf('The Movie %d', i), printf('Movie %d', i), 1950 + i % 75, i % 19,
			   CASE WHEN i % 10 THEN i % 91 / 10.0 + 1 END, CASE WHEN i % 10 THEN i * 37 % 20000 + 1 END, @now - i * 1000 FROM n`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < @n_series)
			 INSERT INTO work (id, kind, root_id, title, sort_title, year, content_rating_age, community_rating, community_votes, added_at)
			 SELECT CAST(printf('s%015d', i) AS BLOB), 'series', CAST(printf('s%015d', i) AS BLOB),
			   printf('Show %d', i), printf('Show %d', i), 1990 + i % 35, i % 19,
			   CASE WHEN i % 10 THEN i % 91 / 10.0 + 1 END, CASE WHEN i % 10 THEN i * 37 % 20000 + 1 END, @now - i * 1000 FROM n`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < @n_series * @seasons)
			 INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, number, added_at)
			 SELECT CAST(printf('z%015d', i) AS BLOB), 'season', CAST(printf('s%015d', (i + @seasons - 1) / @seasons) AS BLOB),
			   CAST(printf('s%015d', (i + @seasons - 1) / @seasons) AS BLOB), printf('Season %d', (i - 1) % @seasons + 1), printf('Season %d', (i - 1) % @seasons + 1),
			   (i - 1) % @seasons + 1, @now FROM n`,
			// The episodes, aired one every @air_every seconds.
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < @n_series * @seasons * @episodes)
			 INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, parent_number, number, release_date, added_at)
			 SELECT CAST(printf('e%015d', i) AS BLOB), 'episode', CAST(printf('z%015d', (i + @episodes - 1) / @episodes) AS BLOB),
			   CAST(printf('s%015d', (i + @seasons * @episodes - 1) / (@seasons * @episodes)) AS BLOB), printf('Episode %d', i), printf('Episode %d', i),
			   ((i + @episodes - 1) / @episodes - 1) % @seasons + 1, (i - 1) % @episodes + 1, date(@now / 1000 - i * @air_every, 'unixepoch'), @now - i * 100 FROM n`,
			// A file, a version and a video track for each movie and episode.
			`INSERT INTO media_file (id, folder_id, rel_path, size, mtime, status, duration_ms)
			 SELECT CAST('f' || substr(CAST(id AS TEXT), 1, 1) || substr(CAST(id AS TEXT), 3) AS BLOB), CASE WHEN kind = 'movie' THEN iif(id <= @last_movie, @movies_folder, @movies_folder2) ELSE iif(root_id <= @last_series, @shows_folder, @shows_folder2) END,
			   CAST(id AS TEXT) || '.mkv', 1 << 30, 1, 'present', CASE kind WHEN 'movie' THEN 6000000 + rid ELSE 1500000 END
			 FROM work WHERE kind IN ('movie', 'episode')`,
			`INSERT INTO media_version (id, created_at) SELECT CAST('v' || substr(CAST(id AS TEXT), 1, 1) || substr(CAST(id AS TEXT), 3) AS BLOB), @now FROM work WHERE kind IN ('movie', 'episode')`,
			`UPDATE media_file SET version_id = CAST('v' || substr(rel_path, 1, 1) || substr(rel_path, 3, length(rel_path) - 6) AS BLOB), part = 1`,
			`INSERT INTO media_version_work (version_id, work_id, ord)
			 SELECT CAST('v' || substr(CAST(id AS TEXT), 1, 1) || substr(CAST(id AS TEXT), 3) AS BLOB), id, 0 FROM work WHERE kind IN ('movie', 'episode')`,
			`INSERT INTO media_track (id, file_id, source, stream_index, type, codec, info)
			 SELECT CAST('t' || substr(CAST(id AS TEXT), 1, 1) || substr(CAST(id AS TEXT), 3) AS BLOB), CAST('f' || substr(CAST(id AS TEXT), 1, 1) || substr(CAST(id AS TEXT), 3) AS BLOB), 'embedded', 0, 'video', 'h264', json_object('width', CASE rid % 3 WHEN 0 THEN 3840 ELSE 1920 END, 'range', CASE rid % 50 WHEN 0 THEN 'hdr10' ELSE 'sdr' END)
			 FROM work WHERE kind IN ('movie', 'episode')`,
			`INSERT INTO work_library (library_id, work_id, is_root, sort_title, added_at, year, runtime_sort, resolutions, dynamic_ranges, rating_sort, last_added_at)
			 SELECT CASE WHEN w.kind = 'movie' THEN iif(w.id <= @last_movie, @movies, @movies2) ELSE iif(w.root_id <= @last_series, @shows, @shows2) END, w.id, w.parent_id IS NULL, w.sort_title, w.added_at,
			   k.year, k.runtime_sort, k.resolutions, k.dynamic_ranges, k.rating_sort,
			   (SELECT max(d.added_at) FROM work d WHERE d.kind IN ('movie', 'episode') AND (d.id = w.id OR d.root_id = w.id OR d.parent_id = w.id))
			 FROM work w JOIN work_browse_keys k ON k.work_id = w.id`,
			// Only episodes get a release key, which the grid never sorts,
			// so its release sort reads the movies as it always has.
			`UPDATE work_library SET release_key = (SELECT w.release_date FROM work w WHERE w.id = work_library.work_id)
			 WHERE work_id IN (SELECT id FROM work WHERE kind = 'episode')`,
			`INSERT INTO genre (name) VALUES ('Drama'), ('Comedy'), ('Action')`,
			`INSERT INTO work_genre (work_id, genre_id) SELECT id, rid % 3 + 1 FROM work WHERE parent_id IS NULL`,
			// 20,000 people, each an actor in two movies.
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 20000)
			 INSERT INTO person (id, name, sort_name) SELECT CAST(printf('p%015d', i) AS BLOB), printf('Mary Actor %d', i), printf('Mary Actor %d', i) FROM n`,
			`INSERT INTO credit (work_id, person_id, role, ord)
			 SELECT id, CAST(printf('p%015d', (rid - 1) % 20000 + 1) AS BLOB), 'actor', 1 FROM work WHERE kind = 'movie'`,
			`INSERT INTO work_fts (rowid, title, alt_titles, people)
			 SELECT w.rid, w.title, '', coalesce((SELECT group_concat(p.name, char(10)) FROM credit c JOIN person p ON p.id = c.person_id WHERE c.work_id = w.id), '')
			 FROM work w`,
			`INSERT INTO person_fts (rowid, name) SELECT rid, name FROM person`,
		} {
			var args []any
			if strings.Contains(stmt, "@") {
				args = []any{sql.Named("now", now), sql.Named("movies", movies), sql.Named("shows", shows),
					sql.Named("movies_folder", moviesFolder), sql.Named("shows_folder", showsFolder),
					sql.Named("movies2", movies2), sql.Named("shows2", shows2),
					sql.Named("movies_folder2", moviesFolder2), sql.Named("shows_folder2", showsFolder2),
					sql.Named("n_movies", s.movies), sql.Named("n_series", s.series), sql.Named("seasons", s.seasons),
					sql.Named("episodes", s.episodes), sql.Named("air_every", s.airEvery),
					// The last movie and series of the first libraries.
					sql.Named("last_movie", fmt.Appendf(nil, "m%015d", s.movies-s.moviesElsewhere)),
					sql.Named("last_series", fmt.Appendf(nil, "s%015d", s.series-s.seriesElsewhere))}
			}
			_, err := tx.ExecContext(ctx, stmt, args...)
			if err != nil {
				return fmt.Errorf("%w in %s", err, stmt)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return movies, shows, moviesFolder
}

// budgetViewer is a user who has watched 2,000 movies, has 300 in progress
// and 50 movies on My List, and is partway through 100 series: an admin
// with every library, or a user granted libs.
func budgetViewer(t *testing.T, db *store.DB, name string, libs ...id.ID) users.Viewer {
	t.Helper()
	svc := users.New(db, events.NewBus())
	var user id.ID
	err := db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		slices.SortFunc(libs, func(a, b id.ID) int { return slices.Compare(a[:], b[:]) })
		u, err := svc.CreateUser(ctx, tx, users.NewUser{Username: name, Admin: len(libs) == 0, Grants: users.Grants{All: len(libs) == 0, Libraries: libs}})
		user = u.ID
		if err != nil {
			return err
		}
		// One statement per call: the driver refuses several with arguments.
		args := []any{sql.Named("user", user), sql.Named("now", time.Now().UnixMilli())}
		for _, stmt := range []string{`
			INSERT INTO user_state (user_id, work_id, played, play_count, position_ms, resumable, rating, last_played_at, updated_at)
			SELECT @user, id, rid % 7 <> 0, rid % 7 <> 0, 600000 * (rid % 7 = 0), rid % 7 = 0,
			  CASE WHEN rid % 5 = 0 THEN rid % 10 + 1 END, @now - rid, @now - rid
			FROM work WHERE kind = 'movie' AND rid <= 2300
			UNION ALL
			SELECT @user, id, 1, 1, 0, 0, NULL, @now - rid, @now - rid
			FROM work WHERE kind = 'episode' AND number <= 6 AND root_id <= CAST(printf('s%015d', 100) AS BLOB)`, `
			INSERT INTO list_work (list_id, work_id, added_at)
			SELECT l.id, w.id, @now - w.rid
			FROM work w, list l
			WHERE l.user_id = @user AND w.kind = 'movie' AND w.rid <= 2000 AND w.rid % 40 = 0`,
		} {
			if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.Viewer(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// smartList is a smart list the budgets time. nested is set for one of
// seasons or episodes.
type smartList struct {
	name   string
	id     id.ID
	nested bool
}

// smartViewer is a second budgetViewer with smart lists, the first five of
// them shown at the end of Home.
func smartViewer(t *testing.T, db *store.DB, q *query.Service, libs ...id.ID) (users.Viewer, []smartList) {
	t.Helper()
	v := budgetViewer(t, db, "smart", libs...)
	svc := users.New(db, events.NewBus())
	var out []smartList
	for _, d := range smartDefs() {
		saved, err := d.q.Saved()
		if err != nil {
			t.Fatal(err)
		}
		list, err := svc.CreateList(ctx, v, d.name, &saved)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, smartList{d.name, list, slices.Contains(d.q.Kinds, catalog.KindEpisode)})
	}
	// Three manual lists, with none, 2 and 200 movies, make ten lists with
	// Watchlist for the Lists screen.
	for _, every := range []int{0, 1000, 10} {
		list, err := svc.CreateList(ctx, v, fmt.Sprintf("manual %d", every), nil)
		if err != nil {
			t.Fatal(err)
		}
		if every == 0 {
			continue
		}
		err = db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO list_work (list_id, work_id, added_at)
				SELECT @list, id, rid FROM work WHERE kind = 'movie' AND rid <= 2000 AND rid % @every = 0`,
				sql.Named("list", list), sql.Named("every", every))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := q.HomeRows(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	var layout []users.HomeRow
	for _, r := range rows {
		i := slices.IndexFunc(out, func(l smartList) bool { return l.id == r.List.ID })
		layout = append(layout, users.HomeRow{ID: r.ID, Shown: r.Shown || i >= 0 && i < 5})
	}
	if err := svc.SetHomeRows(ctx, v, layout); err != nil {
		t.Fatal(err)
	}
	return v, out
}

// smartDefs are the smart lists the budgets time, over generate's works.
func smartDefs() []smartSample {
	var actor id.ID // in movies 123 and 20123
	copy(actor[:], fmt.Sprintf("p%015d", 123))
	movies := []catalog.WorkKind{catalog.KindMovie}
	series := []catalog.WorkKind{catalog.KindSeries}
	hundred := 100
	return []smartSample{
		{"movies, title and rating, by title", query.WorkQuery{Kinds: movies, Where: where(
			query.Rule{Field: "title", Op: "contains", Values: []string{"movie 1"}},
			query.Rule{Field: "rating", Op: "gte", Values: []string{"5"}})}},
		{"movies, one actor, by added", query.WorkQuery{Kinds: movies, Where: where(
			query.Rule{Field: "actor", Op: "anyOf", Values: []string{actor.String()}}),
			Sort: []query.SortKey{{Field: "addedAt"}}}},
		{"movies, unplayed, by last played", query.WorkQuery{Kinds: movies, Where: where(
			query.Rule{Field: "playState", Op: "anyOf", Values: []string{"unplayed"}}),
			Sort: []query.SortKey{{Field: "lastPlayedAt"}}}},
		{"series, by last episode added", query.WorkQuery{Kinds: series, Where: where(
			query.Rule{Field: "genre", Op: "anyOf", Values: []string{"Drama"}}),
			Sort: []query.SortKey{{Field: "lastEpisodeAddedAt"}}}},
		{"episodes, aired this year", query.WorkQuery{Kinds: []catalog.WorkKind{catalog.KindEpisode}, Where: where(
			query.Rule{Field: "releaseDate", Op: "within", Values: []string{"P1Y"}}),
			Sort: []query.SortKey{{Field: "releaseDate"}}}},
		{"movies, capped at 100", query.WorkQuery{Kinds: movies, Where: where(
			query.Rule{Field: "rating", Op: "gte", Values: []string{"6"}}),
			Sort: []query.SortKey{{Field: "addedAt"}}, MaxWorks: &hundred}},
	}
}
