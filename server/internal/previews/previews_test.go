package previews

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/tasks"
)

var ctx = context.Background()

// fakeMedia writes the sheets ffmpeg would, and records what it was asked.
type fakeMedia struct {
	mu       sync.Mutex
	err      error  // returned by every call
	during   func() // run in the middle of every call
	previews []media.PreviewsRequest
}

func (m *fakeMedia) Previews(_ context.Context, req media.PreviewsRequest) (media.Previews, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.previews = append(m.previews, req)
	if m.during != nil {
		m.during()
	}
	if m.err != nil {
		return media.Previews{}, m.err
	}
	p := media.Previews{Interval: req.Interval, Width: req.Width, Height: 180, Columns: req.Columns, Rows: req.Rows, Sheets: 2}
	for _, name := range p.SheetNames() {
		if err := os.WriteFile(filepath.Join(req.Dir, name), []byte("RIFF"), 0o644); err != nil {
			return p, err
		}
	}
	return p, nil
}

// ask is one call of Enqueue.
type ask struct {
	task  string
	items []id.ID
}

// fakeTasks records the items asked for.
type fakeTasks struct {
	mu    sync.Mutex
	asked []ask
}

func (t *fakeTasks) Enqueue(task string, items ...id.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.asked = append(t.asked, ask{task, items})
}

func (t *fakeTasks) take() []ask {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.asked
	t.asked = nil
	return out
}

type fixture struct {
	t        *testing.T
	b        *catalogtest.Builder
	settings *config.Store
	m        *fakeMedia
	tk       *fakeTasks
	s        *Service
	cache    string
	lib      id.ID
	folderID id.ID
	file     id.ID
	changes  int
}

// newFixture makes the service over a library of one probed video file.
func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, b: catalogtest.New(t), m: &fakeMedia{}, tk: &fakeTasks{}, cache: t.TempDir()}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := f.b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Films", Kind: catalog.Movies, Folders: []string{t.TempDir()}, Previews: catalog.Nightly})
		f.lib, f.folderID = l.ID, l.Folders[0].ID
		return err
	})
	f.file = f.add("a.mkv")
	settings, err := config.Open(ctx, f.b.DB, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f.settings = settings
	f.s = New(f.b.DB, Deps{Catalog: f.b.Catalog, Media: f.m, Tasks: f.tk, CacheDir: f.cache})
	f.s.room.Free = func(string) (uint64, error) { return 100 << 30, nil }
	return f
}

// add adds a file at rel in the fixture's folder, probed with a video
// track.
func (f *fixture) add(rel string) id.ID {
	f.t.Helper()
	var file id.ID
	var gen int64
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		st, err := f.b.Catalog.UpsertFiles(ctx, tx, f.folderID, []catalog.FileObservation{{RelPath: rel, Size: 100, MTime: time.UnixMilli(1)}})
		if err == nil {
			file, gen = st[0].ID, st[0].Generation
		}
		return err
	})
	f.probe(file, gen)
	return file
}

// probe records a file probed at gen, with a video track.
func (f *fixture) probe(file id.ID, gen int64) {
	f.t.Helper()
	info, _ := json.Marshal(media.Track{Index: 0, Kind: media.Video, Codec: media.H264, Width: 64, Height: 64, BitDepth: 8})
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return f.b.Catalog.SetProbe(ctx, tx, file, gen, catalog.Probe{
			Summary: catalog.ProbeSummary{Container: "matroska,webm", Duration: 8 * time.Second},
			Tracks:  []catalog.Track{{Index: 0, Type: string(media.Video), Codec: string(media.H264), Info: info}},
		})
	})
}

// change makes the scan see the fixture's file as changed, which bumps its
// generation, and probes it again.
func (f *fixture) change() {
	f.t.Helper()
	f.changes++
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := f.b.Catalog.UpsertFiles(ctx, tx, f.folderID, []catalog.FileObservation{{RelPath: "a.mkv", Size: 1, MTime: time.UnixMilli(int64(1 + f.changes))}})
		return err
	})
	f.probe(f.file, f.gen())
}

func (f *fixture) gen() int64 {
	f.t.Helper()
	cur, err := f.s.deps.Catalog.Generation(ctx, f.file)
	if err != nil {
		f.t.Fatal(err)
	}
	return cur
}

func (f *fixture) run() error {
	return f.s.make(ctx, f.file)
}

// pending is what the make task has left.
func (f *fixture) pending() []id.ID {
	f.t.Helper()
	files, err := f.s.q.Pending(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return files
}

// failed is the files whose previews failed.
func (f *fixture) failed() []tasks.Failure {
	f.t.Helper()
	out, err := f.s.failed(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

// When a file changes while it is read, the result is dropped and nothing
// is recorded. The file's current generation is pending.
func TestStaleResultsAreDropped(t *testing.T) {
	f := newFixture(t)
	f.m.during = f.change
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if f.s.results.Has(f.file, f.gen()) || f.s.results.Has(f.file, f.gen()-1) {
		t.Fatal("a stale result was kept")
	}
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
		t.Fatalf("pending %v", p)
	}
}

func TestPreviews(t *testing.T) {
	f := newFixture(t)
	if _, _, ok := f.s.Previews(ctx, f.file); ok {
		t.Fatal("previews before any were made")
	}
	if a := f.tk.take(); len(a) != 0 {
		t.Fatalf("a missing preview asked for %+v: that's the nightly task's", a)
	}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	req := f.m.previews[0]
	if req.Interval != 10*time.Second || req.Width != 320 || req.Columns != 10 || req.Source.Width != 64 {
		t.Fatalf("asked for %+v", req)
	}
	p, dir, ok := f.s.Previews(ctx, f.file)
	if !ok || p.Sheets != 2 || p.Height != 180 {
		t.Fatalf("previews %+v %v", p, ok)
	}
	for _, name := range p.SheetNames() {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Error(err)
		}
	}
	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v after they were made", p)
	}
}

func TestPending(t *testing.T) {
	f := newFixture(t)
	other := f.add("b.mkv")
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	var never id.ID
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		l, err := f.b.Catalog.CreateLibrary(ctx, tx, catalog.LibraryInput{Name: "Home videos", Kind: catalog.Videos, Folders: []string{t.TempDir()}, Previews: catalog.Never})
		never = l.Folders[0].ID
		return err
	})
	f.folderID = never
	f.add("c.mkv")

	// Only the file missing previews, in a library that makes them, is
	// pending.
	if p := f.pending(); !slices.Equal(p, []id.ID{other}) {
		t.Fatalf("pending %v, want %v", p, other)
	}

	// A library set to Never makes no previews, whatever asked for them.
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, _, err := f.b.Catalog.UpdateLibrary(ctx, tx, f.lib, catalog.LibraryChange{Previews: new(catalog.Never)})
		return err
	})
	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v in a library set to never", p)
	}
	f.file = other
	if err := f.run(); err != nil || len(f.m.previews) != 1 {
		t.Fatalf("made previews in a library set to never: %v", err)
	}
}

// A file media can't read is recorded as failed at its generation. It
// leaves the pending list until it changes or its failure is retried.
func TestUnreadableFilesFail(t *testing.T) {
	for _, kind := range []error{media.ErrSourceCorrupt, media.ErrUnsupported} {
		t.Run(kind.Error(), func(t *testing.T) {
			f := newFixture(t)
			f.m.err = &media.Error{Op: "x", Kind: kind}
			if err := f.run(); err != nil {
				t.Fatal(err)
			}
			if p := f.pending(); len(p) != 0 {
				t.Fatalf("pending %v after it failed", p)
			}
			if fl := f.failed(); len(fl) != 1 || fl[0].Item != f.file || fl[0].Reason == "" || fl[0].At.IsZero() {
				t.Fatalf("failed %+v", fl)
			}
			if _, _, ok := f.s.Previews(ctx, f.file); ok {
				t.Error("previews of a file media can't read")
			}

			// A retry forgets the failure, so the file is pending again.
			again, err := f.s.retry(ctx, f.file)
			if err != nil || !slices.Equal(again, []id.ID{f.file}) {
				t.Fatalf("retry %v %v", again, err)
			}
			if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) || len(f.failed()) != 0 {
				t.Fatalf("pending %v, failed %+v after a retry", p, f.failed())
			}

			// After it fails again, a retry with no items retries every
			// failure.
			if err := f.run(); err != nil {
				t.Fatal(err)
			}
			if again, err := f.s.retry(ctx); err != nil || !slices.Equal(again, []id.ID{f.file}) {
				t.Fatalf("retry of every failure: %v %v", again, err)
			}

			// When the failure was at an earlier generation, the file's
			// current generation is pending and no failure is listed.
			if err := f.run(); err != nil {
				t.Fatal(err)
			}
			f.change()
			if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) || len(f.failed()) != 0 {
				t.Fatalf("changed: pending %v, failed %+v", p, f.failed())
			}
		})
	}
}

// A retry forgets the failures of the files it's given. It forgets nothing
// about a file whose previews were made.
func TestRetry(t *testing.T) {
	f := newFixture(t)
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	made := f.file
	f.m.err = &media.Error{Op: "x", Kind: media.ErrSourceCorrupt}
	var bad []id.ID
	for _, rel := range []string{"b.mkv", "c.mkv", "d.mkv"} {
		f.file = f.add(rel)
		bad = append(bad, f.file)
		if err := f.run(); err != nil {
			t.Fatal(err)
		}
	}
	again, err := f.s.retry(ctx, made, bad[0], bad[1])
	slices.SortFunc(again, func(a, b id.ID) int { return bytes.Compare(a[:], b[:]) })
	if err != nil || !slices.Equal(again, bad[:2]) {
		t.Fatalf("retry %v %v, want %v", again, err, bad[:2])
	}
	if fl := f.failed(); len(fl) != 1 || fl[0].Item != bad[2] {
		t.Fatalf("failed %+v, want %v", fl, bad[2])
	}
	if p := f.pending(); !slices.Equal(p, bad[:2]) {
		t.Fatalf("pending %v, want %v", p, bad[:2])
	}
}

func TestLeft(t *testing.T) {
	f := newFixture(t)
	f.add("b.mkv")
	if left := len(f.pending()); left != 2 {
		t.Fatalf("before: %d left", left)
	}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if left := len(f.pending()); left != 1 {
		t.Fatalf("after one: %d left", left)
	}
}

// Previews made before they were recorded get recorded when the task
// reaches them, without reading the file again.
func TestMadeAreRecorded(t *testing.T) {
	f := newFixture(t)
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM preview_file`)
		return err
	})
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
		t.Fatalf("pending %v", p)
	}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if p := f.pending(); len(f.m.previews) != 1 || len(p) != 0 {
		t.Fatalf("read %d times, pending %v", len(f.m.previews), p)
	}
}

// In a library that makes previews as media is added, a probed file's
// previews are asked for once the probe's transaction commits. In a
// nightly-only library they aren't.
func TestProbed(t *testing.T) {
	f := newFixture(t)
	probed := func(sched catalog.Schedule, fail error) error {
		return f.b.DB.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			lib := catalog.Library{ID: f.lib, Previews: sched}
			f.s.Probed(tx, lib, f.file)
			if a := f.tk.take(); len(a) != 0 {
				t.Errorf("asked for %+v before commit", a)
			}
			return fail
		})
	}
	if err := probed(catalog.Nightly, nil); err != nil {
		t.Fatal(err)
	}
	if a := f.tk.take(); len(a) != 0 {
		t.Fatalf("nightly: asked for %+v", a)
	}
	if err := probed(catalog.NightlyAndAdded, errors.New("rolled back")); err == nil {
		t.Fatal("no error")
	}
	if a := f.tk.take(); len(a) != 0 {
		t.Fatalf("rolled back: asked for %+v", a)
	}
	if err := probed(catalog.NightlyAndAdded, nil); err != nil {
		t.Fatal(err)
	}
	if a := f.tk.take(); len(a) != 1 || a[0].task != TaskMakePreviews || !slices.Equal(a[0].items, []id.ID{f.file}) {
		t.Fatalf("asked for %+v", a)
	}
}

// Through the real runner, previews asked for are made at once. Run now on
// the Make preview thumbnails task makes the previews that files are
// missing.
func TestMakePreviewsRunNow(t *testing.T) {
	f := newFixture(t)
	r := tasks.New(f.b.DB, f.settings, events.NewBus(), tasks.Options{})
	f.s.deps.Tasks = r
	for _, tk := range f.s.Tasks() {
		r.Add(tk)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.Run(runCtx)
	}()
	defer func() { stop(); <-done }()
	made := func(file id.ID) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if _, _, ok := f.s.Previews(ctx, file); ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("no previews made")
			}
		}
	}
	if err := r.Start(ctx, TaskMakePreviews); err != nil {
		t.Fatal(err)
	}
	made(f.file)
}

// Deleting cache/ loses nothing for good. The prune forgets the previews
// recorded as made, so the nightly task finds them missing.
func TestDeletedCacheIsRebuilt(t *testing.T) {
	f := newFixture(t)
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(f.cache); err != nil {
		t.Fatal(err)
	}
	if err := f.s.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
		t.Fatalf("pending %v", p)
	}
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := f.s.Previews(ctx, f.file); !ok {
		t.Error("previews not rebuilt")
	}
}

func TestPrune(t *testing.T) {
	f := newFixture(t)
	if err := f.run(); err != nil {
		t.Fatal(err)
	}
	current := f.s.results.Path(f.file, f.gen())
	old := f.s.results.Path(f.file, f.gen()+7)
	deleted := f.s.results.Path(id.New(), 1)
	for _, d := range []string{old, deleted} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.prune(ctx); err != nil {
		t.Fatal(err)
	}
	for d, keep := range map[string]bool{current: true, old: false, deleted: false, filepath.Dir(deleted): false} {
		if _, err := os.Stat(d); (err == nil) != keep {
			t.Errorf("%s: kept %v, want %v", d, err == nil, keep)
		}
	}
	if p := f.pending(); len(p) != 0 {
		t.Fatalf("pending %v: the prune forgot previews still there", p)
	}
}

func TestFullCacheDisk(t *testing.T) {
	f := newFixture(t)
	f.s.room.Free = func(string) (uint64, error) { return 10 << 20, nil }
	err := f.run()
	if !errors.Is(err, health.ErrDiskFull) || len(f.m.previews) != 0 || len(f.s.Health(ctx)) != 1 {
		t.Fatalf("on a full disk: %v, %d runs, problems %+v", err, len(f.m.previews), f.s.Health(ctx))
	}
	if p := f.pending(); !slices.Equal(p, []id.ID{f.file}) {
		t.Fatalf("pending %v on a full disk", p)
	}
	f.s.room.Free = func(string) (uint64, error) { return 10 << 30, nil }
	if err := f.run(); err != nil || len(f.s.Health(ctx)) != 0 {
		t.Fatalf("with room again: %v %+v", err, f.s.Health(ctx))
	}
}
