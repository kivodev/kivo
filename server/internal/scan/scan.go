package scan

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"sync"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/scan/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// Scanner reconciles library folders into the catalog.
type Scanner struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps
	opt  Options

	newNotifier     func() (notifier, error)
	watchingChanged chan struct{} // buffered 1; signals that the file watching setting was saved
	foldersChanged  chan struct{} // buffered 1; signals that a library's folders were added or removed

	mu       sync.Mutex
	problems map[string]health.Problem // keyed by code plus folder
	asked    map[id.ID]*request        // what each library's next scan covers
}

// request is what a library's next scan covers: every folder, or only the
// directories the watcher saw change, keyed by folder.
type request struct {
	all  bool
	dirs map[id.ID][]string
}

// Options configures a Scanner.
type Options struct {
	// OpenFolder opens a library folder for one run. The default is
	// catalog.OpenFolder.
	OpenFolder func(path string) (fs.FS, func() error, error)
}

// Tasks enqueues items to be done now. tasks.Runner implements it.
type Tasks interface {
	Enqueue(task string, items ...id.ID)
}

// Identifier identifies the folders of files that are probed but not yet
// identified. Scanned asks for a library's to be identified now. Reidentify
// marks files' folders, in the caller's transaction, to be identified
// again. identify.Identifier implements it.
type Identifier interface {
	Scanned(ctx context.Context, lib catalog.Library) error
	Reidentify(ctx context.Context, tx *store.Tx, files []id.ID) error
}

// Prober probes media files.
type Prober interface {
	Probe(ctx context.Context, path string) (media.ProbeResult, error)
}

// Previews asks for a probed file's preview thumbnails, in the write tx
// that stores its probe. *previews.Service implements it.
type Previews interface {
	Probed(tx *store.Tx, lib catalog.Library, file id.ID)
}

// Subtitles asks for a probed file's subtitles to be prepared, in the
// write tx that stores its probe. *subtitles.Service implements it.
type Subtitles interface {
	Probed(tx *store.Tx, lib catalog.Library, file id.ID, tracks []catalog.Track)
}

// Deps are a Scanner's collaborators.
type Deps struct {
	Catalog *catalog.Catalog
	Tasks   Tasks
	Prober  Prober
	// Settings turns file watching on and off. The watcher runs while the
	// setting is on.
	Settings *config.Store
	// Identifier identifies what each scan probed, once the scan ends. It
	// also has moved files, and folders whose NFO or artwork changed,
	// identified again.
	Identifier Identifier
	// Previews and Subtitles ask for the work that follows a file's probe.
	Previews  Previews
	Subtitles Subtitles
}

// New returns a scanner.
func New(db *store.DB, d Deps, opt Options) *Scanner {
	if opt.OpenFolder == nil {
		opt.OpenFolder = catalog.OpenFolder
	}
	return &Scanner{
		db: db, q: dbq.New(db.Reader()), deps: d, opt: opt,
		newNotifier: newFSNotifier, watchingChanged: make(chan struct{}, 1), foldersChanged: make(chan struct{}, 1),
		problems: map[string]health.Problem{}, asked: map[id.ID]*request{},
	}
}

// The tasks scan runs.
const (
	// TaskScanLibraries scans libraries, with each library an item. A scan
	// walks the library's folders, probes new or changed files, and asks
	// for their folders to be identified. Every 12 hours the task scans
	// every library.
	// That timed walk catches what the file watcher can't see (NFS, SMB) or
	// missed.
	TaskScanLibraries = "scan-libraries"
	taskPruneGone     = "prune-gone-files" // nightly: remove the files gone for 30 days
)

// The steps of a library's scan, as its progress names them.
const (
	StepWalk     = "walk"     // finding files, a folder at a time
	StepProbe    = "probe"    // reading new or changed files
	StepIdentify = "identify" // identifying what it read, the Identifier's task as a step of the scan
)

// Tasks returns the package's tasks, for cmd/kivo to add. Two libraries
// are scanned at once. Every library is pending in every run, so the scan
// task shows no pending count.
func (s *Scanner) Tasks() []tasks.Task {
	return []tasks.Task{
		{Name: TaskScanLibraries, Schedule: tasks.Every(12 * time.Hour), Work: tasks.Items{
			Pending: s.libraries, Do: s.scan, Limit: func(config.Settings) int { return 2 },
			Uncounted: true, Failed: s.failed, Retry: s.retry}},
		{Name: taskPruneGone, Schedule: tasks.Nightly, Work: tasks.Func(s.pruneGone)},
	}
}

// libraries lists the libraries a run of every library goes through.
func (s *Scanner) libraries(ctx context.Context) ([]id.ID, error) {
	libs, err := s.deps.Catalog.Libraries(ctx)
	out := make([]id.ID, len(libs))
	for i, l := range libs {
		out[i] = l.ID
	}
	return out, err
}

// Resume asks for a scan of each library that has files a scan didn't
// probe because a restart cut it off. cmd/kivo calls it as the server
// starts.
func (s *Scanner) Resume(ctx context.Context) error {
	libs, err := s.q.Behind(ctx)
	if err != nil {
		return fmt.Errorf("find scans to resume: %w", err)
	}
	s.deps.Tasks.Enqueue(TaskScanLibraries, libs...)
	return nil
}

// AddLibrary creates a library through catalog and, once it's created,
// asks for its first scan. The watcher takes up its folders at once. The
// watcher also picks up EditLibrary's and DeleteLibrary's changes at once.
func (s *Scanner) AddLibrary(ctx context.Context, in catalog.LibraryInput) (id.ID, error) {
	var lib id.ID
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		l, err := s.deps.Catalog.CreateLibrary(ctx, tx, in)
		if err != nil {
			return err
		}
		lib = l.ID
		tx.AfterCommit(func() {
			s.rewatch()
			s.askAll(lib)
		})
		return nil
	})
	return lib, err
}

// EditLibrary edits a library through catalog. It asks for a scan of the
// library when the edit adds folders, and for the removal of each folder
// the edit drops. A dropped folder leaves the folder list at once and isn't
// scanned again. Its files are deleted in the background (see
// catalog.TaskLibraryChanges).
func (s *Scanner) EditLibrary(ctx context.Context, lib id.ID, ch catalog.LibraryChange) error {
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		added, removed, err := s.deps.Catalog.UpdateLibrary(ctx, tx, lib, ch)
		if err != nil {
			return err
		}
		tx.AfterCommit(func() {
			if len(added)+len(removed) > 0 {
				s.rewatch()
			}
			if len(added) > 0 {
				s.askAll(lib)
			}
			for _, r := range removed {
				s.deps.Tasks.Enqueue(catalog.TaskLibraryChanges, r.ID)
			}
		})
		return nil
	})
}

// DeleteLibrary starts deleting a library from the catalog. The library
// leaves the library list at once, so nothing starts new work for it. Its
// files are deleted in the background (see catalog.TaskLibraryChanges).
// The files on disk are untouched.
func (s *Scanner) DeleteLibrary(ctx context.Context, lib id.ID) error {
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if err := s.deps.Catalog.MarkDeleting(ctx, tx, lib); err != nil {
			return err
		}
		tx.AfterCommit(func() {
			s.rewatch()
			s.deps.Tasks.Enqueue(catalog.TaskLibraryChanges, lib)
		})
		return nil
	})
}

// ScanLibrary asks for a scan of every folder of a library.
func (s *Scanner) ScanLibrary(ctx context.Context, lib id.ID) error {
	if _, err := s.deps.Catalog.Library(ctx, lib); err != nil {
		return err
	}
	s.askAll(lib)
	return nil
}

// ScanLibraryFullWalk is like ScanLibrary, but asks for a full walk that
// lists every directory.
func (s *Scanner) ScanLibraryFullWalk(ctx context.Context, lib id.ID) error {
	l, err := s.deps.Catalog.Library(ctx, lib)
	if err != nil {
		return err
	}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		for _, f := range l.Folders {
			if err := dbq.New(tx).RequestFullWalk(ctx, f.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.askAll(lib)
	return nil
}

// RequestDir asks for a partial scan of one directory of a folder. dir is
// relative to the folder. The scan lists the directory itself, and below
// it only what changed. A dir of "." asks for a scan of the whole folder.
func (s *Scanner) RequestDir(ctx context.Context, folder id.ID, dir string) error {
	r, err := s.deps.Catalog.Folder(ctx, folder)
	if err != nil {
		return err
	}
	s.mu.Lock()
	req := s.asked[r.Library]
	if req == nil {
		req = &request{dirs: map[id.ID][]string{}}
		s.asked[r.Library] = req
	}
	// A scan of every folder already covers dir.
	if !req.all && !slices.Contains(req.dirs[folder], dir) {
		req.dirs[folder] = append(req.dirs[folder], dir)
	}
	s.mu.Unlock()
	s.deps.Tasks.Enqueue(TaskScanLibraries, r.Library)
	return nil
}

// askAll asks for a scan of every folder of a library.
func (s *Scanner) askAll(lib id.ID) {
	s.mu.Lock()
	s.asked[lib] = &request{all: true}
	s.mu.Unlock()
	s.deps.Tasks.Enqueue(TaskScanLibraries, lib)
}

// take removes and returns what a library's scan covers: the directories
// asked for, keyed by folder, or nil for every folder.
func (s *Scanner) take(lib id.ID) map[id.ID][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	req := s.asked[lib]
	delete(s.asked, lib)
	if req == nil || req.all {
		return nil
	}
	return req.dirs
}

// askLater asks for a scan of a library after d, such as when a folder
// can't be reached.
func (s *Scanner) askLater(lib id.ID, d time.Duration) {
	time.AfterFunc(d, func() { s.deps.Tasks.Enqueue(TaskScanLibraries, lib) })
}

// failed lists the files that failed to probe at their generation.
func (s *Scanner) failed(ctx context.Context) ([]tasks.Failure, error) {
	rows, err := s.q.ProbeFailures(ctx)
	out := make([]tasks.Failure, len(rows))
	for i, r := range rows {
		out[i] = tasks.Failure{Item: r.ID, Reason: r.ProbeError.String, At: time.UnixMilli(r.Mtime)}
	}
	return out, err
}

// retry forgets why the given files failed to probe, or why every file
// failed when none are given. It returns the files' libraries, whose scans
// then probe the files again.
func (s *Scanner) retry(ctx context.Context, files ...id.ID) ([]id.ID, error) {
	if len(files) == 0 {
		failed, err := s.q.ProbeFailures(ctx)
		if err != nil {
			return nil, err
		}
		for _, f := range failed {
			files = append(files, f.ID)
		}
	}
	libs, err := s.q.FileLibraries(ctx, id.Set(files...))
	if err != nil {
		return nil, err
	}
	err = store.Batches(ctx, s.db, files, func(ctx context.Context, tx *store.Tx, batch []id.ID) error {
		return dbq.New(tx).ForgetProbeFailed(ctx, id.Set(batch...))
	})
	return libs, err
}

const pruneAfter = 30 * 24 * time.Hour

// pruneGone deletes files gone for 30 days, with their versions. Then it
// deletes the works left with nothing: no version, no watch state and no
// play history. It works a batch at a time. If interrupted, it carries on
// from what's left when it next runs.
func (s *Scanner) pruneGone(ctx context.Context) error {
	before := time.Now().Add(-pruneAfter)
	files, err := s.drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) { return s.deps.Catalog.PruneGone(ctx, tx, before) })
	// A season emptied by one pass is deleted by the next pass, and its
	// series by the pass after.
	works := 0
	for n := 1; err == nil && n > 0; {
		n, err = s.drain(ctx, s.deps.Catalog.DeleteEmptyWorks)
		works += n
	}
	if files > 0 || works > 0 {
		tasks.Summarize(ctx, fmt.Sprintf("Removed %d missing files and %d empty titles", files, works))
	}
	return err
}

// drain runs step through store's Drain and returns how many rows the
// committed steps removed.
func (s *Scanner) drain(ctx context.Context, step func(context.Context, *store.Tx) (int, error)) (int, error) {
	n := 0
	err := s.db.Drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) {
		k, err := step(ctx, tx)
		if err == nil {
			tx.AfterCommit(func() { n += k })
		}
		return k, err
	})
	return n, err
}

// Health reports unreachable folders and watcher trouble.
func (s *Scanner) Health(context.Context) []health.Problem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]health.Problem, 0, len(s.problems))
	for _, p := range s.problems {
		out = append(out, p)
	}
	return out
}

// setProblem reports p under key. It keeps the time the problem was first
// reported.
func (s *Scanner) setProblem(key string, p health.Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.problems[key]; ok {
		p.RaisedAt = old.RaisedAt
	}
	s.problems[key] = p
}

func (s *Scanner) clearProblem(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.problems, key)
}
