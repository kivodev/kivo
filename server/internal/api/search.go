package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

func (h *handlers) SearchWorks(ctx context.Context, req gen.SearchWorksRequestObject) (gen.SearchWorksResponseObject, error) {
	pg, err := pagingOf(nil, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	r, err := h.deps.Query.Search(ctx, callerOf(ctx).viewer, req.Params.Q, pg.limit)
	if err != nil {
		return nil, err
	}
	out := gen.SearchResults{
		Works: summaries(r.Works), People: make([]gen.PersonSummary, len(r.People)), Collections: each(r.Collections, collectionSummary),
	}
	for i, p := range r.People {
		out.People[i] = gen.PersonSummary{ID: p.ID, Name: p.Name, Photo: imageRef(p.Photo), Role: p.Role, WorkCount: p.Roots}
	}
	return gen.SearchWorks200JSONResponse(out), nil
}
