package identify

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"slices"
	"sync"

	"github.com/mmcdole/kivo/internal/activity"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/keylock"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/scan"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
	"github.com/mmcdole/kivo/internal/userstate"
	"golang.org/x/sync/errgroup"
)

var (
	// ErrInvalid means FixMatch was given a match the root work can't take, or
	// Unmatch was given a work it can't unmatch. Examples are a series match
	// for a movie, or any match for an extra or another video.
	ErrInvalid = fault.New(fault.Invalid, "invalid")

	// errStale means a file or a work changed while its folder was being
	// identified. The result is dropped, and the next scan identifies the
	// folder again.
	errStale = errors.New("stale")
)

// Identifier decides which root work each folder holds and what describes it,
// and re-identifies on an admin's request. Each decision is written in one
// transaction.
type Identifier struct {
	db   *store.DB
	q    *dbq.Queries
	deps Deps
	opt  Options

	// folders allows only one decision per folder at a time, so two
	// identifications of one series don't both search for it. The second
	// waits, then finds the first one's decision.
	folders keylock.Locks[string]
}

// Options are an Identifier's optional settings.
type Options struct {
	// OpenFolder opens a library folder to read NFO files. The default is
	// catalog.OpenFolder.
	OpenFolder func(path string) (fs.FS, func() error, error)
}

// Tasks asks for items to be done now. Identify uses it to ask for a
// library's scan, which identifies the folders sent back to be identified
// again. tasks.Runner implements it.
type Tasks interface {
	Ask(task string, items ...id.ID)
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
	// Tasks scans a library again once a write sends its folders back.
	Tasks Tasks
	// Markers finds markers for the episodes placed.
	Markers Markers
}

// New returns an Identifier.
func New(db *store.DB, d Deps, opt Options) *Identifier {
	if opt.OpenFolder == nil {
		opt.OpenFolder = catalog.OpenFolder
	}
	return &Identifier{db: db, q: dbq.New(db.Reader()), deps: d, opt: opt}
}

const identifyAtOnce = 4 // how many folders are identified at once

// Identify runs as a scan of the library ends. It identifies the folders of
// the library's files that were probed but not yet identified at their
// current generation. Each folder, or each loose file's root work, is one unit
// of work, and identifyAtOnce of them run at once. A folder that meets a
// provider outage, can't be read, or changed meanwhile is left for the next
// scan.
func (i *Identifier) Identify(ctx context.Context, lib catalog.Library) error {
	rows, err := i.q.Unidentified(ctx, lib.ID)
	if err != nil {
		return err
	}
	var keys []string
	folders := map[string][]catalog.FileState{}
	for _, r := range rows {
		k := folderKey(lib, r.ID, r.RelPath)
		if folders[k] == nil {
			keys = append(keys, k)
		}
		folders[k] = append(folders[k], catalog.FileState{ID: r.ID, RelPath: r.RelPath, Generation: r.Generation})
	}
	var mu sync.Mutex
	done := 0
	tasks.Progress(ctx, StepIdentify, 0, len(keys))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(identifyAtOnce)
	for _, k := range keys {
		g.Go(func() error {
			if err := i.identifyFolder(gctx, lib, folders[k]); err != nil {
				return err
			}
			mu.Lock()
			done++
			tasks.Progress(ctx, StepIdentify, done, len(keys))
			mu.Unlock()
			return nil
		})
	}
	return g.Wait()
}

// StepIdentify is the step of a library's scan that identifies its folders.
const StepIdentify = "identify"

// identifyFolder identifies the folder holding files, and records the files
// as identified. It only returns an error when the scan's context ends. A
// folder that fails is logged and left for the next scan.
func (i *Identifier) identifyFolder(ctx context.Context, lib catalog.Library, files []catalog.FileState) error {
	f, err := i.deps.Catalog.File(ctx, files[0].ID)
	if errors.Is(err, catalog.ErrNotFound) {
		return nil // deleted, or merged into another file by a move
	}
	if err == nil {
		err = i.identify(ctx, lib, f)
	}
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, errStale), errors.Is(err, catalog.ErrUnreachable):
		return nil
	case err != nil:
		slog.WarnContext(ctx, "identify: folder left for the next scan", "file", files[0].RelPath, "err", err)
		return nil
	}
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		for _, f := range files {
			if err := q.SetIdentified(ctx, dbq.SetIdentifiedParams{ID: f.ID, Generation: f.Generation}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Reidentify marks files so their folders are identified again. A scan
// calls it mid-run, when an NFO or artwork changed or a full walk retries
// what was unmatched, and identifies them as the run ends.
func (i *Identifier) Reidentify(ctx context.Context, tx *store.Tx, files []id.ID) error {
	return dbq.New(tx).Reidentify(ctx, id.Set(files...))
}

// reidentifyAndScan is Reidentify for callers outside a scan: it also asks
// for a scan of lib once tx commits, which identifies them.
func (i *Identifier) reidentifyAndScan(ctx context.Context, tx *store.Tx, lib id.ID, files ...id.ID) error {
	if err := i.Reidentify(ctx, tx, files); err != nil {
		return err
	}
	tx.AfterCommit(func() { i.deps.Tasks.Ask(scan.TaskScanLibraries, lib) })
	return nil
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
// before any provider call, so a new file appears at once. Then the unit's
// decision is made and written, provided nothing has changed meanwhile.
//
// Nothing is written when the match row already records the decision, the
// root work already is what the decision describes, and every file is placed
// by it.
func (i *Identifier) identifyUnit(ctx context.Context, s *snapshot) error {
	lib, u := s.lib, s.unit
	ps, err := readPlacements(ctx, i.q, u.fileIDs())
	if err != nil {
		return err
	}
	root, err := i.place(ctx, s, ps)
	if err != nil {
		return err
	}
	switch u.kind {
	case catalog.KindVideo:
		return nil // other videos aren't matched
	case catalog.KindBook:
		return i.describeBook(ctx, root)
	}
	if ps, err = readPlacements(ctx, i.q, u.fileIDs()); err != nil {
		return err
	}
	d, err := i.decide(ctx, s)
	if err != nil {
		return err
	}
	prev, err := i.storedMatch(ctx, s)
	if err != nil {
		return err
	}
	if d.match == nil {
		if d.source == "" || prev.records(d, "") {
			return nil
		}
		return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			return putMatch(ctx, dbq.New(tx), lib.ID, u.subject, d, "")
		})
	}
	p := plan{s: s, root: root, ps: ps, d: d}
	if u.kind == catalog.KindSeries {
		if p.ordering, p.eps, err = i.placeEpisodes(ctx, s, ps, d, root, u.versions); err != nil {
			return err
		}
	}
	if len(p.eps) == 0 && prev.records(d, p.ordering) {
		placed, err := i.placedBy(ctx, u, ps, d, root)
		if err != nil || placed {
			return err
		}
	}
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error { return i.apply(ctx, tx, p, false) })
}

// placedBy reports whether the root is what the decision describes and
// every file of the unit is placed under it, none provisionally. If so,
// writing the decision again would change nothing.
func (i *Identifier) placedBy(ctx context.Context, u *unit, ps placements, d decision, root id.ID) (bool, error) {
	if ok, err := i.describes(ctx, i.q, root, d.match); err != nil || !ok {
		return false, err
	}
	for _, e := range u.entries() {
		p := ps.of(e)
		if p == nil || slices.ContainsFunc(p.works, func(w placedWork) bool { return w.provisional || w.root != root }) {
			return false, nil
		}
	}
	return true, nil
}
