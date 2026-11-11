package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/events"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// TaskRefreshMetadata refreshes the metadata of works that are due. It runs
// nightly, as each library's refresh days say, and at once when a refresh is
// asked for.
const TaskRefreshMetadata = "refresh-metadata"

// refreshTimeout bounds one work's refresh, not counting time spent waiting
// out an outage.
const refreshTimeout = 10 * time.Minute

// Refresh makes a work and the seasons and episodes under it due a metadata
// refresh, and asks for them once the transaction commits. It runs in the
// caller's transaction because identify calls it while accepting a match,
// so the refresh becomes due exactly when the match does. Provisional works
// are skipped, since nothing is matched yet.
func (s *Service) Refresh(ctx context.Context, tx *store.Tx, work id.ID) error {
	rows, err := dbq.New(tx).Subtree(ctx, work)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: work %s", ErrNotFound, work)
	}
	var due []id.ID
	for _, r := range rows {
		if !r.Provisional {
			due = append(due, r.ID)
		}
	}
	return s.due(ctx, tx, due)
}

// RefreshWorks is like Refresh but covers only the given works, not the
// seasons and episodes under them. "Refresh all metadata" makes each of those
// due as a work of its own. The works must not be provisional.
func (s *Service) RefreshWorks(ctx context.Context, tx *store.Tx, works ...id.ID) error {
	return s.due(ctx, tx, works)
}

// due makes works due in tx, and asks for them once tx commits.
func (s *Service) due(ctx context.Context, tx *store.Tx, works []id.ID) error {
	if err := dbq.New(tx).MarkDue(ctx, id.Set(works...)); err != nil {
		return err
	}
	tx.AfterCommit(func() { s.deps.Tasks.Enqueue(TaskRefreshMetadata, works...) })
	return nil
}

// pendingRefreshes lists the task's remaining items: the works due a
// refresh.
func (s *Service) pendingRefreshes(ctx context.Context) ([]id.ID, error) {
	return s.q.DueRefreshes(ctx, time.Now().UnixMilli())
}

// failedRefreshes lists the works whose last refresh failed.
func (s *Service) failedRefreshes(ctx context.Context) ([]tasks.Failure, error) {
	rows, err := s.q.FailedRefreshes(ctx)
	out := make([]tasks.Failure, len(rows))
	for i, r := range rows {
		out[i] = tasks.Failure{Item: r.ID, Reason: r.RefreshError.String, At: time.UnixMilli(r.RefreshedAt.Int64)}
	}
	return out, err
}

// retryRefreshes makes the given failed works due again, or every failed
// work when none are given. It returns the works made due.
func (s *Service) retryRefreshes(ctx context.Context, works ...id.ID) ([]id.ID, error) {
	if len(works) == 0 {
		failed, err := s.q.FailedRefreshes(ctx)
		if err != nil {
			return nil, err
		}
		for _, f := range failed {
			works = append(works, f.ID)
		}
	}
	err := store.Batches(ctx, s.db, works, func(ctx context.Context, tx *store.Tx, batch []id.ID) error {
		return dbq.New(tx).MarkDue(ctx, id.Set(batch...))
	})
	return works, err
}

// refreshWork refreshes one work using its current match.
//
// A provider outage is waited out rather than settling for an old answer. If
// the match changes while the refresh runs, the work's refresh is asked for
// again. The task does that once this one is over, so the work ends up with
// the new match's details. Any other failure is recorded, and the work waits
// for the library's next refresh.
func (s *Service) refreshWork(ctx context.Context, work id.ID) error {
	for {
		rctx, cancel := context.WithTimeout(ctx, refreshTimeout)
		err := s.refresh(rctx, work, refreshing)
		cancel()
		var o *outage
		switch {
		case errors.As(err, &o):
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Until(o.until)):
				continue
			}
		case errors.Is(err, catalog.ErrNotFound):
			return nil // the work was merged or deleted meanwhile
		case err == nil || ctx.Err() != nil:
			return err
		}
		slog.WarnContext(ctx, "metadata: refresh failed", "work", work, "err", err)
		return s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			return dbq.New(tx).MarkRefreshFailed(ctx, dbq.MarkRefreshFailedParams{
				ID: work, Now: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true}, Reason: sql.NullString{String: err.Error(), Valid: true},
			})
		})
	}
}

// refresh gathers a work's provider data, local files and artwork, and the
// artwork of a movie's collection and trailers. It writes them all in one transaction. It
// also records when the work was refreshed, unless a provider was down.
//
// A work that was unmatched meanwhile gets nothing written, because nothing
// describes it. That covers a work that became provisional or now sits under
// a provisional root work.
//
// Admin edits outrank local files, which outrank providers. catalog skips
// locked fields, and NFO fields are laid over the provider's here.
//
// fr is how fresh the providers' cached answers must be.
func (s *Service) refresh(ctx context.Context, work id.ID, fr Freshness) error {
	w, err := s.deps.Catalog.Work(ctx, work)
	if err != nil {
		return err
	}
	if w.Provisional || w.Kind == catalog.KindExtra {
		return nil
	}
	lang, country, err := s.Locale(ctx, work)
	if err != nil {
		return err
	}
	f, err := s.fromProviders(ctx, w, lang, country, fr)
	if err != nil {
		return err
	}
	l, err := s.fromLocal(ctx, w)
	if err != nil {
		return err
	}
	var (
		meta   *catalog.WorkMetadata
		photos []photoPlan
		later  error // a book provider's outage, returned after the local data is written
	)
	switch w.Kind {
	case catalog.KindBook:
		b, err := s.describeBook(ctx, w, country, fr)
		if err != nil {
			return err
		}
		meta, l.cover, f, later = b.meta, b.cover, b.found, b.later
		if photos, err = s.planPhotos(ctx, b.authors); err != nil {
			return err
		}
	default:
		meta = merge(w, f, l.nfo, country)
		if f != nil {
			if photos, err = s.planCastPhotos(ctx, credited(w, meta, f.photos)); err != nil {
				return err
			}
		}
	}
	trailers, err := s.planTrailers(ctx, w, meta)
	if err != nil {
		s.deleteEncodedPhotos(photos)
		return err
	}
	plan, err := s.planArtwork(ctx, w, f, l)
	if err != nil {
		s.deleteEncodedPhotos(photos)
		s.deleteEncodedTrailers(trailers)
		return err
	}
	coll, err := s.planCollectionArt(ctx, f)
	if err != nil {
		s.deleteEncoded(plan.add)
		s.deleteEncodedPhotos(photos)
		s.deleteEncodedTrailers(trailers)
		return err
	}

	wrote := false
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		q := dbq.New(tx)
		unmatched, err := q.Unmatched(ctx, work)
		if err != nil || unmatched {
			return notFoundRow(err)
		}
		if meta != nil {
			thumbs, err := q.TrailerImages(ctx, work)
			if err != nil {
				return err
			}
			if err := s.deps.Catalog.UpdateMetadata(ctx, tx, work, *meta); err != nil {
				return err
			}
			if err := s.recordTrailers(ctx, tx, work, thumbs, trailers); err != nil {
				return err
			}
		}
		if later == nil {
			if err := q.MarkRefreshed(ctx, dbq.MarkRefreshedParams{ID: work, Now: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true}}); err != nil {
				return err
			}
		}
		if err := s.recordPhotos(ctx, tx, photos); err != nil {
			return err
		}
		if err := s.recordCollectionArt(ctx, tx, coll); err != nil {
			return err
		}
		wrote = true
		return record(ctx, q, plan)
	})
	if err != nil {
		err = fmt.Errorf("write the refresh: %w", err)
	}
	if err != nil || !wrote {
		s.deleteEncoded(plan.add)
		s.deleteEncoded(coll.add)
		s.deleteEncodedPhotos(photos)
		s.deleteEncodedTrailers(trailers)
		return err
	}
	for _, r := range plan.remove {
		s.deleteFiles(r.ID, r.Kind)
	}
	s.changed(ctx, work)
	return later
}

// notFoundRow maps sql.ErrNoRows to catalog.ErrNotFound, meaning the work
// was merged or deleted meanwhile. It returns any other error unchanged.
func notFoundRow(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.ErrNotFound
	}
	return err
}

// merge lays local NFO data over the provider's. With neither, there is
// nothing to write, and it returns nil to keep the work as it is.
func merge(w catalog.Work, f *found, n *nfo, country string) *catalog.WorkMetadata {
	if f == nil && n == nil {
		return nil
	}
	m := catalog.WorkMetadata{Title: w.Title, Year: w.Year}
	if f != nil {
		m = f.meta
	}
	if n != nil {
		n.apply(&m, country)
	}
	return &m
}

// credited keeps only the photos of people the refresh writes credits for.
// It keeps none while the work's credits are locked. With an NFO cast, it
// keeps only the people that cast lists.
func credited(w catalog.Work, m *catalog.WorkMetadata, photos []remotePhoto) []remotePhoto {
	if m == nil || slices.Contains(w.LockedFields, catalog.FieldCredits) {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(photos), func(p remotePhoto) bool {
		return !slices.ContainsFunc(m.Credits, func(c catalog.Credit) bool { return slices.Contains(c.ExternalIDs, p.person) })
	})
}

// record writes a refresh's artwork plan in its transaction.
func record(ctx context.Context, q *dbq.Queries, plan artworkPlan) error {
	for _, r := range plan.remove {
		if err := q.DeleteImage(ctx, r.ID); err != nil {
			return err
		}
	}
	for _, p := range plan.add {
		if err := q.InsertImage(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// changed tells viewers of the work's library to refetch it.
func (s *Service) changed(ctx context.Context, work id.ID) {
	lib, err := s.q.WorkLibrary(ctx, work)
	if err != nil {
		return // the event is a hint, not a promise
	}
	s.deps.Events.Publish(events.Event{Kind: events.WorkChanged, Scope: events.Scope{Libraries: []id.ID{lib}}, Data: work})
}
