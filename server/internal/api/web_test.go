package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWebApp(t *testing.T) {
	files := fstest.MapFS{
		"index.html":         {Data: []byte(`<html><head><base href="/" /><meta name="kivo-version" content="dev" /></head></html>`)},
		"assets/app-1a2b.js": {Data: []byte("console.log(1)")},
	}
	app, err := newWebApp(files, "/kivo", "0.3.1-dev+56c0018d")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path string
		code int
		has  string
	}{
		{"/", 200, `<base href="/kivo/"`},
		{"/", 200, `<meta name="kivo-version" content="0.3.1-dev+56c0018d" />`}, // the server's version, which the client sends
		{"/movies/abc", 200, `<base href="/kivo/"`}, // a client route
		{"/assets/app-1a2b.js", 200, "console.log"},
		{"/assets/app-old.js", 404, ""}, // an older build's asset
		{"/favicon.ico", 404, ""},
	} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.has) {
			t.Errorf("%s: %d %q, want %d containing %q", c.path, rec.Code, rec.Body.String(), c.code, c.has)
		}
	}
}

func TestNoTVApp(t *testing.T) {
	// The Samsung TV app carries the TV build itself, so the server serves
	// none: /tv/ is one of the web client's routes like any other path.
	files := fstest.MapFS{
		"index.html":    {Data: []byte(`<html><head><base href="/" /></head>WEB</html>`)},
		"tv/index.html": {Data: []byte(`<html><head><base href="/" /></head>TV</html>`)},
	}
	web, err := newWebApp(files, "/kivo", "0.4.0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{deps: Deps{BasePath: "/kivo"}, mux: http.NewServeMux(), web: web}
	s.mux.Handle("/", s.web)
	for _, p := range []string{"/tv/", "/tv/movies/abc"} {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "WEB") {
			t.Errorf("%s: %d %q, want the web client", p, rec.Code, rec.Body.String())
		}
	}
	// A stale tv/ directory in the build isn't served as a TV page either:
	// its index.html redirects to /tv/, the web client.
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tv/index.html", nil))
	if strings.Contains(rec.Body.String(), "TV") {
		t.Errorf("/tv/index.html served the stale TV page: %d", rec.Code)
	}
}
