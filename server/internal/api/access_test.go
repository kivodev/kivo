package api

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/id"
)

// TestPublicOperations checks which operations the spec opens to callers
// without credentials. Making an operation public then needs a deliberate
// change here too.
func TestPublicOperations(t *testing.T) {
	rs, err := loadRoutes(spec)
	if err != nil {
		t.Fatal(err)
	}
	var public []string
	for _, r := range rs {
		if r.level == "public" {
			public = append(public, r.op)
		}
	}
	slices.Sort(public)
	if want := []string{"acceptInviteLink", "completeSetup", "createDeviceCode", "createSession", "getHealth", "getImage", "getInviteLink",
		"getServer", "getSetup", "pollDeviceCode"}; !slices.Equal(public, want) {
		t.Errorf("public operations %v, want %v", public, want)
	}
}

// TestMissingAccessLevelStopsStartup removes getWork's x-kivo-access, then
// sets it to a value that is not a level. Either change makes loadRoutes,
// and so New, fail with an error naming the operation and its path.
func TestMissingAccessLevelStopsStartup(t *testing.T) {
	const line = "      operationId: getWork\n"
	if bytes.Count(spec, []byte(line)) != 1 {
		t.Fatalf("spec has no single %q", line)
	}
	for name, replace := range map[string][2]string{
		"missing": {"      x-kivo-access: cast\n", ""},
		"unknown": {"      x-kivo-access: cast\n", "      x-kivo-access: owner\n"},
	} {
		at := bytes.Index(spec, []byte(line))
		rest := spec[at:]
		i := bytes.Index(rest, []byte(replace[0]))
		b := slices.Concat(spec[:at], rest[:i], []byte(replace[1]), rest[i+len(replace[0]):])
		_, err := loadRoutes(b)
		msg := fmt.Sprint(err)
		if !errors.Is(err, errSpec) || !strings.Contains(msg, "getWork (GET /api/v1/works/{id})") ||
			name == "unknown" && !strings.Contains(msg, `"owner"`) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestEveryRouteIsRegistered checks that every operation of the spec
// reaches a handler, generated or hand-written, rather than the API's 404.
func TestEveryRouteIsRegistered(t *testing.T) {
	e := newEnv(t, serving{})
	for _, rt := range e.srv.routes {
		r := httptest.NewRequest(rt.method, fill(rt.path, nil), nil)
		if _, pattern := e.srv.mux.Handler(r); pattern != rt.pattern() {
			t.Errorf("%s is served by %q", rt.pattern(), pattern)
		}
	}
}

var pathParam = regexp.MustCompile(`\{(\w+)\}`)

// fill puts IDs into a route's path. It looks up ids by the path's first
// segment after /api/v1/, and uses a random ID when there is none.
func fill(path string, ids map[string]id.ID) string {
	seg := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")[0]
	return pathParam.ReplaceAllStringFunc(path, func(p string) string {
		switch p {
		case "{name}":
			return "main.m3u8"
		case "{part}":
			return "0"
		case "{id}":
			if v, ok := ids[seg]; ok {
				return v.String()
			}
		}
		return id.New().String()
	})
}

// body is a request body that gets past parsing, so the access check is
// what answers.
func (e *env) body(op string) any {
	switch op {
	case "completeSetup", "createSession", "acceptInviteLink":
		return map[string]any{"username": "x", "password": "wrong password"}
	case "startPlay":
		return map[string]any{"workId": e.heat.Work, "capabilities": clientCaps}
	case "createLibrary":
		return map[string]any{"name": "x", "kind": "movies", "folders": []string{"/x"}}
	case "matchWork":
		return map[string]any{"matchId": "tmdb:movie:1"}
	case "updateSession":
		return map[string]any{"name": "x"}
	case "updatePlay":
		return map[string]any{"positionMs": 0, "state": "playing"}
	case "stopPlay":
		return map[string]any{"positionMs": 0}
	case "fallbackPlay":
		return map[string]any{"positionMs": 0, "code": "unsupported"}
	}
	return map[string]any{}
}

// TestNoCredentials calls every route without credentials. Only public
// routes answer. Media routes, whose credential is the play's ID, find no
// such play.
func TestNoCredentials(t *testing.T) {
	e := newEnv(t, serving{})
	for _, rt := range e.srv.routes {
		if rt.upload {
			continue // multipart; checked below by its level like the rest
		}
		w := e.do(rt.method, fill(rt.path, nil), "", e.body(rt.op))
		switch rt.level {
		case public:
			if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized && problemCode(w) != "invalidCredentials" {
				t.Errorf("%s: public route answered %d", rt.op, w.Code)
			}
		case play:
			if w.Code != http.StatusNotFound {
				t.Errorf("%s: %d %s, want 404 for an unknown play", rt.op, w.Code, problemCode(w))
			}
		default:
			if w.Code != http.StatusUnauthorized || problemCode(w) != codeUnauthenticated {
				t.Errorf("%s: %d %s, want 401 unauthenticated", rt.op, w.Code, problemCode(w))
			}
		}
	}
	w := e.do("POST", "/api/v1/works/"+e.heat.Work.String()+"/artwork", "", nil, "Content-Type", "multipart/form-data; boundary=x")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("uploadWorkArtwork: %d", w.Code)
	}
}

// TestNoLibraryAccess calls every route as a user granted no library.
// Nothing seeded comes back. Anything the user can't see is a 404, the
// same as something that doesn't exist. Admin routes are refused.
func TestNoLibraryAccess(t *testing.T) {
	e := newEnv(t, serving{})
	ids := map[string]id.ID{
		"libraries": e.lib, "works": e.heat.Work, "files": e.heat.File, "images": id.New(),
	}
	seeded := append([]string{}, e.seededTitles...)
	for _, x := range []id.ID{e.lib, e.folder, e.heat.Work, e.heat.Version, e.heat.File, e.series, e.episode.Work, e.episode.File} {
		seeded = append(seeded, x.String())
	}
	check := func(name string, w *httptest.ResponseRecorder) {
		t.Helper()
		for _, s := range seeded {
			if strings.Contains(w.Body.String(), s) {
				t.Errorf("%s: response holds seeded %q: %s", name, s, w.Body.String())
			}
		}
	}
	for _, rt := range e.srv.routes {
		if rt.level == public || rt.upload || rt.op == "deleteCurrentSession" {
			continue // signing out would end the session the rest use
		}
		path := fill(rt.path, ids)
		w := e.do(rt.method, path, e.nobodyToken, e.body(rt.op))
		check(rt.op, w)
		switch {
		case rt.level == admin:
			if w.Code != http.StatusForbidden || problemCode(w) != codeForbidden {
				t.Errorf("%s: %d %s, want 403 forbidden", rt.op, w.Code, problemCode(w))
			}
		case rt.level == play, strings.Contains(rt.path, "{id}") && rt.op != "openEventStream":
			if w.Code != http.StatusNotFound || problemCode(w) != codeNotFound {
				t.Errorf("%s: %d %s, want 404", rt.op, w.Code, problemCode(w))
			}
		}
	}
	for _, q := range []string{
		"/api/v1/libraries/" + e.lib.String() + "/works",
		"/api/v1/works/" + e.series.String() + "/children",
		"/api/v1/libraries/" + e.lib.String() + "/collections",
	} {
		w := e.do("GET", q, e.nobodyToken, nil)
		check(q, w)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", q, w.Code)
		}
	}
	for _, q := range []string{"/api/v1/search?q=Seeded", "/api/v1/home", "/api/v1/libraries"} {
		w := e.do("GET", q, e.nobodyToken, nil)
		check(q, w)
		if w.Code != http.StatusOK {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
	// The same requests made as the admin do see the data, which shows the
	// checks above would catch a leak.
	if w := e.do("GET", "/api/v1/search?q=Heatwave", e.adminToken, nil); !strings.Contains(w.Body.String(), e.heat.Work.String()) {
		t.Errorf("admin search: %d %s", w.Code, w.Body.String())
	}
}
