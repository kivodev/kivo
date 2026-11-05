package query

import (
	"context"
	"database/sql"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/users"
)

// Favorites are the works a user marked a favorite, most recently marked
// first. They fill Home's Favorites row and the Favorites page, which lists
// one kind at a time.

// favoritesPage reads the favorites of one kind from @offset. A NULL @kind
// reads every kind.
var favoritesPage = stmt("Favorites", `
SELECT `+summaryColumns+`
FROM user_state us
JOIN work w ON w.id = us.work_id
WHERE us.user_id = @user AND us.favorite
  AND (@kind IS NULL OR w.kind = @kind)
  AND `+visible+`
ORDER BY us.favorited_at DESC, w.sort_title, w.id
LIMIT @limit OFFSET @offset`)

var favoritesCount = stmt("FavoritesCount", `
SELECT count(*)
FROM user_state us
JOIN work w ON w.id = us.work_id
WHERE us.user_id = @user AND us.favorite
  AND (@kind IS NULL OR w.kind = @kind)
  AND `+visible)

// Favorites returns a page of the works the Viewer marked a favorite and
// still sees, most recently marked first. It lists one kind, or every kind
// when kind is empty.
func (s *Service) Favorites(ctx context.Context, v users.Viewer, kind catalog.WorkKind, o GridOptions) (GridPage[WorkSummary], error) {
	limit, err := o.limit()
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	vw := viewerOf(v)
	var k any
	if kind != "" {
		k = string(kind)
	}
	var items []WorkSummary
	var total int
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		if err := rowOf(ctx, tx, favoritesCount, vw.args(sql.Named("kind", k)), &total); err != nil {
			return err
		}
		var err error
		items, err = summaries(ctx, tx, favoritesPage, vw.args(sql.Named("kind", k),
			sql.Named("offset", o.Offset), sql.Named("limit", limit)))
		return err
	})
	if err != nil {
		return GridPage[WorkSummary]{}, err
	}
	return s.summaryPage(ctx, vw, items, total, nil)
}
