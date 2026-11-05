package scan

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
)

const (
	quiet      = 5 * time.Second // a directory is scanned once it has had no events for this long
	resyncDirs = 5 * time.Minute // how often new directories gain watches
)

// notifier is the file-event source. In production it is fsnotify.
type notifier interface {
	Add(path string) error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

type fsnotifier struct{ *fsnotify.Watcher }

func (n fsnotifier) Events() <-chan fsnotify.Event { return n.Watcher.Events }
func (n fsnotifier) Errors() <-chan error          { return n.Watcher.Errors }

func newFSNotifier() (notifier, error) {
	w, err := fsnotify.NewWatcher()
	return fsnotifier{w}, err
}

// Watch turns file events into scan requests until ctx ends, while the file
// watching setting is on.
//
// A directory gets a partial scan once it has gone 5 seconds without an
// event. Every event restarts the wait, so a file caught mid-copy is
// scanned again once the copy goes quiet. Folders added to or dropped from
// libraries are taken up at once. Directories a scan recorded gain watches
// every 5 minutes.
//
// Hitting inotify's watch limit, which every container of the user shares,
// is reported, not fatal. So is an event overflow. NFS and SMB send no
// events either, so the rescan every 12 hours is the backstop.
func (s *Scanner) Watch(ctx context.Context) error {
	for {
		if s.watching() {
			if err := s.watch(ctx); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.watchingChanged:
		}
	}
}

// FileWatchingChanged tells the watcher that the file watching setting was
// saved, so it starts or stops at once. The api package calls it after
// saving.
func (s *Scanner) FileWatchingChanged() {
	select {
	case s.watchingChanged <- struct{}{}:
	default:
	}
}

// rewatch tells the watcher that a library's folders were added or
// removed, so it takes them up at once rather than at its next resync.
func (s *Scanner) rewatch() {
	select {
	case s.foldersChanged <- struct{}{}:
	default:
	}
}

func (s *Scanner) watching() bool { return s.deps.Settings.Get().FileWatching }

// watch runs the watcher until ctx ends or file watching is turned off.
func (s *Scanner) watch(ctx context.Context) error {
	n, err := s.newNotifier()
	if err != nil {
		return err
	}
	defer n.Close()

	w := &watcher{s: s, n: n, ctx: ctx, watched: map[string]bool{}, timers: map[string]timer{}, quiet: make(chan quietDir)}
	defer w.stop()
	w.sync(ctx)
	resync := time.NewTicker(resyncDirs)
	defer resync.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-n.Events():
			w.event(ev)
		case err := <-n.Errors():
			w.trouble(err)
		case <-resync.C:
			w.sync(ctx)
		case <-s.foldersChanged:
			w.sync(ctx)
		case <-s.watchingChanged:
			if !s.watching() {
				s.clearProblem("watch")
				return nil
			}
		case q := <-w.quiet:
			w.requestScan(q)
		}
	}
}

type watcher struct {
	s       *Scanner
	n       notifier
	ctx     context.Context
	folders map[string]id.ID // folder path → folder
	watched map[string]bool
	timers  map[string]timer // directories with events, until they go quiet
	armed   uint64           // how many timers have started, which numbers each one
	quiet   chan quietDir
	full    bool // whether the watch limit was hit
}

type timer struct {
	t *time.Timer
	n uint64
}

// quietDir is a directory whose timer fired, with that timer's number. The
// number tells it apart from a timer that replaced it in the meantime.
type quietDir struct {
	dir string
	n   uint64
}

// sync adds watches for every folder and every directory a scan recorded, so
// the watcher never lists the tree itself.
func (w *watcher) sync(ctx context.Context) {
	libs, err := w.s.deps.Catalog.Libraries(ctx)
	if err != nil {
		return
	}
	w.folders = map[string]id.ID{}
	paths := map[id.ID]string{}
	for _, l := range libs {
		for _, r := range l.Folders {
			w.folders[r.Path] = r.ID
			paths[r.ID] = r.Path
			w.add(r.Path)
		}
	}
	dirs, err := w.s.q.AllDirs(ctx)
	if err != nil {
		return
	}
	for _, d := range dirs {
		if folder, ok := paths[d.FolderID]; ok && d.Path != "." {
			w.add(filepath.Join(folder, filepath.FromSlash(d.Path)))
		}
	}
}

func (w *watcher) add(path string) {
	if w.watched[path] || w.full {
		return
	}
	err := w.n.Add(path)
	switch {
	case errors.Is(err, syscall.ENOSPC):
		w.full = true
		w.s.setProblem("watch", health.Problem{
			Component: "scan", Code: "watchLimit", Severity: health.Info, RaisedAt: time.Now(),
			Message: "The file watch limit (fs.inotify.max_user_watches) is reached. Some folders are only checked by the 12-hour rescan.",
		})
	case err == nil:
		w.watched[path] = true
	}
}

func (w *watcher) event(ev fsnotify.Event) {
	if name := filepath.Base(ev.Name); skipName(name) && name != ignoreFile {
		return // skip hidden files and NAS bookkeeping, which scans never catalogue
	}
	if ev.Has(fsnotify.Create) {
		w.add(ev.Name) // a new directory needs its own watch, and Add on a file fails harmlessly
	}
	// Scan the directory the entry is in, or the entry itself if it is a
	// library folder.
	dir := filepath.Dir(ev.Name)
	if _, _, ok := w.folderOf(dir); !ok {
		dir = ev.Name
	}
	if _, _, ok := w.folderOf(dir); ok {
		w.wait(dir)
	}
}

// wait starts or restarts dir's quiet timer.
func (w *watcher) wait(dir string) {
	if t, ok := w.timers[dir]; ok && t.t.Stop() {
		t.t.Reset(quiet)
		return
	}
	// The timer is new, or it fired and its scan hasn't been taken yet.
	// Either way, a new timer replaces it.
	w.armed++
	n := w.armed
	t := time.AfterFunc(quiet, func() {
		select {
		case w.quiet <- quietDir{dir, n}:
		case <-w.ctx.Done():
		}
	})
	w.timers[dir] = timer{t, n}
}

// requestScan requests the scan of a directory that went quiet.
func (w *watcher) requestScan(q quietDir) {
	if t, ok := w.timers[q.dir]; !ok || t.n != q.n {
		return // an event came after it fired, so a newer timer is waiting
	}
	delete(w.timers, q.dir)
	folder, rel, ok := w.folderOf(q.dir)
	if !ok {
		return // the folder left its library in the meantime
	}
	err := w.s.RequestDir(w.ctx, folder, rel)
	if errors.Is(err, catalog.ErrNotFound) {
		return // the folder is being removed, or its library deleted
	}
	if err != nil && w.ctx.Err() == nil {
		slog.Error("scan: request from watcher", "dir", q.dir, "err", err)
	}
}

func (w *watcher) stop() {
	for _, t := range w.timers {
		t.t.Stop()
	}
}

// trouble handles a watcher error. An overflow means events were dropped,
// so every folder is rescanned. That gives the same result as if no events
// had been missed.
func (w *watcher) trouble(err error) {
	if errors.Is(err, fsnotify.ErrEventOverflow) {
		for path := range w.folders {
			w.wait(path)
		}
		return
	}
	slog.Warn("scan: watcher", "err", err)
}

// folderOf returns the folder a path is in, and the path relative to it in
// slash form ("." for the folder itself).
func (w *watcher) folderOf(p string) (id.ID, string, bool) {
	best, bestPath := id.ID{}, ""
	for path, folder := range w.folders {
		if (p == path || strings.HasPrefix(p, path+string(filepath.Separator))) && len(path) > len(bestPath) {
			best, bestPath = folder, path
		}
	}
	if bestPath == "" {
		return id.ID{}, "", false
	}
	rel, err := filepath.Rel(bestPath, p)
	if err != nil {
		return id.ID{}, "", false
	}
	return best, filepath.ToSlash(rel), true
}
