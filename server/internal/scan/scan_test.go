package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/fsnotify/fsnotify"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/previews"
	"github.com/mmcdole/kivo/internal/subtitles"
)

var ctx = context.Background()

func TestScanFindsNewChangedAndGoneFiles(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/movies")
	folder := l.Folders[0].ID
	fsys := h.folders["/movies"]
	fsys.write("Dune (2021)/Dune.mkv", "dune", old)
	fsys.write("Heat (1995)/Heat.mkv", "heat", old)
	fsys.write("Heat (1995)/notes.txt", "x", old)
	fsys.write("New (2024)/New.mkv", "copying", time.Now()) // modified a moment ago
	fsys.write("Part (2024)/Part.mkv.part", "partial", old) // not a video
	fsys.write(".hidden/Secret.mkv", "s", old)              // hidden
	fsys.write("@eaDir/Thumb.mkv", "s", old)                // NAS bookkeeping
	if asked := h.tasks.of(TaskScanLibraries); !slices.Equal(asked, []id.ID{l.ID}) {
		t.Fatalf("AddLibrary asked for %v", asked)
	}

	h.mustScan(folder)
	got := h.files(folder)
	if len(got) != 3 || !got["Dune (2021)/Dune.mkv"].Present || !got["Heat (1995)/Heat.mkv"].Present || !got["New (2024)/New.mkv"].Present {
		t.Fatalf("files %v", got)
	}
	if n := len(h.unprobed(l.ID)); n != 3 {
		t.Fatalf("%d files to probe, want 3", n)
	}
	h.probeAll()

	// The copy finishes, Heat is deleted, and Dune is replaced.
	later := time.Now().Add(-10 * time.Minute)
	fsys.write("New (2024)/New.mkv", "copied", later)
	fsys.remove("Heat (1995)", later)
	fsys.write("Dune (2021)/Dune.mkv", "dune, remastered", later)
	h.mustScan(folder)
	got = h.files(folder)
	if got["Heat (1995)/Heat.mkv"].Present || !got["New (2024)/New.mkv"].Present {
		t.Fatalf("after changes: %v", got)
	}
	for _, rel := range []string{"Dune (2021)/Dune.mkv", "New (2024)/New.mkv"} {
		if f, _ := h.b.Catalog.File(ctx, got[rel].ID); f.Generation != 2 {
			t.Fatalf("%s, changed, is at generation %d", rel, f.Generation)
		}
	}
	if n := len(h.unprobed(l.ID)); n != 2 {
		t.Fatalf("%d files to probe after the changes, want 2", n)
	}
}

func TestUnchangedRescanListsAndOpensNothing(t *testing.T) {
	// The zero-listing property doesn't depend on size.
	// KIVO_SCAN_FILES=200000 runs the full-size tree, which takes about a
	// minute without -race.
	files := 5_000
	if n, err := strconv.Atoi(os.Getenv("KIVO_SCAN_FILES")); err == nil {
		files = n
	} else if testing.Short() {
		files = 1_000
	}
	h := newHarness(t)
	l := h.library(catalog.Shows, "/tv")
	folder := l.Folders[0].ID
	fsys := h.folders["/tv"]
	for i := range files {
		fsys.write(fmt.Sprintf("Show %d/Season %02d/S%02dE%03d.mkv", i/1000, i/100%10+1, i/100%10+1, i%100), "x", old)
	}
	start := time.Now()
	h.mustScan(folder)
	t.Logf("first scan of %d files: %v", files, time.Since(start))

	fsys.resetCounts()
	start = time.Now()
	h.mustScan(folder)
	lists, opens := fsys.counts()
	t.Logf("unchanged rescan: %v", time.Since(start))
	if lists != 0 || opens != 0 {
		t.Fatalf("an unchanged rescan listed %d directories and opened %d files", lists, opens)
	}

	// One new episode lists only its directory.
	fsys.write("Show 0/Season 02/S02E999.mkv", "x", old) // Show 0 exists at every size
	fsys.resetCounts()
	h.mustScan(folder)
	if lists, _ := fsys.counts(); lists != 1 {
		t.Fatalf("one new file listed %d directories", lists)
	}

	// An admin's full walk lists everything again.
	if err := h.s.ScanLibraryFullWalk(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	fsys.resetCounts()
	h.mustScan(folder)
	if lists, _ := fsys.counts(); lists < files/100 {
		t.Fatalf("a full walk listed only %d directories", lists)
	}
}

func TestUnreachableFolderChangesNothing(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/a", "/b")
	a, b := l.Folders[0].ID, l.Folders[1].ID
	h.folders["/a"].write("A/A.mkv", "a", old)
	h.folders["/b"].write("B/B.mkv", "b", old)
	h.mustScan(a)
	h.mustScan(b)

	for name, breakIt := range map[string]func(){
		"unmounted": func() { h.down["/a"] = true },
		"empty":     func() { h.folders["/a"] = newMemFS() },
	} {
		saved := h.folders["/a"]
		breakIt()
		if err := h.scan(a); err != nil {
			t.Errorf("%s: err %v", name, err)
		}
		if !h.files(a)["A/A.mkv"].Present {
			t.Errorf("%s: a file under an unreachable folder was marked gone", name)
		}
		if r, _ := h.b.Catalog.Folder(ctx, a); r.Reachable {
			t.Errorf("%s: folder still reachable", name)
		}
		if len(h.s.Health(ctx)) == 0 {
			t.Errorf("%s: no health problem", name)
		}
		h.down["/a"], h.folders["/a"] = false, saved
	}

	// The library's other folder still scans.
	h.folders["/b"].write("B2/B2.mkv", "b2", old)
	h.mustScan(b)
	if !h.files(b)["B2/B2.mkv"].Present {
		t.Fatal("the reachable folder didn't scan")
	}
	h.mustScan(a)
	if r, _ := h.b.Catalog.Folder(ctx, a); !r.Reachable || len(h.s.Health(ctx)) != 0 {
		t.Fatalf("a recovered folder: %+v, problems %v", r, h.s.Health(ctx))
	}
}

func TestCrashMidRunMarksNothingGone(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder := l.Folders[0].ID
	fsys := h.folders["/m"]
	for i := range 10 {
		fsys.write(fmt.Sprintf("M%d/M%d.mkv", i, i), "x", old)
	}
	h.mustScan(folder)
	var before int
	h.b.DB.Reader().QueryRow(`SELECT sum(mtime) FROM scan_dir`).Scan(&before)

	later := time.Now().Add(-time.Hour / 2)
	fsys.remove("M0", later)
	for i := 1; i < 10; i++ {
		fsys.write(fmt.Sprintf("M%d/extra.mkv", i), "y", later)
	}
	runCtx, crash := context.WithCancel(ctx)
	n := 0
	fsys.onList = func(string) {
		if n++; n == 4 {
			crash()
		}
	}
	err := h.s.scanFolder(runCtx, folder, ".")
	fsys.onList = nil
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if !h.files(folder)["M0/M0.mkv"].Present {
		t.Fatal("a crashed run marked a file gone")
	}
	var after int
	h.b.DB.Reader().QueryRow(`SELECT sum(mtime) FROM scan_dir`).Scan(&after)
	if after != before {
		t.Fatal("a crashed run advanced directory mtimes")
	}
	h.mustScan(folder) // the run again catches up
	if h.files(folder)["M0/M0.mkv"].Present || !h.files(folder)["M5/extra.mkv"].Present {
		t.Fatal("the rerun didn't catch up")
	}
}

// movie scans one movie file, probes it and gives it a work and version.
func (h *harness) movie(folder id.ID, fsys *memFS, rel, data string) movieItem {
	h.t.Helper()
	fsys.write(rel, data, old)
	h.mustScan(folder)
	h.probeAll()
	f := h.files(folder)[rel]
	w := h.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: rel})
	v := h.b.Version([]id.ID{f.ID}, w)
	return movieItem{file: f.ID, work: w, version: v}
}

type movieItem struct{ file, work, version id.ID }

func TestMoveDetection(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(h *harness, fsys *memFS, folder id.ID)
		move  bool // the new file keeps the old ID
	}{
		{"rename", func(h *harness, fsys *memFS, folder id.ID) { fsys.rename("A/a.mkv", "A/b.mkv", time.Now()) }, true},
		{"move to another directory", func(h *harness, fsys *memFS, folder id.ID) { fsys.rename("A/a.mkv", "B/a.mkv", time.Now()) }, true},
		{"copy", func(h *harness, fsys *memFS, folder id.ID) { fsys.write("B/a.mkv", "same bytes", old) }, false},
		{"size matches, hash doesn't", func(h *harness, fsys *memFS, folder id.ID) {
			fsys.remove("A/a.mkv", time.Now())
			fsys.write("B/a.mkv", "SAME BYTES", old)
		}, false},
		{"two possible sources", func(h *harness, fsys *memFS, folder id.ID) {
			h.movie(folder, fsys, "Twin/a.mkv", "same bytes") // a copy; then both vanish
			fsys.remove("A/a.mkv", time.Now())
			fsys.remove("Twin/a.mkv", time.Now())
			h.mustScan(folder)
			fsys.write("B/a.mkv", "same bytes", old)
		}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			l := h.library(catalog.Movies, "/m")
			folder, fsys := l.Folders[0].ID, h.folders["/m"]
			orig := h.movie(folder, fsys, "A/a.mkv", "same bytes")
			h.identifyAll()
			c.setup(h, fsys, folder)
			h.mustScan(folder) // a move within one run: the new file is found as the old one is swept
			h.probeAll()

			f, err := h.b.Catalog.File(ctx, orig.file)
			moved := err == nil && f.Present && f.RelPath != "A/a.mkv"
			if moved != c.move {
				t.Fatalf("moved = %v (file %+v, %v); want %v", moved, f, err, c.move)
			}
			if c.move {
				v, _ := h.b.Catalog.Version(ctx, orig.version)
				if !slices.Equal(v.Works, []id.ID{orig.work}) || len(h.files(folder)) != 1 {
					t.Fatalf("the moved file lost its version or left a duplicate: %v, %v", v, h.files(folder))
				}
				if got := h.unidentified(folder); !slices.Equal(got, []id.ID{orig.file}) {
					t.Fatalf("unidentified %v; want the moved file identified again", got)
				}
			}
		})
	}
}

func TestMoveAcrossTwoRuns(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	orig := h.movie(folder, fsys, "A/a.mkv", "same bytes")
	fsys.rename("A/a.mkv", "B/a.mkv", time.Now())
	// The first run lists only B, because A's mtime is pinned, as on a
	// filesystem that didn't update it. The old row is present, but its
	// path is gone.
	h.exec(`UPDATE scan_dir SET mtime = ? WHERE path = 'A'`, mtimeOf(h.folders["/m"], "A"))
	h.mustScan(folder)
	h.probeAll()
	if f, _ := h.b.Catalog.File(ctx, orig.file); f.RelPath != "B/a.mkv" {
		t.Fatalf("file %+v; want the old row moved to B", f)
	}
	h.mustScan(folder)
	if got := h.files(folder); len(got) != 1 || !got["B/a.mkv"].Present {
		t.Fatalf("files %v", got)
	}
}

// A probe that runs while the scan is still walking finds the move and
// relinks the old row to the new path. The end of the run must not then
// hide the row as missing from the old path.
func TestMoveProbedDuringTheRunStays(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	orig := h.movie(folder, fsys, "A/a.mkv", "same bytes")
	fsys.rename("A/a.mkv", "B/a.mkv", time.Now())
	fsys.write("C/c.mkv", "other bytes", old) // listed after B, so B's file is committed by then
	probed := false
	fsys.onList = func(dir string) {
		if dir == "C" {
			h.probeAll() // between listing A (a.mkv missing) and the end of the run
			probed = true
		}
	}
	h.mustScan(folder)
	fsys.onList = nil
	if !probed {
		t.Fatal("the probe didn't run during the scan")
	}

	f, err := h.b.Catalog.File(ctx, orig.file)
	if err != nil || !f.Present || f.RelPath != "B/a.mkv" {
		t.Fatalf("file %+v, %v; want the old row present at its new path", f, err)
	}
	if v, err := h.b.Catalog.Version(ctx, orig.version); err != nil || !slices.Equal(v.Works, []id.ID{orig.work}) {
		t.Fatalf("version %+v, %v; want it kept with its work", v, err)
	}
	h.probeAll()
	h.mustScan(folder)
	if got := h.files(folder); len(got) != 2 || !got["B/a.mkv"].Present || !got["C/c.mkv"].Present {
		t.Fatalf("files %v", got)
	}
}

// A scan may find tens of thousands of files missing at once, such as when
// a branch of a share drops out. That is more than one statement could
// bind, so it marks them gone in batches. It records the directory only
// once they're all marked.
func TestScanMarksFortyThousandFilesGone(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("Keep/k.mkv", "k", old)
	h.mustScan(folder)
	const n = 40000
	h.b.BulkFiles(folder, "Gone", n)
	h.exec(`INSERT INTO scan_dir (folder_id, path, mtime, listed_at) VALUES (?, 'Gone', 1, 1)`, folder)
	h.exec(`UPDATE scan_dir SET mtime = 1 WHERE folder_id = ? AND path = '.'`, folder) // so the folder is listed

	h.mustScan(folder)
	var present, gone int
	h.b.DB.Reader().QueryRow(`SELECT count(*) FILTER (WHERE status = 'present'), count(*) FILTER (WHERE status = 'gone')
		FROM media_file WHERE folder_id = ?`, folder).Scan(&present, &gone)
	if present != 1 || gone != n {
		t.Fatalf("%d present, %d gone; want 1 and %d", present, gone, n)
	}
	var dirs int
	h.b.DB.Reader().QueryRow(`SELECT count(*) FROM scan_dir WHERE folder_id = ? AND path = 'Gone'`, folder).Scan(&dirs)
	if dirs != 0 {
		t.Fatal("the vanished directory is still recorded")
	}
}

func mtimeOf(m *memFS, p string) int64 {
	info, _ := m.Stat(p)
	return info.ModTime().UnixMilli()
}

// A file that changes while it's probed keeps nothing of the probe. It's
// left for the next scan, which probes it as it is then.
func TestStaleProbeIsLeftForTheNextScan(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("A/a.mkv", "v1", old)
	h.mustScan(folder)
	file := h.unprobed(l.ID)[0]

	// The file is replaced while it's being probed.
	h.prob.hook = func(string) {
		fsys.write("A/a.mkv", "version two", old.Add(time.Minute))
		h.mustScan(folder)
		h.prob.hook = nil
	}
	if err := h.s.probeFile(ctx, l, file); err != nil {
		t.Fatal(err)
	}
	f, _ := h.b.Catalog.File(ctx, file)
	if len(f.Tracks) != 0 || f.Generation != 2 {
		t.Fatalf("a stale probe was stored: %+v", f)
	}
	if got := h.unprobed(l.ID); !slices.Equal(got, []id.ID{file}) {
		t.Fatalf("files to probe %v; want the current generation", got)
	}
	if got := h.unidentified(folder); len(got) != 0 {
		t.Fatalf("a stale probe left %v to identify", got)
	}
	h.probeAll()
	if f, _ := h.b.Catalog.File(ctx, file); len(f.Tracks) != 2 || len(h.unprobed(l.ID)) != 0 {
		t.Fatalf("the next probe: %+v", f)
	}
}

// A probe asks previews for a file's previews only in a library that makes
// them as files are added.
func TestProbeAsksForPreviewsAsTheLibrarySays(t *testing.T) {
	h := newHarness(t)
	h.s.deps.Previews = previews.New(h.b.DB, previews.Deps{Catalog: h.b.Catalog, Media: noMedia{}, Tasks: h.tasks, CacheDir: t.TempDir()})
	for path, schedule := range map[string]catalog.Schedule{"/nightly": catalog.Nightly, "/added": catalog.NightlyAndAdded} {
		h.folders[path] = newMemFS()
		h.folders[path].write("A/a.mkv", "data", old)
		lib, err := h.s.AddLibrary(ctx, catalog.LibraryInput{Name: path, Kind: catalog.Movies, Folders: []string{catalogtest.Abs(path)}, Previews: schedule})
		if err != nil {
			t.Fatal(err)
		}
		l, _ := h.b.Catalog.Library(ctx, lib)
		h.tasks.reset()
		h.mustScan(l.Folders[0].ID)
		h.probeAll()
		file := h.files(l.Folders[0].ID)["A/a.mkv"].ID
		var want []id.ID
		if schedule == catalog.NightlyAndAdded {
			want = []id.ID{file}
		}
		if got := h.tasks.of(previews.TaskMakePreviews); !slices.Equal(got, want) {
			t.Errorf("%s: asked for the previews of %v, want %v", path, got, want)
		}
	}
}

// A probe asks subtitles to extract a file with an embedded ASS, SSA or PGS
// track, and only such a file.
func TestProbeAsksForExtractedSubtitles(t *testing.T) {
	h := newHarness(t)
	subs := subtitles.New(h.b.DB, subtitles.Deps{
		Catalog: h.b.Catalog, Settings: h.settings, Media: noMedia{}, Tasks: h.tasks, CacheDir: t.TempDir(), SubtitleDir: t.TempDir(),
	}, subtitles.Options{})
	h.s.deps.Subtitles = subs
	h.prob.subtitles = map[string][]media.Track{
		"anime.mkv": {{Index: 2, Kind: media.Subtitle, Codec: "subrip"}, {Index: 3, Kind: media.Subtitle, Codec: "ass"}},
		"old.mkv":   {{Index: 2, Kind: media.Subtitle, Codec: "ssa"}},
		"film.mkv":  {{Index: 2, Kind: media.Subtitle, Codec: "subrip"}, {Index: 3, Kind: media.Subtitle, Codec: "hdmv_pgs_subtitle"}},
		"plain.mkv": {{Index: 2, Kind: media.Subtitle, Codec: "subrip"}, {Index: 3, Kind: media.Subtitle, Codec: "mov_text"}},
	}
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	for _, name := range []string{"anime.mkv", "old.mkv", "film.mkv", "plain.mkv"} {
		fsys.write("A/"+name, name, old)
	}
	h.mustScan(folder)
	h.probeAll()
	files := h.files(folder)
	got := h.tasks.of(subtitles.TaskExtractSubtitles)
	want := []id.ID{files["A/anime.mkv"].ID, files["A/old.mkv"].ID, files["A/film.mkv"].ID}
	if !slices.Equal(sortIDs(got), sortIDs(want)) {
		t.Fatalf("asked for %v, want %v", got, want)
	}
}

func TestProbeStoresResults(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("A/a.mkv", "data", old)
	h.mustScan(folder)
	h.probeAll()
	f, _ := h.b.Catalog.File(ctx, h.files(folder)["A/a.mkv"].ID)
	if f.Probe.Duration != time.Hour || f.Probe.Start != 1400*time.Millisecond || len(f.Tracks) != 2 || len(f.Probe.EdgeHash) != 16 || f.Probe.OSHash == 0 {
		t.Fatalf("file %+v", f)
	}
	if got := h.unidentified(folder); !slices.Equal(got, []id.ID{f.ID}) {
		t.Fatalf("to identify %v; want the probed file", got)
	}

	// A probe that fails for a reason that may pass returns its error, and
	// the file is left for the next scan.
	fsys.write("B/b.mkv", "junk", old)
	h.mustScan(folder)
	h.prob.err = errors.New("ffprobe timed out")
	b := h.files(folder)["B/b.mkv"].ID
	if err := h.s.probeFile(ctx, l, b); err != h.prob.err {
		t.Fatalf("a failed probe: %v", err)
	}
	if got := h.unprobed(l.ID); !slices.Equal(got, []id.ID{b}) {
		t.Fatalf("files to probe %v; want the one that failed", got)
	}

	// An MKV whose attachments can't be read is stored without them.
	h.prob.err = nil
	fsys.write("C/c.mkv", "\x1a\x45\xdf\xa3 not EBML after the magic", old)
	h.mustScan(folder)
	c := h.files(folder)["C/c.mkv"].ID
	if err := h.s.probeFile(ctx, l, c); err != nil {
		t.Fatalf("unreadable attachments: %v", err)
	}
	if f, _ := h.b.Catalog.File(ctx, c); f.Probe.Duration != time.Hour {
		t.Fatalf("unreadable attachments: file %+v", f)
	}
}

// A full walk sends a folder's unmatched videos back to be identified. A
// matched folder and an ordinary scan send nothing.
func TestFullWalkReidentifiesUnmatched(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("Unknown (2020)/Unknown (2020).mkv", "a", old)
	fsys.write("Unknown (2020)/Unknown (2020) - 4K.mkv", "b", old)
	fsys.write("Known (2020)/Known (2020).mkv", "c", old)
	h.mustScan(folder)
	h.probeAll()
	files := h.files(folder)
	unknown := h.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Unknown", Provisional: true})
	h.b.Version([]id.ID{files["Unknown (2020)/Unknown (2020).mkv"].ID}, unknown)
	h.b.Version([]id.ID{files["Unknown (2020)/Unknown (2020) - 4K.mkv"].ID}, unknown)
	h.b.Version([]id.ID{files["Known (2020)/Known (2020).mkv"].ID}, h.b.Work(catalog.WorkInput{Kind: catalog.KindMovie, Title: "Known"}))
	h.identifyAll()

	h.mustScan(folder)
	if p := h.unidentified(folder); len(p) != 0 {
		t.Fatalf("an ordinary scan sent back %v", p)
	}
	if err := h.s.ScanLibraryFullWalk(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	h.mustScan(folder)
	want := []id.ID{files["Unknown (2020)/Unknown (2020) - 4K.mkv"].ID, files["Unknown (2020)/Unknown (2020).mkv"].ID}
	if p := h.unidentified(folder); !slices.Equal(p, want) {
		t.Fatalf("a full walk sent back %v, want the unmatched files %v", p, want)
	}
}

func TestDeletedAndRestoredFileKeepsEverything(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	orig := h.movie(folder, fsys, "A/a.mkv", "bytes")
	fsys.remove("A/a.mkv", time.Now())
	h.mustScan(folder)
	if h.files(folder)["A/a.mkv"].Present {
		t.Fatal("deleted file still present")
	}
	fsys.write("A/a.mkv", "bytes", old)
	h.mustScan(folder)
	got := h.files(folder)["A/a.mkv"]
	if !got.Present || got.ID != orig.file {
		t.Fatalf("restored file %+v, want the original row", got)
	}
	if v, err := h.b.Catalog.Version(ctx, orig.version); err != nil || v.Works[0] != orig.work {
		t.Fatalf("version %+v, %v", v, err)
	}
}

// Files gone for 30 days are pruned, and then the works left with nothing:
// no version, no watch state and no play history. A season and its series
// are pruned once their last episode is.
func TestPruneAfterThirtyDays(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	watched := h.movie(folder, fsys, "A/a.mkv", "bytes")
	unwatched := h.movie(folder, fsys, "B/b.mkv", "other bytes")
	kept := h.movie(folder, fsys, "C/c.mkv", "still here")
	series := h.b.Series("Show")
	ep := h.b.Episode(folder, series, 1, 1)
	epWork, err := h.b.Catalog.Work(ctx, ep.Work)
	if err != nil {
		t.Fatal(err)
	}
	season := epWork.Parent
	user := id.New()
	h.exec(`INSERT INTO user (id, username, created_at) VALUES (?, 'u', 0)`, user)
	h.exec(`INSERT INTO user_state (user_id, work_id, played, updated_at) VALUES (?, ?, 1, 0)`, user, watched.work)
	for _, rel := range []string{"A/a.mkv", "B/b.mkv"} {
		fsys.remove(rel, time.Now())
	}
	h.mustScan(folder)
	h.b.Write(func(ctx context.Context, tx *store.Tx) error { return h.b.MarkGone(ctx, tx, ep.File) })
	if err := h.s.pruneGone(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.b.Catalog.File(ctx, watched.file); err != nil {
		t.Fatal("pruned too early")
	}
	h.exec(`UPDATE media_file SET gone_at = ? WHERE status = 'gone'`, time.Now().Add(-31*24*time.Hour).UnixMilli())
	if err := h.s.pruneGone(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.b.Catalog.File(ctx, watched.file); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("after 31 days: %v", err)
	}
	if _, err := h.b.Catalog.Work(ctx, kept.work); err != nil {
		t.Fatal("a work with a file was deleted")
	}
	// A root work whose files are all gone is pruned like any other work,
	// along with its watch state.
	var states int
	if err := h.b.DB.Reader().QueryRow(`SELECT count(*) FROM user_state WHERE work_id = ?`, watched.work).Scan(&states); err != nil || states != 0 {
		t.Fatalf("%d watch states of the purged movie left (%v)", states, err)
	}
	for name, w := range map[string]id.ID{"watched movie": watched.work, "unwatched movie": unwatched.work, "episode": ep.Work, "season": season, "series": series} {
		if _, err := h.b.Catalog.Work(ctx, w); !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("the %s left with nothing wasn't deleted: %v", name, err)
		}
	}
	if err := h.b.Catalog.CheckInvariants(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSubtitleFilesAttach(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	for _, p := range []string{
		"Dune/Dune.mkv", "Dune/Dune.en.srt", "Dune/Dune.eng.forced.srt", "Dune/Dune.en.sdh.srt", "Dune/Dune.default.de.ass",
		"Dune/Dune.English.srt", "Dune/Subs/2_French.srt", "Dune/Other.en.srt", "Dune/Dune.sub",
	} {
		fsys.write(p, "x", old)
	}
	h.mustScan(folder)
	f, _ := h.b.Catalog.File(ctx, h.files(folder)["Dune/Dune.mkv"].ID)
	got := map[string]catalog.SubtitleFile{}
	for _, x := range f.SubtitleFiles {
		got[x.RelPath] = x
	}
	want := map[string]catalog.SubtitleFile{
		"Dune/Dune.en.srt":         {Language: "en"},
		"Dune/Dune.eng.forced.srt": {Language: "en", Forced: true},
		"Dune/Dune.en.sdh.srt":     {Language: "en", HearingImpaired: true},
		"Dune/Dune.default.de.ass": {Language: "de", Default: true},
		"Dune/Dune.English.srt":    {Language: "en"},
		"Dune/Subs/2_French.srt":   {Language: "fr"},
	}
	if len(got) != len(want) {
		t.Fatalf("attached %v", got)
	}
	for p, w := range want {
		g := got[p]
		if g.Language != w.Language || g.Forced != w.Forced || g.HearingImpaired != w.HearingImpaired || g.Default != w.Default {
			t.Errorf("%s: %+v, want %+v", p, g, w)
		}
	}

	// A subtitle added to Subs/ alone, with the video's directory
	// unchanged, is still attached. Every other track keeps its ID.
	fsys.write("Dune/Subs/3_Spanish.srt", "x", time.Now())
	h.mustScan(folder)
	f, _ = h.b.Catalog.File(ctx, f.ID)
	if len(f.SubtitleFiles) != 7 {
		t.Fatalf("after adding to Subs/: %v", f.SubtitleFiles)
	}
	for _, x := range f.SubtitleFiles {
		if g, ok := got[x.RelPath]; ok && g.ID != x.ID {
			t.Errorf("%s: ID %v became %v", x.RelPath, g.ID, x.ID)
		}
	}

	// After one is removed, the rest keep their IDs.
	fsys.remove("Dune/Dune.en.srt", time.Now().Add(time.Second))
	h.mustScan(folder)
	f, _ = h.b.Catalog.File(ctx, f.ID)
	if len(f.SubtitleFiles) != 6 {
		t.Fatalf("after removing one: %v", f.SubtitleFiles)
	}
	for _, x := range f.SubtitleFiles {
		if g, ok := got[x.RelPath]; ok && g.ID != x.ID {
			t.Errorf("%s: ID %v became %v", x.RelPath, g.ID, x.ID)
		}
	}
}

// TestProbeRecordsAttachments checks that a probe records a container's
// attachments with their kinds, and that probing the file again keeps its
// tracks' IDs.
func TestProbeRecordsAttachments(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	mkv, err := os.ReadFile(filepath.Join("..", "media", "testdata", "attachments.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	fsys.write("A/a.mkv", string(mkv), old)
	h.mustScan(folder)
	h.probeAll()
	file := h.files(folder)["A/a.mkv"].ID
	before, _ := h.b.Catalog.File(ctx, file)
	if len(before.Attachments) != 3 || before.Attachments[0].Kind != catalog.AttachmentFont || before.Attachments[0].Filename != "font.ttf" ||
		len(before.Attachments[0].SHA256) != 64 || before.Attachments[1].Kind != catalog.AttachmentImage {
		t.Fatalf("attachments %+v", before.Attachments)
	}

	if err := h.s.probeFile(ctx, l, file); err != nil {
		t.Fatal(err)
	}
	after, _ := h.b.Catalog.File(ctx, file)
	if len(after.Attachments) != 3 || len(after.Tracks) != len(before.Tracks) {
		t.Fatalf("after probing again: %+v", after)
	}
	for i := range after.Tracks {
		if after.Tracks[i].ID != before.Tracks[i].ID {
			t.Errorf("track %d: ID %v became %v", i, before.Tracks[i].ID, after.Tracks[i].ID)
		}
	}
}

// A changed, added or removed NFO sends its directory's files back to be
// identified, and nothing else.
func TestChangedNFOReidentifiesTheFolder(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("Dune/Dune.mkv", "x", old)
	fsys.write("Dune/Dune - 4K.mkv", "y", old)
	fsys.write("Heat/Heat.mkv", "z", old)
	h.scanLibrary(l.ID)
	if got := h.unidentified(folder); len(got) != 0 {
		t.Fatalf("to identify %v after the scan", got)
	}
	files := h.files(folder)
	want := []id.ID{files["Dune/Dune - 4K.mkv"].ID, files["Dune/Dune.mkv"].ID}

	for _, change := range []func(){
		func() { fsys.write("Dune/movie.nfo", "<movie/>", time.Now().Add(-time.Hour)) },
		func() { fsys.write("Dune/movie.nfo", "<movie>2</movie>", time.Now().Add(-time.Minute*30)) },
		func() { fsys.remove("Dune/movie.nfo", time.Now().Add(-time.Minute*20)) },
	} {
		change()
		h.mustScan(folder)
		if got := h.unidentified(folder); !slices.Equal(got, want) {
			t.Fatalf("to identify %v after an NFO change, want Dune's files %v", got, want)
		}
		h.identifyAll()
	}
}

// A show's NFO and artwork sit in its top folder, above the Season folders
// that hold its videos. A change there sends one video below back to be
// identified.
func TestChangedNFOInAShowFolderReidentifiesIt(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Shows, "/tv")
	folder, fsys := l.Folders[0].ID, h.folders["/tv"]
	fsys.write("Firefly/Season 01/Firefly - S01E01.mkv", "a", old)
	fsys.write("Firefly/Season 01/Firefly - S01E02.mkv", "b", old)
	fsys.write("Other/Season 01/Other - S01E01.mkv", "c", old)
	h.mustScan(folder)
	h.probeAll()
	h.identifyAll()

	for _, change := range []func(){
		func() { fsys.write("Firefly/tvshow.nfo", "<tvshow/>", time.Now().Add(-time.Hour)) },
		func() { fsys.write("Firefly/poster.jpg", "jpeg", time.Now().Add(-time.Minute*30)) },
		func() { fsys.remove("Firefly/tvshow.nfo", time.Now().Add(-time.Minute*20)) },
	} {
		change()
		h.mustScan(folder)
		if p, want := h.unidentified(folder), h.files(folder)["Firefly/Season 01/Firefly - S01E01.mkv"].ID; len(p) != 1 || p[0] != want {
			t.Fatalf("to identify %v after a change in the show folder, want the first episode %v", p, want)
		}
		h.identifyAll()
	}
}

// fakeNotifier delivers events and errors a test sends.
type fakeNotifier struct {
	events chan fsnotify.Event
	errs   chan error
	added  []string
}

func (f *fakeNotifier) Add(p string) error            { f.added = append(f.added, p); return nil }
func (f *fakeNotifier) Events() <-chan fsnotify.Event { return f.events }
func (f *fakeNotifier) Errors() <-chan error          { return f.errs }
func (f *fakeNotifier) Close() error                  { return nil }

// The watcher runs only while the file watching setting is on, and starts
// and stops as soon as a saved change says so, with nothing polling.
func TestWatcherFollowsTheSetting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.library(catalog.Movies, "/m")
		set := func(v bool) {
			if err := h.settings.Update(ctx, func(s *config.Settings) error {
				s.FileWatching = v
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			h.s.FileWatchingChanged()
			synctest.Wait()
		}
		set(false)
		var opened int
		h.s.newNotifier = func() (notifier, error) {
			opened++
			return &fakeNotifier{events: make(chan fsnotify.Event), errs: make(chan error)}, nil
		}
		wctx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- h.s.Watch(wctx) }()
		defer func() { stop(); <-done }()

		time.Sleep(time.Minute)
		synctest.Wait()
		if opened != 0 {
			t.Fatalf("watching with the setting off")
		}
		set(true)
		if opened != 1 {
			t.Fatalf("watcher opened %d times once the setting is on", opened)
		}
		set(false)
		set(true)
		if opened != 2 {
			t.Fatalf("watcher opened %d times after off and on again, want 2", opened)
		}
	})
}

// A library added while the watcher runs, or a folder added to one, is
// watched at once, and its events scan it. A removed folder's events are
// ignored. None of this waits for the 5-minute resync.
func TestWatcherTakesUpLibraryChangesAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		fake := &fakeNotifier{events: make(chan fsnotify.Event), errs: make(chan error)}
		h.s.newNotifier = func() (notifier, error) { return fake, nil }
		wctx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- h.s.Watch(wctx) }()
		defer func() { stop(); <-done }()
		synctest.Wait()

		l := h.library(catalog.Movies, "/m")
		synctest.Wait()
		if !slices.Contains(fake.added, catalogtest.Abs("/m")) {
			t.Fatalf("watches %v after adding a library; want its folder", fake.added)
		}
		h.folders["/n"] = newMemFS()
		if err := h.s.EditLibrary(ctx, l.ID, catalog.LibraryChange{Folders: &[]string{catalogtest.Abs("/n")}}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if !slices.Contains(fake.added, catalogtest.Abs("/n")) {
			t.Fatalf("watches %v after adding a folder; want it", fake.added)
		}
		h.clearRequests()

		fake.events <- fsnotify.Event{Name: catalogtest.Abs("/n/new.mkv"), Op: fsnotify.Create}
		fake.events <- fsnotify.Event{Name: catalogtest.Abs("/m/gone.mkv"), Op: fsnotify.Create} // the dropped folder
		time.Sleep(quiet + time.Millisecond)
		synctest.Wait()
		lib, err := h.b.Catalog.Library(ctx, l.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := h.scanRequests(), []string{lib.Folders[0].ID.String() + " ."}; !slices.Equal(got, want) {
			t.Fatalf("pending scans %v; want only the new folder's, %v", got, want)
		}
		if got := h.tasks.of(TaskScanLibraries); !slices.Equal(got, []id.ID{l.ID}) {
			t.Fatalf("asked for %v; want the library", got)
		}
	})
}

// scanRequests lists the directories asked for, sorted, as folder and
// directory. The directory is "." for a whole folder.
func (h *harness) scanRequests() []string {
	h.s.mu.Lock()
	defer h.s.mu.Unlock()
	var out []string
	for _, req := range h.s.asked {
		for folder, dirs := range req.dirs {
			for _, d := range dirs {
				out = append(out, folder.String()+" "+d)
			}
		}
	}
	slices.Sort(out)
	return out
}

// clearRequests forgets what was asked for, as the scans that take it do.
func (h *harness) clearRequests() {
	h.s.mu.Lock()
	clear(h.s.asked)
	h.s.mu.Unlock()
	h.tasks.reset()
}

func TestWatcherScansEachDirectoryOnceQuiet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		l := h.library(catalog.Movies, "/m", "/n")
		m, n := l.Folders[0].ID, l.Folders[1].ID
		h.folders["/m"].write("A/a.mkv", "x", time.Now().Add(-time.Hour)) // the bubble's clock, not old's
		h.mustScan(m)
		h.clearRequests()

		fake := &fakeNotifier{events: make(chan fsnotify.Event), errs: make(chan error)}
		h.s.newNotifier = func() (notifier, error) { return fake, nil }
		wctx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- h.s.Watch(wctx) }()
		defer func() { stop(); <-done }()
		synctest.Wait()
		if !slices.Contains(fake.added, catalogtest.Abs("/m/A")) {
			t.Fatalf("watches %v; want the scanned directory, from scan_dir", fake.added)
		}

		// During a slow copy, every event restarts the directory's 5
		// seconds, with no limit, so nothing is requested while it goes on.
		for range 20 {
			fake.events <- fsnotify.Event{Name: catalogtest.Abs("/m/A/b.mkv"), Op: fsnotify.Write}
			time.Sleep(4 * time.Second)
		}
		synctest.Wait()
		if got := h.scanRequests(); len(got) != 0 {
			t.Fatalf("requested %v before the directory went quiet", got)
		}
		fake.events <- fsnotify.Event{Name: catalogtest.Abs("/m/.b.mkv.tmp"), Op: fsnotify.Create} // hidden: ignored
		time.Sleep(time.Second + time.Millisecond)
		synctest.Wait()
		if got, want := h.scanRequests(), []string{m.String() + " A"}; !slices.Equal(got, want) {
			t.Fatalf("pending scans %v; want the one directory, %v", got, want)
		}
		h.clearRequests()

		// .ignore is hidden, but it changes what the directory holds.
		fake.events <- fsnotify.Event{Name: catalogtest.Abs("/m/A/.ignore"), Op: fsnotify.Remove}
		time.Sleep(quiet + time.Millisecond)
		synctest.Wait()
		if got, want := h.scanRequests(), []string{m.String() + " A"}; !slices.Equal(got, want) {
			t.Fatalf("after .ignore was removed: %v, want %v", got, want)
		}
		h.clearRequests()

		// Each directory has its own wait. A folder's own entries scan the
		// folder.
		fake.events <- fsnotify.Event{Name: catalogtest.Abs("/m/A/c.mkv"), Op: fsnotify.Create}
		time.Sleep(3 * time.Second)
		fake.events <- fsnotify.Event{Name: catalogtest.Abs("/m/new.mkv"), Op: fsnotify.Create}
		time.Sleep(2*time.Second + time.Millisecond)
		synctest.Wait()
		if got, want := h.scanRequests(), []string{m.String() + " A"}; !slices.Equal(got, want) {
			t.Fatalf("after 5 seconds in A and 2 in the folder: %v, want %v", got, want)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if got, want := h.scanRequests(), []string{m.String() + " .", m.String() + " A"}; !slices.Equal(got, want) {
			t.Fatalf("pending scans %v; want %v", got, want)
		}
		h.clearRequests()

		// An overflow means events were lost, so every folder is rescanned.
		fake.errs <- fsnotify.ErrEventOverflow
		time.Sleep(quiet + time.Millisecond)
		synctest.Wait()
		if got := h.scanRequests(); len(got) != 2 || !slices.Contains(got, n.String()+" .") || !slices.Contains(got, m.String()+" .") {
			t.Fatalf("after an overflow: %v", got)
		}
	})
}

// A partial scan lists its directory, whatever its mtime, and what changed
// below it, and nothing else.
func TestPartialScan(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("A/a.mkv", "a", old)
	fsys.write("B/b.mkv", "b", old)
	h.mustScan(folder)
	h.probeAll()

	fsys.write("A/new.mkv", "new", time.Now()) // just written: recorded at once
	fsys.write("B/new.mkv", "new", time.Now())
	fsys.resetCounts()
	h.scanDir(folder, "A")
	files := h.files(folder)
	if !files["A/new.mkv"].Present || files["B/new.mkv"].Present || !files["B/b.mkv"].Present {
		t.Fatalf("after a partial scan of A: %v", files)
	}
	if lists, _ := fsys.counts(); lists != 1 {
		t.Fatalf("a partial scan of one directory listed %d", lists)
	}

	// A file written in place keeps its directory's mtime. The directory's
	// partial scan still finds it changed and probes it again.
	h.probeAll()
	setDirTime := func(at time.Time) {
		fsys.mu.Lock()
		fsys.nodes["A"].mtime = at
		fsys.mu.Unlock()
	}
	setDirTime(old)
	h.mustScan(folder) // records A's mtime, well before the listing
	fsys.write("A/a.mkv", "a, longer", time.Now())
	setDirTime(old)
	h.mustScan(folder)
	a := h.files(folder)["A/a.mkv"].ID
	if f, _ := h.b.Catalog.File(ctx, a); f.Generation != 1 {
		t.Fatalf("the folder's scan listed an unchanged directory: generation %d", f.Generation)
	}
	h.scanDir(folder, "A")
	if f, _ := h.b.Catalog.File(ctx, a); f.Generation != 2 {
		t.Fatalf("a file changed in place is at generation %d, want 2", f.Generation)
	}

	// A directory gone by the time its scan runs is left to its parent's.
	fsys.remove("B", time.Now())
	h.scanDir(folder, "B")
	if !h.files(folder)["B/b.mkv"].Present {
		t.Fatal("a partial scan of a removed directory marked its files gone")
	}
	h.mustScan(folder)
	if h.files(folder)["B/b.mkv"].Present {
		t.Fatal("the folder's scan missed the removed directory")
	}
}

// A directory asked for while a scan of the whole library waits leaves
// that scan whole.
func TestDirAskedDuringWholeLibraryScan(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m") // asks for the whole library
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("A/a.mkv", "a", old)
	fsys.write("B/b.mkv", "b", old)
	if err := h.s.RequestDir(ctx, folder, "A"); err != nil {
		t.Fatal(err)
	}
	h.scanLibrary(l.ID)
	if files := h.files(folder); !files["A/a.mkv"].Present || !files["B/b.mkv"].Present {
		t.Fatalf("after the library's scan: %v", files)
	}
}

// A folder containing an .ignore file is left out with everything below
// it. Adding or removing the file changes the folder, which is listed
// again.
func TestIgnoreFile(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("Keep (2020)/Keep.mkv", "k", old)
	fsys.write("Junk/a.mkv", "a", old)
	fsys.write("Junk/Deep/b.mkv", "b", old)
	h.mustScan(folder)
	if n := len(h.files(folder)); n != 3 {
		t.Fatalf("%d files before .ignore", n)
	}

	fsys.write("Junk/.ignore", "", time.Now().Add(-time.Minute))
	h.mustScan(folder)
	got := h.files(folder)
	if !got["Keep (2020)/Keep.mkv"].Present || got["Junk/a.mkv"].Present || got["Junk/Deep/b.mkv"].Present {
		t.Fatalf("with Junk/.ignore: %v", got)
	}
	fsys.write("Junk/Deep/c.mkv", "c", time.Now().Add(-time.Minute))
	h.mustScan(folder)
	if _, ok := h.files(folder)["Junk/Deep/c.mkv"]; ok {
		t.Fatal("a file added below an ignored folder was catalogued")
	}

	fsys.remove("Junk/.ignore", time.Now())
	h.mustScan(folder)
	got = h.files(folder)
	if !got["Junk/a.mkv"].Present || !got["Junk/Deep/b.mkv"].Present || !got["Junk/Deep/c.mkv"].Present {
		t.Fatalf("after removing .ignore: %v", got)
	}
}

// Linked directories are followed, but a directory already walked in the
// run isn't walked again, so a loop or a second path to a folder catalogues
// nothing twice. A full walk with no link leaving the folder clears the
// symlink problem.
func TestSymlinkedDirectoriesAreWalkedOnce(t *testing.T) {
	if _, ok := keyOf(must(os.Stat("."))); !ok {
		t.Skip("no device and inode on this platform")
	}
	h := newHarness(t)
	dir, outside := t.TempDir(), t.TempDir()
	l := h.library(catalog.Movies, dir)
	folder := l.Folders[0].ID
	h.s.opt.OpenFolder = catalog.OpenFolder
	for _, p := range []string{"Dune (2021)", "Shared/Heat (1995)"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for p, target := range map[string]string{
		"All":                     ".",           // a loop through the folder
		"Dune (2021)/Up":          "..",          // a loop below it
		"Linked":                  "Shared",      // a second path to a folder
		"Movies (2021)":           "Dune (2021)", // and another
		"Elsewhere":               outside,       // leaves the folder: skipped
		"Shared/Heat (1995)/Self": ".",
	} {
		if err := os.Symlink(target, filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"Dune (2021)/Dune.mkv", "Shared/Heat (1995)/Heat.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.mustScan(folder)
	var paths []string
	for p, f := range h.files(folder) {
		if f.Present {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	// Directories are walked in name order, so the first path to each
	// folder is the one kept.
	if want := []string{"Dune (2021)/Dune.mkv", "Linked/Heat (1995)/Heat.mkv"}; !slices.Equal(paths, want) {
		t.Fatalf("catalogued %v, want %v", paths, want)
	}
	if !slices.ContainsFunc(h.s.Health(ctx), func(p health.Problem) bool { return p.Code == "symlinkSkipped" }) {
		t.Fatal("the link leaving the folder wasn't reported")
	}

	if err := os.Remove(filepath.Join(dir, "Elsewhere")); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ScanLibraryFullWalk(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	h.mustScan(folder)
	if len(h.s.Health(ctx)) != 0 {
		t.Fatalf("after a clean full walk: %v", h.s.Health(ctx))
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestParseLanguage(t *testing.T) {
	for in, want := range map[string]string{"en": "en", "eng": "en", "pt-BR": "pt-BR", "French": "fr", "720p": "", "x264": "", "": ""} {
		if got := parseLanguage(in); got != want {
			t.Errorf("parseLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

// A library's scan probes only what its walk found new or changed, and
// records each file as probed at its generation. Then it identifies the
// library.
func TestLibraryScanProbesWhatChanged(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("Dune/Dune.mkv", "dune", old)
	fsys.write("Heat/Heat.mkv", "heat", old)
	h.scanLibrary(l.ID)
	if got := h.prob.took(); !slices.Equal(got, []string{"Dune.mkv", "Heat.mkv"}) {
		t.Fatalf("probed %v", got)
	}
	if got := h.unprobed(l.ID); len(got) != 0 {
		t.Fatalf("left to probe %v", got)
	}
	if !slices.Equal(h.identified, []id.ID{l.ID}) {
		t.Fatalf("identified %v; want the library once", h.identified)
	}

	fsys.write("Dune/Dune.mkv", "dune, remastered", old.Add(time.Hour))
	h.scanLibrary(l.ID)
	if got := h.prob.took(); !slices.Equal(got, []string{"Dune.mkv"}) {
		t.Fatalf("probed %v; want the changed file only", got)
	}
	dune := h.files(folder)["Dune/Dune.mkv"].ID
	var gen, probed int64
	if err := h.b.DB.Reader().QueryRow(`SELECT generation, probed FROM media_file WHERE id = ?`, dune).Scan(&gen, &probed); err != nil {
		t.Fatal(err)
	}
	if gen != 2 || probed != 2 {
		t.Fatalf("generation %d, probed %d; want both 2", gen, probed)
	}

	h.scanLibrary(l.ID)
	if got := h.prob.took(); len(got) != 0 {
		t.Fatalf("an unchanged scan probed %v", got)
	}
}

// A file the media package can't read is recorded as failed and listed as
// failed. Later scans leave it out. A retry forgets the failure and returns
// its library, whose next scan probes it again.
func TestCorruptFileIsLeftOutUntilRetried(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m")
	folder, fsys := l.Folders[0].ID, h.folders["/m"]
	fsys.write("Good/good.mkv", "good", old)
	fsys.write("Bad/bad.mkv", "bad", old)
	h.prob.bad = map[string]error{"bad.mkv": fmt.Errorf("probe: %w", media.ErrSourceCorrupt)}
	h.scanLibrary(l.ID)
	bad := h.files(folder)["Bad/bad.mkv"].ID
	failed, err := h.s.failed(ctx)
	if err != nil || len(failed) != 1 || failed[0].Item != bad || failed[0].Reason == "" {
		t.Fatalf("failed %+v, %v; want the corrupt file with why", failed, err)
	}
	if got := h.unprobed(l.ID); len(got) != 0 {
		t.Fatalf("left to probe %v; want the failure settled", got)
	}
	if got := h.unidentified(folder); len(got) != 0 {
		t.Fatalf("to identify %v after the scan", got)
	}
	h.prob.took()
	h.scanLibrary(l.ID)
	if got := h.prob.took(); len(got) != 0 {
		t.Fatalf("a later scan probed %v", got)
	}
	h.tasks.reset()
	if err := h.s.Resume(ctx); err != nil || len(h.tasks.of(TaskScanLibraries)) != 0 {
		t.Fatalf("Resume asked for %v, %v; a failed file isn't behind", h.tasks.of(TaskScanLibraries), err)
	}

	h.prob.bad = nil
	libs, err := h.s.retry(ctx)
	if err != nil || !slices.Equal(libs, []id.ID{l.ID}) {
		t.Fatalf("retry returned %v, %v; want the file's library", libs, err)
	}
	if failed, _ := h.s.failed(ctx); len(failed) != 0 {
		t.Fatalf("failed %+v after the retry", failed)
	}
	if got := h.unprobed(l.ID); !slices.Equal(got, []id.ID{bad}) {
		t.Fatalf("left to probe %v; want the retried file", got)
	}
	h.scanLibrary(l.ID)
	if got := h.prob.took(); !slices.Equal(got, []string{"bad.mkv"}) {
		t.Fatalf("probed %v after the retry", got)
	}
	if f, _ := h.b.Catalog.File(ctx, bad); len(f.Tracks) != 2 {
		t.Fatalf("the retried file: %+v", f)
	}
}

// RequestDir asks for the library's scan, which walks only the directories
// asked for. A scan asked for as a whole walks every folder.
func TestRequestDirScansOnlyThatDirectory(t *testing.T) {
	h := newHarness(t)
	l := h.library(catalog.Movies, "/m", "/n")
	m, n := l.Folders[0].ID, l.Folders[1].ID
	h.folders["/m"].write("A/a.mkv", "a", old)
	h.folders["/m"].write("B/b.mkv", "b", old)
	h.folders["/n"].write("C/c.mkv", "c", old)
	h.scanLibrary(l.ID)
	h.tasks.reset()

	h.folders["/m"].write("A/new.mkv", "new a", time.Now())
	h.folders["/m"].write("B/new.mkv", "new b", time.Now())
	h.folders["/n"].write("C/new.mkv", "new c", time.Now())
	if err := h.s.RequestDir(ctx, m, "A"); err != nil {
		t.Fatal(err)
	}
	if got := h.tasks.of(TaskScanLibraries); !slices.Equal(got, []id.ID{l.ID}) {
		t.Fatalf("asked for %v; want the folder's library", got)
	}
	h.folders["/m"].resetCounts()
	h.scanLibrary(l.ID)
	if lists, _ := h.folders["/m"].counts(); lists != 1 {
		t.Fatalf("the scan of one directory listed %d", lists)
	}
	if !h.files(m)["A/new.mkv"].Present || h.files(m)["B/new.mkv"].Present || h.files(n)["C/new.mkv"].Present {
		t.Fatalf("after a scan of A: %v %v", h.files(m), h.files(n))
	}
	if got := h.unprobed(l.ID); len(got) != 0 {
		t.Fatalf("left to probe %v; want A's new file probed", got)
	}

	if err := h.s.ScanLibrary(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	h.scanLibrary(l.ID)
	if !h.files(m)["B/new.mkv"].Present || !h.files(n)["C/new.mkv"].Present {
		t.Fatalf("after a scan of the library: %v %v", h.files(m), h.files(n))
	}
	if err := h.s.RequestDir(ctx, id.New(), "A"); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("a directory of no folder: %v", err)
	}
}

// Resume asks for the libraries with files a scan didn't finish with,
// meaning files not probed at their generation. Files probed but not
// identified are the identify task's to find.
func TestResumeAsksForLibrariesLeftBehind(t *testing.T) {
	h := newHarness(t)
	a := h.library(catalog.Movies, "/a")
	b := h.library(catalog.Movies, "/b")
	h.folders["/a"].write("A/a.mkv", "a", old)
	h.folders["/b"].write("B/b.mkv", "b", old)
	h.scanLibrary(a.ID)
	h.scanLibrary(b.ID)
	h.tasks.reset()
	if err := h.s.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.tasks.of(TaskScanLibraries); len(got) != 0 {
		t.Fatalf("asked for %v with nothing behind", got)
	}

	// A walk cut off before its probes.
	h.folders["/b"].write("B/new.mkv", "new", old)
	h.mustScan(b.Folders[0].ID)
	if err := h.s.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.tasks.of(TaskScanLibraries); !slices.Equal(got, []id.ID{b.ID}) {
		t.Fatalf("asked for %v; want the library left unprobed", got)
	}

	// This file was probed, but the scan was cut off before it was
	// identified.
	h.tasks.reset()
	h.probeAll()
	if err := h.s.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if got := h.tasks.of(TaskScanLibraries); len(got) != 0 {
		t.Fatalf("asked for %v; want no scan for a file only left unidentified", got)
	}
}

// An unreachable folder's library is asked for again an hour later.
func TestUnreachableFolderIsScannedAgainInAnHour(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		l := h.library(catalog.Movies, "/m")
		h.folders["/m"].write("A/a.mkv", "a", time.Now().Add(-time.Hour)) // the bubble's clock, not old's
		h.scanLibrary(l.ID)
		h.tasks.reset()

		h.down["/m"] = true
		h.scanLibrary(l.ID)
		if !h.files(l.Folders[0].ID)["A/a.mkv"].Present {
			t.Fatal("a file under an unreachable folder was marked gone")
		}
		time.Sleep(time.Hour - time.Second)
		synctest.Wait()
		if got := h.tasks.of(TaskScanLibraries); len(got) != 0 {
			t.Fatalf("asked for %v within the hour", got)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := h.tasks.of(TaskScanLibraries); !slices.Equal(got, []id.ID{l.ID}) {
			t.Fatalf("asked for %v after an hour; want the library", got)
		}
	})
}
