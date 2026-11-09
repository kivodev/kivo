package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/query"
)

// listOf converts a list.
func listOf(l query.List) gen.WorkList {
	out := gen.WorkList{ID: l.ID, Name: l.Name, Kind: gen.ListKindManual, BuiltIn: l.BuiltIn, WorkCount: l.Works}
	if l.Smart != nil {
		out.Kind = gen.ListKindSmart
		out.Smart = &gen.WorkListSmart{Query: wireQuery(l.Smart.Query)}
		if len(l.Smart.Errors) > 0 {
			errs := problemFields("/smart/query", l.Smart.Errors)
			out.Smart.Errors = &errs
		}
	}
	return out
}

// savedQuery checks a request's rules and returns them as stored JSON.
func savedQuery(smart *gen.SmartQuery) (*string, error) {
	if smart == nil {
		return nil, nil
	}
	q := workQuery(smart.Query)
	if problems := q.Validate(); len(problems) > 0 {
		return nil, invalid("invalid query", problemFields("/smart/query", problems)...)
	}
	saved, err := q.Saved()
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (h *handlers) ListLists(ctx context.Context, _ gen.ListListsRequestObject) (gen.ListListsResponseObject, error) {
	ls, err := h.deps.Query.Lists(ctx, callerOf(ctx).viewer)
	if err != nil {
		return nil, err
	}
	return gen.ListLists200JSONResponse{Items: each(ls, listOf)}, nil
}

func (h *handlers) CreateList(ctx context.Context, req gen.CreateListRequestObject) (gen.CreateListResponseObject, error) {
	v := callerOf(ctx).viewer
	saved, err := savedQuery(req.Body.Smart)
	if err != nil {
		return nil, err
	}
	listID, err := h.deps.Users.CreateList(ctx, v, req.Body.Name, saved)
	if err != nil {
		return nil, err
	}
	l, err := h.deps.Query.List(ctx, v, listID)
	if err != nil {
		return nil, err
	}
	return gen.CreateList201JSONResponse(listOf(l)), nil
}

func (h *handlers) GetList(ctx context.Context, req gen.GetListRequestObject) (gen.GetListResponseObject, error) {
	l, err := h.deps.Query.List(ctx, callerOf(ctx).viewer, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetList200JSONResponse(listOf(l)), nil
}

func (h *handlers) UpdateList(ctx context.Context, req gen.UpdateListRequestObject) (gen.UpdateListResponseObject, error) {
	v := callerOf(ctx).viewer
	saved, err := savedQuery(req.Body.Smart)
	if err != nil {
		return nil, err
	}
	if req.Body.Name != nil || saved != nil {
		if err := h.deps.Users.UpdateList(ctx, v, req.ID, req.Body.Name, saved); err != nil {
			return nil, err
		}
	}
	l, err := h.deps.Query.List(ctx, v, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateList200JSONResponse(listOf(l)), nil
}

func (h *handlers) DeleteList(ctx context.Context, req gen.DeleteListRequestObject) (gen.DeleteListResponseObject, error) {
	if err := h.deps.Users.DeleteList(ctx, callerOf(ctx).viewer, req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteList204Response{}, nil
}

func (h *handlers) ListListWorks(ctx context.Context, req gen.ListListWorksRequestObject) (gen.ListListWorksResponseObject, error) {
	var kinds []catalog.WorkKind
	for _, k := range deref(req.Params.Kind) {
		if !k.Valid() {
			return nil, invalid("unknown kind", field("kind", "invalid"))
		}
		kinds = append(kinds, catalog.WorkKind(k))
	}
	o, err := gridOptionsOf(req.Params.Offset, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	page, err := h.deps.Query.ListWorks(ctx, callerOf(ctx).viewer, req.ID, kinds, o)
	if err != nil {
		return nil, err
	}
	return gen.ListListWorks200JSONResponse(workPage(page)), nil
}

func (h *handlers) AddListWork(ctx context.Context, req gen.AddListWorkRequestObject) (gen.AddListWorkResponseObject, error) {
	if err := h.deps.Userstate.AddToList(ctx, callerOf(ctx).viewer, req.ID, req.Body.WorkID); err != nil {
		return nil, err
	}
	return gen.AddListWork204Response{}, nil
}

func (h *handlers) RemoveListWork(ctx context.Context, req gen.RemoveListWorkRequestObject) (gen.RemoveListWorkResponseObject, error) {
	if err := h.deps.Userstate.RemoveFromList(ctx, callerOf(ctx).viewer, req.ID, req.WorkID); err != nil {
		return nil, err
	}
	return gen.RemoveListWork204Response{}, nil
}
