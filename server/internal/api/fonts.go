package api

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// fontURL is where a font is served, relative to the API root.
func fontURL(hash string) string { return "fonts/" + hash }

// serveFont serves a font from the subtitles store by its SHA-256. The
// viewer must be able to see a library holding a file that carries the
// font as an attachment. A font that isn't stored and a font the viewer
// may not see get the same 404. Access is checked before If-None-Match is
// answered. A font's bytes never change, so it's cached privately for
// good. Its type is read from its bytes, never from what the carrying file
// declares.
func (s *Server) serveFont(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("sha256")
	f, found, err := s.deps.Subtitles.Font(r.Context(), callerOf(r.Context()).viewer, hash)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if !found {
		s.writeProblem(w, r, errNotFound)
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("ETag", `W/"`+hash+`"`)
	h.Set("Vary", "Accept-Encoding")
	h.Set("X-Content-Type-Options", "nosniff")
	if etagMatches(r.Header.Get("If-None-Match"), hash) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", f.Type)
	// WOFF and WOFF2 are compressed already.
	if f.Type != "font/woff" && f.Type != "font/woff2" && acceptsGzip(r) {
		h.Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		if _, err := gz.Write(f.Data); err == nil {
			_ = gz.Close()
		}
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(f.Data)))
	_, _ = w.Write(f.Data)
}

// etagMatches reports whether an If-None-Match header names the entity tag
// whose value is tag. It compares weakly, as RFC 9110 specifies for
// If-None-Match.
func etagMatches(header, tag string) bool {
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimPrefix(strings.TrimSpace(t), "W/")
		if t == "*" || t == `"`+tag+`"` {
			return true
		}
	}
	return false
}

// acceptsGzip reports whether a request accepts a gzipped response.
func acceptsGzip(r *http.Request) bool {
	for _, e := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(e), ";")
		if strings.EqualFold(strings.TrimSpace(name), "gzip") {
			q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
			return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
		}
	}
	return false
}
