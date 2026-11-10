package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
)

func (h *handlers) ListAPIKeys(ctx context.Context, _ gen.ListAPIKeysRequestObject) (gen.ListAPIKeysResponseObject, error) {
	keys, err := h.deps.Auth.APIKeys(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ListAPIKeys200JSONResponse{Items: each(keys, apiKey)}, nil
}

func (h *handlers) CreateAPIKey(ctx context.Context, req gen.CreateAPIKeyRequestObject) (gen.CreateAPIKeyResponseObject, error) {
	issued, err := h.deps.Auth.CreateAPIKey(ctx, callerOf(ctx).Caller, auth.NewAPIKey{Name: req.Body.Name})
	if err != nil {
		return nil, err
	}
	out := apiKey(issued.Key)
	out.Token = &issued.Token
	return gen.CreateAPIKey201JSONResponse(out), nil
}

func (h *handlers) GetAPIKey(ctx context.Context, req gen.GetAPIKeyRequestObject) (gen.GetAPIKeyResponseObject, error) {
	k, err := h.deps.Auth.APIKey(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetAPIKey200JSONResponse(apiKey(k)), nil
}

// DeleteAPIKey revokes an API key. Its next request fails, and its
// live-events sockets close at once.
func (h *handlers) DeleteAPIKey(ctx context.Context, req gen.DeleteAPIKeyRequestObject) (gen.DeleteAPIKeyResponseObject, error) {
	if err := h.deps.Auth.RevokeAPIKey(ctx, req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteAPIKey204Response{}, nil
}

func apiKey(k auth.APIKey) gen.APIKey {
	out := gen.APIKey{ID: k.ID, Name: k.Name, CreatedAt: k.Created.UTC()}
	if !k.LastUsed.IsZero() {
		out.LastUsedAt = ptr(k.LastUsed.UTC())
	}
	return out
}
