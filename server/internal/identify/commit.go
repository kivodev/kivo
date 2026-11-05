package identify

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path"
	"slices"
	"strconv"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/metadata"
)

// Match statuses.
const (
	statusMatched   = "matched"
	statusUnmatched = "unmatched"
)

// place puts every file of the unit on the unit's root work, the work its
// folder stands for (see root), and returns that root.
//
// A file in no version yet gets a new version. If its regrouped parts
// already covered works under the root, as when cd2 arrives after cd1, the
// new version covers those works. Otherwise it covers provisional works
// under the root, made from the file's name, which can play before any
// match.
//
// A version on another root work, such as a file moved in from another
// folder, moves to provisional works under this root. Its history stays where
// it was. A version takes the edition label its files' names give it, and a
// movie the label its folder's names give it. New files are recorded as
// added, under the first new file's work. A folder whose files are all where
// they belong writes nothing.
func (i *Identifier) place(ctx context.Context, s *snapshot, ps placements) (id.ID, error) {
	root, err := i.q.FolderWork(ctx, dbq.FolderWorkParams{LibraryID: s.lib.ID, Subject: s.unit.subject})
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return id.ID{}, err
	default:
		settled, err := i.settled(ctx, s.unit, ps, root)
		if err != nil || settled {
			return root, err
		}
	}
	err = i.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		if err := check(ctx, dbq.New(tx), s.unit, ps); err != nil {
			return err
		}
		var err error
		if root, err = i.root(ctx, tx, s, ps); err != nil {
			return err
		}
		if err := i.label(ctx, tx, s.unit, root); err != nil {
			return err
		}
		news, err := i.placeFiles(ctx, tx, s, ps, root)
		if err != nil || news.n == 0 {
			return err
		}
		return i.deps.Activity.FilesAdded(ctx, tx, s.lib.ID, news.first, news.n)
	})
	return root, err
}

// settled reports whether every file of the unit is on its root work, each
// version has its edition label, and a movie has its label.
func (i *Identifier) settled(ctx context.Context, u *unit, ps placements, root id.ID) (bool, error) {
	if slices.ContainsFunc(u.entries(), func(e *entry) bool { p := ps.of(e); return p == nil || p.foreign(root) || p.edition != e.edition }) {
		return false, nil
	}
	if u.kind != catalog.KindMovie {
		return true, nil
	}
	st, err := i.q.WorkState(ctx, root)
	return st.Edition.String == u.edition, err
}

// label gives a movie the label its folder's names give it.
func (i *Identifier) label(ctx context.Context, tx *store.Tx, u *unit, root id.ID) error {
	if u.kind != catalog.KindMovie {
		return nil
	}
	st, err := dbq.New(tx).WorkState(ctx, root)
	if err != nil || st.Edition.String == u.edition {
		return err
	}
	return i.deps.Catalog.SetEdition(ctx, tx, root, u.edition)
}

// placeFiles places the unit's files that aren't on its root work yet, and
// counts the new ones. It labels each version with its edition.
func (i *Identifier) placeFiles(ctx context.Context, tx *store.Tx, s *snapshot, ps placements, root id.ID) (added, error) {
	u := s.unit
	var news added
	err := i.deps.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error {
		for _, e := range u.entries() {
			pl := ps.of(e)
			if pl != nil && !pl.foreign(root) {
				if err := relabel(ctx, b, e, pl); err != nil {
					return err
				}
				continue
			}
			works := regrouped(e, ps, root)
			if works == nil {
				var err error
				if works, err = i.provisionalWorks(ctx, tx, u, e, root); err != nil {
					return err
				}
			}
			if pl != nil {
				if _, err := b.ReassignVersion(ctx, pl.version, works); err != nil {
					return err
				}
				if err := relabel(ctx, b, e, pl); err != nil {
					return err
				}
				continue
			}
			if _, err := b.PutVersion(ctx, catalog.VersionInput{Parts: e.ids(), Works: works, Edition: e.edition}); err != nil {
				return err
			}
			news.add(e, works)
			if u.kind == catalog.KindVideo {
				if err := i.describeVideo(ctx, tx, s, e, root); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return news, err
}

// relabel gives a placed version the edition label its files' names give
// it, if it has another.
func relabel(ctx context.Context, b *catalog.Batch, e *entry, pl *placed) error {
	if pl.edition == e.edition {
		return nil
	}
	return b.SetVersionEdition(ctx, pl.version, e.edition)
}

// regrouped returns the works that a part of a new version already covered,
// if those works are under the root work. Otherwise it returns nil.
func regrouped(e *entry, ps placements, root id.ID) []id.ID {
	for _, f := range e.parts {
		if p := ps[f.ID]; p != nil && !p.foreign(root) {
			return p.workIDs()
		}
	}
	return nil
}

// added counts a unit's new files, leaving out extras, and keeps the first
// new file's work.
type added struct {
	first id.ID
	n     int
}

func (a *added) add(e *entry, works []id.ID) {
	if e.extra != "" {
		return
	}
	if a.n == 0 {
		a.first = works[0]
	}
	a.n++
}

// provisionalWorks returns the works a version covers under its root work
// before the root is matched. For an extra, it is the extra's own work.
// For a series, it is the episodes the file name numbers. For a movie, a
// book or another video, it is the root itself.
func (i *Identifier) provisionalWorks(ctx context.Context, tx *store.Tx, u *unit, e *entry, root id.ID) ([]id.ID, error) {
	switch {
	case e.extra != "":
		w, err := i.extraWork(ctx, tx, u, root, e)
		return []id.ID{w}, err
	case u.kind == catalog.KindSeries:
		return i.parsedEpisodes(ctx, tx, root, e)
	}
	return []id.ID{root}, nil
}

// describeVideo titles another video from its name and puts it in its
// folder's collection. It then queues the video's local metadata.
func (i *Identifier) describeVideo(ctx context.Context, tx *store.Tx, s *snapshot, e *entry, work id.ID) error {
	m := catalog.WorkMetadata{Title: e.title}
	if e.dir != "" {
		m.Collection = &catalog.CollectionRef{Key: "folder:" + s.lib.ID.String() + "/" + e.dir, Name: path.Base(e.dir)}
	}
	if err := i.deps.Catalog.UpdateMetadata(ctx, tx, work, m); err != nil {
		return err
	}
	return i.deps.Metadata.Refresh(ctx, tx, work)
}

// parsedEpisodes returns a version's provisional episodes, numbered as its
// name says. A name with no number, or with a date, gets an episode of its
// own.
func (i *Identifier) parsedEpisodes(ctx context.Context, tx *store.Tx, series id.ID, e *entry) ([]id.ID, error) {
	g := e.guess
	seasonNo := g.Season
	switch {
	case g.Special:
		seasonNo = 0
	case !g.AirDate.IsZero():
		seasonNo = g.AirDate.Year // a daily show's seasons are years
	}
	season, err := i.season(ctx, tx, series, seasonNo)
	if err != nil {
		return nil, err
	}
	numbers := g.Episodes
	if len(numbers) == 0 && !g.Special {
		numbers = g.Absolute
	}
	if len(numbers) == 0 {
		in := catalog.WorkInput{Kind: catalog.KindEpisode, Parent: season, ParentNumber: seasonNo, Title: cmp.Or(g.Title, stem(e.parts[0].Rel)), Provisional: true}
		if !g.AirDate.IsZero() {
			in.Title, in.ReleaseDate = g.AirDate.String(), g.AirDate.String()
		}
		w, err := i.deps.Catalog.EnsureWork(ctx, tx, in)
		return []id.ID{w}, err
	}
	var out []id.ID
	for _, n := range numbers {
		w, err := i.deps.Catalog.EnsureWork(ctx, tx, catalog.WorkInput{
			Kind: catalog.KindEpisode, Parent: season, ParentNumber: seasonNo, Number: n, Title: "Episode " + strconv.Itoa(n), Provisional: true,
		})
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out, nil
}

// season returns a series' season. Seasons are never provisional, because
// they have no identity of their own beyond their series and number.
func (i *Identifier) season(ctx context.Context, tx *store.Tx, series id.ID, n int) (id.ID, error) {
	title := "Season " + strconv.Itoa(n)
	if n == 0 {
		title = "Specials"
	}
	return i.deps.Catalog.EnsureWork(ctx, tx, catalog.WorkInput{Kind: catalog.KindSeason, Parent: series, Title: title, Number: n})
}

// extraWork returns an extra's work. In a series, an extra in a season
// folder goes under that season. Otherwise it goes under root itself.
func (i *Identifier) extraWork(ctx context.Context, tx *store.Tx, u *unit, root id.ID, e *entry) (id.ID, error) {
	parent := root
	if u.kind == catalog.KindSeries && e.season >= 0 {
		var err error
		if parent, err = i.season(ctx, tx, root, e.season); err != nil {
			return id.ID{}, err
		}
	}
	return i.extra(ctx, tx, parent, e.title, string(e.extra))
}

// extra returns the extra with the given title and type under parent,
// creating it if it is new.
func (i *Identifier) extra(ctx context.Context, tx *store.Tx, parent id.ID, title, kind string) (id.ID, error) {
	w, err := dbq.New(tx).Extra(ctx, dbq.ExtraParams{ParentID: parent, Title: title, ExtraType: sql.NullString{String: kind, Valid: true}})
	if !errors.Is(err, sql.ErrNoRows) {
		return w, err
	}
	return i.deps.Catalog.EnsureWork(ctx, tx, catalog.WorkInput{Kind: catalog.KindExtra, Parent: parent, Title: title, ExtraType: kind})
}

// plan is a unit's decision, ready to be written.
type plan struct {
	s        *snapshot  // the unit's folder
	root     id.ID      // the root work
	ps       placements // where the unit's files are now
	d        decision
	ordering string                        // a series' ordering
	eps      map[*entry][]metadata.Episode // a series' episodes found for each version still to be placed
	// seasonOf maps each episode to its season, as decided sets them.
	seasonOf map[id.ID]id.ID
}

// apply writes a matched decision in tx. The root work becomes what the
// match describes, in place (see describe). Each version moves to the works
// the decision gives it under the root. A metadata refresh is queued, and
// the decision is recorded.
//
// A root with a manual match accepts only a manual decision. fix means an
// admin's Fix match, which locks the root.
func (i *Identifier) apply(ctx context.Context, tx *store.Tx, p plan, fix bool) error {
	q := dbq.New(tx)
	if err := check(ctx, q, p.s.unit, p.ps); err != nil {
		return err
	}
	st, err := q.WorkState(ctx, p.root)
	if err != nil {
		return err
	}
	if st.Locked && !fix && p.d.source != sourceManual {
		return nil
	}
	p.seasonOf = map[id.ID]id.ID{}
	changed := false
	var episodes []id.ID // episodes in a season that got versions
	err = i.deps.Catalog.Batch(ctx, tx, func(b *catalog.Batch) error {
		var err error
		if changed, err = i.describe(ctx, tx, b, p.root, *p.d.match, fix); err != nil {
			return err
		}
		for _, e := range p.s.unit.entries() {
			pl := p.ps.of(e)
			if pl == nil {
				continue
			}
			works, err := i.decided(ctx, tx, b, p, e)
			if err != nil {
				return err
			}
			if slices.Equal(works, pl.workIDs()) {
				continue
			}
			if err := i.reassign(ctx, tx, b, pl.version, works); err != nil {
				return err
			}
			changed = true
			episodes = append(episodes, p.episodes(works)...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if changed {
		if err := i.matched(ctx, tx, p); err != nil {
			return err
		}
		if len(episodes) > 0 {
			if err := i.deps.Markers.Placed(ctx, tx, p.s.lib, episodes); err != nil {
				return err
			}
		}
	}
	return putMatch(ctx, q, p.s.lib.ID, p.s.unit.subject, p.d, p.ordering)
}

// describe makes the root work what the match describes, unless it already
// is, and reports whether it changed. A root that had a different match loses
// its artwork, since that artwork showed the old match.
func (i *Identifier) describe(ctx context.Context, tx *store.Tx, b *catalog.Batch, root id.ID, c metadata.Match, lock bool) (bool, error) {
	q := dbq.New(tx)
	st, err := q.WorkState(ctx, root)
	if err != nil {
		return false, err
	}
	xs, err := q.ExternalIDsOf(ctx, root)
	if err != nil {
		return false, err
	}
	has := slices.Contains(xs, idRow(c.ID))
	if has && !st.Provisional && (st.Locked || !lock) {
		return false, nil
	}
	if err := b.Describe(ctx, root, c.ID, c.Title, c.Year, lock); err != nil {
		return false, err
	}
	if len(xs) > 0 && !has {
		if err := i.deps.Metadata.ClearArtwork(ctx, tx, root); err != nil {
			return false, err
		}
	}
	return true, nil
}

// describes reports whether a root work is already what a match describes,
// meaning it is matched and carries the match's ID.
func (i *Identifier) describes(ctx context.Context, q *dbq.Queries, root id.ID, c *metadata.Match) (bool, error) {
	st, err := q.WorkState(ctx, root)
	if err != nil || st.Provisional {
		return false, err
	}
	xs, err := q.ExternalIDsOf(ctx, root)
	return slices.Contains(xs, idRow(c.ID)), err
}

// idRow converts an external ID to the row type ExternalIDsOf returns.
func idRow(x catalog.ExternalID) dbq.ExternalIDsOfRow {
	return dbq.ExternalIDsOfRow{Provider: x.Provider, Kind: x.Kind, ExtID: x.ID}
}

// episodes returns those of works that are episodes in a season, as
// decided sets them.
func (p plan) episodes(works []id.ID) []id.ID {
	var out []id.ID
	for _, w := range works {
		if !p.seasonOf[w].IsZero() {
			out = append(out, w)
		}
	}
	return out
}

// matched logs a decision that changed the root work or its versions, and
// queues the root's refresh.
func (i *Identifier) matched(ctx context.Context, tx *store.Tx, p plan) error {
	slog.DebugContext(ctx, "identify: matched", "library", p.s.lib.ID, "subject", p.s.unit.subject, "source", p.d.source,
		"provider", p.d.match.ID.Provider, "id", p.d.match.ID.ID, "score", p.d.score)
	return i.deps.Metadata.Refresh(ctx, tx, p.root)
}

// decided returns the works a version covers under a decision, all under its
// root work. In a series, a version whose episodes weren't found stays where
// it is if it is already under the series. Otherwise it gets provisional
// episodes there.
func (i *Identifier) decided(ctx context.Context, tx *store.Tx, b *catalog.Batch, p plan, e *entry) ([]id.ID, error) {
	u, root := p.s.unit, p.root
	switch {
	case e.extra != "":
		w, err := i.extraWork(ctx, tx, u, root, e)
		return []id.ID{w}, err
	case u.kind == catalog.KindMovie:
		return []id.ID{root}, nil
	}
	found, ok := p.eps[e]
	if !ok {
		pl := p.ps.of(e)
		if !pl.foreign(root) {
			return pl.workIDs(), nil // already settled, or unmatched under the series
		}
		return i.parsedEpisodes(ctx, tx, root, e)
	}
	var out []id.ID
	for _, ep := range found {
		season, err := i.season(ctx, tx, root, ep.Season)
		if err != nil {
			return nil, err
		}
		w, err := i.deps.Catalog.EnsureWork(ctx, tx, catalog.WorkInput{
			Kind: catalog.KindEpisode, Parent: season, ParentNumber: ep.Season, Number: ep.Number,
			Title: cmp.Or(ep.Title, "Episode "+strconv.Itoa(ep.Number)), ReleaseDate: ep.AirDate,
			ExternalIDs: []catalog.ExternalID{ep.ID},
		})
		if err != nil {
			return nil, err
		}
		// An episode is keyed by provider ID within its series. So it keeps
		// its work when the ordering renumbers it, and only its place
		// changes.
		at, err := dbq.New(tx).EpisodePlace(ctx, w)
		if err != nil {
			return nil, err
		}
		if at.ParentID != season || int(at.Number.Int64) != ep.Number {
			if err := b.MoveEpisode(ctx, w, season, ep.Number); err != nil {
				return nil, err
			}
		}
		if p.seasonOf != nil {
			p.seasonOf[w] = season
		}
		out = append(out, w)
	}
	return out, nil
}

// reassign points a version at other works of its root work, through catalog
// and userstate in the caller's transaction. The version's plays and resume
// position follow it (see userstate's FollowVersion).
//
// The version may leave an episode with no versions. If that episode has no
// external ID, it was never an episode of the match: it is provisional, or
// left over from the series' previous match. It is merged into the episode
// replacing it, including its state. A matched episode left empty keeps its
// work, its favorite and its rating. A version coming from another root
// brings no history.
func (i *Identifier) reassign(ctx context.Context, tx *store.Tx, b *catalog.Batch, v id.ID, works []id.ID) error {
	q := dbq.New(tx)
	to, err := q.WorkState(ctx, works[0])
	if err != nil {
		return err
	}
	r, err := b.ReassignVersion(ctx, v, works)
	if err != nil {
		return err
	}
	from, err := q.WorkState(ctx, r.From[0])
	if err != nil || from.RootID != to.RootID {
		return err
	}
	if err := i.deps.Userstate.FollowVersion(ctx, tx, r); err != nil {
		return err
	}
	for _, m := range r.Moves {
		if !m.Emptied {
			continue
		}
		if err := i.mergeEmptied(ctx, tx, b, m.From, m.To); err != nil {
			return err
		}
	}
	return nil
}

func putMatch(ctx context.Context, q *dbq.Queries, lib id.ID, subject string, d decision, ordering string) error {
	p := dbq.PutMatchParams{
		LibraryID: lib, Subject: subject, Source: d.source, Query: d.query, Status: statusUnmatched,
		Score: sql.NullInt64{Int64: int64(d.score), Valid: d.source == sourceSearch}, UpdatedAt: time.Now().UnixMilli(),
		Ordering: sql.NullString{String: ordering, Valid: ordering != ""},
	}
	if c := d.match; c != nil {
		p.Status = statusMatched
		p.Provider = sql.NullString{String: c.ID.Provider, Valid: true}
		p.Kind = sql.NullString{String: c.ID.Kind, Valid: true}
		p.ExtID = sql.NullString{String: c.ID.ID, Valid: true}
		p.Title = sql.NullString{String: c.Title, Valid: c.Title != ""}
		p.Year = sql.NullInt64{Int64: int64(c.Year), Valid: c.Year != 0}
	}
	return q.PutMatch(ctx, p)
}
