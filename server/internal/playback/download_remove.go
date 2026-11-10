package playback

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/store"
)

// Delete deletes one of the user's downloads on the server, once
// its app has every file or no longer wants it. Its preparation stops,
// and its conversion is deleted.
func (s *Downloads) Delete(ctx context.Context, p auth.Caller, download id.ID) error {
	d, err := s.own(ctx, p.User, download)
	if err != nil {
		return err
	}
	if err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		_, err := dbq.New(tx).DeleteDownload(ctx, d.ID)
		return err
	}); err != nil {
		return err
	}
	s.mu.Lock()
	if prep := s.preparing[d.ID]; prep != nil {
		prep.removed = true
		prep.cancel()
	}
	s.mu.Unlock()
	if err := s.deps.Transcoder.DeleteDownload(d.ID.String()); err != nil {
		return err
	}
	return nil
}

// Prune deletes downloads that became ready or failed more than
// keepDownload ago, with their conversions. It also deletes any conversion
// that has no download. It's the nightly "Clean downloads" task.
func (s *Downloads) Prune(ctx context.Context) error {
	before := time.Now().Add(-keepDownload).UnixMilli()
	removed := 0
	for {
		expired, err := s.q.ExpiredDownloads(ctx, dbq.ExpiredDownloadsParams{Before: sql.NullInt64{Int64: before, Valid: true}, Limit: store.BatchSize})
		if err != nil {
			return err
		}
		if len(expired) == 0 {
			break
		}
		if err := s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
			for _, d := range expired {
				if _, err := dbq.New(tx).DeleteDownload(ctx, d); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		for _, d := range expired {
			if err := s.deps.Transcoder.DeleteDownload(d.String()); err != nil {
				return err
			}
		}
		removed += len(expired)
	}
	names, err := s.deps.Transcoder.Downloads()
	if err != nil {
		return err
	}
	var kept []id.ID
	for _, n := range names {
		if d, err := id.Parse(n); err == nil {
			kept = append(kept, d)
		}
	}
	known, err := s.q.DownloadsAmong(ctx, id.Set(kept...))
	if err != nil {
		return err
	}
	for _, n := range names {
		if d, err := id.Parse(n); err == nil && slices.Contains(known, d) {
			continue
		}
		if err := s.deps.Transcoder.DeleteDownload(n); err != nil {
			return err
		}
		removed++
	}
	if removed > 0 {
		slog.InfoContext(ctx, "playback: removed expired downloads", "count", removed)
	}
	return nil
}
