package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/id"
)

func (h *handlers) CreateSession(ctx context.Context, req gen.CreateSessionRequestObject) (gen.CreateSessionResponseObject, error) {
	r := parseSignIn(ctx, req.Body)
	issued, err := h.deps.Auth.SignIn(ctx, r)
	if err != nil {
		return nil, err
	}
	return h.issued(ctx, issued), nil
}

// issued is a new session on the wire. A browser session gets the session
// cookie and the client cookie. An app session gets its token, which is
// shown only once.
func (h *handlers) issued(ctx context.Context, is auth.IssuedSession) signInResponse {
	out := signInResponse{body: session(is.Session, is.Session.ID)}
	if is.Session.Kind != auth.SessionBrowser {
		out.body.Token = &is.Token
		return out
	}
	out.cookies = append(out.cookies, h.sessionCookie(ctx, is.Token))
	if is.ClientID != "" {
		out.cookies = append(out.cookies, h.clientCookie(ctx, is.ClientID))
	}
	return out
}

// signInResponse is the 201 response to a sign-in. It writes itself because
// a browser sign-in needs two Set-Cookie headers, which a generated
// response can't send.
type signInResponse struct {
	body    gen.Session
	cookies []string
}

func (s signInResponse) write(w http.ResponseWriter) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(s.body); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	for _, c := range s.cookies {
		w.Header().Add("Set-Cookie", c)
	}
	w.WriteHeader(http.StatusCreated)
	_, err := buf.WriteTo(w)
	return err
}

func (s signInResponse) VisitCreateSessionResponse(w http.ResponseWriter) error    { return s.write(w) }
func (s signInResponse) VisitCompleteSetupResponse(w http.ResponseWriter) error    { return s.write(w) }
func (s signInResponse) VisitAcceptInviteLinkResponse(w http.ResponseWriter) error { return s.write(w) }
func (s signInResponse) VisitPollDeviceCodeResponse(w http.ResponseWriter) error   { return s.write(w) }

// GetCurrentSession returns the caller's session. A caller using an API key
// has none.
func (h *handlers) GetCurrentSession(ctx context.Context, _ gen.GetCurrentSessionRequestObject) (gen.GetCurrentSessionResponseObject, error) {
	c := callerOf(ctx)
	if c.Session.IsZero() {
		return nil, auth.ErrNotFound
	}
	s, err := h.deps.Auth.Session(ctx, c.Caller, c.Session)
	if err != nil {
		return nil, err
	}
	return gen.GetCurrentSession200JSONResponse(session(s, c.Session)), nil
}

func (h *handlers) DeleteCurrentSession(ctx context.Context, _ gen.DeleteCurrentSessionRequestObject) (gen.DeleteCurrentSessionResponseObject, error) {
	c := callerOf(ctx)
	if err := h.deps.Auth.SignOut(ctx, c.Caller); err != nil {
		return nil, err
	}
	var resp gen.DeleteCurrentSession204Response
	if c.cookie {
		resp.Headers.SetCookie = ptr(h.sessionCookie(ctx, ""))
	}
	return resp, nil
}

func (h *handlers) ListSessions(ctx context.Context, req gen.ListSessionsRequestObject) (gen.ListSessionsResponseObject, error) {
	c := callerOf(ctx)
	user := c.User
	if req.Params.UserID != nil {
		user = *req.Params.UserID
	}
	list, err := h.deps.Auth.Sessions(ctx, c.Caller, user)
	if err != nil {
		return nil, err
	}
	out := gen.SessionList{Items: make([]gen.Session, len(list))}
	for i, s := range list {
		out.Items[i] = session(s, c.Session)
	}
	return gen.ListSessions200JSONResponse(out), nil
}

// DeleteOtherSessions signs the caller's user out everywhere except the
// calling session. Each other client's next request fails, and its
// live-events sockets close at once.
func (h *handlers) DeleteOtherSessions(ctx context.Context, req gen.DeleteOtherSessionsRequestObject) (gen.DeleteOtherSessionsResponseObject, error) {
	if !req.Params.Except.Valid() {
		return nil, invalid("except is current", field("/except", "invalid"))
	}
	if err := h.deps.Auth.SignOutOthers(ctx, callerOf(ctx).Caller); err != nil {
		return nil, err
	}
	return gen.DeleteOtherSessions204Response{}, nil
}

func (h *handlers) GetSession(ctx context.Context, req gen.GetSessionRequestObject) (gen.GetSessionResponseObject, error) {
	c := callerOf(ctx)
	s, err := h.deps.Auth.Session(ctx, c.Caller, req.ID)
	if err != nil {
		return nil, err
	}
	return gen.GetSession200JSONResponse(session(s, c.Session)), nil
}

// UpdateSession renames a browser or app.
func (h *handlers) UpdateSession(ctx context.Context, req gen.UpdateSessionRequestObject) (gen.UpdateSessionResponseObject, error) {
	c := callerOf(ctx)
	s, err := h.deps.Auth.RenameSession(ctx, c.Caller, req.ID, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return gen.UpdateSession200JSONResponse(session(s, c.Session)), nil
}

// DeleteSession signs a browser or app out. Its next request fails, and
// its live-events sockets close at once.
func (h *handlers) DeleteSession(ctx context.Context, req gen.DeleteSessionRequestObject) (gen.DeleteSessionResponseObject, error) {
	if err := h.deps.Auth.RevokeSession(ctx, callerOf(ctx).Caller, req.ID); err != nil {
		return nil, err
	}
	return gen.DeleteSession204Response{}, nil
}

// session is a session on the wire. current marks the caller's own
// session.
func session(s auth.Session, current id.ID) gen.Session {
	out := gen.Session{
		ID: s.ID, Kind: gen.SessionKind(s.Kind), Name: s.Name, CreatedAt: s.Created.UTC(), LastUsedAt: s.LastUsed.UTC(),
		Current: s.ID == current,
	}
	out.Device = clientDevice(s.Device)
	return out
}
