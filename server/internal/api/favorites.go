package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/catalog"
)

func (h *handlers) ListFavorites(ctx context.Context, req gen.ListFavoritesRequestObject) (gen.ListFavoritesResponseObject, error) {
	p := req.Params
	var kind catalog.WorkKind
	if p.Kind != nil {
		if !p.Kind.Valid() {
			return nil, invalid("unknown kind", field("kind", "invalid"))
		}
		kind = catalog.WorkKind(*p.Kind)
	}
	o, err := gridOptionsOf(p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	page, err := h.deps.Query.Favorites(ctx, callerOf(ctx).viewer, kind, o)
	if err != nil {
		return nil, err
	}
	return gen.ListFavorites200JSONResponse(workPage(page)), nil
}
