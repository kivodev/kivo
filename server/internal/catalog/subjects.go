package catalog

import (
	"context"
	"fmt"

	"github.com/mmcdole/kivo/internal/id"
)

// Subjects returns a display name for each ID that tasks work on:
//   - a file is named by the item it plays, or by its path until it's
//     matched;
//   - a work is named by its title, and an episode as "Series · S1 · E2";
//   - a library folder is named by its path;
//   - a library is named by its name.
//
// IDs that name none of these are left out.
func (c *Catalog) Subjects(ctx context.Context, ids []id.ID) (map[id.ID]string, error) {
	rows, err := c.q.Subjects(ctx, id.Set(ids...))
	if err != nil {
		return nil, err
	}
	out := make(map[id.ID]string, len(rows))
	for _, r := range rows {
		switch {
		case WorkKind(r.Kind) == KindEpisode && r.Series != "":
			out[r.ID] = fmt.Sprintf("%s · S%d · E%d", r.Series, r.ParentNumber.Int64, r.Number.Int64)
		case WorkKind(r.Kind) == KindSeason && r.Series != "":
			out[r.ID] = r.Series + " · " + r.Title
		case r.Title != "":
			out[r.ID] = r.Title
		default:
			out[r.ID] = r.Path
		}
	}
	return out, nil
}
