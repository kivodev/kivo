package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
)

func (h *handlers) ListInvites(ctx context.Context, _ gen.ListInvitesRequestObject) (gen.ListInvitesResponseObject, error) {
	is, err := h.deps.Auth.Invites(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ListInvites200JSONResponse{Items: each(is, invite)}, nil
}

func (h *handlers) CreateInvite(ctx context.Context, req gen.CreateInviteRequestObject) (gen.CreateInviteResponseObject, error) {
	b := req.Body
	n := auth.NewInvite{Grants: parseGrants(b.Libraries), Expires: b.ExpiresAt}
	if b.Limits != nil {
		n.Limits = parseLimits(*b.Limits)
	}
	c := callerOf(ctx)
	issued, err := h.deps.Auth.CreateInvite(ctx, c.Caller, n)
	if err != nil {
		return nil, err
	}
	u, err := h.deps.Users.User(ctx, c.User)
	if err != nil {
		return nil, err
	}
	issued.Invite.CreatedBy = u.Username
	out := invite(issued.Invite)
	out.Token = &issued.Token
	return gen.CreateInvite201JSONResponse(out), nil
}

func (h *handlers) DeleteInvite(ctx context.Context, req gen.DeleteInviteRequestObject) (gen.DeleteInviteResponseObject, error) {
	if err := h.deps.Auth.RevokeInvite(ctx, req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteInvite204Response{}, nil
}

func (h *handlers) GetInviteLink(ctx context.Context, req gen.GetInviteLinkRequestObject) (gen.GetInviteLinkResponseObject, error) {
	inv, err := h.deps.Auth.InviteByToken(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	return gen.GetInviteLink200JSONResponse{
		InvitedBy: inv.CreatedBy, ServerName: h.deps.Settings.Get().ServerName, ExpiresAt: inv.Expires.UTC(),
	}, nil
}

func (h *handlers) AcceptInviteLink(ctx context.Context, req gen.AcceptInviteLinkRequestObject) (gen.AcceptInviteLinkResponseObject, error) {
	r := parseSignIn(ctx, req.Body)
	issued, err := h.deps.Auth.AcceptInvite(ctx, req.Token, r)
	if err != nil {
		return nil, err
	}
	return h.issued(ctx, issued), nil
}

func invite(i auth.Invite) gen.Invite {
	return gen.Invite{
		ID: i.ID, Libraries: libraryAccess(i.Grants), Limits: userLimits(i.Limits),
		CreatedAt: i.Created.UTC(), ExpiresAt: i.Expires.UTC(),
	}
}
