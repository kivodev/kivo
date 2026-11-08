package identify_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/identify/parse"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

var ctx = context.Background()

// listing is one entry of the fake provider's catalog.
type listing struct {
	kind  string
	id    string
	name  string
	year  int
	alts  []string
	other []int                // other release years
	also  []catalog.ExternalID // IMDb and TVDB IDs that find it
}

// world is what the fake provider knows, most popular first within a kind.
var world = []listing{
	{kind: "movie", id: "438631", name: "Dune", year: 2021, alts: []string{"Dune: Part One"}, other: []int{2020},
		also: []catalog.ExternalID{{Provider: "imdb", Kind: "movie", ID: "tt1160419"}}},
	{kind: "movie", id: "841", name: "Dune", year: 1984},
	{kind: "movie", id: "78", name: "Blade Runner", year: 1982},
	{kind: "movie", id: "24", name: "Kill Bill: Vol. 1", year: 2003},
	{kind: "movie", id: "603", name: "The Matrix", year: 1999},
	{kind: "movie", id: "949", name: "Heat", year: 1995},
	{kind: "movie", id: "60935", name: "The Thing", year: 2011},
	{kind: "movie", id: "1091", name: "The Thing", year: 1982},
	{kind: "movie", id: "12429", name: "Ponyo", year: 2008, alts: []string{"Gake no Ue no Ponyo"}},
	{kind: "series", id: "63639", name: "The Expanse", year: 2015},
	{kind: "series", id: "1437", name: "Firefly", year: 2002, also: []catalog.ExternalID{{Provider: "tvdb", Kind: "series", ID: "78874"}}},
	{kind: "series", id: "2224", name: "The Daily Show", year: 1996},
	{kind: "series", id: "37854", name: "One Piece", year: 1999, also: []catalog.ExternalID{{Provider: "tvdb", Kind: "series", ID: "81797"}}},
	{kind: "series", id: "57243", name: "Doctor Who", year: 2005},
	{kind: "series", id: "121", name: "Doctor Who", year: 1963},
}

// episodes are the fake provider's episode lists in aired order.
var episodes = map[string][]metadata.Episode{
	"63639": eps("63639", "1x1 Dulcinea 2015-12-14", "1x2 The Big Empty 2015-12-15", "1x3 Remember the Cant 2015-12-22",
		"2x1 Safe 2017-02-01", "2x2 Doors & Corners 2017-02-01", "0x1 Pinball 2016-01-01"),
	"1437": eps("1437", "1x1 The Train Job 2002-09-20", "1x2 Bushwhacked 2002-09-27", "1x3 Our Mrs. Reynolds 2002-10-04",
		"1x4 Serenity 2002-12-20"),
	"2224": eps("2224", "2024x1 Jon Stewart 2024-02-12", "2024x2 Jordan Klepper 2024-02-13"),
	"37854": eps("37854", "1x1 Romance Dawn 1999-10-20", "1x2 Enter the Great Swordsman 1999-11-17", "1x3 Morgan versus Luffy 1999-11-24",
		"2x1 The Great Pirate 2001-01-01", "2x2 Buggy 2001-01-08"),
}

// dvd is Firefly in DVD order, with the pilot, "Serenity", first.
var dvd = map[string][]metadata.Episode{
	"1437": {
		{ID: catalog.ExternalID{Provider: "tmdb", Kind: "episode", ID: "1437-1-4"}, Season: 1, Number: 1, Title: "Serenity"},
		{ID: catalog.ExternalID{Provider: "tmdb", Kind: "episode", ID: "1437-1-1"}, Season: 1, Number: 2, Title: "The Train Job"},
		{ID: catalog.ExternalID{Provider: "tmdb", Kind: "episode", ID: "1437-1-2"}, Season: 1, Number: 3, Title: "Bushwhacked"},
		{ID: catalog.ExternalID{Provider: "tmdb", Kind: "episode", ID: "1437-1-3"}, Season: 1, Number: 4, Title: "Our Mrs. Reynolds"},
	},
}

// eps builds an episode list from "SxN Title YYYY-MM-DD" lines. IDs have
// the form series-season-number, and are stable across orderings.
func eps(series string, lines ...string) []metadata.Episode {
	var out []metadata.Episode
	for _, l := range lines {
		sn, rest, _ := strings.Cut(l, " ")
		s, n, _ := strings.Cut(sn, "x")
		name, date := rest[:len(rest)-11], rest[len(rest)-10:]
		si, _ := strconv.Atoi(s)
		ni, _ := strconv.Atoi(n)
		out = append(out, metadata.Episode{
			ID:     catalog.ExternalID{Provider: "tmdb", Kind: "episode", ID: series + "-" + s + "-" + n},
			Season: si, Number: ni, Title: name, AirDate: date,
		})
	}
	return out
}

// fakeVideo is the video provider that metadata uses in identify's tests.
// It answers from world, without the fetch pipeline. Search returns the
// listings of the kind that share a word with the query, in popularity order,
// filtered by year the way TMDb filters it.
type fakeVideo struct {
	mu       sync.Mutex
	searches []string // each search, including a book's
	episodes []string // series/ordering of each episode-list fetch
	down     bool     // searches and lookups meet an outage
	hook     func()   // runs at each search, outside the lock, to make a change while identify waits on the provider
}

func (f *fakeVideo) Name() string { return "tmdb" }

func (f *fakeVideo) Search(_ context.Context, kind catalog.WorkKind, q metadata.Query, _ string) ([]metadata.Match, error) {
	if f.hook != nil {
		f.hook()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, metadata.ErrUnavailable
	}
	f.searches = append(f.searches, fmt.Sprintf("%s %q %d", kind, q.Title, q.Year))
	words := strings.Fields(parse.Key(q.Title))
	var out []metadata.Match
	for _, t := range world {
		if t.kind != string(kind) || q.Year != 0 && t.year != q.Year {
			continue
		}
		if slices.ContainsFunc(strings.Fields(parse.Key(t.name)), func(w string) bool { return slices.Contains(words, w) }) {
			out = append(out, metadata.Match{ID: catalog.ExternalID{Provider: "tmdb", Kind: t.kind, ID: t.id}, Title: t.name, Year: t.year})
		}
	}
	return out, nil
}

func (f *fakeVideo) Resolve(_ context.Context, x catalog.ExternalID, _ string) (metadata.Match, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return metadata.Match{}, metadata.ErrUnavailable
	}
	for _, t := range world {
		if t.kind == x.Kind && (x.Provider == "tmdb" && t.id == x.ID || slices.Contains(t.also, x)) {
			return metadata.Match{
				ID: catalog.ExternalID{Provider: "tmdb", Kind: t.kind, ID: t.id}, Title: t.name, Year: t.year,
				AltTitles: t.alts, OtherYears: t.other,
			}, nil
		}
	}
	return metadata.Match{}, metadata.ErrNotFound
}

// Describe knows nothing, because identify only asks for refreshes, and the
// tests don't run them.
func (f *fakeVideo) Describe(context.Context, metadata.Ref, string) (*metadata.Description, error) {
	return nil, metadata.ErrNotFound
}

func (f *fakeVideo) Images(context.Context, metadata.Ref, string) (metadata.Images, error) {
	return metadata.Images{}, metadata.ErrNotFound
}

func (f *fakeVideo) Episodes(_ context.Context, series catalog.ExternalID, o metadata.Ordering, _ string, _ time.Duration) ([]metadata.Episode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.episodes = append(f.episodes, series.ID+"/"+cmp.Or(string(o), "aired"))
	switch o {
	case metadata.DVD:
		if l, ok := dvd[series.ID]; ok {
			return l, nil
		}
		return nil, metadata.ErrNotFound
	case metadata.Absolute:
		var out []metadata.Episode
		for _, e := range episodes[series.ID] {
			if e.Season > 0 {
				e.Season, e.Number = 1, len(out)+1
				out = append(out, e)
			}
		}
		return out, nil
	}
	return episodes[series.ID], nil
}

// EpisodeGroups lists the episode groups the fake provider knows. There are
// none.
func (f *fakeVideo) EpisodeGroups(context.Context, catalog.ExternalID, string) ([]string, error) {
	return nil, metadata.ErrNotFound
}

func (f *fakeVideo) Owns(string) bool { return false }

func (f *fakeVideo) Preview(url, _ string) string { return url }

func (f *fakeVideo) searchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.searches)
}

// asins is what the fake book provider knows, by ASIN.
var asins = map[string]string{"B0058ZZZ5A": "Leviathan Wakes", "B005VR5GRQ": "Caliban's War"}

// fakeBooks is the book provider. It answers from asins, and records its
// searches along with the video provider's.
type fakeBooks struct{ video *fakeVideo }

func (f fakeBooks) Name() string { return catalog.ASIN.Provider }

func (f fakeBooks) Search(_ context.Context, _ string, q metadata.BookQuery) ([]metadata.Book, error) {
	f.video.mu.Lock()
	f.video.searches = append(f.video.searches, "book:"+q.Title+"/"+q.Author)
	f.video.mu.Unlock()
	var out []metadata.Book
	for _, asin := range slices.Sorted(maps.Keys(asins)) {
		if strings.Contains(asins[asin], q.Title) {
			out = append(out, metadata.Book{ID: asin, Title: asins[asin]})
		}
	}
	return out, nil
}

func (f fakeBooks) Book(_ context.Context, _, asin string) (*metadata.Book, error) {
	if title, ok := asins[asin]; ok {
		return &metadata.Book{ID: asin, Title: title}, nil
	}
	return nil, metadata.ErrNotFound
}

func (f fakeBooks) Author(context.Context, string, string) (*metadata.Author, error) {
	return nil, metadata.ErrNotFound
}

// fakeAnime is the AniDB mapping.
type fakeAnime map[string]metadata.AniDB

func (f fakeAnime) AniDB(_ context.Context, anidb string) (metadata.AniDB, error) {
	if a, ok := f[anidb]; ok {
		return a, nil
	}
	return metadata.AniDB{}, metadata.ErrNotFound
}

// anime is what the fake mapping knows, by AniDB ID.
var anime = fakeAnime{
	"69":   {TVDB: "81797", Absolute: true},
	"5975": {TMDbMovie: "12429", Season: 1},
}

// env is an identifier over a fresh database, with metadata running over
// the fake providers.
type env struct {
	t        *testing.T
	b        *catalogtest.Builder
	video    *fakeVideo
	meta     *metadata.Service
	state    *userstate.Service
	activity *activity.Service
	id       *identify.Identifier
	disk     map[string]fstest.MapFS // folder path → NFO files on disk
	// offline makes every library folder unreachable.
	offline atomic.Bool
	n       int
	// placed holds the episodes passed to each call of markers' Placed, in
	// order.
	placed [][]id.ID
	// rescans holds the libraries a scan was asked for, in order. onRescan
	// runs as each scan is asked for. refreshes holds the works a metadata
	// refresh was asked for.
	mu        sync.Mutex
	rescans   []id.ID
	onRescan  func(lib id.ID)
	refreshes []id.ID
}

func newEnv(t *testing.T) *env {
	t.Helper()
	b := catalogtest.New(t)
	settings, err := config.Open(ctx, b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, b: b, video: &fakeVideo{}, disk: map[string]fstest.MapFS{}}
	e.meta = metadata.New(b.DB, metadata.Deps{Catalog: b.Catalog, Settings: settings, Events: events.NewBus(), Tasks: e, CacheDir: t.TempDir(), ImageDir: t.TempDir()},
		metadata.Options{Video: e.video, Books: fakeBooks{e.video}, Anime: anime})
	e.activity = activity.New(b.DB, events.NewBus())
	e.state = userstate.New(b.DB, userstate.Deps{Catalog: b.Catalog, Settings: settings, Activity: e.activity, Events: events.NewBus()})
	e.id = identify.New(b.DB, identify.Deps{Catalog: b.Catalog, Metadata: e.meta, Userstate: e.state, Activity: e.activity, Tasks: e, Markers: e}, identify.Options{
		OpenFolder: func(path string) (fs.FS, func() error, error) {
			if e.offline.Load() {
				return nil, nil, errors.New("offline")
			}
			return e.disk[path], func() error { return nil }, nil
		},
	})
	return e
}

// Placed records the episodes placed. The env serves as identify's
// Markers.
func (e *env) Placed(_ context.Context, _ *store.Tx, _ catalog.Library, episodes []id.ID) error {
	e.placed = append(e.placed, episodes)
	return nil
}

// Ask records the libraries a scan was asked for, and the works a refresh
// was asked for. The env serves as the Tasks of both identify and
// metadata.
func (e *env) Ask(task string, items ...id.ID) {
	switch task {
	case scan.TaskScanLibraries:
	case metadata.TaskRefreshMetadata:
		e.mu.Lock()
		e.refreshes = append(e.refreshes, items...)
		e.mu.Unlock()
		return
	default:
		e.t.Errorf("asked for %s", task)
	}
	for _, lib := range items {
		if e.onRescan != nil {
			e.onRescan(lib)
		}
		e.mu.Lock()
		e.rescans = append(e.rescans, lib)
		e.mu.Unlock()
	}
}

// takeRescans returns the libraries a scan was asked for since the last
// take.
func (e *env) takeRescans() []id.ID {
	e.mu.Lock()
	defer e.mu.Unlock()
	libs := e.rescans
	e.rescans = nil
	return libs
}

// takeRefreshes returns the works a refresh was asked for since the last
// take.
func (e *env) takeRefreshes() []id.ID {
	e.mu.Lock()
	defer e.mu.Unlock()
	works := e.refreshes
	e.refreshes = nil
	return works
}

// refreshed reports whether a work is due a metadata refresh and the
// refresh was asked for since the last take.
func (e *env) refreshed(work id.ID) bool {
	e.t.Helper()
	return e.due(work) && slices.Contains(e.takeRefreshes(), work)
}

// due reports whether a work is due a metadata refresh.
func (e *env) due(work id.ID) bool {
	e.t.Helper()
	var due bool
	if err := e.b.DB.Reader().QueryRow(`SELECT refreshed_at IS NULL FROM work WHERE id = ?`, work).Scan(&due); err != nil {
		e.t.Fatal(err)
	}
	return due
}

// refreshedAt records works as refreshed, as the refresh task does, so a
// refresh asked for later makes them due again.
func (e *env) refreshedAt(works ...id.ID) {
	e.t.Helper()
	for _, w := range works {
		e.exec(`UPDATE work SET refreshed_at = 1 WHERE id = ?`, w)
	}
}

// poster gives a work a poster, as a refresh would.
func (e *env) poster(work id.ID) {
	e.t.Helper()
	e.exec(`INSERT INTO image (id, work_id, kind, source, origin, width, height, blurhash, fetched_at)
		VALUES (?, ?, 'poster', 'provider', 'https://image.tmdb.org/t/p/original/poster.jpg', 300, 450, '', 1)`, id.New(), work)
}

// artwork counts a work's artwork.
func (e *env) artwork(work id.ID) int {
	e.t.Helper()
	var n int
	if err := e.b.DB.Reader().QueryRow(`SELECT count(*) FROM image WHERE work_id = ?`, work).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// library adds a library of kind with one folder.
func (e *env) library(kind catalog.LibraryKind) catalog.Library {
	e.t.Helper()
	lib, _ := e.b.Library(kind)
	l, err := e.b.Catalog.Library(ctx, lib)
	if err != nil {
		e.t.Fatal(err)
	}
	e.disk[l.Folders[0].Path] = fstest.MapFS{}
	return l
}

// add records files as a scan would. A rel may end in "@size" to set the
// file's size in bytes. The default size is 1 GiB.
func (e *env) add(lib catalog.Library, rels ...string) []id.ID {
	e.t.Helper()
	var out []id.ID
	for _, rel := range rels {
		size := int64(1 << 30)
		if r, s, ok := strings.Cut(rel, "@"); ok {
			rel = r
			size, _ = strconv.ParseInt(s, 10, 64)
		}
		e.b.Write(func(ctx context.Context, tx *store.Tx) error {
			e.n++
			st, err := e.b.Catalog.UpsertFiles(ctx, tx, lib.Folders[0].ID, []catalog.FileObservation{{RelPath: rel, Size: size, MTime: time.UnixMilli(int64(e.n))}})
			if err == nil {
				out = append(out, st[0].ID)
			}
			return err
		})
	}
	return out
}

// move moves a file to rel in lib, as a scan's move detection finds it.
// The file keeps its ID and version at its new path, and is identified
// again.
func (e *env) move(lib catalog.Library, file id.ID, rel string) {
	e.t.Helper()
	found := e.add(lib, rel)[0]
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		if err := e.b.MarkGone(ctx, tx, file); err != nil {
			return err
		}
		if err := e.b.Catalog.Relink(ctx, tx, file, found); err != nil {
			return err
		}
		return e.id.Reidentify(ctx, tx, []id.ID{file})
	})
}

// nfo puts an NFO file in the library's folder.
func (e *env) nfo(lib catalog.Library, rel, body string) {
	e.disk[lib.Folders[0].Path][rel] = &fstest.MapFile{Data: []byte(body)}
}

// run probes each file at its generation and identifies its library. It
// handles one file at a time, in order, as a scan does for a file that is
// new, changed or sent back to be identified.
func (e *env) run(files ...id.ID) {
	e.t.Helper()
	if err := e.runCtx(ctx, files...); err != nil {
		e.t.Fatalf("identify %v: %v", files, err)
	}
}

// again sends files' folders back to be identified, as a changed NFO, an
// admin's request or a full walk does, and then runs them.
func (e *env) again(files ...id.ID) {
	e.t.Helper()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error { return e.id.Reidentify(ctx, tx, files) })
	e.run(files...)
}

func (e *env) runCtx(ctx context.Context, files ...id.ID) error {
	e.t.Helper()
	for _, f := range files {
		e.exec(`UPDATE media_file SET probed = generation, probe_error = NULL WHERE id = ?`, f)
		file, err := e.b.Catalog.File(ctx, f)
		if err != nil {
			return err
		}
		if err := e.identify(ctx, file.Library); err != nil {
			return err
		}
	}
	return nil
}

// identify identifies a library, as the library's scan does when it ends.
func (e *env) identify(ctx context.Context, lib id.ID) error {
	e.t.Helper()
	l, err := e.b.Catalog.Library(ctx, lib)
	if err != nil {
		return err
	}
	return e.id.Identify(ctx, l)
}

// exec runs a statement in a write, for setting up rows as scan would.
func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
}

// pending lists the files that were probed but not identified at their
// generation. The next scan identifies them.
func (e *env) pending() []id.ID {
	e.t.Helper()
	rows, err := e.b.DB.Reader().Query(`SELECT id FROM media_file WHERE status = 'present' AND probed = generation
		AND probe_error IS NULL AND identified IS NOT generation ORDER BY id`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []id.ID
	for rows.Next() {
		var f id.ID
		if err := rows.Scan(&f); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

// workOf returns the works a file's version covers.
func (e *env) workOf(file id.ID) []catalog.Work {
	e.t.Helper()
	var v id.ID
	if err := e.b.DB.Reader().QueryRow(`SELECT version_id FROM media_file WHERE id = ? AND version_id IS NOT NULL`, file).Scan(&v); err != nil {
		e.t.Fatalf("version of %v: %v", file, err)
	}
	ver, err := e.b.Catalog.Version(ctx, v)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []catalog.Work
	for _, w := range ver.Works {
		got, err := e.b.Catalog.Work(ctx, w)
		if err != nil {
			e.t.Fatal(err)
		}
		out = append(out, got)
	}
	return out
}

func (e *env) versionOf(file id.ID) id.ID {
	e.t.Helper()
	var v id.ID
	if err := e.b.DB.Reader().QueryRow(`SELECT version_id FROM media_file WHERE id = ? AND version_id IS NOT NULL`, file).Scan(&v); err != nil {
		e.t.Fatalf("version of %v: %v", file, err)
	}
	return v
}

// fix runs an admin's Fix match on the first work a version covers.
func (e *env) fix(version id.ID, c metadata.Match) error {
	v, err := e.b.Catalog.Version(ctx, version)
	if err != nil {
		return err
	}
	_, err = e.id.FixMatch(ctx, e.viewer(), v.Works[0], c.ID)
	return err
}

// viewer returns the Viewer of a new user who can see every library.
func (e *env) viewer() users.Viewer {
	e.t.Helper()
	e.n++
	return e.viewerOf(e.newUser(fmt.Sprintf("viewer%d", e.n)))
}

// viewerOf returns a user's Viewer.
func (e *env) viewerOf(user id.ID) users.Viewer {
	e.t.Helper()
	v, err := users.New(e.b.DB, nil).Viewer(ctx, user)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func (e *env) newUser(name string) id.ID {
	u := users.New(e.b.DB, nil)
	var p id.ID
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		user, err := u.CreateUser(ctx, tx, users.NewUser{Username: name, Grants: users.Grants{All: true}})
		p = user.ID
		return err
	})
	return p
}

// dump prints every work as a tree: roots by title, children by season and
// episode, each with its external IDs, flags and versions.
func (e *env) dump() string {
	e.t.Helper()
	type work struct {
		id, parent                       id.ID
		kind, title, edition, extra, ext string
		collection                       string
		year, season, episode            int
		provisional, locked              bool
		versions                         []string
	}
	rows, err := e.b.DB.Reader().Query(`SELECT id, kind, parent_id, title, coalesce(year, 0), coalesce(CASE kind WHEN 'season' THEN number ELSE parent_number END, -1),
		coalesce(CASE kind WHEN 'episode' THEN number END, 0),
		coalesce(edition, ''), coalesce(extra_type, ''), provisional, locked FROM work`)
	if err != nil {
		e.t.Fatal(err)
	}
	var all []*work
	for rows.Next() {
		w := &work{}
		var parent *id.ID
		if err := rows.Scan(&w.id, &w.kind, &parent, &w.title, &w.year, &w.season, &w.episode, &w.edition, &w.extra, &w.provisional, &w.locked); err != nil {
			e.t.Fatal(err)
		}
		if parent != nil {
			w.parent = *parent
		}
		all = append(all, w)
	}
	rows.Close()
	for _, w := range all {
		xs, err := e.b.Catalog.Work(ctx, w.id)
		if err != nil {
			e.t.Fatal(err)
		}
		var ext []string
		for _, x := range xs.ExternalIDs {
			ext = append(ext, x.Provider+":"+x.ID)
		}
		w.ext = strings.Join(ext, ",")
		if err := e.b.DB.Reader().QueryRow(`SELECT coalesce(group_concat(c.name, ', '), '') FROM collection_work cw
			JOIN collection c ON c.id = cw.collection_id WHERE cw.work_id = ?`, w.id).Scan(&w.collection); err != nil {
			e.t.Fatal(err)
		}
		vr, err := e.b.DB.Reader().Query(`SELECT vw.version_id, (SELECT count(*) FROM media_version_work x WHERE x.version_id = vw.version_id),
			(SELECT group_concat(f.rel_path, ' + ') FROM (SELECT f.rel_path FROM media_file f
			 WHERE f.version_id = vw.version_id ORDER BY f.part) f),
			(SELECT coalesce(v.edition, '') FROM media_version v WHERE v.id = vw.version_id)
			FROM media_version_work vw WHERE vw.work_id = ?`, w.id)
		if err != nil {
			e.t.Fatal(err)
		}
		for vr.Next() {
			var v id.ID
			var n int
			var parts, edition string
			if err := vr.Scan(&v, &n, &parts, &edition); err != nil {
				e.t.Fatal(err)
			}
			if n > 1 {
				parts += fmt.Sprintf(" (covers %d)", n)
			}
			if edition != "" {
				parts += fmt.Sprintf(" edition=%q", edition)
			}
			w.versions = append(w.versions, parts)
		}
		vr.Close()
		slices.Sort(w.versions)
	}
	order := func(a, b *work) int {
		return cmp.Or(cmp.Compare(boolInt(a.kind == "extra"), boolInt(b.kind == "extra")), cmp.Compare(a.season, b.season), cmp.Compare(a.episode, b.episode),
			cmp.Compare(a.title, b.title), cmp.Compare(a.year, b.year), cmp.Compare(a.edition, b.edition), cmp.Compare(a.ext, b.ext),
			slices.Compare(a.versions, b.versions))
	}
	var b strings.Builder
	var print func(w *work, depth int)
	print = func(w *work, depth int) {
		ind := strings.Repeat("  ", depth)
		fmt.Fprintf(&b, "%s%s %q", ind, w.kind, w.title)
		if w.year != 0 {
			fmt.Fprintf(&b, " %d", w.year)
		}
		if w.season >= 0 {
			fmt.Fprintf(&b, " S%02d", w.season)
		}
		if w.episode != 0 {
			fmt.Fprintf(&b, "E%02d", w.episode)
		}
		if w.extra != "" {
			fmt.Fprintf(&b, " [%s]", w.extra)
		}
		if w.edition != "" {
			fmt.Fprintf(&b, " edition=%q", w.edition)
		}
		if w.ext != "" {
			fmt.Fprintf(&b, " {%s}", w.ext)
		}
		if w.collection != "" {
			fmt.Fprintf(&b, " in %q", w.collection)
		}
		if w.provisional {
			b.WriteString(" provisional")
		}
		if w.locked {
			b.WriteString(" locked")
		}
		b.WriteString("\n")
		for _, v := range w.versions {
			fmt.Fprintf(&b, "%s  = %s\n", ind, v)
		}
		var kids []*work
		for _, k := range all {
			if k.parent == w.id {
				kids = append(kids, k)
			}
		}
		slices.SortFunc(kids, order)
		for _, k := range kids {
			print(k, depth+1)
		}
	}
	var roots []*work
	for _, w := range all {
		if w.parent.IsZero() {
			roots = append(roots, w)
		}
	}
	slices.SortFunc(roots, order)
	for _, w := range roots {
		print(w, 0)
	}
	return b.String()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
