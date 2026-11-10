package markers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/atomicfile"
	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/markers/detect"
	"github.com/mmcdole/kivo/internal/tasks"
)

// resultDir is the folder in cache/ that keeps fingerprints.
const resultDir = "markers"

// result returns the folder of a file's fingerprint at a generation:
// cache/markers/<xx>/<file>/<generation>v<version>/ (see cachedir.Path
// and cachedir.Leaf). Because the folder names the fingerprint version,
// a fingerprint of an older version counts as not made, and it is pruned
// like an older generation's. The folder is built in cache/markers/.tmp/
// (see cachedir.MakeTemp) and renamed into place whole, so a fingerprint
// that's there is complete.
func (s *Service) result(file id.ID, gen int64) string {
	return cachedir.Path(s.resultDir(), file, file.String(), cachedir.Leaf(gen, detect.FingerprintVersion))
}

// resultDir is cache/markers.
func (s *Service) resultDir() string {
	return filepath.Join(s.deps.CacheDir, resultDir)
}

// commit moves a result built in tmp into place if the file is still at the
// generation it was built from, and reports whether it did. Otherwise the
// result is left to be removed, and the file in its current state stays
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
		slog.DebugContext(ctx, "markers: stale result dropped", "file", f.ID)
		return false, nil
	}
	return true, s.room.Err(atomicfile.RenameDir(tmp, s.result(f.ID, cur)))
}

// prune is the nightly "Clean intro and credits" task. It first removes the
// fingerprints no file needs: those of files that no longer exist, and
// those of a file's older generations and versions. It then removes the
// markers found and decisions made at older generations. A file gone from
// disk keeps its fingerprints while its row is kept, in case it comes back.
func (s *Service) prune(ctx context.Context) error {
	removed, kept, err := cachedir.Prune(ctx, s.resultDir(), cachedir.PruneOptions{
		Version: detect.FingerprintVersion, Generations: s.generations, Progress: tasks.Progress,
	})
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

// generations reads the current generation of each file in files that
// still has a row.
func (s *Service) generations(ctx context.Context, files []id.ID) (map[id.ID]int64, error) {
	rows, err := s.q.FileGenerations(ctx, id.Set(files...))
	gens := make(map[id.ID]int64, len(rows))
	for _, r := range rows {
		gens[r.ID] = r.Generation
	}
	return gens, err
}
