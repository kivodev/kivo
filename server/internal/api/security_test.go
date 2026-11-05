package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return v
}

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s changed (go test -update to accept):\n%s", path, got)
	}
}

// TestSecurityHeaders keeps the headers that every response carries, for
// both an API response and the web client, in a golden file.
func TestSecurityHeaders(t *testing.T) {
	e := newEnv(t, serving{})
	var b strings.Builder
	for _, path := range []string{"/api/v1/setup", "/", "/api/v1/no-such-route"} {
		w := e.do("GET", path, "", nil)
		fmt.Fprintf(&b, "GET %s: %d\n", path, w.Code)
		for _, h := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "Content-Type", "Cache-Control"} {
			fmt.Fprintf(&b, "  %s: %s\n", h, w.Header().Get(h))
		}
		if w.Header().Get("X-Request-Id") == "" {
			t.Errorf("%s: no request ID", path)
		}
	}
	golden(t, "headers.golden", b.String())
}

// webLogin signs in with a web session and returns its session cookie.
func (e *env) webLogin(headers ...string) *http.Cookie {
	e.t.Helper()
	w := e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword}, headers...)
	if w.Code != http.StatusCreated {
		e.t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	session, _ := loginCookies(e.t, w)
	return session
}

// loginCookies are a browser sign-in's session cookie and client cookie.
func loginCookies(t *testing.T, w *httptest.ResponseRecorder) (session, client *http.Cookie) {
	t.Helper()
	cs := w.Result().Cookies()
	if len(cs) != 2 || !strings.HasSuffix(cs[0].Name, "kivo_session") || !strings.HasSuffix(cs[1].Name, "kivo_client") {
		t.Fatalf("login set %v", cs)
	}
	return cs[0], cs[1]
}

func TestCookieSessionNeedsSameOrigin(t *testing.T) {
	e := newEnv(t, serving{})
	c := e.webLogin("Origin", "http://example.com")
	scan := "/api/v1/libraries/" + e.lib.String() + "/scan"
	for _, tc := range []struct {
		name    string
		headers []string
		want    int
	}{
		{"same origin", []string{"Origin", "http://example.com"}, http.StatusAccepted},
		{"cross origin", []string{"Origin", "https://evil.example"}, http.StatusForbidden},
		{"no Origin", nil, http.StatusForbidden},
		{"another port", []string{"Origin", "http://example.com:8080"}, http.StatusForbidden},
	} {
		w := e.do("POST", scan, "", map[string]any{}, append(tc.headers, "Cookie", c.String())...)
		if w.Code != tc.want {
			t.Errorf("%s: %d %s, want %d", tc.name, w.Code, problemCode(w), tc.want)
		}
		if tc.want == http.StatusForbidden && problemCode(w) != codeCrossOrigin {
			t.Errorf("%s: code %q", tc.name, problemCode(w))
		}
	}
	// A GET with a cookie needs no Origin. A token isn't subject to the
	// rule.
	if w := e.do("GET", "/api/v1/home", "", nil, "Cookie", c.String()); w.Code != http.StatusOK {
		t.Errorf("cookie GET: %d", w.Code)
	}
	if w := e.do("POST", scan, e.adminToken, map[string]any{}, "Origin", "https://evil.example"); w.Code != http.StatusAccepted {
		t.Errorf("token from another origin: %d", w.Code)
	}
	// A WebSocket upgrade is held to the rule too.
	w := e.do("GET", "/api/v1/events", "", nil, "Cookie", c.String(), "Origin", "https://evil.example",
		"Connection", "Upgrade", "Upgrade", "websocket", "Sec-WebSocket-Version", "13", "Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if w.Code != http.StatusForbidden || problemCode(w) != codeCrossOrigin {
		t.Errorf("cross-origin upgrade: %d %s", w.Code, problemCode(w))
	}
	// And signing in from another origin is refused outright.
	w = e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword}, "Origin", "https://evil.example")
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-origin login: %d", w.Code)
	}
}

func TestOtherAuthorizationSchemeUsesCookie(t *testing.T) {
	e := newEnv(t, serving{})
	c := e.webLogin("Origin", "http://example.com")
	w := e.do("GET", "/api/v1/users/me", "", nil, "Cookie", c.String(), "Authorization", "Basic cHJveHk6cHJveHk=")
	if w.Code != http.StatusOK {
		t.Errorf("Basic header and a session cookie: %d %s", w.Code, problemCode(w))
	}
}

func TestAnswersAreNotStored(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("POST", "/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword})
	if w.Code != http.StatusCreated || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("sign-in: %d, Cache-Control %q", w.Code, w.Header().Get("Cache-Control"))
	}
	w = e.do("GET", "/api/v1/users/me", e.adminToken, nil)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("users/me: Cache-Control %q", w.Header().Get("Cache-Control"))
	}
}

func TestForwardedForOnlyFromTrustedProxy(t *testing.T) {
	e := newEnv(t, serving{Options: Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}})
	for _, tc := range []struct {
		remote, xff, proto string
		ip                 string
		secure             bool
	}{
		{"203.0.113.7:1234", "198.51.100.1", "https", "203.0.113.7", false}, // not a proxy: ignored
		{"10.0.0.2:1234", "198.51.100.1", "https", "198.51.100.1", true},
		{"10.0.0.2:1234", "198.51.100.1, 10.0.0.3", "", "198.51.100.1", false}, // proxies chained
		{"10.0.0.2:1234", "6.6.6.6, 198.51.100.1", "http", "198.51.100.1", false},
		{"10.0.0.2:1234", "", "", "10.0.0.2", false},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		if tc.xff != "" {
			r.Header.Set("X-Forwarded-For", tc.xff)
		}
		if tc.proto != "" {
			r.Header.Set("X-Forwarded-Proto", tc.proto)
		}
		ip, secure, _ := e.srv.client(r)
		if ip != netip.MustParseAddr(tc.ip) || secure != tc.secure {
			t.Errorf("%+v: got %v secure=%v", tc, ip, secure)
		}
	}
}

func TestCookieFlags(t *testing.T) {
	e := newEnv(t, serving{Options: Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.1/32")}}})
	login := func(mod func(*http.Request)) *http.Cookie {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(`{"username":"admin","password":"`+adminPassword+`"}`))
		r.Header.Set("Content-Type", "application/json")
		mod(r)
		w := httptest.NewRecorder()
		e.srv.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("login: %d %s", w.Code, w.Body.String())
		}
		c, client := loginCookies(t, w)
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
			t.Errorf("cookie %s isn't HttpOnly and SameSite=Strict", c)
		}
		// The client cookie matches the session cookie, except that it is
		// SameSite=Lax.
		if !client.HttpOnly || client.SameSite != http.SameSiteLaxMode || client.Secure != c.Secure || client.Path != c.Path ||
			client.MaxAge != c.MaxAge || client.Name != strings.Replace(c.Name, "kivo_session", "kivo_client", 1) {
			t.Errorf("client cookie %s beside %s", client, c)
		}
		// The cookie it set signs the next request in.
		r2 := httptest.NewRequest("GET", "/api/v1/sessions/current", nil)
		mod(r2)
		r2.AddCookie(c)
		w2 := httptest.NewRecorder()
		e.srv.ServeHTTP(w2, r2)
		if w2.Code != http.StatusOK {
			t.Errorf("signed-in request with %s: %d", c.Name, w2.Code)
		}
		return c
	}
	plain := login(func(*http.Request) {})
	if plain.Name != "kivo_session" || plain.Secure || plain.Path != "/" {
		t.Errorf("plain HTTP: %s", plain)
	}
	tlsC := login(func(r *http.Request) { r.TLS = &tls.ConnectionState{} })
	if tlsC.Name != "__Host-kivo_session" || !tlsC.Secure || tlsC.Path != "/" {
		t.Errorf("TLS: %s", tlsC)
	}
	proxied := login(func(r *http.Request) { r.RemoteAddr = "10.0.0.1:443"; r.Header.Set("X-Forwarded-Proto", "https") })
	if proxied.Name != "__Host-kivo_session" || !proxied.Secure {
		t.Errorf("trusted HTTPS proxy: %s", proxied)
	}
	spoofed := login(func(r *http.Request) { r.RemoteAddr = "192.0.2.9:443"; r.Header.Set("X-Forwarded-Proto", "https") })
	if spoofed.Name != "kivo_session" || spoofed.Secure {
		t.Errorf("untrusted proxy header: %s", spoofed)
	}
}

// TestBasePath serves the whole app under /kivo, including the API, the
// web client and the cookie.
func TestBasePath(t *testing.T) {
	e := newEnv(t, serving{BasePath: "/kivo"})
	if w := e.do("GET", "/kivo/api/v1/setup", "", nil); w.Code != http.StatusOK {
		t.Errorf("API under the base path: %d", w.Code)
	}
	if w := e.do("GET", "/api/v1/setup", "", nil); w.Code != http.StatusNotFound {
		t.Errorf("API outside the base path: %d", w.Code)
	}
	if w := e.do("GET", "/kivo/libraries/x", "", nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `<base href="/kivo/"`) {
		t.Errorf("web client route: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/kivo", "", nil); w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/kivo/" {
		t.Errorf("bare base path: %d %s", w.Code, w.Header().Get("Location"))
	}
	w := e.do("POST", "/kivo/api/v1/sessions", "", map[string]string{"username": "admin", "password": adminPassword})
	if session, client := loginCookies(t, w); session.Path != "/kivo/" || client.Path != "/kivo/" {
		t.Errorf("cookies: %s, %s", session, client)
	}
	if w := e.do("GET", "/kivo/api/v1/home", e.adminToken, nil); w.Code != http.StatusOK {
		t.Errorf("an authenticated route: %d", w.Code)
	}
}

// TestLogsHoldNoSecrets drives sign-in, playback and media requests. It
// then checks that the log holds no tokens, session IDs, play IDs or raw
// paths.
func TestLogsHoldNoSecrets(t *testing.T) {
	e := newEnv(t, serving{})
	c := e.webLogin()
	e.do("GET", "/api/v1/home", "", nil, "Cookie", c.String())
	w := e.do("POST", "/api/v1/plays", e.adminToken, map[string]any{"workId": e.heat.Work, "capabilities": clientCaps})
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	ps := decode[map[string]any](t, w)
	url := "/api/v1/" + ps["url"].(string)
	e.do("GET", url, "", nil)
	e.do("GET", "/api/v1/plays/"+e.heat.Work.String()+"/files/0", "", nil)
	e.do("GET", "/api/v1/works/"+e.heat.Work.String(), e.adminToken, nil)

	logs := e.logs.String()
	// The Logs page and its download, which show the same lines.
	logs += e.do("GET", "/api/v1/logs?level=debug&limit=5000", e.adminToken, nil).Body.String()
	logs += e.do("GET", "/api/v1/logs/file", e.adminToken, nil).Body.String()
	secrets := []string{e.adminToken, c.Value, e.adminSession.String(), ps["id"].(string), e.heat.Work.String(), "Heatwave"}
	for _, s := range secrets {
		if strings.Contains(logs, s) {
			t.Errorf("the log holds %q", s)
		}
	}
	if !strings.Contains(logs, "GET /api/v1/plays/{id}/files/{part}") {
		t.Errorf("the log has no route pattern:\n%s", logs)
	}
}

func TestBodyLimit(t *testing.T) {
	e := newEnv(t, serving{})
	w := e.do("POST", "/api/v1/sessions", "", map[string]string{"username": strings.Repeat("a", 2<<20), "password": "x"})
	if w.Code != http.StatusRequestEntityTooLarge || problemCode(w) != codeInvalidRequest {
		t.Fatalf("2 MB body: %d %s", w.Code, w.Body)
	}
}

func TestBodyReadTimeout(t *testing.T) {
	e := newEnv(t, serving{})
	saved := jsonBody
	jsonBody.timeout = 100 * time.Millisecond
	t.Cleanup(func() { jsonBody = saved })
	ts := httptest.NewServer(e.srv)
	defer ts.Close()

	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// The body stops short of its length and the client goes quiet.
	fmt.Fprint(conn, "POST /api/v1/sessions HTTP/1.1\r\nHost: kivo\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{\"user")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	res, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("no answer to a stalled body: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("stalled body: %d", res.StatusCode)
	}
}

func TestProblemMapping(t *testing.T) {
	// A problem document carries the request ID and no internal text.
	e := newEnv(t, serving{})
	w := e.do("GET", "/api/v1/works/"+e.heat.Work.String()+"x", e.adminToken, nil)
	p := decode[map[string]any](t, w)
	if w.Code != http.StatusBadRequest || p["code"] != codeInvalidRequest || p["requestId"] != w.Header().Get("X-Request-Id") || p["requestId"] == "" {
		t.Errorf("bad ID: %d %v", w.Code, p)
	}

	// A client that hung up, as a player does with segments it no longer
	// wants, isn't a server error.
	gone, cancel := context.WithCancel(ctx)
	cancel()
	rec := httptest.NewRecorder()
	e.srv.writeError(rec, httptest.NewRequestWithContext(gone, "GET", "/api/v1/plays/x/hls/9.m4s", nil), fmt.Errorf("segment: %w", context.Canceled))
	if rec.Code != statusClientClosed || strings.Contains(e.logs.String(), "request failed") {
		t.Errorf("a client that hung up: %d, logs %s", rec.Code, e.logs.String())
	}

	// Nor is a query that the client's leaving interrupted, as when the
	// web client cancels a refetch. SQLite stops such a query with an
	// interrupt rather than context.Canceled.
	left, leave := context.WithCancel(ctx)
	time.AfterFunc(50*time.Millisecond, leave)
	err := e.b.DB.Read(left, func(tx *sql.Tx) error {
		var n int
		return tx.QueryRowContext(left, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n) SELECT count(*) FROM n`).Scan(&n)
	})
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("an interrupted query: %v", err)
	}
	rec = httptest.NewRecorder()
	e.srv.writeError(rec, httptest.NewRequestWithContext(left, "GET", "/api/v1/libraries", nil), err)
	if rec.Code != statusClientClosed || strings.Contains(e.logs.String(), "request failed") || !strings.Contains(e.logs.String(), "request cancelled") {
		t.Errorf("a query the client's leaving interrupted: %d, logs %s", rec.Code, e.logs.String())
	}
}
