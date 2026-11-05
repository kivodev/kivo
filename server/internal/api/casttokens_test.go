package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/config"
)

// castToken creates a cast token as the session of token. It fails the
// test if the token isn't made.
func (e *env) castToken(token string) map[string]any {
	e.t.Helper()
	w := e.do("POST", "/api/v1/cast-tokens", token, map[string]any{"receiverName": "Living Room TV", "receiverKind": "web"})
	if w.Code != http.StatusCreated {
		e.t.Fatalf("create cast token: %d %s", w.Code, w.Body.String())
	}
	return decode[map[string]any](e.t, w)
}

func TestCastTokenCreate(t *testing.T) {
	e := newEnv(t, serving{})
	if err := e.d.Settings.Update(ctx, func(s *config.Settings) error {
		s.PublicURL = "https://kivo.example.com"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ct := e.castToken(e.adminToken)
	if tok, _ := ct["token"].(string); len(tok) < 40 {
		t.Errorf("token: %v", ct["token"])
	}
	if ct["serverId"] != e.d.Settings.ServerID().String() {
		t.Errorf("serverId: %v", ct["serverId"])
	}
	expires, err := time.Parse(time.RFC3339, ct["expiresAt"].(string))
	if err != nil || time.Until(expires) < 23*time.Hour || time.Until(expires) > 24*time.Hour {
		t.Errorf("expiresAt: %v %v", ct["expiresAt"], err)
	}
	urls, _ := ct["urls"].([]any)
	if len(urls) == 0 || urls[len(urls)-1] != "https://kivo.example.com" {
		t.Errorf("urls: local addresses first, then the public URL: %v", urls)
	}

	for _, body := range []map[string]any{
		{"receiverName": "", "receiverKind": "web"},
		{"receiverName": strings.Repeat("x", 101), "receiverKind": "web"},
		{"receiverName": "TV", "receiverKind": "toaster"},
		{"receiverName": "TV"},
	} {
		if w := e.do("POST", "/api/v1/cast-tokens", e.adminToken, body); w.Code != http.StatusBadRequest {
			t.Errorf("%v: %d %s", body, w.Code, w.Body.String())
		}
	}

	// An API key has no session to give a token to. A cast token can't
	// make another one.
	key := decode[map[string]any](t, e.do("POST", "/api/v1/api-keys", e.adminToken, map[string]any{"name": "script"}))["token"].(string)
	if w := e.do("POST", "/api/v1/cast-tokens", key, map[string]any{"receiverName": "TV", "receiverKind": "web"}); w.Code != http.StatusNotFound {
		t.Errorf("an API key's cast token: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/cast-tokens", ct["token"].(string), map[string]any{"receiverName": "TV", "receiverKind": "web"}); w.Code != http.StatusForbidden {
		t.Errorf("a cast token's cast token: %d", w.Code)
	}
}

// TestCastTokenScope calls every route with a cast token. The cast routes
// answer as an ordinary user, even for an admin's token. Every other route
// except the public ones is refused.
func TestCastTokenScope(t *testing.T) {
	e := newEnv(t, serving{})
	tok := e.castToken(e.adminToken)["token"].(string)
	for _, rt := range e.srv.routes {
		if rt.level == public || rt.level == play || rt.level == cast || rt.upload {
			continue
		}
		w := e.do(rt.method, fill(rt.path, nil), tok, e.body(rt.op))
		if w.Code != http.StatusForbidden || problemCode(w) != codeForbidden {
			t.Errorf("%s: %d %s, want 403 forbidden", rt.op, w.Code, problemCode(w))
		}
	}

	// It plays as the session's user, named for the receiver.
	w := e.do("POST", "/api/v1/plays", tok, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	ps := decode[map[string]any](t, w)
	sid := ps["id"].(string)
	media := "/api/v1/" + ps["url"].(string)
	if w := e.do("PATCH", "/api/v1/plays/"+sid, tok, map[string]any{"positionMs": 1000, "state": "playing"}); w.Code != http.StatusNoContent {
		t.Errorf("heartbeat: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", media, "", nil); w.Code != http.StatusOK {
		t.Errorf("media: %d", w.Code)
	}
	plays := decode[map[string]any](t, e.do("GET", "/api/v1/plays", e.adminToken, nil))["items"].([]any)
	if len(plays) != 1 || plays[0].(map[string]any)["clientName"] != "Living Room TV" {
		t.Errorf("plays: %v", plays)
	}
	for _, path := range []string{"/api/v1/works/" + e.heat.Work.String(), "/api/v1/home", "/api/v1/bandwidth-test?size=10"} {
		if w := e.do("GET", path, tok, nil); w.Code != http.StatusOK && w.Code != http.StatusNoContent {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}

	// An admin's token doesn't carry the admin's power over the plays of
	// other users.
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE user SET all_libraries = 1 WHERE id = ?`, e.nobody)
		return err
	})
	friend := e.session(e.nobody)
	theirs := decode[map[string]any](t, e.do("POST", "/api/v1/plays", friend, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps}))["id"].(string)
	if w := e.do("POST", "/api/v1/plays/"+theirs+"/stop", tok, map[string]any{}); w.Code != http.StatusNotFound {
		t.Errorf("an admin's tok token stopping another user's play: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/plays/"+theirs+"/stop", e.adminToken, map[string]any{}); w.Code != http.StatusNoContent {
		t.Errorf("the admin's own session stopping it: %d", w.Code)
	}

	if w := e.do("POST", "/api/v1/plays/"+sid+"/stop", tok, map[string]any{"positionMs": 2000}); w.Code != http.StatusNoContent {
		t.Errorf("stop: %d", w.Code)
	}
}

// TestCastTokenEnds checks that a cast token ends when the receiver
// deletes it and when its session ends. The token's plays end with it.
func TestCastTokenEnds(t *testing.T) {
	e := newEnv(t, serving{})
	// Deleted by the receiver.
	cast := e.castToken(e.adminToken)["token"].(string)
	if w := e.do("DELETE", "/api/v1/cast-tokens/current", e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("a session deleting a cast token: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/cast-tokens/current", cast, nil); w.Code != http.StatusNoContent {
		t.Errorf("delete: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/home", cast, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("after delete: %d", w.Code)
	}

	// Ended with its session. The next heartbeat gets 401, and the media
	// gets 404.
	session := e.session(e.admin)
	cast = e.castToken(session)["token"].(string)
	ps := decode[map[string]any](t, e.do("POST", "/api/v1/plays", cast, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps}))
	media := "/api/v1/" + ps["url"].(string)
	if w := e.do("GET", media, "", nil); w.Code != http.StatusOK {
		t.Fatalf("media: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/sessions/current", session, nil); w.Code != http.StatusNoContent {
		t.Fatalf("sign out: %d", w.Code)
	}
	if w := e.do("PATCH", "/api/v1/plays/"+ps["id"].(string), cast, map[string]any{"positionMs": 0, "state": "playing"}); w.Code != http.StatusUnauthorized {
		t.Errorf("heartbeat after sign-out: %d", w.Code)
	}
	if w := e.do("GET", media, "", nil); w.Code != http.StatusNotFound {
		t.Errorf("media after sign-out: %d", w.Code)
	}

	// Ended with its user.
	cast = e.castToken(e.adminToken)["token"].(string)
	if _, err := e.d.Auth.Authenticate(ctx, cast); err != nil {
		t.Fatal(err)
	}
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE user SET disabled = 1 WHERE id = ?`, e.admin)
		return err
	})
	if _, err := e.d.Auth.Authenticate(ctx, cast); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("a disabled user's cast token: %v", err)
	}
}

func TestCastCORS(t *testing.T) {
	e := newEnv(t, serving{})
	cast := e.castToken(e.adminToken)["token"].(string)
	preflight := func(path, method, origin, headers string) *http.Request {
		r, _ := http.NewRequest("OPTIONS", path, nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", method)
		if headers != "" {
			r.Header.Set("Access-Control-Request-Headers", headers)
		}
		r.Header.Set("Access-Control-Request-Private-Network", "true")
		return r
	}
	send := func(r *http.Request) (int, http.Header) {
		w := httptest.NewRecorder()
		e.srv.ServeHTTP(w, r)
		return w.Code, w.Header()
	}

	// The receiver's preflights for the cast routes and server info.
	for _, c := range []struct{ path, method string }{
		{"/api/v1/plays", "POST"},
		{"/api/v1/plays/" + e.heat.Work.String(), "PATCH"},
		{"/api/v1/plays/" + e.heat.Work.String() + "/stop", "POST"},
		{"/api/v1/works/" + e.heat.Work.String(), "GET"},
		{"/api/v1/images/" + e.heat.Work.String(), "GET"},
		{"/api/v1/cast-tokens/current", "DELETE"},
		{"/api/v1/server", "GET"},
	} {
		code, h := send(preflight(c.path, c.method, castReceiverOrigin, "authorization, content-type, kivo-app, kivo-app-version, kivo-platform, kivo-os-version, kivo-device-model"))
		if code != http.StatusNoContent || h.Get("Access-Control-Allow-Origin") != castReceiverOrigin ||
			h.Get("Access-Control-Allow-Methods") != c.method || h.Get("Access-Control-Allow-Headers") != "authorization, content-type, kivo-app, kivo-app-version, kivo-platform, kivo-os-version, kivo-device-model" ||
			h.Get("Access-Control-Allow-Private-Network") != "true" || h.Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("preflight %s %s: %d %v", c.method, c.path, code, h)
		}
	}
	// Refused preflights are from another origin, for a route the receiver
	// doesn't use, or with a header it doesn't send.
	for _, r := range []*http.Request{
		preflight("/api/v1/plays", "POST", "https://evil.example", "authorization"),
		preflight("/api/v1/settings", "GET", castReceiverOrigin, "authorization"),
		preflight("/api/v1/users/me", "GET", castReceiverOrigin, "authorization"),
		preflight("/api/v1/plays", "POST", castReceiverOrigin, "authorization, x-other"),
	} {
		if code, h := send(r); code != http.StatusForbidden || h.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("preflight %s %s from %s: %d %v", r.Header.Get("Access-Control-Request-Method"), r.URL.Path, r.Header.Get("Origin"), code, h)
		}
	}
	// Media routes answer any origin's range requests.
	code, h := send(preflight("/api/v1/plays/"+e.heat.Work.String()+"/hls/main.m3u8", "GET", "https://anything.example", "range"))
	if code != http.StatusNoContent || h.Get("Access-Control-Allow-Origin") != "*" || h.Get("Access-Control-Allow-Headers") != "range" {
		t.Errorf("media preflight: %d %v", code, h)
	}

	// The answers themselves.
	w := e.do("GET", "/api/v1/home", cast, nil, "Origin", castReceiverOrigin)
	if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != castReceiverOrigin || !strings.Contains(w.Header().Get("Vary"), "Origin") {
		t.Errorf("home from the receiver: %d %v", w.Code, w.Header())
	}
	w = e.do("GET", "/api/v1/server", "", nil, "Origin", castReceiverOrigin)
	if w.Header().Get("Access-Control-Allow-Origin") != castReceiverOrigin {
		t.Errorf("server info from the receiver: %v", w.Header())
	}
	w = e.do("GET", "/api/v1/home", cast, nil, "Origin", "https://evil.example")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("home from another origin: %v", w.Header())
	}
	if code, h := send(preflight("/api/v1/plays", "POST", "https://cast.kivo.watch.evil.example", "authorization")); code != http.StatusForbidden || h.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("preflight from a lookalike origin: %d %v", code, h)
	}
	w = e.do("GET", "/api/v1/settings", e.adminToken, nil, "Origin", castReceiverOrigin)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("settings from the receiver: %v", w.Header())
	}
}

// TestCastTokenNotLogged makes, uses and deletes a cast token. It then
// checks that the token appears in none of the log, the Logs page and the
// audit.
func TestCastTokenNotLogged(t *testing.T) {
	e := newEnv(t, serving{})
	cast := e.castToken(e.adminToken)["token"].(string)
	ps := decode[map[string]any](t, e.do("POST", "/api/v1/plays", cast, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps}))
	e.do("PATCH", "/api/v1/plays/"+ps["id"].(string), cast, map[string]any{"positionMs": 0, "state": "playing"})
	e.do("GET", "/api/v1/settings", cast, nil)
	e.do("DELETE", "/api/v1/cast-tokens/current", cast, nil)
	e.do("GET", "/api/v1/home", cast, nil)

	logs := e.logs.String()
	logs += e.do("GET", "/api/v1/logs?level=debug&limit=5000", e.adminToken, nil).Body.String()
	var audit strings.Builder
	rows, err := e.b.DB.Reader().QueryContext(ctx, `SELECT action, coalesce(remote_ip, '') FROM audit`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var a, ip string
		if err := rows.Scan(&a, &ip); err != nil {
			t.Fatal(err)
		}
		audit.WriteString(a + " " + ip + "\n")
	}
	if strings.Contains(logs, cast) || strings.Contains(audit.String(), cast) {
		t.Error("the log or the audit holds the cast token")
	}
	if !strings.Contains(audit.String(), "cast_token_created") {
		t.Errorf("no audit of the cast token: %s", audit.String())
	}
}
