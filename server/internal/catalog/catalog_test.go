package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/store/storetest"
	"github.com/mmcdole/kivo/internal/tasks"
)

var ctx = context.Background()

// libraries returns the libraries that work_library lists w in.
func libraries(t *testing.T, b *catalogtest.Builder, w id.ID) []id.ID {
	t.Helper()
	rows, err := b.DB.Reader().Query(`SELECT library_id FROM work_library WHERE work_id = ? ORDER BY library_id`, w)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []id.ID
	for rows.Next() {
		var l id.ID
		if err := rows.Scan(&l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

func write(t *testing.T, b *catalogtest.Builder, fn func(ctx context.Context, tx *store.Tx) error) error {
	t.Helper()
	return b.DB.Write(ctx, fn)
}

// matchRev reads a work's match revision. identify checks its decisions
// against it.
func matchRev(t *testing.T, b *catalogtest.Builder, w id.ID) int {
	t.Helper()
	return count(t, b, `SELECT match_rev FROM work WHERE id = ?`, w)
}

func count(t *testing.T, b *catalogtest.Builder, query string, args ...any) int {
	t.Helper()
	var n int
	if err := b.DB.Reader().QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// libraryChanges returns the work of the catalog's task.
func libraryChanges(b *catalogtest.Builder) tasks.Items {
	for _, t := range b.Catalog.Tasks() {
		if t.Name == catalog.TaskLibraryChanges {
			return t.Work.(tasks.Items)
		}
	}
	panic("no task " + catalog.TaskLibraryChanges)
}

// remove runs one item of the catalog's task to completion, as the runner
// would.
func remove(t *testing.T, b *catalogtest.Builder, item id.ID) {
	t.Helper()
	if err := libraryChanges(b).Do(ctx, item); err != nil {
		t.Fatal(err)
	}
}

// removals returns the items the catalog's task has left.
func removals(t *testing.T, b *catalogtest.Builder) []id.ID {
	t.Helper()
	got, err := libraryChanges(b).Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestLibraryLifecycle(t *testing.T) {
	b := catalogtest.New(t)
	var lib id.ID
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{
			Name: "Movies", Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/a"), catalogtest.Abs("/b/")}, UseLocalMetadata: true,
		})
		lib = l.ID
		return err
	})
	l, err := b.Catalog.Library(ctx, lib)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Folders) != 2 || l.Folders[1].Path != catalogtest.Abs("/b") || l.Previews != catalog.Nightly || !l.UseLocalMetadata {
		t.Fatalf("library %+v", l)
	}

	// Dropping folder /b takes it off the folder list at once. Its removal
	// then deletes its files, and the movie leaves that library.
	m := b.Movie(l.Folders[1].ID, "Dune", 2021)
	var added, removed []catalog.Folder
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		added, removed, err = b.Catalog.UpdateLibrary(ctx, tx, lib, catalog.LibraryChange{Name: new("Films"), Folders: &[]string{catalogtest.Abs("/a"), catalogtest.Abs("/c")}})
		return err
	})
	if len(added) != 1 || added[0].Path != catalogtest.Abs("/c") || len(removed) != 1 || removed[0].Path != catalogtest.Abs("/b") {
		t.Fatalf("added folders %+v, removed %+v", added, removed)
	}
	l, _ = b.Catalog.Library(ctx, lib)
	if l.Name != "Films" || len(l.Folders) != 2 || l.Folders[1].Path != catalogtest.Abs("/c") {
		t.Fatalf("library %+v", l)
	}
	if _, err := b.Catalog.Folder(ctx, removed[0].ID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("folder being removed, for a scan: %v", err)
	}
	if got := removals(t, b); !slices.Equal(got, []id.ID{removed[0].ID}) {
		t.Fatalf("removals %v; want the dropped folder", got)
	}
	remove(t, b, removed[0].ID)
	if got := libraries(t, b, m.Work); len(got) != 0 {
		t.Fatalf("movie still in %v", got)
	}
	if n := count(t, b, `SELECT count(*) FROM library_folder WHERE id = ?`, removed[0].ID); n != 0 {
		t.Fatal("the removed folder's row is still there")
	}

	// Deleting the library takes it off the list at once. The deletion then
	// removes every folder's files and deletes the library. The work stays.
	n := b.Movie(l.Folders[0].ID, "Heat", 1995)
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.MarkDeleting(ctx, tx, lib) })
	if _, err := b.Catalog.Library(ctx, lib); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("library being deleted: %v", err)
	}
	if libs, _ := b.Catalog.Libraries(ctx); len(libs) != 0 {
		t.Fatalf("libraries %+v; want the one being deleted left out", libs)
	}
	if err := write(t, b, func(ctx context.Context, tx *store.Tx) error { return b.Catalog.MarkDeleting(ctx, tx, lib) }); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("deleting it again: %v", err)
	}
	remove(t, b, lib)
	if n := count(t, b, `SELECT count(*) FROM library WHERE id = ?`, lib); n != 0 {
		t.Fatal("the deleted library's row is still there")
	}
	if _, err := b.Catalog.Work(ctx, n.Work); err != nil {
		t.Fatalf("the work must stay for its watch state: %v", err)
	}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Movies", Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/a")}})
		lib = l.ID
		return err
	})

	err = write(t, b, func(ctx context.Context, tx *store.Tx) error {
		_, _, err := b.Catalog.UpdateLibrary(ctx, tx, lib, catalog.LibraryChange{Folders: &[]string{}})
		return err
	})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("an edit leaving no folder: %v", err)
	}
	for _, in := range []catalog.LibraryInput{
		{Name: "x", Kind: "music", Folders: []string{catalogtest.Abs("/a")}},
		{Name: "x", Kind: catalog.Movies},
		{Name: "x", Kind: catalog.Movies, Folders: []string{"relative"}},
	} {
		err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
			_, err := b.Catalog.CreateLibrary(ctx, tx, in)
			return err
		})
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%+v: %v", in, err)
		}
	}
}

func TestUpsertFilesBumpsGenerationOnContentChange(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	upsert := func(obs ...catalog.FileObservation) []catalog.FileState {
		var st []catalog.FileState
		b.Write(func(ctx context.Context, tx *store.Tx) error {
			var err error
			st, err = b.Catalog.UpsertFiles(ctx, tx, folder, obs)
			return err
		})
		return st
	}
	t0 := time.UnixMilli(1000)
	st := upsert(catalog.FileObservation{RelPath: "a.mkv", Size: 10, MTime: t0})
	if !st[0].Changed || st[0].Generation != 1 {
		t.Fatalf("new file %+v", st[0])
	}
	if st = upsert(catalog.FileObservation{RelPath: "a.mkv", Size: 10, MTime: t0}); st[0].Changed || st[0].Generation != 1 {
		t.Fatalf("unchanged file %+v", st[0])
	}
	st = upsert(catalog.FileObservation{RelPath: "a.mkv", Size: 11, MTime: t0})
	if !st[0].Changed || st[0].Generation != 2 {
		t.Fatalf("changed file %+v", st[0])
	}

	// A probe of the old generation is stale and changes nothing.
	err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.SetProbe(ctx, tx, st[0].ID, 1, catalog.Probe{Summary: catalog.ProbeSummary{Container: "mkv"}})
	})
	if !errors.Is(err, catalog.ErrStale) {
		t.Fatalf("stale probe: %v", err)
	}
	tracks := []catalog.Track{{Index: 0, Type: "video", Codec: "hevc", Default: true}, {Index: 1, Type: "audio", Codec: "eac3", Language: "en"}}
	chapters := []catalog.Chapter{{Index: 0, Start: 0, End: time.Minute, Title: "Intro"}}
	attachments := []catalog.Attachment{{Kind: "font", Filename: "a.ttf", MimeType: "font/ttf", Size: 10, SHA256: "ab12"}}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.SetProbe(ctx, tx, st[0].ID, 2, catalog.Probe{Summary: catalog.ProbeSummary{
			Container: "mkv", Duration: 2 * time.Hour, Bitrate: 8_000_000, EdgeHash: []byte{1, 2}, OSHash: 1 << 63,
		}, Tracks: tracks, Chapters: chapters, Attachments: attachments})
	})
	f, err := b.Catalog.File(ctx, st[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Probe.Container != "mkv" || f.Probe.Duration != 2*time.Hour || f.Probe.OSHash != 1<<63 || len(f.Tracks) != 2 ||
		f.Tracks[1].Language != "en" || len(f.Chapters) != 1 || f.Chapters[0].End != time.Minute || f.FolderPath == "" ||
		!slices.Equal(f.Attachments, attachments) || f.Tracks[0].ID.IsZero() || f.Tracks[0].ID == f.Tracks[1].ID {
		t.Fatalf("file %+v", f)
	}

	// A re-probe keeps the ID of each track found again with the same type
	// and codec at its stream index. Here that is the retagged audio track.
	// The video track's codec changed, so it's a new track.
	tracks = []catalog.Track{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "eac3", Language: "de"}}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.SetProbe(ctx, tx, st[0].ID, 2, catalog.Probe{Summary: catalog.ProbeSummary{Container: "mkv"}, Tracks: tracks})
	})
	again, err := b.Catalog.File(ctx, st[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Tracks) != 2 || again.Tracks[1].ID != f.Tracks[1].ID || again.Tracks[1].Language != "de" ||
		again.Tracks[0].ID == f.Tracks[0].ID || again.Tracks[0].Codec != "h264" || len(again.Attachments) != 0 {
		t.Fatalf("after a re-probe %+v, before %+v", again.Tracks, f.Tracks)
	}
}

func TestWorkLeavesLibraryWithItsLastVersion(t *testing.T) {
	b := catalogtest.New(t)
	lib, folder := b.Library(catalog.Shows)
	series := b.Series("The Expanse")
	e1 := b.Episode(folder, series, 1, 1)
	e2 := b.Episode(folder, series, 2, 1)
	season, _ := b.Catalog.Work(ctx, e2.Work)

	if got := libraries(t, b, series); !slices.Equal(got, []id.ID{lib}) {
		t.Fatalf("series in %v, want %v", got, lib)
	}
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.MarkGone(ctx, tx, e2.File) })
	if got := libraries(t, b, series); !slices.Equal(got, []id.ID{lib}) {
		t.Fatalf("series in %v after one episode went", got)
	}
	if got := libraries(t, b, season.Parent); len(got) != 0 {
		t.Fatalf("a season whose only episode went is still in %v", got)
	}
	// The file comes back at its path.
	f, _ := b.Catalog.File(ctx, e2.File)
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := b.Catalog.UpsertFiles(ctx, tx, folder, []catalog.FileObservation{{RelPath: f.RelPath, Size: f.Size, MTime: f.MTime}})
		return err
	})
	if got := libraries(t, b, season.Parent); !slices.Equal(got, []id.ID{lib}) {
		t.Fatalf("season in %v after the file returned", got)
	}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.DeleteFiles(ctx, tx, []id.ID{e1.File, e2.File})
	})
	if got := libraries(t, b, series); len(got) != 0 {
		t.Fatalf("series in %v with every file deleted", got)
	}
	if _, err := b.Catalog.Work(ctx, e1.Work); err != nil {
		t.Fatalf("a deleted file's work must stay for its watch state: %v", err)
	}
}

// A root work is in one library, the one its first version is in. A
// version in another library can't join it. The root moves only with all
// its versions.
func TestRootIsInOneLibrary(t *testing.T) {
	b := catalogtest.New(t)
	lib1, folder1 := b.Library(catalog.Movies)
	lib2, folder2 := b.Library(catalog.Movies)
	heat := b.Movie(folder1, "Heat", 1995)
	if got := libraries(t, b, heat.Work); !slices.Equal(got, []id.ID{lib1}) {
		t.Fatalf("Heat is in %v, want %v", got, lib1)
	}
	elsewhere := b.File(folder2, "Heat (1995)/Heat (1995) 4K.mkv")
	put := func(works ...id.ID) (id.ID, error) {
		var v id.ID
		err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
			return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
				var err error
				v, err = cb.PutVersion(ctx, catalog.VersionInput{Parts: []id.ID{elsewhere}, Works: works})
				return err
			})
		})
		return v, err
	}
	if _, err := put(heat.Work); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a version in another library joined the root: %v", err)
	}
	other := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Heat", Year: 1995})
	v, err := put(other)
	if err != nil {
		t.Fatal(err)
	}
	if got := libraries(t, b, other); !slices.Equal(got, []id.ID{lib2}) {
		t.Fatalf("the other root is in %v, want %v", got, lib2)
	}
	err = write(t, b, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
			_, err := cb.ReassignVersion(ctx, v, []id.ID{heat.Work})
			return err
		})
	})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a version reassigned to a root in another library: %v", err)
	}

	move := func(root, lib id.ID) error {
		return write(t, b, func(ctx context.Context, tx *store.Tx) error { return b.Catalog.MoveRoot(ctx, tx, root, lib) })
	}
	if err := move(heat.Work, lib2); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("a root moved away from its version: %v", err)
	}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.DeleteFiles(ctx, tx, []id.ID{elsewhere})
	})
	moved := b.File(folder2, "Heat (1995)/Heat (1995).mkv")
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		if err := b.MarkGone(ctx, tx, heat.File); err != nil {
			return err
		}
		return b.Catalog.Relink(ctx, tx, heat.File, moved)
	})
	if err := move(heat.Work, lib2); err != nil {
		t.Fatal(err)
	}
	if got := libraries(t, b, heat.Work); !slices.Equal(got, []id.ID{lib2}) {
		t.Fatalf("the moved root is in %v, want %v", got, lib2)
	}
}

// A work that gains a version after having none counts as newly added. That
// holds whether a file is grouped under it or a match points a version at
// it. A work that already has a version keeps its date.
func TestReturningWorkIsNewlyAdded(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	m := b.Movie(folder, "Heat", 1995)
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.DeleteFiles(ctx, tx, []id.ID{m.File}) })
	other := b.Movie(folder, "Ronin", 1998)
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work SET added_at = 1`)
		return err
	})
	added := func(w id.ID) int64 {
		return int64(count(t, b, `SELECT added_at FROM work WHERE id = ?`, w))
	}

	back := b.File(folder, "Heat (1995)/Heat.mkv")
	b.Version([]id.ID{back}, m.Work)
	if added(m.Work) == 1 || int64(count(t, b, `SELECT added_at FROM work_library WHERE work_id = ?`, m.Work)) != added(m.Work) {
		t.Errorf("a returning movie kept its old date")
	}
	b.Version([]id.ID{b.File(folder, "Ronin (1998)/Ronin 4K.mkv")}, other.Work)
	if added(other.Work) != 1 {
		t.Errorf("a second version re-dated the movie")
	}

	// A provisional work's version is reassigned to a matched work that has
	// no version.
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.DeleteFiles(ctx, tx, []id.ID{back}) })
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work SET added_at = 1 WHERE id = ?`, m.Work)
		return err
	})
	p := b.Movie(folder, "Heat", 0)
	if err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
			_, err := cb.ReassignVersion(ctx, p.Version, []id.ID{m.Work})
			return err
		})
	}); err != nil {
		t.Fatal(err)
	}
	if added(m.Work) == 1 {
		t.Errorf("a matched movie returning through a reassignment kept its old date")
	}
}

func TestExtrasDoNotMakeTheirParentAvailable(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	movie := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Dune"})
	trailer := b.Work(catalog.WorkInput{Kind: catalog.KindExtra, Parent: movie, Title: "Trailer", ExtraType: "trailer"})
	b.Version([]id.ID{b.File(folder, "Dune/Trailers/t.mkv")}, trailer)
	if got := libraries(t, b, movie); len(got) != 0 {
		t.Fatalf("a movie with only a trailer is in %v", got)
	}
}

func TestPutVersionRegroupsParts(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	movie := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Lawrence of Arabia"})
	cd1 := b.File(folder, "L/L cd1.mkv")
	cd2 := b.File(folder, "L/L cd2.mkv")
	v1 := b.Version([]id.ID{cd1}, movie)
	v2 := b.Version([]id.ID{cd1, cd2}, movie)
	if v1 == v2 {
		t.Fatal("regrouping kept the one-part version")
	}
	if v, _ := b.Catalog.Version(ctx, v2); !slices.Equal(v.Parts, []id.ID{cd1, cd2}) {
		t.Fatalf("parts %v", v.Parts)
	}
	if _, err := b.Catalog.Version(ctx, v1); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("old version: %v", err)
	}
	// The same parts again only replace the works.
	other := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Other"})
	if v3 := b.Version([]id.ID{cd1, cd2}, other); v3 != v2 {
		t.Fatal("same parts made a new version")
	}

	_, folder2 := b.Library(catalog.Movies)
	elsewhere := b.File(folder2, "x.mkv")
	err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
			_, err := cb.PutVersion(ctx, catalog.VersionInput{Parts: []id.ID{cd1, elsewhere}, Works: []id.ID{movie}})
			return err
		})
	})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("parts in two libraries: %v", err)
	}
}

func TestReassignVersion(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	wrong := b.Movie(folder, "The Thing", 2011)
	right := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "The Thing", Year: 1982})
	before := matchRev(t, b, wrong.Work)

	var r catalog.Reassignment
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
			var err error
			r, err = cb.ReassignVersion(ctx, wrong.Version, []id.ID{right})
			return err
		})
	})
	if !slices.Equal(r.From, []id.ID{wrong.Work}) || !slices.Equal(r.Moves, []catalog.Move{{From: wrong.Work, To: right, Emptied: true}}) {
		t.Fatalf("reassignment %+v", r)
	}
	if after, got := matchRev(t, b, wrong.Work), matchRev(t, b, right); after != before+1 || got != 1 {
		t.Fatalf("match revs %d → %d, new work's %d", before, after, got)
	}
	if len(libraries(t, b, wrong.Work)) != 0 || len(libraries(t, b, right)) != 1 {
		t.Fatal("work_library didn't follow the version")
	}
}

func TestUpdateMetadataRespectsLocks(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	m := b.Movie(folder, "Dune", 2021)

	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.EditMetadata(ctx, tx, m.Work, catalog.WorkMetadata{Title: "Dune: Part One", Genres: []string{"Mine"}},
			catalog.FieldTitle, catalog.FieldGenres)
	})
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.UpdateMetadata(ctx, tx, m.Work, catalog.WorkMetadata{
			Title: "Dune", Year: 2021, Overview: "Spice.", Genres: []string{"Science Fiction"},
			Credits: []catalog.Credit{{Name: "Timothée Chalamet", Role: "actor", Character: "Paul"}},
		})
	})
	got, _ := b.Catalog.Work(ctx, m.Work)
	if got.Title != "Dune: Part One" || !slices.Equal(got.LockedFields, []string{catalog.FieldGenres, catalog.FieldTitle}) {
		t.Fatalf("locked fields were overwritten: %+v", got)
	}
	var genre string
	if err := b.DB.Reader().QueryRow(`SELECT g.name FROM work_genre wg JOIN genre g ON g.id = wg.genre_id WHERE wg.work_id = ?`, m.Work).Scan(&genre); err != nil || genre != "Mine" {
		t.Fatalf("genre %q, %v", genre, err)
	}
	var overview string
	if err := b.DB.Reader().QueryRow(`SELECT overview FROM work WHERE id = ?`, m.Work).Scan(&overview); err != nil || overview != "Spice." {
		t.Fatalf("an unlocked field wasn't updated: %q, %v", overview, err)
	}
}

func search(t *testing.T, b *catalogtest.Builder, term string) []id.ID {
	t.Helper()
	return matches(t, b, `SELECT w.id FROM work_fts f JOIN work w ON w.rid = f.rowid WHERE work_fts MATCH ? ORDER BY w.id`, term)
}

func searchPeople(t *testing.T, b *catalogtest.Builder, term string) []id.ID {
	t.Helper()
	return matches(t, b, `SELECT p.id FROM person_fts f JOIN person p ON p.rid = f.rowid WHERE person_fts MATCH ? ORDER BY p.id`, term)
}

func matches(t *testing.T, b *catalogtest.Builder, query, term string) []id.ID {
	t.Helper()
	rows, err := b.DB.Reader().Query(query, `"`+term+`"`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []id.ID
	for rows.Next() {
		var w id.ID
		if err := rows.Scan(&w); err != nil {
			t.Fatal(err)
		}
		out = append(out, w)
	}
	return out
}

func TestSearchIndexFollowsTitlesAndCredits(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	m := b.Movie(folder, "Amélie", 2001)
	if got := search(t, b, "amelie"); !slices.Equal(got, []id.ID{m.Work}) {
		t.Fatalf("search amelie: %v", got)
	}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.UpdateMetadata(ctx, tx, m.Work, catalog.WorkMetadata{
			Title: "Le Fabuleux Destin", AltTitles: []string{"Amelie from Montmartre"},
			Credits: []catalog.Credit{{Name: "Audrey Tautou", Role: "actor"}},
		})
	})
	for _, term := range []string{"fabuleux", "montmartre", "tautou"} {
		if got := search(t, b, term); !slices.Equal(got, []id.ID{m.Work}) {
			t.Errorf("search %s: %v", term, got)
		}
	}
	if got := search(t, b, "amélie"); !slices.Equal(got, []id.ID{m.Work}) {
		t.Errorf("the alternative title should still match: %v", got)
	}
	if got := search(t, b, "fab"); got != nil {
		t.Errorf("a fragment of a word matched: %v", got)
	}
	if got := searchPeople(t, b, "tautou"); len(got) != 1 {
		t.Errorf("a new person isn't in the people index: %v", got)
	}

	s := b.Movie(folder, "Spider-Man: No Way Home", 2021)
	for _, term := range []string{"spider", "man", "spiderman", "no way home"} {
		if got := search(t, b, term); !slices.Equal(got, []id.ID{s.Work}) {
			t.Errorf("search %s: %v", term, got)
		}
	}
}

func TestEnsureWork(t *testing.T) {
	b := catalogtest.New(t)
	dune := catalog.ExternalID{Provider: "tmdb", Kind: "movie", ID: "438631"}
	a := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Dune", ExternalIDs: []catalog.ExternalID{dune}})
	// A root work is never found by its description. Two folders with the
	// same match are two roots, each with the ID.
	if got := b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Dune (2021)", ExternalIDs: []catalog.ExternalID{dune}}); got == a {
		t.Fatal("an external ID joined two roots")
	}
	if w, _ := b.Catalog.Work(ctx, a); !slices.Equal(w.ExternalIDs, []catalog.ExternalID{dune}) {
		t.Fatalf("external IDs %v", w.ExternalIDs)
	}

	series := b.Series("Show")
	s0 := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: "Specials", Number: 0})
	if again := b.Work(catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: "Season 0", Number: 0}); again != s0 {
		t.Fatal("season 0 was made twice")
	}
	for _, in := range []catalog.WorkInput{
		{Kind: catalog.KindEpisode, Title: "orphan"},
		{Kind: catalog.KindMovie, Title: "child", Parent: series},
		{Kind: catalog.KindMovie},
		{Kind: catalog.KindSeries, Title: "s", Edition: "Final Cut"},
	} {
		err := write(t, b, func(ctx context.Context, tx *store.Tx) error { _, err := b.Catalog.EnsureWork(ctx, tx, in); return err })
		if !errors.Is(err, catalog.ErrInvalid) {
			t.Errorf("%+v: %v", in, err)
		}
	}
}

func TestSortTitle(t *testing.T) {
	for title, want := range map[string]string{
		"The Expanse":             "Expanse",
		"A":                       "A",
		"An Officer":              "Officer",
		"¡Three Amigos!":          "Three Amigos!", // sorts under T
		"¿Qué pasó ayer?":         "Qué pasó ayer?",
		"The 'Burbs":              "'Burbs", // other punctuation stays, so it sorts first
		"*batteries not included": "*batteries not included",
	} {
		if got := catalog.SortTitle(title); got != want {
			t.Errorf("SortTitle(%q) = %q, want %q", title, got, want)
		}
	}
}

// TestLockedSortTitle checks that a sort title an admin edited survives a
// refresh that changes the title.
func TestLockedSortTitle(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	m := b.Movie(folder, "Three Amigos", 1986)
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.EditMetadata(ctx, tx, m.Work, catalog.WorkMetadata{SortTitle: "Amigos"}, catalog.FieldSortTitle)
	})
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.UpdateMetadata(ctx, tx, m.Work, catalog.WorkMetadata{Title: "¡Three Amigos!"})
	})
	var sortTitle, copied string
	err := b.DB.Reader().QueryRow(`SELECT w.sort_title, wl.sort_title FROM work w JOIN work_library wl ON wl.work_id = w.id WHERE w.id = ?`, m.Work).Scan(&sortTitle, &copied)
	if err != nil || sortTitle != "Amigos" || copied != "Amigos" {
		t.Errorf("sort title %q, in work_library %q, %v; want the edited Amigos", sortTitle, copied, err)
	}
}

func TestMergeEpisode(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Shows)
	series := b.Series("Show")
	prov := b.Episode(folder, series, 1, 1)
	matched := b.Episode(folder, series, 1, 2)
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error { return cb.MergeEpisode(ctx, prov.Work, matched.Work) })
	})
	if _, err := b.Catalog.Work(ctx, prov.Work); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("merged episode: %v", err)
	}
	if v, _ := b.Catalog.Version(ctx, prov.Version); !slices.Equal(v.Works, []id.ID{matched.Work}) {
		t.Fatalf("the merged episode's version covers %v", v.Works)
	}
	other := b.Episode(folder, b.Series("Other"), 1, 1)
	err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error { return cb.MergeEpisode(ctx, other.Work, matched.Work) })
	})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("merging another series' episode: %v", err)
	}
}

func TestRelink(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	m := b.Movie(folder, "Heat", 1995)
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.MarkGone(ctx, tx, m.File) })
	found := b.File(folder, "Renamed/Heat.mkv")
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.Relink(ctx, tx, m.File, found) })

	f, err := b.Catalog.File(ctx, m.File)
	if err != nil || f.RelPath != "Renamed/Heat.mkv" || !f.Present {
		t.Fatalf("relinked file %+v, %v", f, err)
	}
	if _, err := b.Catalog.File(ctx, found); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("found row: %v", err)
	}
	if len(libraries(t, b, m.Work)) != 1 {
		t.Fatal("the relinked movie isn't available")
	}
	other := b.Movie(folder, "Other", 2000)
	err = write(t, b, func(ctx context.Context, tx *store.Tx) error { return b.Catalog.Relink(ctx, tx, m.File, other.File) })
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("relinking onto a file with a version: %v", err)
	}
}

// A scan marks a file gone only while it's still at the path it was missing
// from. A file that a probe has meanwhile moved to its new path stays.
func TestMarkGoneOnlyAtTheMissingPath(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	m := b.Movie(folder, "Heat", 1995)
	old, _ := b.Catalog.File(ctx, m.File)
	found := b.File(folder, "Renamed/Heat.mkv")
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.Relink(ctx, tx, m.File, found) })

	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.MarkGone(ctx, tx, folder, []catalog.FileRef{{ID: m.File, RelPath: old.RelPath}})
	})
	if f, err := b.Catalog.File(ctx, m.File); err != nil || !f.Present {
		t.Fatalf("a file moved meanwhile was marked gone: %+v, %v", f, err)
	}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.MarkGone(ctx, tx, folder, []catalog.FileRef{{ID: m.File, RelPath: "Renamed/Heat.mkv"}})
	})
	if f, _ := b.Catalog.File(ctx, m.File); f.Present || len(libraries(t, b, m.Work)) != 0 {
		t.Fatalf("missing from its new path, it's still present: %+v", f)
	}
}

// Deleting a library of 40,000 files goes a batch at a time. If it's
// interrupted partway, the library stays off the list with the rest of its
// files. When it runs again, it carries on and finishes.
func TestDeletingALargeLibraryResumes(t *testing.T) {
	b := catalogtest.New(t)
	lib, folder := b.Library(catalog.Movies)
	const n = 40000
	b.BulkFiles(folder, "Films", n)
	heat := b.Movie(folder, "Heat", 1995)
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.MarkDeleting(ctx, tx, lib) })

	// A batch fails partway, as when the disk fills.
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TEMP TRIGGER fail BEFORE DELETE ON media_file
			WHEN old.rel_path = 'Films/20000.mkv' BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
		return err
	})
	if err := libraryChanges(b).Do(ctx, lib); err == nil {
		t.Fatal("the failing batch didn't fail the deletion")
	}
	left := count(t, b, `SELECT count(*) FROM media_file WHERE folder_id = ?`, folder)
	if left == 0 || left >= n {
		t.Fatalf("%d files left; want the batches before the failing one committed", left)
	}
	if _, err := b.Catalog.Library(ctx, lib); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("the library half deleted: %v; want it still off the list", err)
	}

	// Cancelled between batches, it stops.
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if err := libraryChanges(b).Do(stopped, lib); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}

	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DROP TRIGGER temp.fail`)
		return err
	})
	if got := removals(t, b); !slices.Equal(got, []id.ID{lib}) {
		t.Fatalf("removals %v; want the half-deleted library", got)
	}
	remove(t, b, lib)
	if c := count(t, b, `SELECT count(*) FROM media_file WHERE folder_id = ?`, folder); c != 0 {
		t.Fatalf("%d files left", c)
	}
	if c := count(t, b, `SELECT count(*) FROM library WHERE id = ?`, lib); c != 0 {
		t.Fatal("the library is still there")
	}
	if _, err := b.Catalog.Work(ctx, heat.Work); err != nil || len(libraries(t, b, heat.Work)) != 0 {
		t.Fatalf("the work: %v, in %v; want it kept, in no library", err, libraries(t, b, heat.Work))
	}
	remove(t, b, lib) // running it again after the end does nothing
	if got := removals(t, b); len(got) != 0 {
		t.Fatalf("removals %v after the deletion", got)
	}
}

func TestCollections(t *testing.T) {
	b := catalogtest.New(t)
	_, folder := b.Library(catalog.Movies)
	m := b.Movie(folder, "Dune", 2021)
	join := func(key string) {
		b.Write(func(ctx context.Context, tx *store.Tx) error {
			return b.Catalog.UpdateMetadata(ctx, tx, m.Work, catalog.WorkMetadata{Title: "Dune", Collection: &catalog.CollectionRef{Key: key, Name: key}})
		})
	}
	join("tmdb:1")
	join("tmdb:2") // a work is in one TMDb collection
	var n int
	if err := b.DB.Reader().QueryRow(`SELECT count(*) FROM collection_work WHERE work_id = ?`, m.Work).Scan(&n); err != nil || n != 1 {
		t.Fatalf("in %d collections, %v", n, err)
	}
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.UpdateMetadata(ctx, tx, m.Work, catalog.WorkMetadata{
			Title: "Dune", Collection: &catalog.CollectionRef{Key: "tmdb:2", Name: "A Dune Collection"},
		})
	})
	var sortName string
	if err := b.DB.Reader().QueryRow(`SELECT sort_name FROM collection WHERE key = 'tmdb:2'`).Scan(&sortName); err != nil || sortName != "Dune Collection" {
		t.Errorf("collection sort name %q, %v; want its name without the article", sortName, err)
	}
}

func TestSubjects(t *testing.T) {
	b := catalogtest.New(t)
	_, movies := b.Library(catalog.Movies)
	showsLib, shows := b.Library(catalog.Shows)
	movie := b.Movie(movies, "Dune", 2021)
	episode := b.Episode(shows, b.Series("Severance"), 2, 3)
	unmatched := b.File(movies, "New/new.mkv")
	got, err := b.Catalog.Subjects(ctx, []id.ID{movie.File, movie.Work, episode.File, unmatched, movies, showsLib, id.New()})
	if err != nil {
		t.Fatal(err)
	}
	want := map[id.ID]string{
		movie.File: "Dune", movie.Work: "Dune", episode.File: "Severance · S2 · E3",
		unmatched: "New/new.mkv", movies: catalogtest.Abs("/media/movies1"), showsLib: "shows 2",
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
}

func TestCheckInvariantsCatchesDrift(t *testing.T) {
	db := storetest.New(t)
	c := catalog.New(db, events.NewBus())
	if err := c.CheckInvariants(ctx); err != nil {
		t.Fatalf("empty catalog: %v", err)
	}
	err := db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO media_version (id, created_at) VALUES (?, 0)`, id.New())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CheckInvariants(ctx); err == nil {
		t.Fatal("a version with no parts or works passed")
	}
}

// Every season has a number, and every episode has its season's number as
// its parent number. If a read found no number on a season row, the season
// would show as Season 0.
func TestCheckInvariantsCatchesMissingNumbers(t *testing.T) {
	for name, drift := range map[string]string{
		"season without a number":   `UPDATE work SET number = NULL WHERE kind = 'season'`,
		"episode without a season":  `UPDATE work SET parent_number = NULL WHERE kind = 'episode'`,
		"episode in another season": `UPDATE work SET parent_number = 2 WHERE kind = 'episode'`,
	} {
		t.Run(name, func(t *testing.T) {
			b := catalogtest.New(t)
			_, folder := b.Library(catalog.Shows)
			b.Episode(folder, b.Series("Severance"), 1, 1)
			if err := b.Catalog.CheckInvariants(ctx); err != nil {
				t.Fatalf("before: %v", err)
			}
			if err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
				_, err := tx.ExecContext(ctx, drift)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := b.Catalog.CheckInvariants(ctx); err == nil {
				t.Fatal("passed")
			}
			// Put it back for the check after the test.
			if err := write(t, b, func(ctx context.Context, tx *store.Tx) error {
				_, err := tx.ExecContext(ctx, `UPDATE work SET number = 1 WHERE kind = 'season';
					UPDATE work SET parent_number = 1 WHERE kind = 'episode'`)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A batch that moves a whole season rebuilds each work once at the flush.
// It still rebuilds the season whose episodes were merged away before the
// flush.
func TestBatchLibraries(t *testing.T) {
	b := catalogtest.New(t)
	lib, folder := b.Library(catalog.Shows)
	series := b.Series("Show")
	var eps []catalogtest.Item
	for e := 1; e <= 5; e++ {
		eps = append(eps, b.Episode(folder, series, 1, e))
	}
	var to id.ID
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		return b.Catalog.Batch(ctx, tx, func(cb *catalog.Batch) error {
			var err error
			to, err = b.Catalog.EnsureWork(ctx, tx, catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: "Season 2", Number: 2})
			if err != nil {
				return err
			}
			for e, it := range eps {
				w, err := b.Catalog.EnsureWork(ctx, tx, catalog.WorkInput{Kind: catalog.KindEpisode, Parent: to, Title: "x", ParentNumber: 2, Number: e + 1})
				if err != nil {
					return err
				}
				if _, err := cb.ReassignVersion(ctx, it.Version, []id.ID{w}); err != nil {
					return err
				}
				if err := cb.MergeEpisode(ctx, it.Work, w); err != nil {
					return err
				}
			}
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM work_library WHERE work_id = ?`, to).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("the new season is in %d libraries before the flush", n)
			}
			return nil
		})
	})
	if got := libraries(t, b, to); len(got) != 1 || got[0] != lib {
		t.Fatalf("new season in %v", got)
	}
	var emptied int
	if err := b.DB.Reader().QueryRow(`SELECT count(*) FROM work_library wl JOIN work w ON w.id = wl.work_id WHERE w.kind = 'season' AND w.number = 1`).Scan(&emptied); err != nil || emptied != 0 {
		t.Fatalf("emptied season still in %d libraries (%v)", emptied, err)
	}
}

// libraryChanged is published for a library when it's created, edited or
// starts being deleted, and when a root work appears in it or leaves it. It is
// published after the commit. It is not published when a write leaves the
// library as it was.
func TestLibraryChanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := catalogtest.New(t)
		sub := b.Bus.Subscribe(func(events.Scope) bool { return true })
		defer sub.Close()
		expect := func(what string, want ...id.ID) {
			t.Helper()
			time.Sleep(3 * time.Second) // waits out libraryChanged's interval
			synctest.Wait()
			var told []id.ID
			for len(sub.C()) > 0 {
				e := <-sub.C()
				if e.Kind != events.LibraryChanged {
					t.Fatalf("%s: event %+v", what, e)
				}
				told = append(told, e.Scope.Libraries...)
			}
			if !slices.Equal(told, want) {
				t.Fatalf("%s: told %v; want %v", what, told, want)
			}
		}
		lib, folder := b.Library(catalog.Movies)
		expect("a new library", lib)
		b.Write(func(ctx context.Context, tx *store.Tx) error {
			_, _, err := b.Catalog.UpdateLibrary(ctx, tx, lib, catalog.LibraryChange{Name: new("Films")})
			return err
		})
		expect("a rename", lib)
		heat := b.Movie(folder, "Heat", 1995)
		expect("a film added", lib)
		series := b.Series("Show")
		b.Episode(folder, series, 1, 1)
		expect("a series' first episode", lib)
		b.Episode(folder, series, 1, 2)
		expect("another episode")
		dup := b.Movie(folder, "Heat (dup)", 1995)
		expect("a duplicate added", lib)
		b.Write(func(ctx context.Context, tx *store.Tx) error { return b.MarkGone(ctx, tx, dup.File) })
		expect("the duplicate's file gone", lib)
		b.Write(func(ctx context.Context, tx *store.Tx) error { return b.MarkGone(ctx, tx, heat.File) })
		expect("its files gone", lib)
		err := b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			if err := b.Catalog.MarkDeleting(ctx, tx, lib); err != nil {
				return err
			}
			return errors.New("rolled back")
		})
		if err == nil {
			t.Fatal("the write didn't fail")
		}
		expect("a rolled-back write")
		b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.MarkDeleting(ctx, tx, lib) })
		expect("the library being deleted", lib)
	})
}

// The task's items are the libraries being deleted, then the folders being
// removed from libraries that aren't being deleted. A deleted library's
// folders are removed with it.
func TestRemovalsListLibrariesThenFolders(t *testing.T) {
	b := catalogtest.New(t)
	var libs [2]catalog.Library
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		for i, name := range []string{"a", "b"} {
			l, err := b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{
				Name: name, Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/" + name + "1"), catalogtest.Abs("/" + name + "2")},
			})
			if err != nil {
				return err
			}
			libs[i] = l
		}
		return nil
	})
	if got := removals(t, b); len(got) != 0 {
		t.Fatalf("removals %v; want none", got)
	}
	var removed []catalog.Folder
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		_, removed, err = b.Catalog.UpdateLibrary(ctx, tx, libs[1].ID, catalog.LibraryChange{Folders: &[]string{catalogtest.Abs("/b1")}})
		return err
	})
	b.Write(func(ctx context.Context, tx *store.Tx) error { return b.Catalog.MarkDeleting(ctx, tx, libs[0].ID) })
	if got, want := removals(t, b), []id.ID{libs[0].ID, removed[0].ID}; !slices.Equal(got, want) {
		t.Fatalf("removals %v; want %v", got, want)
	}

	remove(t, b, libs[0].ID)
	remove(t, b, removed[0].ID)
	if got := removals(t, b); len(got) != 0 {
		t.Fatalf("removals %v after both", got)
	}
	remove(t, b, id.New()) // an item that is already removed counts as done
	if l, err := b.Catalog.Library(ctx, libs[1].ID); err != nil || len(l.Folders) != 1 || l.Folders[0].Path != catalogtest.Abs("/b1") {
		t.Fatalf("the library kept %+v, %v", l, err)
	}
}

// A folder removal cut off partway is still listed. When it runs again, it
// carries on from the files left.
func TestRemovingAFolderResumes(t *testing.T) {
	b := catalogtest.New(t)
	var l catalog.Library
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		var err error
		l, err = b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Movies", Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/a"), catalogtest.Abs("/b")}})
		return err
	})
	folder := l.Folders[1].ID
	const n = 3 * store.BatchSize
	b.BulkFiles(folder, "Films", n)
	heat := b.Movie(folder, "Heat", 1995)
	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, _, err := b.Catalog.UpdateLibrary(ctx, tx, l.ID, catalog.LibraryChange{Folders: &[]string{catalogtest.Abs("/a")}})
		return err
	})

	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TEMP TRIGGER fail BEFORE DELETE ON media_file
			WHEN old.rel_path = 'Films/`+fmt.Sprint(n/2)+`.mkv' BEGIN SELECT RAISE(ABORT, 'disk full'); END`)
		return err
	})
	if err := libraryChanges(b).Do(ctx, folder); err == nil {
		t.Fatal("the failing batch didn't fail the removal")
	}
	left := count(t, b, `SELECT count(*) FROM media_file WHERE folder_id = ?`, folder)
	if left == 0 || left >= n {
		t.Fatalf("%d files left; want the batches before the failing one committed", left)
	}
	if got := removals(t, b); !slices.Equal(got, []id.ID{folder}) {
		t.Fatalf("removals %v; want the half-removed folder", got)
	}

	b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DROP TRIGGER temp.fail`)
		return err
	})
	remove(t, b, folder)
	if c := count(t, b, `SELECT count(*) FROM media_file WHERE folder_id = ?`, folder); c != 0 {
		t.Fatalf("%d files left", c)
	}
	if c := count(t, b, `SELECT count(*) FROM library_folder WHERE id = ?`, folder); c != 0 {
		t.Fatal("the folder is still there")
	}
	if got := libraries(t, b, heat.Work); len(got) != 0 {
		t.Fatalf("the movie is still in %v", got)
	}
	if got := removals(t, b); len(got) != 0 {
		t.Fatalf("removals %v after the removal", got)
	}
}
