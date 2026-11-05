package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
)

func (h *handlers) GetSetup(ctx context.Context, _ gen.GetSetupRequestObject) (gen.GetSetupResponseObject, error) {
	need, err := h.deps.Auth.NeedsSetup(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetSetup200JSONResponse{Required: need, ServerName: h.deps.Settings.Get().ServerName}, nil
}

func (h *handlers) CompleteSetup(ctx context.Context, req gen.CompleteSetupRequestObject) (gen.CompleteSetupResponseObject, error) {
	r := parseSignIn(ctx, req.Body)
	issued, err := h.deps.Auth.Setup(ctx, r)
	if err != nil {
		return nil, err
	}
	return h.issued(ctx, issued), nil
}
