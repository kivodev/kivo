package previews

import (
	"context"
	"fmt"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/previews/dbq"
	"github.com/mmcdole/kivo/internal/tasks"
)

// prune is the nightly "Clean up preview thumbnails" task. It removes the
// previews no file needs: those of files that no longer exist, and those of
// a file's earlier generations. Then it forgets previews recorded as made
// that are no longer in the cache, so they're made again. A file gone from
// disk keeps its previews while its row is kept, in case it comes back.
func (s *Service) prune(ctx context.Context) error {
	removed, kept, err := cachedir.Prune(ctx, s.dir, cachedir.PruneOptions{Generations: s.generations, Progress: tasks.Progress})
	if err != nil {
		return err
	}
	made, err := s.q.Made(ctx)
	if err != nil {
		return err
	}
	var lost []dbq.MadeRow
	for _, m := range made {
		if !s.has(m.FileID, m.Generation) {
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
	tasks.Summarize(ctx, fmt.Sprintf("Removed %d cached previews. %d files still have previews.", removed, kept))
	return nil
}

// generations reads the current generation of each given file that still
// has a row.
func (s *Service) generations(ctx context.Context, files []id.ID) (map[id.ID]int64, error) {
	rows, err := s.q.FileGenerations(ctx, id.Set(files...))
	gens := make(map[id.ID]int64, len(rows))
	for _, r := range rows {
		gens[r.ID] = r.Generation
	}
	return gens, err
}
