package markers

import (
	"context"
	"fmt"

	"github.com/mmcdole/kivo/internal/tasks"
)

// prune is the nightly "Clean intro and credits" task. It first removes the
// fingerprints no file needs: those of files that no longer exist, and
// those of a file's older generations and versions. It then removes the
// markers found and decisions made at older generations. A file gone from
// disk keeps its fingerprints while its row is kept, in case it comes back.
func (s *Service) prune(ctx context.Context) error {
	removed, kept, err := s.results.Prune(ctx, tasks.Progress)
	if err != nil {
		return err
	}
	stale, err := s.pruneMarkers(ctx)
	if err != nil {
		return err
	}
	tasks.Summarize(ctx, fmt.Sprintf("Removed %d fingerprints and %d stale markers. %d files still have fingerprints.", removed, stale, kept))
	return nil
}
