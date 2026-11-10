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

// The read budgets, at 50k works while a scan is writing.
const (
	browseBudget = 50 * time.Millisecond
	homeBudget   = 100 * time.Millisecond
	searchBudget = 100 * time.Millisecond
)

// TestBudgets times each read against 50k generated works (40,000 movies and
// 400 series of 24 episodes). Meanwhile another goroutine writes batches of
// 500 files, shaped like a scan's. The test fails when a read's p95 is over
// its budget.
//
// It runs only when KIVO_BUDGET=1, which make budget sets, and runs alone
// without the race detector. Timings taken beside other packages' tests
// mean nothing, and skipping it keeps a plain go test ./... deterministic.
func TestBudgets(t *testing.T) {
	if os.Getenv("KIVO_BUDGET") != "1" {
		t.Skip("times reads; make budget runs it (KIVO_BUDGET=1)")
	}
	db := storetest.New(t)
	movies, shows, moviesFolder := generate(t, db)
	v := budgetViewer(t, db, "admin")
	q := query.New(db)
	sv, smart := smartViewer(t, db, q)

	stop := scanWriter(t, db, moviesFolder)
	defer stop()

	deep := query.GridOptions{Offset: 20000} // halfway through the movies
	type budgetCase struct {
		name    string
		budget  time.Duration
		enforce bool
		read    func() error
	}
	tests := []budgetCase{
		{"browse movies by title", browseBudget, true, browse(q, v, movies, query.BrowseOptions{})},
		{"browse movies by added", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Sort: query.ByAdded})},
		{"browse shows by title", browseBudget, true, browse(q, v, shows, query.BrowseOptions{})},
		{"browse movies by release", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Sort: query.ByRelease})},
		{"browse movies by rating", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Sort: query.ByCommunityRating})},
		{"browse shows by rating", browseBudget, true, browse(q, v, shows, query.BrowseOptions{Sort: query.ByCommunityRating})},
		{"browse movies by my rating", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Sort: query.ByMyRating})},
		{"browse movies by runtime", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Sort: query.ByRuntime})},
		{"browse movies by last played", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Sort: query.ByLastPlayed})},
		{"browse shows by last episode added", browseBudget, true, browse(q, v, shows, query.BrowseOptions{Sort: query.ByLastEpisodeAdded})},
		// A page reads every row its offset skips, and counts the grid for
		// its total. So a deep page's time is logged rather than held to a
		// budget.
		{"browse movies by title, deep", browseBudget, false, browse(q, v, movies, query.BrowseOptions{GridOptions: deep})},
		{"browse movies by rating, deep", browseBudget, false, browse(q, v, movies, query.BrowseOptions{Sort: query.ByCommunityRating, GridOptions: deep})},
		{"browse movies by my rating, deep", browseBudget, false, browse(q, v, movies, query.BrowseOptions{Sort: query.ByMyRating, GridOptions: deep})},
		{"browse movies by last played, deep", browseBudget, false, browse(q, v, movies, query.BrowseOptions{Sort: query.ByLastPlayed, GridOptions: deep})},
		// Filters on per-user state and genres are checked on every work to
		// count the matches. So this read's time is logged rather than held
		// to a budget.
		{"browse movies, genre and unplayed", browseBudget, false, browse(q, v, movies, query.BrowseOptions{Filter: query.Filter{Genres: []string{"Drama"}, Played: query.Unplayed}})},
		{"browse movies, 4K", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Filter: query.Filter{Resolutions: []query.Resolution{query.Resolution4K}}})},
		{"browse movies, HDR", browseBudget, true, browse(q, v, movies, query.BrowseOptions{Filter: query.Filter{Ranges: []query.Range{query.HDR}}})},
		{"home", homeBudget, true, func() error { _, _, err := q.Home(ctx, v); return err }},
		{"my list", browseBudget, true, func() error {
			lists, err := q.Lists(ctx, v)
			if err != nil {
				return err
			}
			_, err = q.ListWorks(ctx, v, lists[0].ID, nil, query.GridOptions{})
			return err
		}},
		{"search", searchBudget, true, func() error { _, err := q.Search(ctx, v, "movie 123", 0); return err }},
		{"search, a person", searchBudget, true, func() error { _, err := q.Search(ctx, v, "mary actor 123", 0); return err }},
		// Every movie and every person has "the" and a word starting with
		// "m", so this query has the most matches a query can have.
		{"search, a word in every title", searchBudget, true, func() error { _, err := q.Search(ctx, v, "the", 0); return err }},
		{"search, one letter", searchBudget, true, func() error { _, err := q.Search(ctx, v, "m", 0); return err }},
	}
	// Smart lists are measured pending a decision on how they read, so none
	// is held to a budget yet. A list's page reads its works, which counts
	// them too, and its header reads the list with its count.
	for _, l := range smart {
		tests = append(tests,
			budgetCase{"smart " + l.name, browseBudget, false, func() error {
				_, err := q.ListWorks(ctx, sv, l.id, nil, query.GridOptions{})
				return err
			}},
			budgetCase{"smart " + l.name + ", count", browseBudget, false, func() error { _, err := q.List(ctx, sv, l.id); return err }})
	}
	tests = append(tests,
		budgetCase{"home, 5 smart rows", homeBudget, false, func() error { _, _, err := q.Home(ctx, sv); return err }},
		budgetCase{"lists screen, 10 lists", browseBudget, false, func() error { _, err := q.CountedLists(ctx, sv); return err }})
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
		held := ""
		if !tt.enforce {
			held = ", not held"
		}
		t.Logf("%-50s p50 %6.1f ms  p95 %6.1f ms (budget %v%s)", tt.name, ms(p50), ms(p95), tt.budget, held)
		if tt.enforce && p95 > tt.budget {
			t.Errorf("%s: p95 %v over its budget of %v", tt.name, p95, tt.budget)
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

// generate fills db with 40,000 movies and 400 series of 2 seasons of 12
// episodes, in two libraries. Each movie and episode has a file, a version
// and a video track. It adds 20,000 people credited on the movies, and
// indexes everything for search. It writes the tables directly, which only
// a test may do, because building 50k works through catalog one at a time
// would take minutes.
func generate(t *testing.T, db *store.DB) (movies, shows, moviesFolder id.ID) {
	t.Helper()
	cat := catalog.New(db, events.NewBus())
	var showsFolder id.ID
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
		now := time.Now().UnixMilli()
		for _, stmt := range []string{
			// 40,000 movies. Each ID is a letter and a number, padded to 16
			// bytes.
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 40000)
			 INSERT INTO work (id, kind, root_id, title, sort_title, year, content_rating_age, community_rating, community_votes, added_at)
			 SELECT CAST(printf('m%015d', i) AS BLOB), 'movie', CAST(printf('m%015d', i) AS BLOB),
			   printf('The Movie %d', i), printf('Movie %d', i), 1950 + i % 75, i % 19,
			   CASE WHEN i % 10 THEN i % 91 / 10.0 + 1 END, CASE WHEN i % 10 THEN i * 37 % 20000 + 1 END, @now - i * 1000 FROM n`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 400)
			 INSERT INTO work (id, kind, root_id, title, sort_title, year, content_rating_age, community_rating, community_votes, added_at)
			 SELECT CAST(printf('s%015d', i) AS BLOB), 'series', CAST(printf('s%015d', i) AS BLOB),
			   printf('Show %d', i), printf('Show %d', i), 1990 + i % 35, i % 19,
			   CASE WHEN i % 10 THEN i % 91 / 10.0 + 1 END, CASE WHEN i % 10 THEN i * 37 % 20000 + 1 END, @now - i * 1000 FROM n`,
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 800)
			 INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, number, added_at)
			 SELECT CAST(printf('z%015d', i) AS BLOB), 'season', CAST(printf('s%015d', (i + 1) / 2) AS BLOB),
			   CAST(printf('s%015d', (i + 1) / 2) AS BLOB), printf('Season %d', 2 - i % 2), printf('Season %d', 2 - i % 2),
			   2 - i % 2, @now FROM n`,
			// Episodes aired one every 9 hours over the last ten years.
			`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 9600)
			 INSERT INTO work (id, kind, parent_id, root_id, title, sort_title, parent_number, number, release_date, added_at)
			 SELECT CAST(printf('e%015d', i) AS BLOB), 'episode', CAST(printf('z%015d', (i + 11) / 12) AS BLOB),
			   CAST(printf('s%015d', (i + 23) / 24) AS BLOB), printf('Episode %d', i), printf('Episode %d', i),
			   2 - ((i + 11) / 12) % 2, (i - 1) % 12 + 1, date(@now / 1000 - i * 32400, 'unixepoch'), @now - i * 100 FROM n`,
			// A file, a version and a video track for each movie and episode.
			`INSERT INTO media_file (id, folder_id, rel_path, size, mtime, status, duration_ms)
			 SELECT CAST('f' || substr(CAST(id AS TEXT), 1, 1) || substr(CAST(id AS TEXT), 3) AS BLOB), CASE kind WHEN 'movie' THEN @movies_folder ELSE @shows_folder END,
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
			 SELECT CASE WHEN w.kind = 'movie' THEN @movies ELSE @shows END, w.id, w.parent_id IS NULL, w.sort_title, w.added_at,
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
					sql.Named("movies_folder", moviesFolder), sql.Named("shows_folder", showsFolder)}
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

// budgetViewer is an admin who has watched 2,000 movies, has 300 in progress
// and 50 movies on My List, and is partway through 100 series.
func budgetViewer(t *testing.T, db *store.DB, name string) users.Viewer {
	t.Helper()
	svc := users.New(db, events.NewBus())
	var user id.ID
	err := db.Write(context.Background(), func(ctx context.Context, tx *store.Tx) error {
		u, err := svc.CreateUser(ctx, tx, users.NewUser{Username: name, Admin: true, Grants: users.Grants{All: true}})
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

// smartList is a smart list the budgets time.
type smartList struct {
	name string
	id   id.ID
}

// smartViewer is a second budgetViewer with smart lists, the first five of
// them shown at the end of Home.
func smartViewer(t *testing.T, db *store.DB, q *query.Service) (users.Viewer, []smartList) {
	t.Helper()
	v := budgetViewer(t, db, "smart")
	var actor id.ID // in movies 123 and 20123
	copy(actor[:], fmt.Sprintf("p%015d", 123))
	movies := []catalog.WorkKind{catalog.KindMovie}
	series := []catalog.WorkKind{catalog.KindSeries}
	hundred := 100
	defs := []struct {
		name string
		q    query.WorkQuery
	}{
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
	svc := users.New(db, events.NewBus())
	var out []smartList
	for _, d := range defs {
		saved, err := d.q.Saved()
		if err != nil {
			t.Fatal(err)
		}
		list, err := svc.CreateList(ctx, v, d.name, &saved)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, smartList{d.name, list})
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
