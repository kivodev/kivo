package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmcdole/kivo/internal/store"

	"github.com/mmcdole/kivo/internal/catalog"
)

// storeFont puts a font in the subtitles store and records it as an
// attachment of the seeded film. It returns the font's hash.
func (e *env) storeFont(data []byte, ext string, stored bool) string {
	e.t.Helper()
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if stored {
		p := filepath.Join(e.cache, "fonts", hash[:2], hash+ext)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			e.t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			e.t.Fatal(err)
		}
	}
	f, err := e.b.Catalog.File(ctx, e.heat.File)
	if err != nil {
		e.t.Fatal(err)
	}
	attached := append(f.Attachments, catalog.Attachment{Kind: catalog.AttachmentFont, Filename: "a" + ext, MimeType: "application/x-truetype-font", Size: int64(len(data)), SHA256: hash})
	e.b.Write(func(ctx context.Context, tx *store.Tx) error {
		return e.b.Catalog.SetProbe(ctx, tx, f.ID, f.Generation, catalog.Probe{Summary: f.Probe, Tracks: f.Tracks, Chapters: f.Chapters, Attachments: attached})
	})
	return hash
}

func TestFonts(t *testing.T) {
	e := newEnv(t, serving{})
	ttf := append([]byte("\x00\x01\x00\x00\x00\x0aOS/2"), bytes.Repeat([]byte("glyphs "), 200)...)
	woff2 := append([]byte("wOF2"), bytes.Repeat([]byte{7}, 300)...)
	hash := e.storeFont(ttf, ".ttf", true)
	compressed := e.storeFont(woff2, ".woff2", true)
	unstored := e.storeFont([]byte("\x00\x01\x00\x00unstored"), ".ttf", false)
	other := strings.Repeat("ab", 32) // no file carries it

	// A signed-in viewer who may see the film gets the font. It is
	// cacheable for good and typed by its bytes.
	w := e.do("GET", "/api/v1/fonts/"+hash, e.adminToken, nil)
	h := w.Header()
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), ttf) || h.Get("Content-Type") != "font/ttf" ||
		h.Get("Cache-Control") != "private, max-age=31536000, immutable" || h.Get("ETag") != `W/"`+hash+`"` ||
		h.Get("Vary") != "Accept-Encoding" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Encoding") != "" {
		t.Fatalf("font: %d %v", w.Code, h)
	}

	// The font is gzipped when the client accepts gzip. WOFF2 never is,
	// because it's compressed already.
	w = e.do("GET", "/api/v1/fonts/"+hash, e.adminToken, nil, "Accept-Encoding", "br, gzip;q=0.8")
	zr, err := gzip.NewReader(w.Body)
	if err != nil || w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip: %v %v", err, w.Header())
	}
	if b, _ := io.ReadAll(zr); !bytes.Equal(b, ttf) {
		t.Fatal("gzipped font differs")
	}
	if w := e.do("GET", "/api/v1/fonts/"+hash, e.adminToken, nil, "Accept-Encoding", "gzip;q=0"); w.Header().Get("Content-Encoding") != "" {
		t.Fatal("gzipped though refused")
	}
	w = e.do("GET", "/api/v1/fonts/"+compressed, e.adminToken, nil, "Accept-Encoding", "gzip")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "font/woff2" || w.Header().Get("Content-Encoding") != "" || !bytes.Equal(w.Body.Bytes(), woff2) {
		t.Fatalf("woff2: %d %v", w.Code, w.Header())
	}

	// Revalidation matches the ETag weakly, whether the request sends it
	// strong or weak.
	for _, tag := range []string{`W/"` + hash + `"`, `"` + hash + `"`, `"x", W/"` + hash + `"`, "*"} {
		if w := e.do("GET", "/api/v1/fonts/"+hash, e.adminToken, nil, "If-None-Match", tag); w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Errorf("If-None-Match %s: %d", tag, w.Code)
		}
	}
	if w := e.do("GET", "/api/v1/fonts/"+hash, e.adminToken, nil, "If-None-Match", `W/"`+other+`"`); w.Code != http.StatusOK {
		t.Errorf("another ETag: %d", w.Code)
	}

	// Not signed in: 401.
	if w := e.do("GET", "/api/v1/fonts/"+hash, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", w.Code)
	}

	// A font that isn't visible, isn't carried by any file, isn't stored,
	// or isn't a hash gets the same 404. Access is checked before
	// If-None-Match.
	missing := e.do("GET", "/api/v1/fonts/"+other, e.adminToken, nil)
	if missing.Code != http.StatusNotFound || problemCode(missing) != "notFound" {
		t.Fatalf("a font no file carries: %d %s", missing.Code, missing.Body)
	}
	for name, w := range map[string]*httptest.ResponseRecorder{
		"not visible":          e.do("GET", "/api/v1/fonts/"+hash, e.nobodyToken, nil),
		"not visible, cached":  e.do("GET", "/api/v1/fonts/"+hash, e.nobodyToken, nil, "If-None-Match", `W/"`+hash+`"`),
		"not stored":           e.do("GET", "/api/v1/fonts/"+unstored, e.adminToken, nil),
		"uppercase":            e.do("GET", "/api/v1/fonts/"+strings.ToUpper(hash), e.adminToken, nil),
		"short":                e.do("GET", "/api/v1/fonts/"+hash[:63], e.adminToken, nil),
		"with an extension":    e.do("GET", "/api/v1/fonts/"+hash+".ttf", e.adminToken, nil),
		"another viewer's 404": e.do("GET", "/api/v1/fonts/"+other, e.nobodyToken, nil),
	} {
		if w.Code != missing.Code || problemCode(w) != problemCode(missing) || w.Header().Get("ETag") != "" {
			t.Errorf("%s: %d %s %v", name, w.Code, w.Body, w.Header())
		}
	}
}
