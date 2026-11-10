package subtitles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/subtitles/dbq"
	"github.com/mmcdole/kivo/internal/tasks"
)

// prune is the nightly "Clean subtitles" task. It works in three steps:
//   - It removes extracted subtitles no file needs: those of files that no
//     longer exist, and those of a file's earlier generations.
//   - It removes the fonts no result lists.
//   - It forgets subtitles recorded as extracted that are no longer in the
//     cache, or whose fonts aren't all in the store, so they're extracted
//     again.
//
// A file missing from disk keeps its subtitles as long as its row is kept,
// in case the file comes back.
func (s *Service) prune(ctx context.Context) error {
	removed, kept, err := cachedir.Prune(ctx, s.resultDir(), cachedir.PruneOptions{Generations: s.generations, Progress: tasks.Progress})
	if err != nil {
		return err
	}
	fonts, err := s.pruneFonts()
	if err != nil {
		return err
	}
	made, err := s.q.Made(ctx)
	if err != nil {
		return err
	}
	var lost []dbq.MadeRow
	for _, m := range made {
		ok, err := s.complete(m.FileID, m.Generation)
		if err != nil {
			return err
		}
		if !ok {
			lost = append(lost, m)
		}
	}
	err = s.db.Write(ctx, func(ctx context.Context, tx *store.Tx) error {
		for _, m := range lost {
			if err := dbq.New(tx).Unmake(ctx, dbq.UnmakeParams(m)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	tasks.Summarize(ctx, fmt.Sprintf("Removed %d cached subtitles and %d fonts. %d files still have cached subtitles.", removed, fonts, kept))
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

// pruneFonts removes the fonts no subtitle result lists, and returns how
// many it removed. It holds the store's lock throughout, so no extraction
// can add and list a font in the meantime.
func (s *Service) pruneFonts() (int, error) {
	s.fonts.Lock()
	defer s.fonts.Unlock()
	lists, err := filepath.Glob(filepath.Join(s.resultDir(), "*", "*", "*", fontsList))
	if err != nil {
		return 0, err
	}
	listed := map[string]bool{}
	for _, l := range lists {
		names, err := readFontsList(l)
		if err != nil {
			return 0, err
		}
		for _, n := range names {
			listed[n] = true
		}
	}
	stored, err := filepath.Glob(filepath.Join(s.deps.CacheDir, fontStore, "*", "*"))
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, p := range stored {
		if listed[filepath.Base(p)] {
			continue
		}
		if err := os.Remove(p); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
