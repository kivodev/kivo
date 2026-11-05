package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/query"
)

func (h *handlers) GetHome(ctx context.Context, _ gen.GetHomeRequestObject) (gen.GetHomeResponseObject, error) {
	rows, err := h.deps.Query.Home(ctx, callerOf(ctx).viewer)
	if err != nil {
		return nil, err
	}
	return gen.GetHome200JSONResponse(gen.Home{Rows: each(rows, homeRow)}), nil
}

// homeRow converts a Home row. The row's ID is its kind, except for a
// Recently Added row, whose ID is "recentlyAdded:<libraryId>".
func homeRow(r query.HomeRow) gen.HomeRow {
	out := gen.HomeRow{ID: string(r.Kind), Kind: gen.HomeRowKind(r.Kind), Items: summaries(r.Items)}
	if l := r.Library; !l.ID.IsZero() {
		out.ID += ":" + l.ID.String()
		out.Library = &gen.LibraryRef{ID: l.ID, Name: l.Name, Kind: gen.LibraryKind(l.Kind)}
	}
	return out
}
