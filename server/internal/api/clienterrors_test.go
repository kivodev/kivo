package api

import (
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/id"
)

const clientErrors = "/api/v1/client-errors"

func TestClientErrorNeedsSignIn(t *testing.T) {
	e := newEnv(t, serving{})
	report := map[string]any{"kind": "error", "message": "boom"}
	if w := e.do("POST", clientErrors, "", report); w.Code != http.StatusUnauthorized {
		t.Errorf("signed out: %d", w.Code)
	}
	// A browser session's cookie counts, from its own origin only.
	c := e.webLogin("Origin", "http://example.com")
	if w := e.do("POST", clientErrors, "", report, "Cookie", c.String(), "Origin", "https://evil.example"); w.Code != http.StatusForbidden {
		t.Errorf("cookie from another origin: %d", w.Code)
	}
	if w := e.do("POST", clientErrors, "", report, "Cookie", c.String(), "Origin", "http://example.com"); w.Code != http.StatusNoContent {
		t.Errorf("cookie: %d %s", w.Code, w.Body.String())
	}
	if strings.Count(e.logs.String(), "client: error reported") != 1 {
		t.Errorf("only the signed-in report is logged: %s", e.logs.String())
	}
}

func TestClientErrorLogged(t *testing.T) {
	e := newEnv(t, serving{})
	stack := make([]string, 15)
	for i := range stack {
		stack[i] = "at f (http://tv.local/tv/assets/index.js:1:" + strings.Repeat("9", 400) + ")"
	}
	stack[0] = "at g (http://tv.local/tv/assets/chunk.js?v=2#x)"
	w := e.do("POST", clientErrors, e.adminToken, map[string]any{
		"kind":    "unhandledRejection",
		"message": "failed\x1b[31m http://nas:8484/api/v1/search?q=secret-search " + strings.Repeat("m", 600),
		"stack":   stack,
		"route":   "/works/0198a3c2-7e41-7c3b-9b1a-5f1c2d3e4f50/files",
		"version": "1.2.3+abc",
		"engine":  map[string]any{"platform": "tizen", "tizenVersion": "8.0", "chromiumVersion": "108", "extra": "x"},
		// Fields the API doesn't take are ignored, never logged.
		"cookies": "kivo_session=leak", "headers": map[string]string{"Authorization": "Bearer leak"}, "url": "http://x/?q=leak",
	}, "Cookie", "kivo_session=cookieleak", "X-Secret", "headerleak")
	if w.Code != http.StatusNoContent {
		t.Fatalf("report: %d %s", w.Code, w.Body.String())
	}

	// The Logs page shows it at Warn, with the caller's user and the
	// request's ID, and never the session's ID.
	page := decode[logPage](t, e.do("GET", "/api/v1/logs?level=warning&q=client", e.adminToken, nil))
	if len(page.Items) != 1 || page.Items[0].Level != "warning" || page.Items[0].Message != "client: error reported" {
		t.Fatalf("logs: %+v", page)
	}
	attrs := map[string]string{}
	for _, a := range page.Items[0].Attributes {
		attrs[a.Key] = a.Value
	}
	want := map[string]string{
		"kind": "unhandledRejection", "user": e.admin.String(), "request": w.Header().Get("X-Request-Id"),
		"route": "/works/$id/files", "client_version": "1.2.3+abc", "platform": "tizen", "tizen": "8.0", "chromium": "108",
	}
	for k, v := range want {
		if attrs[k] != v {
			t.Errorf("%s = %q, want %q", k, attrs[k], v)
		}
	}
	if m := attrs["message"]; len([]rune(m)) != maxErrorMessage || !strings.HasPrefix(m, "failed [31m http://nas:8484/api/v1/search m") {
		t.Errorf("message %q", m)
	}
	if attrs["request"] == "" {
		t.Error("no request ID")
	}
	for _, k := range []string{"session", "api_key"} {
		if _, ok := attrs[k]; ok {
			t.Errorf("the log holds %s", k)
		}
	}
	frames := strings.Split(attrs["stack"], "\n")
	if len(frames) != maxStackFrames || frames[0] != "at g (http://tv.local/tv/assets/chunk.js)" || len(frames[1]) != maxStackFrame {
		t.Errorf("%d frames: %q", len(frames), frames)
	}

	// Nothing the caller sent beyond the report's fields reaches the log,
	// nor its token, nor a URL's query.
	log := e.logs.String()
	for _, leak := range []string{e.adminToken, e.adminSession.String(), "leak", "secret-search", "v=2", "extra"} {
		if strings.Contains(log, leak) {
			t.Errorf("the log holds %q", leak)
		}
	}
}

func TestClientErrorFromAPIKey(t *testing.T) {
	e := newEnv(t, serving{})
	k := decode[map[string]any](t, e.do("POST", "/api/v1/api-keys", e.adminToken, map[string]any{"name": "Script"}))
	token, _ := k["token"].(string)
	keyID, _ := k["id"].(string)
	if token == "" || keyID == "" {
		t.Fatalf("api key: %v", k)
	}
	if w := e.do("POST", clientErrors, token, map[string]any{"kind": "error", "message": "from a key"}); w.Code != http.StatusNoContent {
		t.Fatalf("report: %d %s", w.Code, w.Body.String())
	}
	var line string
	for l := range strings.SplitSeq(e.logs.String(), "\n") {
		if strings.Contains(l, "client: error reported") {
			line = l
		}
	}
	if !strings.Contains(line, e.admin.String()) {
		t.Errorf("no user: %s", line)
	}
	if strings.Contains(line, keyID) || strings.Contains(line, "api_key") {
		t.Errorf("the log holds the key's ID: %s", line)
	}
}

func TestClientErrorValidation(t *testing.T) {
	e := newEnv(t, serving{})
	for name, body := range map[string]map[string]any{
		"unknown kind": {"kind": "crash", "message": "x"},
		"no kind":      {"message": "x"},
	} {
		if w := e.do("POST", clientErrors, e.adminToken, body); w.Code != http.StatusBadRequest || problemCode(w) != codeInvalidRequest {
			t.Errorf("%s: %d %s", name, w.Code, problemCode(w))
		}
	}
	if w := e.do("POST", clientErrors, e.adminToken, "not an object"); w.Code != http.StatusBadRequest {
		t.Errorf("not an object: %d", w.Code)
	}

	// A route that isn't a template, and a version or engine that isn't a
	// short word, are dropped, and the rest is logged.
	w := e.do("POST", clientErrors, e.adminToken, map[string]any{
		"kind": "chunk", "message": "chunk failed", "route": "/search?q=private", "version": "1.0 <script>",
		"engine": map[string]any{"platform": strings.Repeat("t", 40)},
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("report: %d %s", w.Code, w.Body.String())
	}
	var log string
	for l := range strings.SplitSeq(e.logs.String(), "\n") {
		if strings.Contains(l, "client: error reported") {
			log = l
		}
	}
	if !strings.Contains(log, "chunk failed") {
		t.Fatalf("not logged: %s", e.logs.String())
	}
	for _, leak := range []string{"private", "<script>", "route=", "platform=", "client_version="} {
		if strings.Contains(log, leak) {
			t.Errorf("the log holds %q", leak)
		}
	}
}

func TestClientErrorRateLimit(t *testing.T) {
	e := newEnv(t, serving{})
	report := map[string]any{"kind": "error", "message": "again"}
	send := func(token string) *http.Response {
		t.Helper()
		return e.do("POST", clientErrors, token, report).Result()
	}
	for i := range callerErrorReports {
		if r := send(e.adminToken); r.StatusCode != http.StatusNoContent {
			t.Fatalf("report %d: %d", i+1, r.StatusCode)
		}
	}
	r := send(e.adminToken)
	if r.StatusCode != http.StatusTooManyRequests || r.Header.Get("Retry-After") == "" {
		t.Fatalf("over the session's limit: %d, Retry-After %q", r.StatusCode, r.Header.Get("Retry-After"))
	}
	if n := strings.Count(e.logs.String(), "client: error reported"); n != callerErrorReports {
		t.Errorf("%d reports logged, want %d: the excess is dropped silently", n, callerErrorReports)
	}

	// Another session from the same address has its own count, until the
	// address's runs out.
	sent := callerErrorReports
	for sent < addrErrorReports {
		token := e.session(e.admin)
		for range callerErrorReports {
			if r := send(token); r.StatusCode != http.StatusNoContent {
				t.Fatalf("report %d from the address: %d", sent+1, r.StatusCode)
			}
			sent++
		}
	}
	if r := send(e.session(e.admin)); r.StatusCode != http.StatusTooManyRequests {
		t.Errorf("over the address's limit: %d", r.StatusCode)
	}
	if w := e.do("POST", clientErrors, e.session(e.admin), report); problemCode(w) != codeRateLimited {
		t.Errorf("code %q", problemCode(w))
	}
}

// Reports are counted by address as auth counts sign-in attempts: two
// IPv6 addresses in one /64 share a count, and IPv4-mapped is IPv4.
func TestClientErrorReportsByAddress(t *testing.T) {
	var r errorReports
	now := time.Now()
	// Each report comes from a new session, so only the address's count
	// can run out.
	fresh := func() caller {
		var c caller
		c.Session = id.New()
		return c
	}
	for _, tc := range []struct{ a, b string }{
		{"2001:db8:1:2::1", "2001:db8:1:2:ffff::9"},
		{"192.0.2.7", "::ffff:192.0.2.7"},
	} {
		r = errorReports{}
		a, b := netip.MustParseAddr(tc.a), netip.MustParseAddr(tc.b)
		for i := range addrErrorReports {
			ip := a
			if i%2 == 1 {
				ip = b
			}
			if wait := r.take(fresh(), ip, now); wait != 0 {
				t.Fatalf("%s report %d: wait %v", tc.a, i+1, wait)
			}
		}
		if r.take(fresh(), b, now) == 0 {
			t.Errorf("%s and %s are counted apart", tc.a, tc.b)
		}
	}
	r = errorReports{}
	for range addrErrorReports {
		r.take(fresh(), netip.MustParseAddr("2001:db8:1:2::1"), now)
	}
	if r.take(fresh(), netip.MustParseAddr("2001:db8:1:3::1"), now) != 0 {
		t.Error("another /64 shares the count")
	}
}

// A file: page sends no Origin, so no file:// or opaque origin is allowed
// across origins. A request from one gets its answer with no CORS headers.
func TestServerInfoNoFileOrigins(t *testing.T) {
	e := newEnv(t, serving{})
	for _, origin := range []string{"file://", "null"} {
		w := e.do("GET", "/api/v1/server", "", nil, "Origin", origin)
		if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: %d allowed %q", origin, w.Code, w.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}
