package subtitles

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/atomicfile"
	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/health"
	"github.com/mmcdole/kivo/internal/id"
)

// resultDir is the folder in cache/ that keeps extracted subtitles.
const resultDir = "subtitles"

// result is the folder of a file's extracted subtitles at a generation:
// cache/subtitles/<xx>/<file>/<generation>/ (cachedir.Path). It's built
// in cache/subtitles/.tmp/ (cachedir.MakeTemp) and moved into place
// whole, so a result that exists is complete.
func (s *Service) result(file id.ID, gen int64) string {
	return cachedir.Path(s.resultDir(), file, file.String(), cachedir.Leaf(gen, 0))
}

// resultDir is cache/subtitles.
func (s *Service) resultDir() string {
	return filepath.Join(s.deps.CacheDir, resultDir)
}

// has reports whether a file's subtitles are extracted at a generation.
func (s *Service) has(file id.ID, gen int64) bool {
	_, err := os.Stat(s.result(file, gen))
	return err == nil
}

// complete reports whether a file's subtitles are extracted at a
// generation, and every font the result lists is in the store.
func (s *Service) complete(file id.ID, gen int64) (bool, error) {
	if !s.has(file, gen) {
		return false, nil
	}
	names, err := readFontsList(filepath.Join(s.result(file, gen), fontsList))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return true, nil // extracted, and no fonts are needed
	case err != nil:
		return false, err
	}
	return s.allFontsStored(names), nil
}

// commit moves a result built in tmp into place if the file is still at the
// generation it was built from, and reports whether it did. Otherwise the
// result is left for removal, and the file's current generation stays
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
		slog.DebugContext(ctx, "subtitles: stale result dropped", "file", f.ID)
		return false, nil
	}
	return true, s.room.Err(atomicfile.RenameDir(tmp, s.result(f.ID, cur)))
}

// Health implements health.Reporter. It reports a full cache disk, and
// OpenSubtitles being down or refusing the key or the account.
func (s *Service) Health(ctx context.Context) []health.Problem {
	return append(s.room.Health(ctx), s.openSubtitles.health(s.deps.Settings.Get())...)
}
