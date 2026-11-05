package api

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// placeholder stands in for the web client in a build without one.
//
//go:embed placeholder
var placeholder embed.FS

// webApp serves the web client's files, and index.html for every other path
// so client-side routes load. API paths never reach it, because the API
// answers its own 404s.
type webApp struct {
	files fs.FS
	index []byte
}

// newWebApp reads the client's index.html and points its <base href> at the
// base path. The client builds its asset, route and API URLs from it.
func newWebApp(files fs.FS, base string) (*webApp, error) {
	if files == nil {
		files, _ = fs.Sub(placeholder, "placeholder")
	}
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil, err
	}
	const tag = `<base href="/"`
	if !bytes.Contains(index, []byte(tag)) {
		return nil, fmt.Errorf("index.html has no %s />", tag)
	}
	index = bytes.Replace(index, []byte(tag), []byte(`<base href="`+html.EscapeString(base+"/")+`"`), 1)
	return &webApp{files: files, index: index}, nil
}

func (a *webApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(a.files, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // hashed names
			}
			http.ServeFileFS(w, r, a.files, name)
			return
		}
		// A missing file with an extension, such as an asset from an older
		// build, is a 404. Serving index.html in its place would fail as a
		// script.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(a.index)
}
