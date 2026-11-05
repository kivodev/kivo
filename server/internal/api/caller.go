package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/users"
)

// caller is who a request is from, by session or API key. For viewer and
// admin routes it also holds the Viewer, built once from current grants.
type caller struct {
	auth.Caller
	viewer users.Viewer
	cookie bool // authenticated by the session cookie
}

func callerOf(ctx context.Context) caller {
	c, _ := ctx.Value(keyCaller).(caller)
	return c
}

var errUnauthenticated = problem{status: http.StatusUnauthorized, code: codeUnauthenticated}

// errCastOnly answers a cast token on a route outside the cast level.
var errCastOnly = problem{status: http.StatusForbidden, code: codeForbidden, detail: "a cast token reaches only the cast routes"}

// authenticate resolves the request's token and checks that it meets lv.
// The token comes from a Bearer Authorization header, else from the
// live-events upgrade's kivo-bearer subprotocol, else from the session
// cookie, and never from the query string.
func (s *Server) authenticate(r *http.Request, lv level) (caller, error) {
	token, cookie := credentials(r)
	if token == "" {
		return caller{}, errUnauthenticated
	}
	if cookie && (unsafeMethod(r.Method) || isUpgrade(r)) && !sameOrigin(r) {
		return caller{}, problem{status: http.StatusForbidden, code: codeCrossOrigin}
	}
	a, err := s.deps.Auth.Authenticate(r.Context(), token)
	if errors.Is(err, auth.ErrUnauthenticated) {
		return caller{}, errUnauthenticated
	}
	if err != nil {
		return caller{}, err
	}
	c := caller{Caller: a, cookie: cookie}
	if !a.CastToken.IsZero() && lv != cast {
		return caller{}, errCastOnly
	}
	if err := s.deps.Auth.SeeDevice(r.Context(), a, requestOf(r.Context()).device); err != nil {
		slog.WarnContext(r.Context(), "api: record a session's device", "err", err) // the request goes on without it
	}
	if lv == signedIn {
		return c, nil
	}
	if c.viewer, err = s.viewerOf(r.Context(), a); err != nil {
		return caller{}, err
	}
	if lv == admin && !c.viewer.Admin() {
		return caller{}, problem{status: http.StatusForbidden, code: codeForbidden}
	}
	return c, nil
}

// viewerOf builds the caller's Viewer from its user's current grants. A
// cast token's Viewer never has admin power.
func (s *Server) viewerOf(ctx context.Context, a auth.Caller) (users.Viewer, error) {
	v, err := s.deps.Users.Viewer(ctx, a.User)
	if errors.Is(err, users.ErrDisabled) || errors.Is(err, users.ErrNotFound) {
		return users.Viewer{}, errUnauthenticated
	}
	if err != nil {
		return users.Viewer{}, err
	}
	if !a.CastToken.IsZero() {
		v = v.WithoutAdmin()
	}
	return v, nil
}

func credentials(r *http.Request) (token string, cookie bool) {
	scheme, t, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(t), false
	}
	if t, ok := socketBearer(r); ok {
		return t, false
	}
	if c, err := r.Cookie(cookieName(sessionCookieName, requestOf(r.Context()).secure)); err == nil {
		return c.Value, true
	}
	return "", false
}

// bearerProtocol prefixes the token a client offers as a subprotocol on
// the live-events upgrade, beside eventsProtocol.
const bearerProtocol = "kivo-bearer."

// socketBearer returns the token offered as a kivo-bearer.<token>
// subprotocol on the live-events upgrade. A page's WebSocket can't set a
// header, so the packaged TV app, which has no cookie, sends its token
// this way, as Kubernetes takes bearer tokens on its WebSockets. It keeps
// the token out of URLs and access logs. No other route reads it.
func socketBearer(r *http.Request) (string, bool) {
	if requestOf(r.Context()).route != eventsRoute || !isUpgrade(r) {
		return "", false
	}
	for _, v := range r.Header.Values("Sec-WebSocket-Protocol") {
		for p := range strings.SplitSeq(v, ",") {
			if t, ok := strings.CutPrefix(strings.TrimSpace(p), bearerProtocol); ok {
				return t, true
			}
		}
	}
	return "", false
}

// The session cookie of a browser session is always HttpOnly and
// SameSite=Strict. Over TLS, or behind a trusted HTTPS proxy, it is also
// Secure and has the __Host- prefix, which requires Path=/. Over plain
// HTTP on the LAN it is an ordinary cookie scoped to the base path. The
// client cookie holds the browser's client ID the same way, except that it
// is SameSite=Lax and stays after the browser signs out.
const (
	sessionCookieName = "kivo_session"
	clientCookieName  = "kivo_client"
	cookieMaxAge      = 400 * 24 * time.Hour // the most browsers keep; auth expires idle browser sessions after 30 days
)

func cookieName(name string, secure bool) string {
	if secure {
		return "__Host-" + name
	}
	return name
}

func (s *Server) sessionCookie(ctx context.Context, token string) string {
	c := s.cookie(ctx, sessionCookieName, token)
	c.SameSite = http.SameSiteStrictMode
	if token == "" {
		c.MaxAge = -1
	}
	return c.String()
}

func (s *Server) clientCookie(ctx context.Context, clientID string) string {
	c := s.cookie(ctx, clientCookieName, clientID)
	c.SameSite = http.SameSiteLaxMode
	return c.String()
}

func (s *Server) cookie(ctx context.Context, name, value string) *http.Cookie {
	secure := requestOf(ctx).secure
	c := &http.Cookie{
		Name: cookieName(name, secure), Value: value, Path: s.deps.BasePath + "/",
		MaxAge: int(cookieMaxAge.Seconds()), HttpOnly: true, Secure: secure,
	}
	if secure {
		c.Path = "/"
	}
	return c
}

// browserClientID returns the client ID in the request's client cookie. It
// makes a new ID when the request sent none, or sent one the server didn't
// make.
func browserClientID(ctx context.Context) string {
	if id := requestOf(ctx).clientCookie; len(id) == clientIDLen {
		return id
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// clientIDLen is the length of a client ID the server makes, which is 16
// random bytes in unpadded base64url.
const clientIDLen = 22
