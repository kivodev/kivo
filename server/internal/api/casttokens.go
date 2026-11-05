package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
)

// CreateCastToken gives a Cast receiver a token to play as the caller's
// user. It includes the addresses the caller would use to reach the
// server.
func (h *handlers) CreateCastToken(ctx context.Context, req gen.CreateCastTokenRequestObject) (gen.CreateCastTokenResponseObject, error) {
	c := callerOf(ctx)
	ip := requestOf(ctx).ip
	n := auth.NewCastToken{Name: req.Body.ReceiverName, Receiver: auth.CastReceiver(req.Body.ReceiverKind)}
	issued, err := h.deps.Auth.IssueCastToken(ctx, c.Caller, ip, n)
	if err != nil {
		return nil, err
	}
	local, public := h.addresses(ip)
	urls := append([]string{}, local...)
	if public != "" {
		urls = append(urls, public)
	}
	return gen.CreateCastToken201JSONResponse{
		Token: issued.Token, ExpiresAt: issued.Expires.UTC(), ServerID: h.deps.Settings.ServerID(), Urls: urls,
	}, nil
}

// DeleteCurrentCastToken ends the caller's cast token. A receiver calls it
// as it closes.
func (h *handlers) DeleteCurrentCastToken(ctx context.Context, _ gen.DeleteCurrentCastTokenRequestObject) (gen.DeleteCurrentCastTokenResponseObject, error) {
	if err := h.deps.Auth.RevokeCastToken(ctx, callerOf(ctx).Caller); err != nil {
		return nil, err
	}
	return gen.DeleteCurrentCastToken204Response{}, nil
}
