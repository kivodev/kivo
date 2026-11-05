package identify

import (
	"context"

	"github.com/mmcdole/kivo/internal/identify/dbq"
	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// mergeEmptied handles an episode that a version left with no versions. If
// the episode has no external ID, it is merged into the episode of the same
// series that replaced it. Such an episode is a provisional one a match
// placed, or one left over from the series' previous match. Its watch state
// moves first, because deleting the episode would otherwise lose it. Any
// other emptied work stays as it is.
func (i *Identifier) mergeEmptied(ctx context.Context, tx *store.Tx, b *catalog.Batch, from, into id.ID) error {
	q := dbq.New(tx)
	f, err := q.WorkState(ctx, from)
	if err != nil {
		return err
	}
	t, err := q.WorkState(ctx, into)
	if err != nil {
		return err
	}
	if catalog.WorkKind(f.Kind) != catalog.KindEpisode || catalog.WorkKind(t.Kind) != catalog.KindEpisode || f.RootID != t.RootID {
		return nil
	}
	if xs, err := q.ExternalIDsOf(ctx, from); err != nil || len(xs) > 0 {
		return err
	}
	if err := i.deps.Userstate.MergeEpisode(ctx, tx, from, into); err != nil {
		return err
	}
	return b.MergeEpisode(ctx, from, into)
}
