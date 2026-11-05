package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/query"
)

func (h *handlers) GetPerson(ctx context.Context, req gen.GetPersonRequestObject) (gen.GetPersonResponseObject, error) {
	p, err := h.deps.Query.Person(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetPerson200JSONResponse(person(p)), nil
}

func (h *handlers) UpdatePerson(ctx context.Context, req gen.UpdatePersonRequestObject) (gen.UpdatePersonResponseObject, error) {
	v := callerOf(ctx).viewer
	if _, err := h.deps.Query.Person(ctx, v, req.ID); err != nil {
		return nil, err
	}
	into, err := h.deps.Catalog.RenamePerson(ctx, req.ID, req.Body.Name)
	if err != nil {
		return nil, err
	}
	p, err := h.deps.Query.Person(ctx, v, into)
	if err != nil {
		return nil, err
	}
	return gen.UpdatePerson200JSONResponse(person(p)), nil
}

// person is a person's page on the wire.
func person(p query.Person) gen.Person {
	out := gen.Person{
		ID: p.ID, Name: p.Name, Overview: opt(p.Overview), Photo: imageRef(p.Photo), Works: summaries(p.Roots),
		BookSeries: []gen.BookSeries{}, Standalone: summaries(p.Standalone), Narrated: summaries(p.Narrated),
	}
	for _, s := range p.Series {
		out.BookSeries = append(out.BookSeries, bookSeries(s))
	}
	return out
}
