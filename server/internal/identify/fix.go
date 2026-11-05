package identify

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/fault"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/metadata"
	"github.com/mmcdole/kivo/internal/users"
)

// requestWait bounds the provider calls made while a client waits. These
// are Fix match's searches and lookups, and an edit's episode groups.
const requestWait = 10 * time.Second

// ErrBookMatch means a book's "Fix match" chose something other than an
// Audible book.
var ErrBookMatch = fault.New(fault.Invalid, "a book matches an Audible book")

// FixMatch re-describes a root work, in place, as the match an admin chose
// by its provider ID. The root keeps its own ID, history and watch state. It
// takes the match's ID, is refreshed from it, and its match is locked
// against automatic changes.
//
// The root work is the one the work is under, such as the series of an
// episode or season. A series' episodes are looked up again by the name of
// each version under it. A book instead takes the Audible book's ASIN as an
// admin's edit, and is refreshed from it.
//
// The root's folder is held while this runs, and everything is written in
// one transaction. FixMatch returns the work, or its root when the work
// was removed along with the old episodes. The work must be one the Viewer
// can see.
func (i *Identifier) FixMatch(ctx context.Context, v users.Viewer, work id.ID, match catalog.ExternalID) (id.ID, error) {
	w, err := i.visibleWork(ctx, v, work)
	if err != nil {
		return id.ID{}, err
	}
	if w.Kind == catalog.KindBook {
		if match.Provider != catalog.ASIN.Provider || match.Kind != catalog.ASIN.Kind {
			return id.ID{}, fault.About(ErrBookMatch, fault.Field{Name: "/matchId", Code: "invalid"})
		}
		return work, i.deps.Metadata.MatchBook(ctx, work, match.ID)
	}
	root, err := i.matchable(ctx, w, match)
	if err != nil {
		return id.ID{}, err
	}
	lang, _, err := i.deps.Metadata.Locale(ctx, root)
	if err != nil {
		return id.ID{}, err
	}
	rctx, cancel := context.WithTimeout(ctx, requestWait)
	c, err := i.deps.Metadata.Resolve(rctx, match, lang)
	cancel()
	if err != nil {
		return id.ID{}, err
	}
	if err := i.fixMatch(ctx, root, c); err != nil {
		return id.ID{}, err
	}
	if _, err := i.deps.Catalog.Work(ctx, work); errors.Is(err, catalog.ErrNotFound) {
		return root, nil
	}
	return work, nil
}

// visibleWork returns a work the Viewer can see.
func (i *Identifier) visibleWork(ctx context.Context, v users.Viewer, work id.ID) (catalog.Work, error) {
	if err := i.deps.Catalog.Visible(ctx, v, work); err != nil {
		return catalog.Work{}, err
	}
	return i.deps.Catalog.Work(ctx, work)
}

// matchable returns the root work a work's Fix match describes: its series, or
// the movie itself. An extra or another video has no
// match, and a match of the wrong kind is refused.
func (i *Identifier) matchable(ctx context.Context, w catalog.Work, match catalog.ExternalID) (id.ID, error) {
	if w.Kind == catalog.KindExtra {
		return id.ID{}, fmt.Errorf("%w: an extra has no match to fix", ErrInvalid)
	}
	root := w.Root
	t, err := i.deps.Catalog.Work(ctx, root)
	if err != nil {
		return id.ID{}, err
	}
	if t.Kind != catalog.KindMovie && t.Kind != catalog.KindSeries || match.Kind != string(t.Kind) {
		return id.ID{}, fmt.Errorf("%w: a %s can't be matched to a %s", ErrInvalid, t.Kind, match.Kind)
	}
	return root, nil
}

// fixMatch describes the root work as c, locks it, and places its folders'
// versions under it.
func (i *Identifier) fixMatch(ctx context.Context, root id.ID, c metadata.Match) error {
	held, unlock, err := i.holdFolders(ctx, root)
	if err != nil {
		return err
	}
	defer unlock()
	d := decision{source: sourceManual, match: &c, score: 100}
	var plans []plan
	for _, h := range held {
		if !h.link.IsZero() && h.link != root {
			continue // another root's folder, holding a file that came from this root
		}
		p := plan{s: h.snap, root: root, ps: h.placed, d: d}
		if h.snap.unit.kind == catalog.KindSeries {
			rctx, cancel := context.WithTimeout(ctx, requestWait)
			p.ordering, p.eps, err = i.placeEpisodes(rctx, h.snap, h.placed, d, root, h.snap.unit.versions)
			cancel()
			if err != nil {
				return err
			}
		}
		plans = append(plans, p)
	}
	return i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		for _, p := range plans {
			if err := i.apply(ctx, tx, p, true); err != nil {
				return err
			}
		}
		if err := i.deps.Metadata.Refresh(ctx, tx, root); err != nil || len(plans) > 0 {
			return err
		}
		err := i.deps.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error {
			_, err := i.describe(ctx, tx, b, root, c, true)
			return err
		})
		if err != nil {
			return err
		}
		q := dbq.New(tx)
		f, err := q.FolderOf(ctx, root)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return putMatch(ctx, q, f.LibraryID, f.Subject, d, "")
	})
}

// folder is a held folder that a work's files are in.
type folder struct {
	snap   *snapshot  // the folder, with the unit those files belong to
	placed placements // where that unit's files are placed
	link   id.ID      // the root work the folder is linked to, if any
}

// holdFolders locks every folder that holds the work's present files, each
// decided as one unit, and returns them with their units read. It locks in a
// fixed order so two requests can't deadlock waiting on each other. A folder
// whose files belong to no unit, such as samples, is left out. The caller
// must call unlock.
func (i *Identifier) holdFolders(ctx context.Context, work id.ID) (held []folder, unlock func(), err error) {
	files, err := i.q.FilesOfWork(ctx, work)
	if err != nil {
		return nil, nil, err
	}
	type at struct {
		key  string
		lib  catalog.Library
		file catalog.File
	}
	var keys []at
	libs := map[id.ID]catalog.Library{}
	for _, row := range files {
		f, err := i.deps.Catalog.File(ctx, row.ID)
		if err != nil {
			return nil, nil, err
		}
		lib, ok := libs[f.Library]
		if !ok {
			if lib, err = i.deps.Catalog.Library(ctx, f.Library); err != nil {
				return nil, nil, err
			}
			libs[f.Library] = lib
		}
		if k := lockKey(lib, f.RelPath); !slices.ContainsFunc(keys, func(x at) bool { return x.key == k }) {
			keys = append(keys, at{key: k, lib: lib, file: f})
		}
	}
	slices.SortFunc(keys, func(a, b at) int { return strings.Compare(a.key, b.key) })
	var unlocks []func()
	unlock = func() {
		for _, u := range slices.Backward(unlocks) {
			u()
		}
	}
	for _, k := range keys {
		u, err := i.folders.Lock(ctx, k.key)
		if err != nil {
			unlock()
			return nil, nil, err
		}
		unlocks = append(unlocks, u)
		s, err := i.readSnapshot(ctx, k.lib, k.file)
		if err != nil {
			unlock()
			return nil, nil, err
		}
		if s.unit == nil {
			continue
		}
		ps, err := readPlacements(ctx, i.q, s.unit.fileIDs())
		if err != nil {
			unlock()
			return nil, nil, err
		}
		link, err := i.q.FolderWork(ctx, dbq.FolderWorkParams{LibraryID: s.lib.ID, Subject: s.unit.subject})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			unlock()
			return nil, nil, err
		}
		held = append(held, folder{snap: s, placed: ps, link: link})
	}
	return held, unlock, nil
}

// Unmatch returns a movie or series to the state identify gives files that
// nothing matched.
//
// The work stays, with its watch state, but loses its external IDs,
// metadata and artwork. It takes its title and year from its file names and
// becomes provisional. A series' episodes are numbered from their names
// again, and their state follows them.
//
// Its folders' decisions become a manual unmatched decision, and the work
// is locked, so automatic matching leaves it alone until a FixMatch.
// Unmatch returns the work it unmatched. The work must be one the Viewer
// can see.
func (i *Identifier) Unmatch(ctx context.Context, v users.Viewer, work id.ID) (id.ID, error) {
	w, err := i.visibleWork(ctx, v, work)
	if err != nil {
		return id.ID{}, err
	}
	if w.Kind != catalog.KindMovie && w.Kind != catalog.KindSeries {
		return id.ID{}, fmt.Errorf("%w: only a movie or series can be unmatched", ErrInvalid)
	}

	held, unlock, err := i.holdFolders(ctx, work)
	if err != nil {
		return id.ID{}, err
	}
	defer unlock()
	held = slices.DeleteFunc(held, func(h folder) bool { return !h.link.IsZero() && h.link != work })
	if len(held) == 0 {
		return id.ID{}, fmt.Errorf("%w: the work has no files to name it by", ErrInvalid)
	}
	u := held[0].snap.unit
	title, year := cmp.Or(u.title, stem(u.files()[0].Rel)), u.year

	err = i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return i.deps.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error {
			if err := b.Unmatch(ctx, work, title, year); err != nil {
				return err
			}
			if err := i.deps.Metadata.ClearArtwork(ctx, tx, work); err != nil {
				return err
			}
			q := dbq.New(tx)
			for _, h := range held {
				if w.Kind == catalog.KindSeries {
					for _, e := range h.snap.unit.versions {
						p := h.placed.of(e)
						if p == nil || slices.ContainsFunc(p.works, func(x placedWork) bool { return x.root != work }) {
							continue // not an episode of this series
						}
						works, err := i.parsedEpisodes(ctx, tx, work, e)
						if err != nil {
							return err
						}
						if slices.Equal(works, p.workIDs()) {
							continue
						}
						if err := i.reassign(ctx, tx, b, p.version, works); err != nil {
							return err
						}
					}
				}
				if err := putMatch(ctx, q, h.snap.lib.ID, h.snap.unit.subject, decision{source: sourceManual}, ""); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return id.ID{}, err
	}
	return work, nil
}
