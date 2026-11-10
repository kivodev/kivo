package previews

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/mmcdole/kivo/internal/atomicfile"
	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
)

// result returns the folder of a file's previews at a generation, which is
// cache/previews/<xx>/<file>/<generation>/ (see cachedir.Path). Previews
// are made in cache/previews/.tmp/ (see cachedir.MakeTemp) and moved
// into place whole, so previews that are in place are complete.
func (s *Service) result(file id.ID, gen int64) string {
	return cachedir.Path(s.dir, file, file.String(), cachedir.Leaf(gen, 0))
}

// has reports whether a file's previews are made at a generation.
func (s *Service) has(file id.ID, gen int64) bool {
	_, err := os.Stat(s.result(file, gen))
	return err == nil
}

// commit moves a result built in tmp into place if the file is still at the
// generation it was built from, and reports whether it did. Otherwise the
// result is left to be removed, and the file's current generation stays
// pending.
func (s *Service) commit(ctx context.Context, f catalog.File, tmp string) (bool, error) {
	cur, err := s.current(ctx, f.ID)
	if errors.Is(err, catalog.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if cur != f.Generation {
		slog.DebugContext(ctx, "previews: stale result dropped", "file", f.ID)
		return false, nil
	}
	return true, s.room.Err(atomicfile.RenameDir(tmp, s.result(f.ID, cur)))
}

// Health implements health.Reporter. It reports a full cache disk.
func (s *Service) Health(ctx context.Context) []health.Problem {
	return s.room.Health(ctx)
}
