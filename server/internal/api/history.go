package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/query"
)

func (h *handlers) ListCurrentUserPlayEvents(ctx context.Context, req gen.ListCurrentUserPlayEventsRequestObject) (gen.ListCurrentUserPlayEventsResponseObject, error) {
	p, err := h.deps.Query.History(ctx, callerOf(ctx).viewer, query.Cursor(deref(req.Params.After)), deref(req.Params.Limit))
	if err != nil {
		return nil, err
	}
	return gen.ListCurrentUserPlayEvents200JSONResponse(playEventFeed(p)), nil
}

func (h *handlers) ListPlayEvents(ctx context.Context, req gen.ListPlayEventsRequestObject) (gen.ListPlayEventsResponseObject, error) {
	v, after, limit := callerOf(ctx).viewer, query.Cursor(deref(req.Params.After)), deref(req.Params.Limit)
	var p query.Page[query.PlayEvent]
	var err error
	if u := req.Params.UserID; u != nil {
		p, err = h.deps.Query.UserHistory(ctx, v, *u, after, limit)
	} else {
		p, err = h.deps.Query.ServerHistory(ctx, v, after, limit)
	}
	if err != nil {
		return nil, err
	}
	return gen.ListPlayEvents200JSONResponse(playEventFeed(p)), nil
}

func playEventFeed(p query.Page[query.PlayEvent]) gen.PlayEventFeed {
	out := gen.PlayEventFeed{Items: []gen.PlayEvent{}, Next: opt(string(p.Next))}
	for _, e := range p.Items {
		out.Items = append(out.Items, gen.PlayEvent{
			ID: e.ID, Work: summary(e.Work), UserID: e.User, Username: e.Username,
			StartedAt: e.Started, EndedAt: e.Ended, PositionMs: ms(e.Position), Completed: e.Completed,
		})
	}
	return out
}
