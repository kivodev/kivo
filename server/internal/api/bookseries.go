package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

func (h *handlers) GetBookSeries(ctx context.Context, req gen.GetBookSeriesRequestObject) (gen.GetBookSeriesResponseObject, error) {
	s, err := h.deps.Query.Series(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetBookSeries200JSONResponse(bookSeries(s)), nil
}

func (h *handlers) UpdateBookSeries(ctx context.Context, req gen.UpdateBookSeriesRequestObject) (gen.UpdateBookSeriesResponseObject, error) {
	v := callerOf(ctx).viewer
	if _, err := h.deps.Query.Series(ctx, v, req.ID); err != nil {
		return nil, err
	}
	into, err := h.deps.Catalog.RenameSeries(ctx, req.ID, req.Body.Name)
	if err != nil {
		return nil, err
	}
	s, err := h.deps.Query.Series(ctx, v, into)
	if err != nil {
		return nil, err
	}
	return gen.UpdateBookSeries200JSONResponse(bookSeries(s)), nil
}
