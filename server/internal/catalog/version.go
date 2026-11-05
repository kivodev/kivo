package catalog

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/catalog/dbq"
	"github.com/mmcdole/kivo/internal/id"
)

// Version is a playable unit. Parts are its files in play order, and Works
// are the works it covers, in order. Edition is the cut its files' edition
// tag names, such as "Final Cut", or empty.
type Version struct {
	ID      id.ID
	Parts   []id.ID
	Works   []id.ID
	Edition string
}

// VersionOrder is what orders a work's versions.
type VersionOrder struct {
	Edition string
	Height  int   // the first part's picture height; zero for none
	Size    int64 // the size of all its parts
}

// CompareVersions orders a work's versions: untagged versions first, then
// by edition label, ignoring case, and within an edition the taller picture
// first, then the larger size.
func CompareVersions(a, b VersionOrder) int {
	return cmp.Or(compareEditions(a.Edition, b.Edition), cmp.Compare(b.Height, a.Height), cmp.Compare(b.Size, a.Size))
}

func compareEditions(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return -1
	case b == "":
		return 1
	}
	return cmp.Or(cmp.Compare(strings.ToLower(a), strings.ToLower(b)), cmp.Compare(a, b))
}

// VersionInput is a version to make. Parts are its files in play order, and
// Works are the works it covers, in order. Edition is its edition label.
type VersionInput struct {
	Parts   []id.ID
	Works   []id.ID
	Edition string
}

// Reassignment describes what ReassignVersion changed. userstate.FollowVersion
// reads it to move watch state.
type Reassignment struct {
	Version id.ID
	From    []id.ID // the works the version covered before
	Moves   []Move  // one per work in From the version no longer covers, in From's order
}

// Move pairs a work a version left with the new work that takes its place.
// When the version covers as many works as before, To is the work at the
// same position. Otherwise To is the first new work.
type Move struct {
	From, To id.ID
	Emptied  bool // From is left with no version
}

// PutVersion makes files into one version covering works. If the parts
// already form exactly one version, that version's works are replaced.
// Otherwise any version they belonged to is dissolved, leaving its other
// files to be grouped again, and a new version is made. A work that had no
// version gets a new added time, so it counts as recently added again.
func (b *Batch) PutVersion(ctx context.Context, in VersionInput) (id.ID, error) {
	if len(in.Parts) == 0 || len(in.Works) == 0 || hasDuplicates(in.Parts) || hasDuplicates(in.Works) {
		return id.ID{}, fmt.Errorf("%w: a version needs distinct parts and works", ErrInvalid)
	}
	q := b.q()
	if err := readd(ctx, q, in.Works); err != nil {
		return id.ID{}, err
	}
	libs, err := q.FileLibraries(ctx, id.Set(in.Parts...))
	if err != nil {
		return id.ID{}, err
	}
	if len(libs) != 1 {
		return id.ID{}, fmt.Errorf("%w: a version's parts must be in one library", ErrInvalid)
	}
	if err := inRootLibrary(ctx, q, libs[0], in.Works); err != nil {
		return id.ID{}, err
	}
	old, err := q.VersionsOfFiles(ctx, id.Set(in.Parts...))
	if err != nil {
		return id.ID{}, err
	}
	affected, err := q.WorksOfVersions(ctx, id.Set(old...))
	if err != nil {
		return id.ID{}, err
	}
	affected = append(affected, in.Works...)

	if len(old) == 1 {
		parts, err := q.VersionParts(ctx, old[0])
		if err != nil {
			return id.ID{}, err
		}
		if slices.EqualFunc(parts, in.Parts, func(p dbq.VersionPartsRow, f id.ID) bool { return p.FileID == f }) {
			if err := setWorks(ctx, q, old[0], in.Works); err != nil {
				return id.ID{}, err
			}
			if err := q.SetVersionEdition(ctx, dbq.SetVersionEditionParams{ID: old[0], Edition: nullString(in.Edition)}); err != nil {
				return id.ID{}, err
			}
			if err := b.add(ctx, q, affected); err != nil {
				return id.ID{}, err
			}
			return old[0], nil
		}
	}
	if err := deleteVersions(ctx, q, old); err != nil {
		return id.ID{}, err
	}
	v := id.New()
	if err := q.InsertVersion(ctx, dbq.InsertVersionParams{ID: v, Edition: nullString(in.Edition), CreatedAt: time.Now().UnixMilli()}); err != nil {
		return id.ID{}, err
	}
	for i, f := range in.Parts {
		err := q.SetPart(ctx, dbq.SetPartParams{VersionID: v, Part: sql.NullInt64{Int64: int64(i + 1), Valid: true}, FileID: f})
		if err != nil {
			return id.ID{}, err
		}
	}
	if err := setWorks(ctx, q, v, in.Works); err != nil {
		return id.ID{}, err
	}
	if err := b.add(ctx, q, affected); err != nil {
		return id.ID{}, err
	}
	return v, nil
}

// inRootLibrary checks that the root works of works are in lib, which is
// the library of a version they are about to gain. A root with no library
// yet is put in lib. All of a root's versions are in its library, so a
// root in another library is ErrInvalid.
func inRootLibrary(ctx context.Context, q *dbq.Queries, lib id.ID, works []id.ID) error {
	ids := id.Set(works...)
	if err := q.SetRootLibrary(ctx, dbq.SetRootLibraryParams{LibraryID: lib, Ids: ids}); err != nil {
		return err
	}
	elsewhere, err := q.RootsElsewhere(ctx, dbq.RootsElsewhereParams{LibraryID: lib, Ids: ids})
	if err != nil {
		return err
	}
	if elsewhere != 0 {
		return fmt.Errorf("%w: a version must be in its root work's library", ErrInvalid)
	}
	return nil
}

// readd sets the added time of works with no version to now, because they
// are about to gain one. A work that comes back after its files were
// deleted counts as newly added.
func readd(ctx context.Context, q *dbq.Queries, works []id.ID) error {
	none, err := q.VersionlessWorks(ctx, id.Set(works...))
	if err != nil || len(none) == 0 {
		return err
	}
	return q.SetAddedAt(ctx, dbq.SetAddedAtParams{AddedAt: time.Now().UnixMilli(), Ids: id.Set(none...)})
}

// deleteVersions detaches the versions' files and then deletes the versions.
func deleteVersions(ctx context.Context, q *dbq.Queries, versions []id.ID) error {
	if len(versions) == 0 {
		return nil
	}
	if err := q.ClearParts(ctx, id.Set(versions...)); err != nil {
		return err
	}
	return q.DeleteVersions(ctx, id.Set(versions...))
}

func setWorks(ctx context.Context, q *dbq.Queries, v id.ID, works []id.ID) error {
	if err := q.DeleteVersionWorks(ctx, v); err != nil {
		return err
	}
	for i, w := range works {
		if err := q.InsertVersionWork(ctx, dbq.InsertVersionWorkParams{VersionID: v, WorkID: w, Ord: int64(i)}); err != nil {
			return err
		}
	}
	return nil
}

// ReassignVersion points a version at different works and bumps the match
// revision of every work involved. A work that had no version gets a new
// added time, as PutVersion does.
func (b *Batch) ReassignVersion(ctx context.Context, v id.ID, works []id.ID) (Reassignment, error) {
	if len(works) == 0 || hasDuplicates(works) {
		return Reassignment{}, fmt.Errorf("%w: a version needs distinct works", ErrInvalid)
	}
	q := b.q()
	from, err := q.VersionWorks(ctx, v)
	if err != nil {
		return Reassignment{}, err
	}
	if len(from) == 0 {
		return Reassignment{}, fmt.Errorf("%w: version %s", ErrNotFound, v)
	}
	lib, err := q.VersionLibrary(ctx, v)
	if err != nil {
		return Reassignment{}, err
	}
	if err := inRootLibrary(ctx, q, lib, works); err != nil {
		return Reassignment{}, err
	}
	if err := readd(ctx, q, works); err != nil {
		return Reassignment{}, err
	}
	if err := setWorks(ctx, q, v, works); err != nil {
		return Reassignment{}, err
	}
	all := append(slices.Clone(from), works...)
	if err := q.BumpMatchRev(ctx, id.Set(all...)); err != nil {
		return Reassignment{}, err
	}
	r := Reassignment{Version: v, From: from}
	for i, w := range from {
		if slices.Contains(works, w) {
			continue
		}
		has, err := q.WorkHasVersions(ctx, w)
		if err != nil {
			return Reassignment{}, err
		}
		m := Move{From: w, To: works[0], Emptied: has == 0}
		if len(from) == len(works) {
			m.To = works[i]
		}
		r.Moves = append(r.Moves, m)
	}
	if err := b.add(ctx, q, all); err != nil {
		return Reassignment{}, err
	}
	return r, nil
}

// SetVersionEdition labels a version with the cut its files' edition tag
// names. An empty label clears it.
func (b *Batch) SetVersionEdition(ctx context.Context, v id.ID, label string) error {
	return b.q().SetVersionEdition(ctx, dbq.SetVersionEditionParams{ID: v, Edition: nullString(label)})
}

// MergeEpisode folds an episode into the episode of the same series that
// replaced it. Its versions move over and it is deleted. It is for an
// episode with no external ID that a match emptied, either a provisional
// episode or one left from the series' previous match. userstate moves its
// watch state first, in the same transaction.
func (b *Batch) MergeEpisode(ctx context.Context, from, into id.ID) error {
	q := b.q()
	f, err := episodesOfOneSeries(ctx, q, from, into)
	if err != nil {
		return err
	}
	has, err := q.WorkHasVersions(ctx, from)
	if err != nil {
		return err
	}
	if has != 0 {
		if err := readd(ctx, q, []id.ID{into}); err != nil { // into is about to gain from's versions
			return err
		}
	}
	if err := q.MoveVersionWorks(ctx, dbq.MoveVersionWorksParams{Into: into, From: from}); err != nil {
		return err
	}
	if err := q.DropVersionWorks(ctx, from); err != nil { // links of versions that covered both episodes
		return err
	}
	if err := unindex(ctx, b.tx, f.Rid); err != nil {
		return err
	}
	if err := b.add(ctx, q, []id.ID{f.ParentID}); err != nil { // the season from leaves
		return err
	}
	if err := q.DeleteWork(ctx, from); err != nil {
		return err
	}
	if err := q.BumpMatchRev(ctx, id.Set(into)); err != nil {
		return err
	}
	return b.add(ctx, q, []id.ID{into})
}

// episodesOfOneSeries returns from's work when from and into are two
// different episodes of one series, and ErrInvalid otherwise.
func episodesOfOneSeries(ctx context.Context, q *dbq.Queries, from, into id.ID) (dbq.Work, error) {
	f, err := q.GetWork(ctx, from)
	if err != nil {
		return dbq.Work{}, notFound(err)
	}
	t, err := q.GetWork(ctx, into)
	if err != nil {
		return dbq.Work{}, notFound(err)
	}
	if from == into || WorkKind(f.Kind) != KindEpisode || WorkKind(t.Kind) != KindEpisode || f.RootID != t.RootID {
		return dbq.Work{}, fmt.Errorf("%w: an episode merges only into another episode of its series", ErrInvalid)
	}
	return f, nil
}

// Version returns a version's parts and works, in order, and its edition
// label.
func (c *Catalog) Version(ctx context.Context, v id.ID) (Version, error) {
	parts, err := c.q.VersionParts(ctx, v)
	if err != nil {
		return Version{}, err
	}
	if len(parts) == 0 {
		return Version{}, fmt.Errorf("%w: version %s", ErrNotFound, v)
	}
	works, err := c.q.VersionWorks(ctx, v)
	if err != nil {
		return Version{}, err
	}
	edition, err := c.q.VersionEdition(ctx, v)
	if err != nil {
		return Version{}, err
	}
	out := Version{ID: v, Works: works, Edition: edition.String}
	for _, p := range parts {
		out.Parts = append(out.Parts, p.FileID)
	}
	return out, nil
}

// VersionOfFile returns the version a file is a part of.
func (c *Catalog) VersionOfFile(ctx context.Context, file id.ID) (Version, error) {
	vs, err := c.q.VersionsOfFiles(ctx, id.Set(file))
	if err != nil {
		return Version{}, err
	}
	if len(vs) == 0 {
		return Version{}, fmt.Errorf("%w: file %s", ErrNotFound, file)
	}
	return c.Version(ctx, vs[0])
}

// VersionsOf returns a work's versions with no gone part, in the order
// CompareVersions gives.
func (c *Catalog) VersionsOf(ctx context.Context, work id.ID) ([]Version, error) {
	rows, err := c.q.VersionsOfWork(ctx, work)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(rows, func(a, b dbq.VersionsOfWorkRow) int {
		return CompareVersions(VersionOrder{a.Edition, int(a.Height), a.Size}, VersionOrder{b.Edition, int(b.Height), b.Size})
	})
	out := make([]Version, 0, len(rows))
	for _, r := range rows {
		ver, err := c.Version(ctx, r.VersionID)
		if err != nil {
			return nil, err
		}
		out = append(out, ver)
	}
	return out, nil
}

func hasDuplicates(ids []id.ID) bool {
	seen := map[id.ID]bool{}
	for _, x := range ids {
		if seen[x] {
			return true
		}
		seen[x] = true
	}
	return false
}
