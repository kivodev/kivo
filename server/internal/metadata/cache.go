package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/mmcdole/kivo/internal/metadata/dbq"
	"github.com/mmcdole/kivo/internal/store"
	"github.com/mmcdole/kivo/internal/tasks"
)

// How long provider data stays fresh. Expired entries are kept for
// keepStale after expiry, to serve while a provider is down.
const (
	day          = 24 * time.Hour
	ttlSearch    = day
	ttlRecent    = 7 * day  // movies released within recentFor, or not released yet
	ttlOlder     = 30 * day // older movies, and collections
	ttlContinues = day      // a series still airing, and its seasons
	ttlEnded     = 30 * day
	ttlAuthor    = 30 * day // an author's biography and photo
	ttlNotFound  = day
	recentFor    = 365 * day
	keepStale    = 30 * day
)

// movieTTL returns the cache lifetime of a movie released on date. date is
// YYYY-MM-DD, or empty when unknown, which counts as recent.
func movieTTL(date string, now time.Time) time.Duration {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil || now.Sub(t) < recentFor {
		return ttlRecent
	}
	return ttlOlder
}

// seriesTTL returns the cache lifetime of a series and its seasons.
func seriesTTL(ended bool) time.Duration {
	if ended {
		return ttlEnded
	}
	return ttlContinues
}

func fixed(d time.Duration) func([]byte) time.Duration {
	return func([]byte) time.Duration { return d }
}

// prune is the daily pruning of metadata's caches. It removes long-expired
// provider responses, then image files that nothing refers to.
func (s *Service) prune(ctx context.Context) error {
	n, err := s.pruneProviderCache(ctx)
	if err != nil {
		return err
	}
	files, err := s.pruneImages(ctx)
	if err != nil {
		return err
	}
	tasks.Summarize(ctx, fmt.Sprintf("Removed %d expired metadata responses and %d images", n, files))
	return nil
}

// pruneProviderCache deletes entries expired too long ago to stand in for a
// provider. It deletes a batch at a time and returns how many it deleted.
func (s *Service) pruneProviderCache(ctx context.Context) (int64, error) {
	var total int64
	before := time.Now().Add(-keepStale).UnixMilli()
	err := s.db.Drain(ctx, func(ctx context.Context, tx *store.Tx) (int, error) {
		n, err := dbq.New(tx).PruneCached(ctx, dbq.PruneCachedParams{Before: before, Limit: store.BatchSize})
		if err == nil {
			tx.AfterCommit(func() { total += n })
		}
		return int(n), err
	})
	if err != nil {
		return total, err
	}
	return total, nil
}
