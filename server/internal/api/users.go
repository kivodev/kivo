package api

import (
	"context"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/playback"
	"github.com/mmcdole/kivo/internal/users"
)

func (h *handlers) ListUsers(ctx context.Context, _ gen.ListUsersRequestObject) (gen.ListUsersResponseObject, error) {
	us, err := h.deps.Users.Users(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ListUsers200JSONResponse{Items: each(us, user)}, nil
}

func (h *handlers) CreateUser(ctx context.Context, req gen.CreateUserRequestObject) (gen.CreateUserResponseObject, error) {
	b := req.Body
	nu := users.NewUser{Username: b.Username, Admin: deref(b.Admin), Grants: parseGrants(b.Libraries)}
	if b.Limits != nil {
		nu.Limits = parseLimits(*b.Limits)
	}
	if b.Parental != nil {
		nu.Parental = parseParental(*b.Parental)
	}
	u, err := h.deps.Auth.CreateUser(ctx, callerOf(ctx).Caller, nu, b.Password)
	if err != nil {
		return nil, err
	}
	return gen.CreateUser201JSONResponse(user(u)), nil
}

func (h *handlers) GetUser(ctx context.Context, req gen.GetUserRequestObject) (gen.GetUserResponseObject, error) {
	u, err := h.deps.Users.User(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetUser200JSONResponse(user(u)), nil
}

func (h *handlers) UpdateUser(ctx context.Context, req gen.UpdateUserRequestObject) (gen.UpdateUserResponseObject, error) {
	b := req.Body
	c := users.Change{Username: b.Username, Admin: b.Admin, Disabled: b.Disabled}
	if b.Libraries != nil {
		c.Grants = ptr(parseGrants(*b.Libraries))
	}
	if b.Limits != nil {
		c.Limits = ptr(parseLimits(*b.Limits))
	}
	if b.Parental != nil {
		c.Parental = ptr(parseParental(*b.Parental))
	}
	if err := h.deps.Users.Update(ctx, callerOf(ctx).viewer.User(), req.ID, c); err != nil {
		return nil, err
	}
	u, err := h.deps.Users.User(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateUser200JSONResponse(user(u)), nil
}

func (h *handlers) DeleteUser(ctx context.Context, req gen.DeleteUserRequestObject) (gen.DeleteUserResponseObject, error) {
	if err := h.deps.Users.Delete(ctx, callerOf(ctx).viewer.User(), req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteUser204Response{}, nil
}

// SetUserPassword sets a user's password and signs out the user's
// sessions. Their live-events sockets close at once.
func (h *handlers) SetUserPassword(ctx context.Context, req gen.SetUserPasswordRequestObject) (gen.SetUserPasswordResponseObject, error) {
	if err := h.deps.Auth.SetPassword(ctx, callerOf(ctx).Caller, req.ID, req.Body.Password); err != nil {
		return nil, err
	}
	return gen.SetUserPassword204Response{}, nil
}

// ChangeCurrentUserPassword changes the caller's own password. The
// live-events sockets of the sessions it signs out close at once.
func (h *handlers) ChangeCurrentUserPassword(ctx context.Context, req gen.ChangeCurrentUserPasswordRequestObject) (gen.ChangeCurrentUserPasswordResponseObject, error) {
	err := h.deps.Auth.ChangePassword(ctx, callerOf(ctx).Caller, auth.PasswordChange{
		Current:       req.Body.CurrentPassword,
		New:           req.Body.Password,
		SignOutOthers: req.Body.SignOutOthers,
		RemoteIP:      requestOf(ctx).ip,
	})
	if err != nil {
		return nil, err
	}
	return gen.ChangeCurrentUserPassword204Response{}, nil
}

func (h *handlers) GetCurrentUser(ctx context.Context, _ gen.GetCurrentUserRequestObject) (gen.GetCurrentUserResponseObject, error) {
	u, err := h.deps.Users.User(ctx, callerOf(ctx).viewer.User())
	if err != nil {
		return nil, err
	}
	return gen.GetCurrentUser200JSONResponse(h.currentUser(u)), nil
}

func (h *handlers) UpdateCurrentUser(ctx context.Context, req gen.UpdateCurrentUserRequestObject) (gen.UpdateCurrentUserResponseObject, error) {
	b, v := req.Body, callerOf(ctx).viewer
	var c users.PreferencesChange
	if b.Libraries != nil {
		order := make([]users.LibraryChoice, len(*b.Libraries))
		for i, l := range *b.Libraries {
			order[i] = users.LibraryChoice{Library: l.ID, Pinned: l.Pinned}
		}
		c.Libraries = &order
	}
	if p := b.Playback; p != nil {
		c.Playback = &users.PlaybackChange{
			AudioLanguage: p.AudioLanguage, SubtitleLanguage: p.SubtitleLanguage, SubtitleMode: (*string)(p.SubtitleMode),
			RememberTracks: p.RememberTracks, AutoplayNext: p.AutoplayNext, BookSpeed: p.BookSpeed,
		}
	}
	if err := h.deps.Users.EditPreferences(ctx, v, c); err != nil {
		return nil, err
	}
	u, err := h.deps.Users.User(ctx, v.User())
	if err != nil {
		return nil, err
	}
	return gen.UpdateCurrentUser200JSONResponse(h.currentUser(u)), nil
}

func user(u users.User) gen.User {
	return gen.User{
		ID: u.ID, Username: u.Username, Admin: u.Admin, Disabled: u.Disabled, OnlyAdmin: u.OnlyAdmin, CreatedAt: u.Created.UTC(),
		Libraries: libraryAccess(u.Grants), Limits: userLimits(u.Limits),
		Parental: gen.ParentalControls{MaxAge: u.Parental.Ceiling, BlockUnrated: u.Parental.BlockUnrated},
	}
}

// currentUser is the signed-in user, whose remote streaming limit is the
// one that applies to them: their own, else the server's.
func (h *handlers) currentUser(u users.User) gen.CurrentUser {
	pb := u.Playback
	limits := u.Limits
	limits.RemoteQualityCap = playback.RemoteCap(u, h.deps.Settings.Get().RemoteQualityCap)
	return gen.CurrentUser{ID: u.ID, Username: u.Username, Admin: u.Admin, Playback: gen.PlaybackPreferences{
		AudioLanguage: opt(pb.AudioLanguage), SubtitleLanguage: opt(pb.SubtitleLanguage), SubtitleMode: gen.SubtitleMode(pb.SubtitleMode),
		RememberTracks: pb.RememberTracks, AutoplayNext: pb.AutoplayNext, BookSpeed: pb.BookSpeed,
	}, Limits: userLimits(limits)}
}

func parseParental(p gen.ParentalControls) users.Parental {
	return users.Parental{Ceiling: p.MaxAge, BlockUnrated: p.BlockUnrated}
}
