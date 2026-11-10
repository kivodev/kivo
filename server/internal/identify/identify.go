package identify

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"slices"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/keylock"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/userstate"
)

var (
	// ErrInvalid means FixMatch was given a match the root work can't take, or
	// Unmatch was given a work it can't unmatch. Examples are a series match
	// for a movie, or any match for an extra or another video.
	ErrInvalid = fault.New(fault.Invalid, "invalid")

	// errStale means a file or a work changed while its folder was being
	// identified. The result is dropped, and the folder stays pending.
	errStale = errors.New("stale")
)

// Identifier decides which root work each folder holds and what describes it,
// and re-identifies on an admin's request. Each decision is written in one
// transaction.
type Identifier struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps
	// openFolder opens a library folder to read NFO files.
	openFolder func(path string) (fs.FS, func() error, error)

	// folders allows only one decision per folder at a time, so two
	// identifications of one series don't both search for it. The second
	// waits, then finds the first one's decision.
	folders keylock.Locks[string]
}

// Tasks enqueues items to be done now, or on behalf of an item of the task
// a task is a step of. tasks.Runner implements it.
type Tasks interface {
	Enqueue(task string, items ...id.ID)
	EnqueueFor(owner id.ID, task string, items ...id.ID)
}

// Markers asks for markers for a library's episodes that got versions,
// whether by a match or by an admin's fix. It is called in the transaction
// that places the versions. *markers.Service implements it.
type Markers interface {
	Placed(ctx context.Context, tx *store.Tx, lib catalog.Library, episodes []id.ID) error
}

// Deps are an Identifier's collaborators.
type Deps struct {
	Catalog  *catalog.Catalog
	Metadata *metadata.Service
	// State carries a version's watch state between episodes of a series.
	Userstate *userstate.Service
	// Activity records the files a scan added.
	Activity *activity.Service
	// Tasks identifies the folders of the files enqueued.
	Tasks Tasks
	// Markers finds markers for the episodes placed.
	Markers Markers
	// ScanStep is where the identify task shows: as the last step of a
	// library's scan. Scanned enqueues for the library's item of it.
	ScanStep tasks.StepOf
}

// New returns an Identifier.
func New(db *store.DB, d Deps) *Identifier {
	return &Identifier{db: db, q: dbq.New(db.Reader()), deps: d, openFolder: catalog.OpenFolder}
}

// TaskIdentifyMedia identifies folders, with each item a file whose folder
// needs identifying. A file needs it when it is probed but not identified at
// its generation. Items are done as they're enqueued: by a scan as it ends,
// and by a refresh or an edit that sends a work's folders back. The task also
// looks for files left pending as the server starts and then each hour, which
// is when a folder that met a provider outage or couldn't be read is tried
// again. It is a step of a library's scan, so it has no row of its own: what
// a scan asked for shows on the scan's row, and nothing else shows.
const TaskIdentifyMedia = "identify-media"

const identifyAtOnce = 4 // how many folders are identified at once

// Tasks returns the package's tasks, for cmd/kivo to add.
func (i *Identifier) Tasks() []tasks.Task {
	return []tasks.Task{{Name: TaskIdentifyMedia, Schedule: tasks.OnDemand, StepOf: i.deps.ScanStep, Work: tasks.Items{
		Pending: i.pending, Do: i.identifyFolder, PendingCount: i.pendingCount,
		Limit: func(config.Settings) int { return identifyAtOnce }}}}
}

// pending lists one file for each folder that needs identifying, in every
// library, by path.
func (i *Identifier) pending(ctx context.Context) ([]id.ID, error) {
	libs, err := i.deps.Catalog.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	var out []id.ID
	for _, l := range libs {
		files, err := i.unidentified(ctx, l)
		if err != nil {
			return nil, err
		}
		out = append(out, files...)
	}
	return out, nil
}

// pendingCount counts the files that need identifying. Pending lists one of
// them for each folder.
func (i *Identifier) pendingCount(ctx context.Context) (int, error) {
	n, err := i.q.CountUnidentified(ctx)
	return int(n), err
}

// unidentified lists, by path, the first file that needs identifying in each
// of a library's folders. A folder is identified whole, or a loose file's
// root work is, so one file stands for the rest.
func (i *Identifier) unidentified(ctx context.Context, lib catalog.Library) ([]id.ID, error) {
	rows, err := i.q.Unidentified(ctx, lib.ID)
	if err != nil {
		return nil, err
	}
	var out []id.ID
	seen := map[string]bool{}
	for _, r := range rows {
		if k := folderKey(lib, r.ID, r.RelPath); !seen[k] {
			seen[k] = true
			out = append(out, r.ID)
		}
	}
	return out, nil
}

// Scanned has the folders of a library's files that need identifying
// identified now, as the last step of the library's scan. A scan calls it as
// it ends, once the library's new and changed files are all probed.
func (i *Identifier) Scanned(ctx context.Context, lib catalog.Library) error {
	files, err := i.unidentified(ctx, lib)
	if err != nil {
		return err
	}
	i.deps.Tasks.EnqueueFor(lib.ID, TaskIdentifyMedia, files...)
	return nil
}

// identifyFolder identifies the folder holding a file, and records as
// identified the folder's files that needed it. It returns nil at once when
// none of them does, as when another item has identified the folder. It only
// returns an error when it can't read what to do or its context ends. A
// folder that meets a provider outage, can't be read, or changed meanwhile
// is logged and left pending.
func (i *Identifier) identifyFolder(ctx context.Context, file id.ID) error {
	f, err := i.deps.Catalog.File(ctx, file)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil // deleted, or merged into another file by a move
	}
	if err != nil {
		return err
	}
	lib, err := i.deps.Catalog.Library(ctx, f.Library)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil // the library is being deleted
	}
	if err != nil {
		return err
	}
	// A folder's files are a range of paths: '0' follows '/'. A loose file
	// is a range of one.
	from, before := f.RelPath, f.RelPath+"\x00"
	if folder := folderOf(lib.Kind, f.RelPath); folder != "" {
		from, before = folder+"/", folder+"0"
	}
	rows, err := i.q.UnidentifiedBetween(ctx, dbq.UnidentifiedBetweenParams{LibraryID: lib.ID, From: from, Before: before})
	if err != nil {
		return err
	}
	// A movie folder inside this one is a folder of its own.
	key := folderKey(lib, f.ID, f.RelPath)
	rows = slices.DeleteFunc(rows, func(r dbq.UnidentifiedBetweenRow) bool { return folderKey(lib, r.ID, r.RelPath) != key })
	if len(rows) == 0 {
		return nil
	}
	err = i.identify(ctx, lib, f)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, errStale), errors.Is(err, catalog.ErrUnreachable):
		return nil
	case err != nil:
		slog.WarnContext(ctx, "identify: folder left for a later pass", "file", f.RelPath, "err", err)
		return nil
	}
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		for _, r := range rows {
			if err := q.SetIdentified(ctx, dbq.SetIdentifiedParams{ID: r.ID, Generation: r.Generation}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Reidentify marks files so their folders are identified again. A scan
// calls it mid-run, when an NFO or artwork changed or a full walk retries
// what was unmatched, and has them identified as the run ends.
func (i *Identifier) Reidentify(ctx context.Context, tx *store.Tx, files []id.ID) error {
	return dbq.New(tx).Reidentify(ctx, id.Set(files...))
}

// identify identifies every unit in the folder of file f. It holds the
// folder's lock so no other identification decides it meanwhile.
func (i *Identifier) identify(ctx context.Context, lib catalog.Library, f catalog.File) error {
	unlock, err := i.folders.Lock(ctx, lockKey(lib, f.RelPath))
	if err != nil {
		return err
	}
	defer unlock()
	p, units, err := i.classifyFolder(ctx, lib, f)
	if err != nil {
		return err
	}
	for _, u := range units {
		s, err := i.unitSnapshot(lib, p, u)
		if err != nil {
			return err
		}
		if err := i.identifyUnit(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// identifyUnit identifies one unit of a folder.
//
// First the unit's files are put on its root work, in their own transaction
// before any provider call, so a new file appears at once. Then what is
// stored about the unit is read, once, and the unit's decision is made from
// it. The decision is written only if what is stored doesn't already hold
// it, and only if nothing has changed meanwhile.
func (i *Identifier) identifyUnit(ctx context.Context, s *snapshot) error {
	lib, u := s.lib, s.unit
	root, err := i.place(ctx, s)
	if err != nil {
		return err
	}
	switch u.kind {
	case catalog.KindVideo:
		return nil // other videos aren't matched
	case catalog.KindBook:
		return i.describeBook(ctx, root)
	}
	stored, err := i.readStored(ctx, s, root)
	if err != nil {
		return err
	}
	d, err := i.decide(ctx, s, stored)
	if err != nil {
		return err
	}
	if d.match == nil {
		if d.source == "" || stored.records(d, "") {
			return nil
		}
		return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			return putMatch(ctx, dbq.New(tx), lib.ID, u.subject, d, "")
		})
	}
	p := plan{s: s, root: root, ps: stored.ps, d: d, ordering: stored.ordering}
	if u.kind == catalog.KindSeries {
		if p.eps, err = i.placeEpisodes(ctx, s, stored, d); err != nil {
			return err
		}
	}
	if len(p.eps) == 0 && stored.upToDate(u, d) {
		return nil
	}
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error { return i.apply(ctx, tx, p, false) })
}
