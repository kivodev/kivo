package api

import (
	"net/http"
	"strconv"

	"github.com/mmcdole/kivo/internal/id"
)

// serveImage serves pre-resized artwork to anyone with its ID, which can't
// be guessed. Google Home and Android's cast notification fetch a cast's
// artwork without credentials, and this lets them show it. An image ID's
// content never changes, so it's cached privately for good.
func (s *Server) serveImage(w http.ResponseWriter, r *http.Request) {
	img, err := id.Parse(r.PathValue("id"))
	if err != nil {
		s.writeProblem(w, r, invalid("the image ID is malformed", field("id", "invalid")))
		return
	}
	width := 0
	if q := r.URL.Query().Get("width"); q != "" {
		if width, err = strconv.Atoi(q); err != nil || width < 1 {
			s.writeProblem(w, r, invalid("width must be a positive integer", field("width", "invalid")))
			return
		}
	}
	path, err := s.deps.Metadata.ImagePath(r.Context(), img, width)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	f, modified, err := openServed(path)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", modified, f)
}
