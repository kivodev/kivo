package identify

import (
	"bytes"
	"context"
	"slices"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
)

// placed is a version as the catalog has it, with its parts, its edition
// label and the works it covers, in order.
type placed struct {
	version id.ID
	parts   []id.ID
	edition string
	works   []placedWork
}

type placedWork struct {
	id, root    id.ID
	kind        catalog.WorkKind
	provisional bool
	matchRev    int64
}

func (p *placed) workIDs() []id.ID {
	out := make([]id.ID, len(p.works))
	for i, w := range p.works {
		out[i] = w.id
	}
	return out
}

// foreign reports whether the version covers a work of a root work other than
// root, meaning its file came from another folder.
func (p *placed) foreign(root id.ID) bool {
	return slices.ContainsFunc(p.works, func(w placedWork) bool { return w.root != root })
}

// placements records where a unit's files are placed, keyed by file.
type placements map[id.ID]*placed

// of returns the version an entry's files make up exactly, or nil.
func (ps placements) of(e *entry) *placed {
	p := ps[e.parts[0].ID]
	if p == nil || !slices.Equal(p.parts, e.ids()) {
		return nil
	}
	return p
}

// anchor returns the root work most of the unit's placed works hang from. For a
// folder with no link yet, that is the work the folder was before it was
// renamed or moved.
func (ps placements) anchor() (id.ID, bool) {
	counts := map[id.ID]int{}
	var best id.ID
	for _, p := range ps.versions() {
		for _, w := range p.works {
			a := w.root
			if counts[a]++; counts[a] > counts[best] {
				best = a
			}
		}
	}
	return best, !best.IsZero()
}

// versions returns each placed version once, in a stable order.
func (ps placements) versions() []*placed {
	var out []*placed
	for _, p := range ps {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b *placed) int { return bytes.Compare(a.version[:], b.version[:]) })
	return out
}

// readPlacements reads where files are placed, through q. q may be in or
// out of a transaction.
func readPlacements(ctx context.Context, q *dbq.Queries, files []id.ID) (placements, error) {
	rows, err := q.Placements(ctx, id.Set(files...))
	if err != nil {
		return nil, err
	}
	byVersion := map[id.ID]*placed{}
	ps := placements{}
	var versions []id.ID
	for _, r := range rows {
		p := byVersion[r.VersionID]
		if p == nil {
			p = &placed{version: r.VersionID, edition: r.Edition.String}
			byVersion[r.VersionID] = p
			versions = append(versions, r.VersionID)
		}
		ps[r.FileID] = p
		if slices.ContainsFunc(p.works, func(w placedWork) bool { return w.id == r.WorkID }) {
			continue // there is one row per part
		}
		p.works = append(p.works, placedWork{
			id: r.WorkID, root: r.RootID, kind: catalog.WorkKind(r.Kind), provisional: r.Provisional, matchRev: r.MatchRev,
		})
	}
	if len(versions) == 0 {
		return ps, nil
	}
	parts, err := q.VersionParts(ctx, id.Set(versions...))
	if err != nil {
		return nil, err
	}
	for _, r := range parts {
		p := byVersion[r.VersionID]
		p.parts = append(p.parts, r.FileID)
	}
	return ps, nil
}

// check fails with errStale if a unit's files changed content, or its works
// changed match, since ps was read. A decision made from the old state must
// not be written over the new one.
func check(ctx context.Context, q *dbq.Queries, u *unit, ps placements) error {
	files := u.files()
	ids := u.fileIDs()
	gens, err := q.FileGenerations(ctx, id.Set(ids...))
	if err != nil {
		return err
	}
	if len(gens) != len(files) {
		return errStale // gone, or merged into another file by a move
	}
	want := map[id.ID]int64{}
	for _, f := range files {
		want[f.ID] = f.Gen
	}
	for _, g := range gens {
		if want[g.ID] != g.Generation {
			return errStale
		}
	}
	revs := map[id.ID]int64{}
	for _, p := range ps.versions() {
		for _, w := range p.works {
			revs[w.id] = w.matchRev
		}
	}
	if len(revs) == 0 {
		return nil
	}
	ids = ids[:0]
	for w := range revs {
		ids = append(ids, w)
	}
	cur, err := q.MatchRevs(ctx, id.Set(ids...))
	if err != nil {
		return err
	}
	if len(cur) != len(revs) {
		return errStale // merged into another file
	}
	for _, r := range cur {
		if revs[r.ID] != r.MatchRev {
			return errStale
		}
	}
	return nil
}
