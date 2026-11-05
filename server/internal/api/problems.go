package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

func (h *handlers) ListProblems(ctx context.Context, _ gen.ListProblemsRequestObject) (gen.ListProblemsResponseObject, error) {
	ps := h.deps.Problems.Open(ctx)
	items := make([]gen.ServerProblem, len(ps))
	for i, p := range ps {
		items[i] = gen.ServerProblem{
			ID: p.ID, Component: p.Component, Code: p.Code, Message: p.Message,
			Severity: gen.ProblemSeverity(p.Severity.String()), RaisedAt: p.RaisedAt.UTC(),
		}
	}
	return gen.ListProblems200JSONResponse{Items: items}, nil
}

func (h *handlers) DismissProblem(ctx context.Context, req gen.DismissProblemRequestObject) (gen.DismissProblemResponseObject, error) {
	if !h.deps.Problems.Dismiss(ctx, req.ID) {
		return nil, errNotFound
	}
	return gen.DismissProblem204Response{}, nil
}
