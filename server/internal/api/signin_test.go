package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/mmcdole/kivo/internal/api/internal/gen"
	"github.com/mmcdole/kivo/internal/auth"
	"github.com/mmcdole/kivo/internal/config"
	"github.com/mmcdole/kivo/internal/release"
)

// A TV starts a code, and a signed-in user looks it up and approves it.
// The TV's poll then gets an app session for that user, only once. The
// session is listed with its platform and signs out like any other.
func TestDeviceCode(t *testing.T) {
	e := newEnv(t, serving{BasePath: "/kivo"})
	w := e.do("POST", "/kivo/api/v1/device-codes", "", map[string]any{"name": "Living Room"}, "Kivo-Platform", "androidTv")
	dl := decode[map[string]any](t, w)
	code, _ := dl["code"].(string)
	secret, _ := dl["secret"].(string)
	server := e.d.Settings.ServerID().String()
	if w.Code != http.StatusCreated || len(code) != 6 || secret == "" || dl["interval"] != 2.0 ||
		dl["linkUrl"] != "http://example.com/kivo/link?code="+code+"&server="+server {
		t.Fatalf("start: %d %v", w.Code, dl)
	}
	expires, _ := time.Parse(time.RFC3339, dl["expiresAt"].(string))
	if d := time.Until(expires); d < 4*time.Minute || d > 5*time.Minute {
		t.Errorf("expires in %v", d)
	}

	poll := func() *http.Response {
		return e.do("POST", "/kivo/api/v1/device-codes/poll", "", map[string]any{"secret": secret}).Result()
	}
	if r := poll(); r.StatusCode != http.StatusAccepted {
		t.Fatalf("poll while waiting: %d", r.StatusCode)
	}
	// Only a signed-in user looks a code up or answers it.
	for _, r := range []struct{ method, path string }{
		{"GET", ""}, {"POST", "/approve"}, {"POST", "/deny"},
	} {
		if w := e.do(r.method, "/kivo/api/v1/device-codes/"+code+r.path, "", nil); w.Code != http.StatusUnauthorized {
			t.Errorf("signed out %s %s: %d", r.method, r.path, w.Code)
		}
	}
	got := decode[map[string]any](t, e.do("GET", "/kivo/api/v1/device-codes/"+strings.ToLower(code), e.nobodyToken, nil))
	// httptest's client, 192.0.2.1, isn't on a local network.
	if got["name"] != "Living Room" || got["platform"] != "androidTv" || got["nearby"] != false || got["startedAt"] == nil {
		t.Errorf("look-up %v", got)
	}
	if w := e.do("GET", "/kivo/api/v1/device-codes/ZZZZZZ", e.nobodyToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("unknown code: %d", w.Code)
	}
	if w := e.do("POST", "/kivo/api/v1/device-codes/"+code+"/approve", e.nobodyToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	for _, answer := range []string{"approve", "deny"} {
		if w := e.do("POST", "/kivo/api/v1/device-codes/"+code+"/"+answer, e.adminToken, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s after approving: %d", answer, w.Code)
		}
	}

	w = e.do("POST", "/kivo/api/v1/device-codes/poll", "", map[string]any{"secret": secret})
	s := decode[map[string]any](t, w)
	token, _ := s["token"].(string)
	if w.Code != http.StatusCreated || token == "" || s["kind"] != "app" || s["name"] != "Living Room" || platformOf(s) != "androidTv" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("poll once approved: %d %v", w.Code, s)
	}
	if u := decode[map[string]any](t, e.do("GET", "/kivo/api/v1/users/me", token, nil)); u["username"] != "nobody" {
		t.Errorf("the TV is signed in as %v", u["username"])
	}
	for _, sec := range []string{secret, "wrong"} {
		w := e.do("POST", "/kivo/api/v1/device-codes/poll", "", map[string]any{"secret": sec})
		if w.Code != http.StatusGone || problemCode(w) != "expired" {
			t.Errorf("poll with a used or wrong secret: %d %s", w.Code, problemCode(w))
		}
	}
	sessions := decode[list](t, e.do("GET", "/kivo/api/v1/sessions", e.nobodyToken, nil)).Items
	i := slices.IndexFunc(sessions, func(s map[string]any) bool { return s["name"] == "Living Room" })
	if i < 0 || platformOf(sessions[i]) != "androidTv" {
		t.Fatalf("sessions %v", sessions)
	}
	if w := e.do("DELETE", "/kivo/api/v1/sessions/"+sessions[i]["id"].(string), e.nobodyToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("sign the TV out: %d", w.Code)
	}
	if w := e.do("GET", "/kivo/api/v1/users/me", token, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("the signed-out TV: %d", w.Code)
	}

	for _, body := range []map[string]any{
		{"name": ""}, {"name": "Den", "kind": "robot"},
	} {
		if w := e.do("POST", "/kivo/api/v1/device-codes", "", body); w.Code != http.StatusBadRequest {
			t.Errorf("start %v: %d", body, w.Code)
		}
	}
}

// Don't allow refuses the device. Its next poll gets 403 denied, and the
// code can't be approved afterwards.
func TestDeviceCodeDenied(t *testing.T) {
	e := newEnv(t, serving{})
	dl := decode[map[string]any](t, e.do("POST", "/api/v1/device-codes", "", map[string]any{"name": "Den"}, "Kivo-Platform", "iPad"))
	code := dl["code"].(string)
	if w := e.do("POST", "/api/v1/device-codes/"+code+"/deny", e.nobodyToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("deny: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/v1/device-codes/poll", "", map[string]any{"secret": dl["secret"]}); w.Code != http.StatusForbidden || problemCode(w) != "denied" {
		t.Errorf("poll after denial: %d %s", w.Code, problemCode(w))
	}
	if w := e.do("POST", "/api/v1/device-codes/"+code+"/approve", e.nobodyToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("approve after denial: %d", w.Code)
	}
}

// A browser links itself. Its poll sets the session and client cookies and
// carries no token. This works only from the server's own origin.
func TestDeviceCodeBrowser(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("POST", "/api/v1/device-codes", "", map[string]any{"name": "Firefox on Linux", "kind": "browser"},
		"Origin", "http://example.com")
	dl := decode[map[string]any](t, w)
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %v", w.Code, dl)
	}
	if w := e.do("POST", "/api/v1/device-codes/"+dl["code"].(string)+"/approve", e.nobodyToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	poll := map[string]any{"secret": dl["secret"]}
	if w := e.do("POST", "/api/v1/device-codes/poll", "", poll, "Origin", "https://evil.example"); w.Code != http.StatusForbidden || problemCode(w) != codeCrossOrigin {
		t.Errorf("cross-origin poll: %d %s", w.Code, problemCode(w))
	}
	w = e.do("POST", "/api/v1/device-codes/poll", "", poll, "Origin", "http://example.com")
	s := decode[map[string]any](t, w)
	if w.Code != http.StatusCreated || s["token"] != nil || s["kind"] != "browser" || platformOf(s) != "browser" {
		t.Fatalf("poll: %d %v", w.Code, s)
	}
	cookie, _ := loginCookies(t, w)
	if u := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", "", nil, "Cookie", cookie.String())); u["username"] != "nobody" {
		t.Errorf("the browser is signed in as %v", u["username"])
	}
}

// A user lists where they're signed in and signs a session out. That
// client's next request fails, and its live-events socket closes with 4401
// at the next recheck.
func TestSessions(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	tv := e.session(e.nobody)
	tvID := decode[map[string]any](t, e.do("GET", "/api/v1/sessions/current", tv, nil))["id"].(string)
	phone := e.session(e.nobody)
	got := decode[list](t, e.do("GET", "/api/v1/sessions", phone, nil))
	if len(got.Items) != 3 || slices.IndexFunc(got.Items, func(s map[string]any) bool { return s["id"] == tvID }) < 0 {
		t.Fatalf("nobody's sessions %v", got.Items)
	}
	var current int
	for _, s := range got.Items {
		if s["current"] == true {
			current++
		}
		if s["name"] != "test" || s["kind"] != "app" || s["lastUsedAt"] == nil {
			t.Errorf("session %v", s)
		}
	}
	if current != 1 {
		t.Errorf("%d sessions marked as this one: %v", current, got.Items)
	}

	// Another user's sessions are hidden from anyone but an admin.
	if w := e.do("GET", "/api/v1/sessions?userId="+e.nobody.String(), e.session(e.nobody), nil); w.Code != http.StatusOK {
		t.Errorf("own sessions by userId: %d", w.Code)
	}
	if got := decode[list](t, e.do("GET", "/api/v1/sessions?userId="+e.nobody.String(), e.adminToken, nil)); len(got.Items) != 4 {
		t.Errorf("an admin listing nobody's: %d", len(got.Items))
	}
	admins := decode[list](t, e.do("GET", "/api/v1/sessions", e.adminToken, nil)).Items
	if w := e.do("GET", "/api/v1/sessions?userId="+admins[0]["id"].(string), phone, nil); w.Code != http.StatusNotFound {
		t.Errorf("listing an unknown user: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/sessions/"+admins[0]["id"].(string), phone, nil); w.Code != http.StatusNotFound {
		t.Errorf("signing out the admin's: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/sessions/"+admins[0]["id"].(string), phone, nil); w.Code != http.StatusNotFound {
		t.Errorf("getting the admin's: %d", w.Code)
	}
	if s := decode[map[string]any](t, e.do("GET", "/api/v1/sessions/"+tvID, e.adminToken, nil)); s["id"] != tvID || s["current"] != false {
		t.Errorf("an admin getting the TV's: %v", s)
	}

	conn, ctx := e.dial(tv)
	read(t, ctx, conn) // the snapshot
	if w := e.do("DELETE", "/api/v1/sessions/"+tvID, phone, nil); w.Code != http.StatusNoContent {
		t.Fatalf("sign out the TV: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/home", tv, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("the TV's next request: %d", w.Code)
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != closeSessionEnded {
		t.Errorf("the TV's socket: %v", err)
	}
	if w := e.do("GET", "/api/v1/home", phone, nil); w.Code != http.StatusOK {
		t.Errorf("the phone was signed out too: %d", w.Code)
	}
}

// Signing out closes the session's own sockets at the next recheck. A
// device code on a client the user is already signed in on closes the
// sockets of the session it replaces.
func TestSignOutAndDeviceCodeCloseSockets(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	closes := func(what string, conn *websocket.Conn, ctx context.Context) {
		t.Helper()
		if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != closeSessionEnded {
			t.Errorf("%s: %v", what, err)
		}
	}

	tv := e.session(e.nobody)
	conn, ctx := e.dial(tv)
	read(t, ctx, conn) // the snapshot
	if w := e.do("DELETE", "/api/v1/sessions/current", tv, nil); w.Code != http.StatusNoContent {
		t.Fatalf("sign out: %d %s", w.Code, w.Body.String())
	}
	closes("the signed-out session's socket", conn, ctx)

	link := func() string {
		t.Helper()
		qc := decode[map[string]any](t, e.do("POST", "/api/v1/device-codes", "", map[string]any{"name": "Den", "clientId": "tv-1"}, "Kivo-Platform", "androidTv"))
		if w := e.do("POST", "/api/v1/device-codes/"+qc["code"].(string)+"/approve", e.nobodyToken, nil); w.Code != http.StatusNoContent {
			t.Fatalf("approve: %d %s", w.Code, w.Body.String())
		}
		return decode[map[string]any](t, e.do("POST", "/api/v1/device-codes/poll", "", map[string]any{"secret": qc["secret"]}))["token"].(string)
	}
	conn, ctx = e.dial(link())
	read(t, ctx, conn) // the snapshot
	link()
	closes("the replaced session's socket", conn, ctx)
}

// A browser signing in again sends the client cookie it was given, and an
// app sends its clientId. Either way, the user's earlier session on that
// client ends, and its socket closes at the next recheck. The list keeps
// one session for the client.
func TestOneSessionPerClient(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	creds := map[string]string{"username": "admin", "password": adminPassword}
	first, client := loginCookies(t, e.do("POST", "/api/v1/sessions", "", creds))
	second, again := loginCookies(t, e.do("POST", "/api/v1/sessions", "", creds, "Cookie", client.String()))
	if again.Value != client.Value {
		t.Errorf("the client cookie changed from %s to %s", client.Value, again.Value)
	}
	if w := e.do("GET", "/api/v1/home", "", nil, "Cookie", first.String()); w.Code != http.StatusUnauthorized {
		t.Errorf("the replaced session: %d", w.Code)
	}
	browsers := func() int {
		var n int
		for _, s := range decode[list](t, e.do("GET", "/api/v1/sessions", "", nil, "Cookie", second.String())).Items {
			if s["kind"] == "browser" {
				n++
			}
		}
		return n
	}
	if n := browsers(); n != 1 {
		t.Errorf("%d browser sessions, want 1", n)
	}
	// A browser without the cookie, such as a private window, is another client.
	e.do("POST", "/api/v1/sessions", "", creds)
	if n := browsers(); n != 2 {
		t.Errorf("%d browser sessions after another browser signed in, want 2", n)
	}

	app := map[string]any{"username": "admin", "password": adminPassword, "kind": "app", "name": "Phone", "clientId": "phone-1"}
	phone := decode[map[string]any](t, e.do("POST", "/api/v1/sessions", "", app))["token"].(string)
	conn, ctx := e.dial(phone)
	read(t, ctx, conn) // the snapshot
	w := e.do("POST", "/api/v1/sessions", "", app)
	if w.Code != http.StatusCreated || len(w.Result().Cookies()) != 0 {
		t.Fatalf("app sign-in: %d %v", w.Code, w.Result().Cookies())
	}
	if w := e.do("GET", "/api/v1/home", phone, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("the phone's replaced session: %d", w.Code)
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != closeSessionEnded {
		t.Errorf("the replaced session's socket: %v", err)
	}
	if w := e.do("GET", "/api/v1/home", e.adminToken, nil); w.Code != http.StatusOK {
		t.Errorf("the admin's other session: %d", w.Code)
	}
	app["clientId"] = strings.Repeat("x", 101)
	if w := e.do("POST", "/api/v1/sessions", "", app); w.Code != http.StatusBadRequest {
		t.Errorf("an overlong clientId: %d", w.Code)
	}
}

// The packaged TV app signs in with a password for an app session, named
// for the TV and listed with its platform. Signing in again with the kept
// clientId replaces the TV's session, so the list names the TV once.
func TestPackagedTVSession(t *testing.T) {
	e := newEnv(t, serving{})
	for platform, name := range map[string]string{"tizen": "Samsung TV (Tizen 9.0)"} {
		req := map[string]any{"username": "admin", "password": adminPassword, "kind": "app", "name": name, "clientId": platform + "-1"}
		signIn := func() string {
			t.Helper()
			w := e.do("POST", "/api/v1/sessions", "", req, "Kivo-Platform", platform)
			s := decode[map[string]any](t, w)
			token, _ := s["token"].(string)
			if w.Code != http.StatusCreated || token == "" || s["kind"] != "app" || platformOf(s) != platform || len(w.Result().Cookies()) != 0 {
				t.Fatalf("%s sign-in: %d %v %v", platform, w.Code, s, w.Result().Cookies())
			}
			return token
		}
		first := signIn()
		second := signIn()
		if w := e.do("GET", "/api/v1/home", first, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s: the replaced session: %d", platform, w.Code)
		}
		var tvs []map[string]any
		for _, s := range decode[list](t, e.do("GET", "/api/v1/sessions", second, nil)).Items {
			if platformOf(s) == platform {
				tvs = append(tvs, s)
			}
		}
		if len(tvs) != 1 || tvs[0]["name"] != name || tvs[0]["current"] != true {
			t.Errorf("%s sessions %v", platform, tvs)
		}
	}
}

// Sign out everywhere else ends every other session of the caller's user
// and closes their sockets. It keeps the caller's session, other users'
// sessions and API keys.
func TestSignOutEverywhereElse(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	key := decode[map[string]any](t, e.do("POST", "/api/v1/api-keys", e.adminToken, map[string]any{"name": "Script"}))["token"].(string)
	tv := e.session(e.admin)
	conn, ctx := e.dial(tv)
	read(t, ctx, conn) // the snapshot
	here := e.session(e.admin)

	if w := e.do("DELETE", "/api/v1/sessions", here, nil); w.Code != http.StatusBadRequest {
		t.Errorf("without except: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/sessions?except=everything", here, nil); w.Code != http.StatusBadRequest {
		t.Errorf("except=everything: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/sessions?except=current", key, nil); w.Code != http.StatusNotFound {
		t.Errorf("an API key's caller: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/sessions?except=current", here, nil); w.Code != http.StatusNoContent {
		t.Fatalf("sign out everywhere else: %d %s", w.Code, w.Body.String())
	}
	for name, tok := range map[string]string{"the TV": tv, "setup's": e.adminToken} {
		if w := e.do("GET", "/api/v1/home", tok, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s session: %d", name, w.Code)
		}
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != closeSessionEnded {
		t.Errorf("the TV's socket: %v", err)
	}
	for name, tok := range map[string]string{"this": here, "the API key's": key, "another user's": e.nobodyToken} {
		if w := e.do("GET", "/api/v1/users/me", tok, nil); w.Code != http.StatusOK {
			t.Errorf("%s session was signed out: %d", name, w.Code)
		}
	}
	if got := decode[list](t, e.do("GET", "/api/v1/sessions", here, nil)).Items; len(got) != 1 || got[0]["current"] != true {
		t.Errorf("sessions left: %v", got)
	}
}

// An admin setting a user's password signs out the user's sessions. Their
// requests are refused at once, and their sockets close at the next
// recheck.
func TestSetPasswordClosesSockets(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	conn, ctx := e.dial(e.nobodyToken)
	read(t, ctx, conn) // the snapshot
	w := e.do("POST", "/api/v1/users/"+e.nobody.String()+"/set-password", e.adminToken, map[string]any{"password": "a fresh password"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("set password: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/home", e.nobodyToken, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("the user's next request: %d", w.Code)
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != closeSessionEnded {
		t.Errorf("the user's socket: %v", err)
	}
}

// A user changes their own password by giving the current one. When asked
// to, the change signs out their other sessions and closes their sockets,
// keeping the current session.
func TestChangeCurrentUserPassword(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	tv := e.session(e.admin)
	conn, ctx := e.dial(tv)
	read(t, ctx, conn) // the snapshot
	here := e.adminToken
	change := func(current, next string, others bool) *httptest.ResponseRecorder {
		return e.do("POST", "/api/v1/users/me/change-password", here,
			map[string]any{"currentPassword": current, "password": next, "signOutOthers": others})
	}

	if w := e.do("POST", "/api/v1/users/me/change-password", "", map[string]any{"currentPassword": adminPassword, "password": "a new password", "signOutOthers": false}); w.Code != http.StatusUnauthorized || problemCode(w) != codeUnauthenticated {
		t.Errorf("signed out: %d %s", w.Code, problemCode(w))
	}
	if w := change("wrong password", "a new password", true); w.Code != http.StatusBadRequest || problemCode(w) != "wrongPassword" {
		t.Errorf("a wrong current password: %d %s", w.Code, problemCode(w))
	}
	if w := change(adminPassword, "short", true); w.Code != http.StatusBadRequest {
		t.Errorf("a short new password: %d", w.Code)
	}
	if w := change(adminPassword, "a new password", true); w.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/v1/home", tv, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("the TV's session: %d", w.Code)
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != closeSessionEnded {
		t.Errorf("the TV's socket: %v", err)
	}
	if w := e.do("GET", "/api/v1/users/me", here, nil); w.Code != http.StatusOK {
		t.Errorf("this session was signed out: %d", w.Code)
	}
	signIn := map[string]string{"username": "admin", "password": "a new password"}
	if w := e.do("POST", "/api/v1/sessions", "", signIn); w.Code != http.StatusCreated {
		t.Errorf("sign in with the new password: %d", w.Code)
	}
}

// A password change shows in the activity: an admin's own with the
// address it came from, one an admin set with who set it, and a reset
// from the command line marked as one.
func TestPasswordChangedActivity(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("POST", "/api/v1/users/me/change-password", e.adminToken,
		map[string]any{"currentPassword": adminPassword, "password": "a new password", "signOutOthers": false})
	if w.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", w.Code, w.Body.String())
	}
	w = e.do("POST", "/api/v1/users/"+e.nobody.String()+"/set-password", e.adminToken, map[string]any{"password": "a fresh password"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("set password: %d %s", w.Code, w.Body.String())
	}
	if err := e.d.Auth.SetPassword(context.Background(), auth.Caller{}, e.nobody, "a reset password"); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	for _, ev := range decode[list](t, e.do("GET", "/api/v1/activity?group=signIns", e.adminToken, nil)).Items {
		if ev["kind"] == "passwordChanged" {
			got = append(got, ev["password"].(map[string]any))
		}
	}
	want := []map[string]any{
		{"userId": e.nobody.String(), "username": "nobody", "commandLine": true},
		{"userId": e.nobody.String(), "username": "nobody", "changedBy": "admin"},
		{"userId": e.admin.String(), "username": "admin", "address": "192.0.2.1"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("passwordChanged events %v, want %v", got, want)
	}
}

// An admin creates an API key and sees its token once. A script uses the
// key the way an app uses its token, with admin access, until the key is
// revoked.
func TestAPIKeys(t *testing.T) {
	recheckSoon(t)
	e := newEnv(t, serving{})
	if w := e.do("GET", "/api/v1/api-keys", e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("a non-admin listing: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/api-keys", e.nobodyToken, map[string]any{"name": "Mine"}); w.Code != http.StatusForbidden {
		t.Errorf("a non-admin creating: %d", w.Code)
	}
	if w := e.do("POST", "/api/v1/api-keys", e.adminToken, map[string]any{"name": ""}); w.Code != http.StatusBadRequest {
		t.Errorf("no name: %d", w.Code)
	}

	w := e.do("POST", "/api/v1/api-keys", e.adminToken, map[string]any{"name": "Home Assistant"})
	k := decode[map[string]any](t, w)
	token, _ := k["token"].(string)
	if w.Code != http.StatusCreated || token == "" || k["name"] != "Home Assistant" || k["createdAt"] == nil || k["lastUsedAt"] != nil {
		t.Fatalf("create: %d %v", w.Code, k)
	}
	keys := decode[list](t, e.do("GET", "/api/v1/api-keys", e.adminToken, nil)).Items
	if len(keys) != 1 || keys[0]["id"] != k["id"] || keys[0]["token"] != nil || keys[0]["lastUsedAt"] != nil {
		t.Fatalf("list: %v", keys)
	}
	var stored []byte
	if err := e.b.DB.Reader().QueryRow(`SELECT token_hash FROM api_key`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	body, ok := strings.CutPrefix(token, "kivo_k_")
	if !ok {
		t.Errorf("the key %q doesn't start with kivo_k_", token)
	}
	if raw, _ := base64.RawURLEncoding.DecodeString(body); sha256.Sum256(raw) != [32]byte(stored) {
		t.Error("what's stored isn't the SHA-256 of the token's body")
	}

	// Used as a bearer token, the key reaches admin routes and acts as the
	// admin.
	if w := e.do("GET", "/api/v1/users", token, nil); w.Code != http.StatusOK {
		t.Errorf("an admin route with the key: %d", w.Code)
	}
	if me := decode[map[string]any](t, e.do("GET", "/api/v1/users/me", token, nil)); me["username"] != "admin" {
		t.Errorf("the key's user: %v", me)
	}
	// The key has no session to show or sign out.
	if w := e.do("GET", "/api/v1/sessions/current", token, nil); w.Code != http.StatusNotFound {
		t.Errorf("the key's session: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/sessions/current", token, nil); w.Code != http.StatusNotFound {
		t.Errorf("signing the key out: %d", w.Code)
	}
	if got := decode[map[string]any](t, e.do("GET", "/api/v1/api-keys/"+k["id"].(string), e.adminToken, nil)); got["lastUsedAt"] == nil || got["token"] != nil {
		t.Errorf("after use: %v", got)
	}
	for _, s := range decode[list](t, e.do("GET", "/api/v1/sessions", e.adminToken, nil)).Items {
		if s["id"] == k["id"] {
			t.Errorf("the key is among the admin's sessions: %v", s)
		}
	}
	if w := e.do("DELETE", "/api/v1/sessions/"+k["id"].(string), e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("revoking it as a session: %d", w.Code)
	}

	conn, ctx := e.dial(token)
	read(t, ctx, conn) // the snapshot
	if w := e.do("DELETE", "/api/v1/api-keys/"+k["id"].(string), e.nobodyToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("a non-admin revoking it: %d", w.Code)
	}
	if w := e.do("DELETE", "/api/v1/api-keys/"+k["id"].(string), e.adminToken, nil); w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/users", token, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("a revoked key: %d", w.Code)
	}
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != closeSessionEnded {
		t.Errorf("the key's socket: %v", err)
	}
	if w := e.do("GET", "/api/v1/api-keys/"+k["id"].(string), e.adminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("a revoked key's page: %d", w.Code)
	}
	if keys := decode[list](t, e.do("GET", "/api/v1/api-keys", e.adminToken, nil)).Items; len(keys) != 0 {
		t.Errorf("after revoking: %v", keys)
	}
}

func TestServerInfo(t *testing.T) {
	e := newEnv(t, serving{Version: "1.2.3"})
	if err := e.d.Settings.Update(ctx, func(s *config.Settings) error {
		s.ServerName, s.PublicURL = "Den", "https://kivo.example.com"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	anon := decode[map[string]any](t, e.do("GET", "/api/v1/server", "", nil))
	if len(anon) != 4 || anon["castProtocol"] != float64(castProtocol) || anon["id"] != e.d.Settings.ServerID().String() || anon["name"] != "Den" || anon["version"] != "1.2.3" {
		t.Errorf("signed out: %v", anon)
	}
	in := decode[map[string]any](t, e.do("GET", "/api/v1/server", e.nobodyToken, nil))
	urls, _ := in["localUrls"].([]any)
	if in["id"] != anon["id"] || in["publicUrl"] != "https://kivo.example.com" || urls == nil {
		t.Fatalf("signed in: %v", in)
	}
	for _, u := range urls {
		p, err := url.Parse(u.(string))
		if err != nil || p.Port() != "8484" || p.Scheme != "http" {
			t.Errorf("local URL %v: listening on :8484, and 127.0.0.1 isn't local to anyone else", u)
		}
	}
	// A revoked token is as good as none.
	if err := e.d.Auth.RevokeSession(ctx, auth.Caller{User: e.admin}, e.adminSession); err != nil {
		t.Fatal(err)
	}
	if got := decode[map[string]any](t, e.do("GET", "/api/v1/server", e.adminToken, nil)); len(got) != 4 {
		t.Errorf("revoked token: %v", got)
	}
}

// The newer release is in an admin's server info only.
func TestServerInfoNewRelease(t *testing.T) {
	e := newEnv(t, serving{Version: "0.4.0", Newer: release.Release{Version: "0.5.0", URL: "https://github.com/kivodev/kivo/releases/tag/v0.5.0"}})
	got := decode[gen.ServerInfo](t, e.do("GET", "/api/v1/server", e.adminToken, nil)).NewRelease
	if want := (gen.NewRelease{Version: "0.5.0", URL: "https://github.com/kivodev/kivo/releases/tag/v0.5.0"}); got == nil || *got != want {
		t.Errorf("admin: %+v, want %+v", got, want)
	}
	for who, token := range map[string]string{"signed out": "", "not an admin": e.nobodyToken} {
		if got := decode[gen.ServerInfo](t, e.do("GET", "/api/v1/server", token, nil)).NewRelease; got != nil {
			t.Errorf("%s: %+v", who, got)
		}
	}
	e = newEnv(t, serving{Version: "0.4.0"})
	if got := decode[gen.ServerInfo](t, e.do("GET", "/api/v1/server", e.adminToken, nil)).NewRelease; got != nil {
		t.Errorf("none out: %+v", got)
	}
}

// platformOf is the platform a session on the wire names in its device.
func platformOf(s map[string]any) any {
	d, _ := s["device"].(map[string]any)
	return d["platform"]
}

// A user renames its own sessions, and an admin anyone's. Another user's
// session is 404 to a non-admin, and a name must have a character.
func TestRenameSession(t *testing.T) {
	e := newEnv(t, serving{})
	path := "/api/v1/sessions/" + e.adminSession.String()
	w := e.do("PATCH", path, e.adminToken, map[string]any{"name": "  Anna's iPhone "})
	if s := decode[map[string]any](t, w); w.Code != http.StatusOK || s["name"] != "Anna's iPhone" || s["current"] != true {
		t.Fatalf("rename own: %d %v", w.Code, s)
	}
	if s := decode[map[string]any](t, e.do("GET", path, e.adminToken, nil)); s["name"] != "Anna's iPhone" {
		t.Errorf("after the rename: %v", s)
	}
	if w := e.do("PATCH", path, e.adminToken, map[string]any{"name": "   "}); w.Code != http.StatusBadRequest {
		t.Errorf("a blank name: %d", w.Code)
	}
	if w := e.do("PATCH", path, e.nobodyToken, map[string]any{"name": "Mine"}); w.Code != http.StatusNotFound {
		t.Errorf("another user's session: %d", w.Code)
	}
	others := decode[list](t, e.do("GET", "/api/v1/sessions", e.nobodyToken, nil)).Items
	if len(others) != 1 {
		t.Fatalf("nobody's sessions %v", others)
	}
	w = e.do("PATCH", "/api/v1/sessions/"+others[0]["id"].(string), e.adminToken, map[string]any{"name": "Den"})
	if s := decode[map[string]any](t, w); w.Code != http.StatusOK || s["name"] != "Den" || s["current"] != false {
		t.Errorf("an admin renaming another user's session: %d %v", w.Code, s)
	}
}
