package scan

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"maps"
	"path"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/scan/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// run is one scan of one folder, or of one directory in it.
type run struct {
	s       *Scanner
	folder  catalog.Folder
	lib     catalog.Library
	files   files // the names the library's kind catalogues
	fsys    fs.FS
	dir     string // where the run starts, or "." for the whole folder
	full    bool   // whether every directory is listed, changed or not
	now     time.Time
	known   map[string]dbq.ListDirsRow // directories as of the last run
	kids    map[string][]string        // known directories by parent, sorted
	listed  map[string]dirState        // directories listed this run, to record at the end
	seen    map[string]bool            // directories present this run, listed or not
	visits  map[dirKey]bool            // directories walked this run, by device and inode
	linkOut bool                       // whether a link leaving the folder was skipped this run
	gone    []catalog.FileRef          // files missing from where they were, with that path
	added   int
}

// scan scans one library. It walks the library's folders, or only the
// directories the watcher asked for. It then probes the files not probed
// at their generation, and asks for their folders to be identified. A
// folder that can't be reached is left as it is, and the library is
// scanned again in an hour.
func (s *Scanner) scan(ctx context.Context, lib id.ID) error {
	l, err := s.deps.Catalog.Library(ctx, lib)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil // the library is being deleted
	}
	if err != nil {
		return err
	}
	dirs := s.take(lib)
	tasks.Progress(ctx, StepWalk, 0, len(l.Folders))
	for i, f := range l.Folders {
		walk := []string{"."}
		if dirs != nil {
			walk = dirs[f.ID]
		}
		for _, d := range walk {
			err := s.scanFolder(ctx, f.ID, d)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				slog.WarnContext(ctx, "scan: folder left for the next scan", "folder", f.Path, "dir", d, "err", err)
			}
		}
		tasks.Progress(ctx, StepWalk, i+1, len(l.Folders))
	}
	if err := s.probeAll(ctx, l); err != nil {
		return err
	}
	return s.deps.Identifier.Scanned(ctx, l)
}

// scanFolder reconciles one folder, or one directory of it and what changed
// below that directory. New and changed files are committed as each
// directory is listed. Gone files and directory mtimes are committed only
// at the end, so a crash never marks anything missing.
func (s *Scanner) scanFolder(ctx context.Context, folderID id.ID, dir string) error {
	folder, err := s.deps.Catalog.Folder(ctx, folderID)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil // the folder was removed from its library
	}
	if err != nil {
		return err
	}
	r, err := s.newRun(ctx, folder, dir)
	if err != nil {
		return err
	}
	fsys, closeFolder, err := s.opt.OpenFolder(folder.Path)
	if err != nil {
		return s.unreachable(ctx, folder, err)
	}
	defer closeFolder()
	r.fsys = fsys
	if err := r.reachable(); err != nil {
		return s.unreachable(ctx, folder, err)
	}
	if _, err := fs.Stat(r.fsys, r.dir); r.dir != "." && errors.Is(err, fs.ErrNotExist) {
		return nil // the directory was removed, and the scan of the directory above sees it gone
	}
	err = r.walk(ctx, r.dir)
	if err == nil {
		err = r.finish(ctx)
	}
	if errors.Is(err, catalog.ErrNotFound) {
		return nil // the folder was removed from its library during the run
	}
	if err != nil {
		return err
	}
	s.clearProblem("unreachable:" + folder.ID.String())
	if r.full && !r.linkOut {
		s.clearProblem("symlink:" + folder.ID.String()) // every directory was listed, and no link left the folder
	}
	slog.InfoContext(ctx, "scan: finished", "folder", folder.Path, "dir", r.dir, "full", r.full, "seen", len(r.seen), "listed", len(r.listed),
		"new_or_changed", r.added, "gone", len(r.gone), "duration", time.Since(r.now).Round(time.Millisecond))
	return nil
}

// newRun starts a run over dir in folder, from what the last runs
// recorded.
func (s *Scanner) newRun(ctx context.Context, folder catalog.Folder, dir string) (*run, error) {
	lib, err := s.deps.Catalog.Library(ctx, folder.Library)
	if err != nil {
		return nil, err
	}
	dirs, err := s.q.ListDirs(ctx, folder.ID)
	if err != nil {
		return nil, err
	}
	r := &run{
		s: s, folder: folder, lib: lib, files: filesOf(lib.Kind), dir: dir, now: time.Now(),
		known: make(map[string]dbq.ListDirsRow, len(dirs)), kids: map[string][]string{},
		listed: map[string]dirState{}, seen: map[string]bool{}, visits: map[dirKey]bool{},
	}
	for _, d := range dirs {
		r.known[d.Path] = d
		if d.Path != "." {
			r.kids[path.Dir(d.Path)] = append(r.kids[path.Dir(d.Path)], d.Path)
		}
	}
	for _, k := range r.kids {
		slices.Sort(k)
	}
	if dir == "." {
		if r.full, err = s.fullWalkDue(ctx, folder.ID); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// fullWalkEvery is how often a run lists every directory, because editing a
// file in place doesn't change its directory's mtime.
const fullWalkEvery = 7 * 24 * time.Hour

func (s *Scanner) fullWalkDue(ctx context.Context, folder id.ID) (bool, error) {
	sr, err := s.q.GetScanFolder(ctx, folder)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil // the first scan lists everything anyway
	}
	if err != nil {
		return false, err
	}
	return sr.FullRequested || !sr.FullWalkAt.Valid || time.UnixMilli(sr.FullWalkAt.Int64).Before(time.Now().Add(-fullWalkEvery)), nil
}

// finish commits what the run learned, a batch per transaction.
//
// It first marks files gone, each only if it's still at the path it was
// missing from, so a move a probe found during the run stands. Once every
// file is settled, it records directory mtimes and removes directories that
// disappeared. Last, it records the run itself.
//
// A crash partway leaves the rest for the next run, which lists again every
// directory whose mtime wasn't recorded.
func (r *run) finish(ctx context.Context) error {
	err := store.Batches(ctx, r.s.db, r.gone, func(ctx context.Context, tx *store.Tx, batch []catalog.FileRef) error {
		return r.s.deps.Catalog.MarkGone(ctx, tx, r.folder.ID, batch)
	})
	if err != nil {
		return err
	}
	listed := slices.Sorted(maps.Keys(r.listed))
	err = store.Batches(ctx, r.s.db, listed, func(ctx context.Context, tx *store.Tx, batch []string) error {
		q := dbq.New(tx)
		for _, dir := range batch {
			st := r.listed[dir]
			err := q.PutDir(ctx, dbq.PutDirParams{FolderID: r.folder.ID, Path: dir, Mtime: st.mtime.UnixMilli(), ListedAt: r.now.UnixMilli(), LocalMetadataHash: st.localMetadataHash})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	var vanished []string
	for dir := range r.known {
		if !r.seen[dir] && within(dir, r.dir) {
			vanished = append(vanished, dir)
		}
	}
	slices.Sort(vanished)
	err = store.Batches(ctx, r.s.db, vanished, func(ctx context.Context, tx *store.Tx, batch []string) error {
		q := dbq.New(tx)
		for _, dir := range batch {
			if err := q.DeleteDir(ctx, dbq.DeleteDirParams{FolderID: r.folder.ID, Path: dir}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if r.full {
		if err := r.reidentifyUnmatched(ctx); err != nil {
			return err
		}
	}
	return r.s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if r.full {
			if err := dbq.New(tx).FullWalkDone(ctx, dbq.FullWalkDoneParams{FolderID: r.folder.ID, At: r.now.UnixMilli()}); err != nil {
				return err
			}
		}
		if err := r.s.setFolderReachable(ctx, tx, r.folder.ID, true); err != nil {
			return err
		}
		// TODO: tell webhooks the scan finished (webhooks.Notify).
		return dbq.New(tx).SetFolderScanned(ctx, dbq.SetFolderScannedParams{ID: r.folder.ID, LastScanAt: sql.NullInt64{Int64: r.now.UnixMilli(), Valid: true}})
	})
}

// reidentifyUnmatched sends the folder's unmatched videos back to be
// identified. That way a later full walk finds a movie or series the provider
// didn't list at first.
func (r *run) reidentifyUnmatched(ctx context.Context) error {
	unmatched, err := r.s.q.UnmatchedFiles(ctx, r.folder.ID)
	if err != nil {
		return err
	}
	files := make([]id.ID, len(unmatched))
	for i, f := range unmatched {
		files[i] = f.ID
	}
	return store.Batches(ctx, r.s.db, files, func(ctx context.Context, tx *store.Tx, batch []id.ID) error {
		return r.s.deps.Identifier.Reidentify(ctx, tx, batch)
	})
}
