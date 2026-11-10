package userstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/users"
	"github.com/mmcdole/kivo/internal/userstate/dbq"
)

// Change is a user's own edit of their state for a work. A nil field is
// left as it is.
type Change struct {
	// Played marks the work played or unplayed by hand. For a series or
	// season it marks every episode under it. Either way the position goes
	// back to the start.
	Played *bool
	// Offline is a position played from a download, outside any play.
	Offline *Offline
}

// Offline is a position a user reached in a download while the server
// couldn't be reached. Update writes it to each of the version's works
// whose state is older than At, so the latest play wins whichever client it
// was on. It is written the same way as a play's progress. It finishes a
// work at the played threshold, which for a book means 5 minutes or less
// left. It is also recorded as a play in the watch history.
type Offline struct {
	Version  id.ID         // the version played, which must belong to the work
	Position time.Duration // on the version's timeline
	At       time.Time     // when the client reached the position
}

// Update applies a change to the Viewer's user's state for a work the
// Viewer can see. It makes one write, then notifies the user's clients of
// the works it changed. An offline position for a version that doesn't
// belong to the work returns catalog.ErrNotFound.
func (s *Service) Update(ctx context.Context, v users.Viewer, work id.ID, c Change) error {
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return err
	}
	user := v.User()
	var off *position
	if o := c.Offline; o != nil {
		var err error
		if off, err = s.offlinePosition(ctx, user, work, *o); err != nil {
			return err
		}
	}
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		named := []id.ID{work}
		if off != nil {
			if err := offline(ctx, q, off); err != nil {
				return err
			}
			named = append(named, off.works...)
		}
		at := time.Now().UnixMilli()
		if c.Played != nil {
			works, err := q.Episodes(ctx, work)
			if err != nil {
				return err
			}
			for _, w := range works {
				if err := q.SetPlayed(ctx, dbq.SetPlayedParams{UserID: user, WorkID: w, Played: *c.Played, At: at}); err != nil {
					return err
				}
			}
			named = append(named, works...)
		}
		s.changed(tx, user, distinct(named)...)
		return nil
	})
}

// RemoveFromContinue takes a work the Viewer can see out of the Viewer's
// user's Continue Watching or Continue Listening, keeping its position and
// played state. An episode or season takes its whole series out. The work
// comes back with the next play of it, or of any episode of the series.
func (s *Service) RemoveFromContinue(ctx context.Context, v users.Viewer, work id.ID) error {
	if err := s.deps.Catalog.Visible(ctx, v, work); err != nil {
		return err
	}
	w, err := s.deps.Catalog.Work(ctx, work)
	if err != nil {
		return err
	}
	target := work
	if w.Kind == catalog.KindEpisode || w.Kind == catalog.KindSeason {
		target = w.Root
	}
	user := v.User()
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		err := dbq.New(tx).RemoveFromContinue(ctx, dbq.RemoveFromContinueParams{UserID: user, WorkID: target, At: time.Now().UnixMilli()})
		if err != nil {
			return err
		}
		s.changed(tx, user, distinct([]id.ID{work, target})...)
		return nil
	})
}

// distinct returns ids without repeats, keeping the first of each.
func distinct(ids []id.ID) []id.ID {
	seen := make(map[id.ID]bool, len(ids))
	return slices.DeleteFunc(ids, func(x id.ID) bool {
		if seen[x] {
			return true
		}
		seen[x] = true
		return false
	})
}

// offlinePosition builds a user's offline position in a work, to be written
// the same way as a play's position.
func (s *Service) offlinePosition(ctx context.Context, user, work id.ID, o Offline) (*position, error) {
	p, err := s.position(ctx, id.New(), user, o.Version, o.Position, o.At)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(p.works, work) {
		return nil, fmt.Errorf("%w: the version isn't the work's", catalog.ErrNotFound)
	}
	return p, nil
}

// offline writes an offline position to each of the version's works whose
// state is older, with a play event for each.
func offline(ctx context.Context, q *dbq.Queries, p *position) error {
	for _, w := range p.works {
		st, err := getState(ctx, q, p.user, w)
		if err != nil {
			return err
		}
		if st.UpdatedAt >= p.at.UnixMilli() {
			continue // the server has something newer
		}
		if err := writePosition(ctx, q, p, w, false); err != nil {
			return err
		}
	}
	return nil
}

// SetRating sets a user's rating of a work. A nil rating clears it.
func (s *Service) SetRating(ctx context.Context, user, work id.ID, rating *int) error {
	r := sql.NullInt64{}
	if rating != nil {
		r = sql.NullInt64{Int64: int64(*rating), Valid: true}
	}
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).SetRating(ctx, dbq.SetRatingParams{UserID: user, WorkID: work, Rating: r, At: time.Now().UnixMilli()})
	})
	if err != nil {
		return err
	}
	return nil
}

// Resume returns where a user left off in a work, for a play of version,
// on the version's timeline. It returns zero to start from the beginning.
// That includes a position before the "Continue Watching" floor, which
// pages show as "Play" rather than "Resume". The position belongs to the
// work and applies to its versions of the same edition as the version it
// was saved on. A version of another edition starts from the beginning.
func (s *Service) Resume(ctx context.Context, user, work, version id.ID) (time.Duration, error) {
	st, err := s.q.GetState(ctx, dbq.GetStateParams{UserID: user, WorkID: work})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !st.Resumable {
		return 0, nil
	}
	same, err := s.sameEdition(ctx, st.VersionID, version)
	if err != nil || !same {
		return 0, err
	}
	return time.Duration(st.PositionMs) * time.Millisecond, nil
}

// sameEdition reports whether version is of the edition that saved, the
// version a position was saved on, is. A position saved on no known
// version, or on one that is gone, applies to every version.
func (s *Service) sameEdition(ctx context.Context, saved, version id.ID) (bool, error) {
	if saved.IsZero() || saved == version {
		return true, nil
	}
	a, err := s.deps.Catalog.Version(ctx, saved)
	if errors.Is(err, catalog.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	b, err := s.deps.Catalog.Version(ctx, version)
	if err != nil {
		return false, err
	}
	return a.Edition == b.Edition, nil
}

// LastVersion returns the version a user last played of a work, or zero if
// none. The version may be gone.
func (s *Service) LastVersion(ctx context.Context, user, work id.ID) (id.ID, error) {
	st, err := s.q.GetState(ctx, dbq.GetStateParams{UserID: user, WorkID: work})
	if errors.Is(err, sql.ErrNoRows) {
		return id.ID{}, nil
	}
	return st.VersionID, err
}

// TrackChoice is the audio and subtitle tracks a user last chose for a
// series, by language and kind.
type TrackChoice struct {
	AudioLanguage, SubtitleLanguage string
	SubtitleKind                    string // off, forced, full or sdh
}

// TrackChoice returns a user's last choice for a series, if any.
func (s *Service) TrackChoice(ctx context.Context, user, series id.ID) (TrackChoice, bool, error) {
	c, err := s.q.GetTrackChoice(ctx, dbq.GetTrackChoiceParams{UserID: user, SeriesID: series})
	if errors.Is(err, sql.ErrNoRows) {
		return TrackChoice{}, false, nil
	}
	if err != nil {
		return TrackChoice{}, false, err
	}
	return TrackChoice{AudioLanguage: c.AudioLanguage.String, SubtitleLanguage: c.SubtitleLanguage.String, SubtitleKind: c.SubtitleKind.String}, true, nil
}

// SeriesChoice returns the series whose choice applies to a user's play of
// a work, and the user's last choice for it, if any. The series is zero
// unless the user remembers track choices and the work is an episode of a
// series. When only the choice can't be read, the series is still
// returned with the error.
func (s *Service) SeriesChoice(ctx context.Context, u users.User, work id.ID) (series id.ID, c TrackChoice, ok bool, err error) {
	if !u.Playback.RememberTracks {
		return id.ID{}, TrackChoice{}, false, nil
	}
	w, err := s.deps.Catalog.Work(ctx, work)
	if err != nil || w.Kind != catalog.KindEpisode || w.Root.IsZero() {
		return id.ID{}, TrackChoice{}, false, err
	}
	c, ok, err = s.TrackChoice(ctx, u.ID, w.Root)
	return w.Root, c, ok, err
}

// SetTrackChoice remembers a user's audio and subtitle choice for a series.
func (s *Service) SetTrackChoice(ctx context.Context, user, series id.ID, c TrackChoice) error {
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).SetTrackChoice(ctx, dbq.SetTrackChoiceParams{
			UserID: user, SeriesID: series, AudioLanguage: nullString(c.AudioLanguage),
			SubtitleLanguage: nullString(c.SubtitleLanguage), SubtitleKind: nullString(c.SubtitleKind), UpdatedAt: time.Now().UnixMilli(),
		})
	})
	if err != nil {
		return err
	}
	return nil
}

// MergeEpisode moves an episode's watch state to the episode of its series
// that replaced it. It runs in the same transaction, before catalog's
// Batch.MergeEpisode deletes the old episode. The two states are unioned:
//   - The result is played if either was.
//   - The latest position, with the version it is on, and the latest
//     rating win.
//   - The play count comes from the events. It is never summed, so nothing
//     counts twice.
func (s *Service) MergeEpisode(ctx context.Context, tx *store.Tx, from, into id.ID) error {
	q := dbq.New(tx)
	if err := q.MoveEvents(ctx, dbq.MoveEventsParams{Into: into, From: from}); err != nil {
		return err
	}
	if err := q.MoveListWorks(ctx, dbq.MoveListWorksParams{Into: into, From: from}); err != nil {
		return err
	}
	if err := q.DeleteWorkFromLists(ctx, from); err != nil {
		return err
	}
	states, err := q.StatesOfWork(ctx, from)
	if err != nil {
		return err
	}
	for _, a := range states {
		b, err := q.GetState(ctx, dbq.GetStateParams{UserID: a.UserID, WorkID: into})
		if errors.Is(err, sql.ErrNoRows) {
			b = dbq.UserState{UserID: a.UserID, WorkID: into}
		} else if err != nil {
			return err
		}
		events, err := q.CompletedPlays(ctx, dbq.CompletedPlaysParams{UserID: a.UserID, WorkID: into})
		if err != nil {
			return err
		}
		if err := q.PutState(ctx, union(a, b, events)); err != nil {
			return err
		}
	}
	return q.DeleteStates(ctx, from)
}

// union merges a, the work going away, into b. The play count comes from
// the completed events, or from a count set by hand where that is higher,
// because marking played by hand records no event.
func union(a, b dbq.UserState, completedEvents int64) dbq.PutStateParams {
	latest := b
	if a.LastPlayedAt.Int64 > b.LastPlayedAt.Int64 {
		latest = a
	}
	rating := b.Rating
	if a.Rating.Valid && (!b.Rating.Valid || a.UpdatedAt > b.UpdatedAt) {
		rating = a.Rating
	}
	return dbq.PutStateParams{
		UserID: b.UserID, WorkID: b.WorkID,
		Played:       a.Played || b.Played,
		PlayCount:    max(completedEvents, a.PlayCount, b.PlayCount),
		PositionMs:   latest.PositionMs,
		Resumable:    latest.Resumable,
		VersionID:    latest.VersionID,
		Rating:       rating,
		LastPlayedAt: latest.LastPlayedAt,
		UpdatedAt:    max(a.UpdatedAt, b.UpdatedAt),
	}
}

// FollowVersion moves watch state after catalog.ReassignVersion moved a
// version between episodes of one series. It runs in the same transaction.
// The version's plays, the played mark they make, and the resume position
// move to the episode the version now covers. So what you did with the file
// stays with the file, whatever order the version's moves come in. The
// episode keeps its rating and any played mark set by hand.
// An episode left with no version (Move.Emptied) that was never an episode
// of the series' match is then merged into its replacement by MergeEpisode.
func (s *Service) FollowVersion(ctx context.Context, tx *store.Tx, r catalog.Reassignment) error {
	q := dbq.New(tx)
	for _, m := range r.Moves {
		a, b := m.From, m.To
		moved, err := q.MoveVersionEvents(ctx, dbq.MoveVersionEventsParams{Into: b, From: a, VersionID: r.Version})
		if err != nil {
			return err
		}
		plays := map[id.ID]int64{}
		for _, ev := range moved {
			n := plays[ev.UserID]
			if ev.Completed {
				n++
			}
			plays[ev.UserID] = n
		}
		states, err := q.StatesOfWork(ctx, a)
		if err != nil {
			return err
		}
		for _, sa := range states {
			if _, ok := plays[sa.UserID]; !ok {
				plays[sa.UserID] = 0
			}
		}
		for user, n := range plays {
			if err := moveResume(ctx, q, user, a, b, n); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveResume moves the resume position and n plays from a to b. Once its
// last play has moved, a is no longer played.
func moveResume(ctx context.Context, q *dbq.Queries, user, a, b id.ID, n int64) error {
	sa, err := getState(ctx, q, user, a)
	if err != nil {
		return err
	}
	sb, err := getState(ctx, q, user, b)
	if err != nil {
		return err
	}
	if sa.Resumable && sa.LastPlayedAt.Int64 >= sb.LastPlayedAt.Int64 {
		sb.PositionMs, sb.Resumable, sb.LastPlayedAt, sb.VersionID = sa.PositionMs, sa.Resumable, sa.LastPlayedAt, sa.VersionID
		sa.PositionMs, sa.Resumable = 0, false
	}
	sb.PlayCount += n
	sa.PlayCount = max(0, sa.PlayCount-n)
	sa.Played = sa.Played && (n == 0 || sa.PlayCount > 0)
	sb.Played = sb.Played || n > 0
	now := time.Now().UnixMilli()
	for _, st := range []dbq.UserState{sa, sb} {
		err := q.PutState(ctx, dbq.PutStateParams{
			UserID: st.UserID, WorkID: st.WorkID, Played: st.Played, PlayCount: st.PlayCount, PositionMs: st.PositionMs,
			Resumable: st.Resumable, VersionID: st.VersionID, Rating: st.Rating, LastPlayedAt: st.LastPlayedAt, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func getState(ctx context.Context, q *dbq.Queries, user, work id.ID) (dbq.UserState, error) {
	st, err := q.GetState(ctx, dbq.GetStateParams{UserID: user, WorkID: work})
	if errors.Is(err, sql.ErrNoRows) {
		return dbq.UserState{UserID: user, WorkID: work}, nil
	}
	return st, err
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
