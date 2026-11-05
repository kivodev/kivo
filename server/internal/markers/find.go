package markers

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/dbq"
	"github.com/mmcdole/kivo/internal/markers/detect"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// fromFingerprint is the source of a marker that detection found.
const fromFingerprint = "fingerprint"

// What detection decided for a file, as its marker_file row records it.
const (
	decidedDone = "done" // it found markers
	decidedNone = "none" // it found none
)

// playedLately is how recently a series must have been played for its
// seasons to have their markers found first.
const playedLately = 30 * 24 * time.Hour

// Placed asks for the markers of each placed episode's files once tx
// commits. It does so only when lib finds markers as media is added.
func (s *Service) Placed(ctx context.Context, tx *store.Tx, lib catalog.Library, episodes []id.ID) error {
	if lib.Markers != catalog.NightlyAndAdded {
		return nil
	}
	files, err := dbq.New(tx).EpisodeFiles(ctx, id.Set(episodes...))
	if err != nil {
		return err
	}
	tx.AfterCommit(func() { s.deps.Tasks.Ask(TaskFindMarkers, files...) })
	return nil
}

// fail records that a file's fingerprint failed at its generation, with
// the reason. It settles a file only for a failure. The done and none
// outcomes are recorded with the markers, by storeFile.
func (s *Service) fail(ctx context.Context, f catalog.File, failure error) error {
	slog.WarnContext(ctx, "markers: can't fingerprint it", "file", f.RelPath, "err", failure)
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		return dbq.New(tx).SetFailed(ctx, dbq.SetFailedParams{
			FileID: f.ID, Generation: f.Generation, Rules: detect.RulesVersion,
			Reason: sql.NullString{String: failure.Error(), Valid: true}, UpdatedAt: time.Now().UnixMilli(),
		})
	})
}

// pending lists what the task has left, in the order markerSeasons gives.
// It lists the episode files, in every library that finds markers, whose
// markers aren't decided at their generation by the current rules and
// haven't failed at it. Without Chromaprint there's nothing to
// fingerprint, so nothing is pending.
func (s *Service) pending(ctx context.Context) ([]id.ID, error) {
	if !s.deps.Media.Chromaprint(ctx) {
		return nil, nil
	}
	rows, err := s.q.MarkerSeasons(ctx, detect.RulesVersion)
	if err != nil {
		return nil, err
	}
	var out []id.ID
	for _, sn := range markerSeasons(rows, time.Now()) {
		for _, f := range sn.files {
			if !f.Settled {
				out = append(out, f.ID)
			}
		}
	}
	return out, nil
}

// left counts the files left.
func (s *Service) left(ctx context.Context) (int, error) {
	if !s.deps.Media.Chromaprint(ctx) {
		return 0, nil // nothing to fingerprint, so nothing pending
	}
	left, err := s.q.CountPending(ctx, detect.RulesVersion)
	return int(left), err
}

// failed lists the files whose fingerprint failed at their generation.
func (s *Service) failed(ctx context.Context) ([]tasks.Failure, error) {
	rows, err := s.q.Failed(ctx)
	out := make([]tasks.Failure, len(rows))
	for i, r := range rows {
		out[i] = tasks.Failure{Item: r.FileID, Reason: r.Reason.String, At: time.UnixMilli(r.UpdatedAt)}
	}
	return out, err
}

// retry forgets the failures of the given files, or of every file when
// none are given. It returns the files to fingerprint again.
func (s *Service) retry(ctx context.Context, files ...id.ID) ([]id.ID, error) {
	var out []id.ID
	err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		var err error
		out, err = dbq.New(tx).Forget(ctx, dbq.ForgetParams{Ids: id.Set(files...), All: len(files) == 0})
		return err
	})
	return out, err
}

// current cheaply reads a file's current generation, to find its result.
func (s *Service) current(ctx context.Context, file id.ID) (int64, error) {
	gen, err := s.q.FileGeneration(ctx, file)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: file %s", catalog.ErrNotFound, file)
	}
	return gen, err
}

// markerSeason is a season the nightly task looks at.
type markerSeason struct {
	id            id.ID
	played, added int64
	files         []dbq.MarkerSeasonsRow
}

// markerSeasons groups the rows by season, in the order the nightly task
// does them. Seasons of series played within playedLately of now come
// first, most recently played first. The rest follow, ordered by their
// episodes' latest addition.
func markerSeasons(rows []dbq.MarkerSeasonsRow, now time.Time) []markerSeason {
	var out []markerSeason
	for _, r := range rows {
		if n := len(out); n == 0 || out[n-1].id != r.SeasonID {
			played := r.PlayedAt
			if now.Sub(time.UnixMilli(played)) > playedLately {
				played = 0
			}
			out = append(out, markerSeason{id: r.SeasonID, played: played, added: r.AddedAt})
		}
		out[len(out)-1].files = append(out[len(out)-1].files, r)
	}
	slices.SortStableFunc(out, func(a, b markerSeason) int {
		return cmp.Or(cmp.Compare(b.played, a.played), cmp.Compare(b.added, a.added))
	})
	return out
}

// decide decides the markers of a season's episodes from their files'
// kept fingerprints, without reading any media, and stores them.
func (s *Service) decide(ctx context.Context, season id.ID) error {
	sn, err := s.season(ctx, season)
	if err != nil || len(sn.files) == 0 {
		return err
	}
	s.fingerprints(&sn)
	return s.storeMarkers(ctx, sn, detect.Decide(sn.markerFiles()))
}

// storeMarkers stores each fingerprinted file's markers at the generation
// they were found for. They replace the markers found before, and the
// file's decision is recorded with them. A file that changed since is left
// for its next run.
func (s *Service) storeMarkers(ctx context.Context, sn seasonFiles, found map[id.ID][]detect.Marker) error {
	return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		for _, f := range sn.files {
			if f.fingerprint == nil {
				continue
			}
			if err := storeFile(ctx, q, f, found[f.ID]); err != nil {
				return err
			}
		}
		return nil
	})
}

// storeFile stores one file's markers and its marker_file row through q,
// unless the file changed since the markers were found.
func storeFile(ctx context.Context, q *dbq.Queries, f seasonFile, found []detect.Marker) error {
	cur, err := q.FileGeneration(ctx, f.ID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && cur != f.Generation {
		return nil
	}
	if err != nil {
		return err
	}
	if err := q.DeleteFoundMarkers(ctx, f.ID); err != nil {
		return err
	}
	for _, m := range found {
		err := q.InsertMarker(ctx, dbq.InsertMarkerParams{
			FileID: f.ID, Generation: f.Generation, Kind: m.Kind, Source: fromFingerprint,
			StartMs: m.Start.Milliseconds(), EndMs: sql.NullInt64{Int64: m.End.Milliseconds(), Valid: true},
			Final: m.Final, Reason: m.Reason,
		})
		if err != nil {
			return err
		}
	}
	state := decidedNone
	if len(found) > 0 {
		state = decidedDone
	}
	return q.SetDecided(ctx, dbq.SetDecidedParams{
		FileID: f.ID, Generation: f.Generation, Rules: detect.RulesVersion, State: state, UpdatedAt: time.Now().UnixMilli(),
	})
}

// pruneMarkers removes the markers and decisions recorded for files at an
// older generation than the file's current one. It works a batch at a time
// and returns how many rows it removed.
func (s *Service) pruneMarkers(ctx context.Context) (int64, error) {
	var total int64
	for _, prune := range []func(*dbq.Queries, context.Context, int64) (int64, error){(*dbq.Queries).PruneMarkers, (*dbq.Queries).PruneDecided} {
		err := s.db.Drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) {
			n, err := prune(dbq.New(tx), ctx, store.BatchSize)
			if err == nil {
				tx.AfterCommit(func() { total += n })
			}
			return int(n), err
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// wanted reports whether any shows library finds markers, which means the
// nightly task has something to do.
func (s *Service) wanted(ctx context.Context) (bool, error) {
	ok, err := s.q.Wanted(ctx)
	if err != nil {
		return false, err
	}
	return ok, nil
}
