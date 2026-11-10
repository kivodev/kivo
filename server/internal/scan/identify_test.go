package scan

import (
	"context"
	"io/fs"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/catalog/catalogtest"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate"
)

// flow is a scanner and the identifier over one database and one library
// folder in memory, with their tasks on a task runner, as the server wires
// them. start begins a server's life and stop ends it, so a test can
// restart it.
type flow struct {
	t    *testing.T
	b    *catalogtest.Builder
	fsys *memFS
	prob *fakeProber
	find *noMatches

	r    *tasks.Runner
	s    *Scanner
	id   *identify.Identifier
	asks *fakeTasks // what was asked of the runner
	stop func()
}

func newFlow(t *testing.T) *flow {
	return &flow{t: t, b: catalogtest.New(t), fsys: newMemFS(), prob: &fakeProber{}, find: &noMatches{}}
}

// wire builds the services over a new runner. When lost is set, what they
// ask of the runner is dropped, as it is when the server stops before the
// runner gets to it.
func (f *flow) wire(lost bool) {
	f.t.Helper()
	db := f.b.DB
	settings, err := config.Open(ctx, db, config.Options{})
	if err != nil {
		f.t.Fatal(err)
	}
	f.r = tasks.New(db, settings, events.NewBus(), tasks.Options{})
	f.asks = &fakeTasks{}
	var ask Tasks = asking{f.asks, f.r}
	if lost {
		ask = f.asks
	}
	// Only the scanner reads fsys, so its counts are the scans' alone.
	// Identify looks for NFO files in a folder that has none.
	open := func(string) (fs.FS, func() error, error) { return f.fsys, func() error { return nil }, nil }
	nfos := func(string) (fs.FS, func() error, error) { return fstest.MapFS{}, func() error { return nil }, nil }
	meta := metadata.New(db, metadata.Deps{Catalog: f.b.Catalog, Settings: settings, Events: events.NewBus(), Tasks: ask, CacheDir: f.t.TempDir(), ImageDir: f.t.TempDir()},
		metadata.Options{Video: f.find})
	act := activity.New(db, events.NewBus())
	state := userstate.New(db, userstate.Deps{Catalog: f.b.Catalog, Settings: settings, Activity: act, Events: events.NewBus()})
	f.id = identify.New(db, identify.Deps{Catalog: f.b.Catalog, Metadata: meta, Userstate: state, Activity: act, Tasks: ask, Markers: noMarkers{}},
		identify.Options{OpenFolder: nfos})
	f.s = New(db, Deps{
		Catalog: f.b.Catalog, Tasks: ask, Prober: f.prob, Settings: settings, Identifier: f.id, Previews: noPreviews{}, Subtitles: noSubtitles{},
	}, Options{OpenFolder: open})
	for _, t := range slices.Concat(f.s.Tasks(), f.id.Tasks(), meta.Tasks()) {
		f.r.Add(t)
	}
}

// start wires a server and runs it.
func (f *flow) start() {
	f.t.Helper()
	f.wire(false)
	f.run()
}

// run resumes the scans cut off and runs the tasks, as the server does
// when it starts.
func (f *flow) run() {
	f.t.Helper()
	if err := f.s.Resume(ctx); err != nil {
		f.t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { _ = f.r.Run(runCtx); close(done) }()
	var once sync.Once
	f.stop = func() { once.Do(func() { cancel(); <-done }) }
	f.t.Cleanup(f.stop)
}

// asking records what is asked of the runner and passes it on.
type asking struct {
	*fakeTasks
	r *tasks.Runner
}

func (a asking) Enqueue(task string, items ...id.ID) {
	a.fakeTasks.Enqueue(task, items...)
	a.r.Enqueue(task, items...)
}

type noMarkers struct{}

func (noMarkers) Placed(context.Context, *store.Tx, catalog.Library, []id.ID) error { return nil }

// noMatches is a video provider that lists nothing, so every movie stays
// unmatched. It counts the searches.
type noMatches struct {
	mu       sync.Mutex
	searches int
}

func (*noMatches) Name() string { return "tmdb" }

func (n *noMatches) Search(context.Context, catalog.WorkKind, metadata.Query, string) ([]metadata.Match, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.searches++
	return nil, nil
}

func (n *noMatches) searched() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.searches
}

func (*noMatches) Resolve(context.Context, catalog.ExternalID, string) (metadata.Match, error) {
	return metadata.Match{}, metadata.ErrNotFound
}

func (*noMatches) Describe(context.Context, metadata.Ref, string, metadata.Freshness) (*metadata.Description, error) {
	return nil, metadata.ErrNotFound
}

func (*noMatches) Images(context.Context, metadata.Ref, string) (metadata.Images, error) {
	return metadata.Images{}, metadata.ErrNotFound
}

func (*noMatches) Episodes(context.Context, catalog.ExternalID, metadata.Ordering, string, metadata.Freshness) ([]metadata.Episode, error) {
	return nil, metadata.ErrNotFound
}

func (*noMatches) EpisodeGroups(context.Context, catalog.ExternalID, string) ([]string, error) {
	return nil, metadata.ErrNotFound
}

func (*noMatches) Owns(string) bool { return false }

// wait waits until cond holds.
func (f *flow) wait(what string, cond func() bool) {
	f.t.Helper()
	for deadline := time.Now().Add(20 * time.Second); !cond(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting until %s", what)
		}
	}
}

// pending lists, by path, the files that are probed but not identified.
func (f *flow) pending() []string {
	f.t.Helper()
	rows, err := f.b.DB.Reader().Query(`SELECT rel_path FROM media_file WHERE status = 'present'
		AND probed = generation AND probe_error IS NULL AND identified IS NOT generation ORDER BY rel_path`)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, rel)
	}
	return out
}

// works counts the works the library's files were placed on.
func (f *flow) works() int {
	f.t.Helper()
	var n int
	if err := f.b.DB.Reader().QueryRow(`SELECT count(DISTINCT vw.work_id) FROM media_file f JOIN media_version_work vw ON vw.version_id = f.version_id`).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// identifyTask is the identify task's row of the Tasks page.
func (f *flow) identifyTask() tasks.Status {
	f.t.Helper()
	all, err := f.r.Tasks(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return all[slices.IndexFunc(all, func(s tasks.Status) bool { return s.Name == identify.TaskIdentifyMedia })]
}

// A scan hands the files it probed to the identify task, which identifies
// them with nothing else asking. Refreshing one unmatched work then
// identifies its folder again without a scan of the library: nothing is
// listed, opened or probed.
func TestScanHandsOffToIdentify(t *testing.T) {
	f := newFlow(t)
	f.fsys.write("Arrival (2016)/Arrival (2016).mkv", "arrival", old)
	f.fsys.write("Arrival (2016)/Arrival (2016) - 4K.mkv", "arrival in 4K", old)
	f.fsys.write("Heat (1995)/Heat (1995).mkv", "heat", old)
	f.start()
	lib, err := f.s.AddLibrary(ctx, catalog.LibraryInput{Name: "Movies", Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/m")}})
	if err != nil {
		t.Fatal(err)
	}
	f.wait("the scan's files are identified", func() bool { return f.works() == 2 && len(f.pending()) == 0 })
	if got := f.asks.of(identify.TaskIdentifyMedia); len(got) != 2 {
		t.Fatalf("the scan asked to identify %v, want one file for each folder", got)
	}

	var arrival id.ID
	if err := f.b.DB.Reader().QueryRow(`SELECT w.id FROM work w WHERE w.title = 'Arrival'`).Scan(&arrival); err != nil {
		t.Fatal(err)
	}
	svc := users.New(f.b.DB, events.NewBus())
	var admin users.User
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		admin, err = svc.CreateUser(ctx, tx, users.NewUser{Username: "admin", Admin: true, Grants: users.Grants{All: true}})
		return err
	})
	viewer, err := svc.Viewer(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.asks.reset()
	f.fsys.resetCounts()
	f.prob.took()
	searched := f.find.searched()
	if err := f.id.Refresh(ctx, viewer, arrival); err != nil {
		t.Fatal(err)
	}
	f.wait("the folder is searched for again", func() bool { return f.find.searched() > searched && len(f.pending()) == 0 })
	f.stop()
	if got := f.asks.of(identify.TaskIdentifyMedia); len(got) != 1 {
		t.Errorf("the refresh asked to identify %v, want one file of the work's folder", got)
	}
	if got := f.asks.of(TaskScanLibraries); len(got) != 0 {
		t.Errorf("the refresh asked for a scan of %v (the library is %v)", got, lib)
	}
	if lists, opens := f.fsys.counts(); lists != 0 || opens != 0 {
		t.Errorf("the refresh listed %d directories and opened %d files", lists, opens)
	}
	if probed := f.prob.took(); len(probed) != 0 {
		t.Errorf("the refresh probed %v", probed)
	}
}

// Files left probed but not identified when the server stops, and files
// sent back to be identified, are identified as the next server starts,
// without a scan. The task's row counts them until then.
func TestIdentifyResumesAfterRestart(t *testing.T) {
	f := newFlow(t)
	f.fsys.write("Arrival (2016)/Arrival (2016).mkv", "arrival", old)
	f.start()
	lib, err := f.s.AddLibrary(ctx, catalog.LibraryInput{Name: "Movies", Kind: catalog.Movies, Folders: []string{catalogtest.Abs("/m")}})
	if err != nil {
		t.Fatal(err)
	}
	f.wait("the first scan's file is identified", func() bool { return f.works() == 1 && len(f.pending()) == 0 })
	f.stop()

	// A scan probes a new file, and the server stops before the identify
	// task is asked. The first file is sent back as well.
	f.fsys.write("Heat (1995)/Heat (1995).mkv", "heat", old.Add(time.Hour))
	f.wire(true)
	if err := f.s.scan(ctx, lib); err != nil {
		t.Fatal(err)
	}
	f.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE media_file SET identified = NULL`)
		return err
	})
	want := []string{"Arrival (2016)/Arrival (2016).mkv", "Heat (1995)/Heat (1995).mkv"}
	if got := f.pending(); !slices.Equal(got, want) {
		t.Fatalf("pending %v, want %v", got, want)
	}

	f.fsys.resetCounts()
	f.wire(false)
	if row := f.identifyTask(); row.Section() != tasks.SectionOnDemand || row.PendingCount != 2 {
		t.Errorf("the task's row as the server starts: section %s, %d pending", row.Section(), row.PendingCount)
	}
	f.run()
	f.wait("both files are identified", func() bool { return f.works() == 2 && len(f.pending()) == 0 })
	f.stop()
	if got := f.asks.of(TaskScanLibraries); len(got) != 0 {
		t.Errorf("the restart asked for a scan of %v", got)
	}
	if lists, _ := f.fsys.counts(); lists != 0 {
		t.Errorf("the restart listed %d directories", lists)
	}
	// A row's count is reused for a while, so a new runner reads it.
	f.wire(false)
	if row := f.identifyTask(); row.PendingCount != 0 || row.Running {
		t.Errorf("the task's row once all is identified: %d pending, running %v", row.PendingCount, row.Running)
	}
}
