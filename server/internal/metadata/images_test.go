package metadata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
)

// bombPNG returns a 1×1 PNG whose header claims width×height. It is tiny on
// disk but enormous once decoded.
func bombPNG(width, height uint32) []byte {
	b := pngBytes(1, 1)
	ihdr := b[8+8 : 8+8+13] // signature, length and type, then IHDR's data
	binary.BigEndian.PutUint32(ihdr[0:], width)
	binary.BigEndian.PutUint32(ihdr[4:], height)
	binary.BigEndian.PutUint32(b[8+8+13:], crc32.ChecksumIEEE(b[8+4:8+8+13]))
	return b
}

func jpegBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func animatedGIF() []byte {
	g := &gif.GIF{}
	for i := range 3 {
		frame := image.NewPaletted(image.Rect(0, 0, 40, 60), palette.Plan9)
		for p := range frame.Pix {
			frame.Pix[p] = uint8(i * 50)
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// webpHeader returns the start of a WebP with a VP8X chunk claiming w×h.
func webpHeader(w, h int) []byte {
	b := make([]byte, 30)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], 22)
	copy(b[8:], "WEBPVP8X")
	binary.LittleEndian.PutUint32(b[16:], 10)
	b[24], b[25], b[26] = byte(w-1), byte((w-1)>>8), byte((w-1)>>16)
	b[27], b[28], b[29] = byte(h-1), byte((h-1)>>8), byte((h-1)>>16)
	return b
}

func TestCheckImage(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		w, h int // zero when rejected
	}{
		{"png", pngBytes(30, 45), 30, 45},
		{"jpeg", jpegBytes(64, 48), 64, 48},
		{"animated gif", animatedGIF(), 40, 60},
		{"webp", webpHeader(1000, 1500), 1000, 1500},
		{"decompression bomb", bombPNG(50000, 50000), 0, 0},
		{"webp bomb", webpHeader(10000, 10000), 0, 0},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><script>alert(1)</script></svg>`), 0, 0},
		{"svg with a prolog", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), 0, 0},
		{"html", []byte(`<html><body>not an image</body></html>`), 0, 0},
		{"empty", nil, 0, 0},
	} {
		w, h, err := checkImage(tc.data)
		if tc.w == 0 {
			if !errors.Is(err, ErrBadImage) {
				t.Errorf("%s: got %d×%d, %v; want ErrBadImage", tc.name, w, h, err)
			}
			continue
		}
		if err != nil || w != tc.w || h != tc.h {
			t.Errorf("%s: got %d×%d, %v; want %d×%d", tc.name, w, h, err, tc.w, tc.h)
		}
	}
}

func TestEncodeImages(t *testing.T) {
	img := imager(t)
	e := newEnvWith(t, img)
	full := jpegBytes(320, 240)

	// These are rejected before anything decodes them, so media never sees
	// them.
	for name, data := range map[string][]byte{
		"decompression bomb": bombPNG(50000, 50000),
		"svg":                []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
	} {
		if _, err := e.s.encode(ctx, artworkPoster, data); !errors.Is(err, ErrBadImage) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if f, ok := img.(*fakeImager); ok && f.count() != 0 {
		t.Errorf("media was asked to decode %d rejected images", f.count())
	}

	// An animated GIF becomes a still of its first frame.
	p, err := e.s.encode(ctx, artworkPoster, animatedGIF())
	if err != nil {
		t.Fatalf("animated GIF: %v", err)
	}
	for _, w := range imageWidths[artworkPoster] {
		if _, err := os.Stat(e.s.path(p.ID, w)); err != nil {
			t.Errorf("width %d: %v", w, err)
		}
	}
	if p.Width != 40 || p.Height != 60 || len(p.Blurhash) != 28 {
		t.Errorf("animated GIF recorded as %+v", p)
	}

	// A truncated JPEG passes the header check. Decoding it is media's job,
	// in a child process, and ffmpeg may salvage it. Either way, only a
	// usable image or ErrBadImage comes back, and no file is left over.
	_, err = e.s.encode(ctx, artworkBackdrop, full[:len(full)/2])
	if err != nil && !errors.Is(err, ErrBadImage) {
		t.Errorf("truncated JPEG: %v", err)
	}
	if _, ok := img.(*fakeImager); ok && !errors.Is(err, ErrBadImage) {
		t.Errorf("truncated JPEG with the fake: %v", err)
	}
	left, _ := filepath.Glob(filepath.Join(e.cache, "images", cachedir.Temp, "*"))
	if len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func TestImagePathServesOnlyReencodedFiles(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, "Dune (2021)/Dune (2021).mkv")
	e.mustRefresh(w)
	poster := e.images(w)[artworkPoster]

	for want, width := range map[int]int{240: 100, 480: 300, 960: 960, 961: 5000} {
		p, err := e.s.ImagePath(ctx, poster.ID, width)
		if err != nil {
			t.Fatal(err)
		}
		if want == 961 {
			want = 960 // nothing wider exists
		}
		if !strings.HasPrefix(p, filepath.Join(e.cache, "images")+string(filepath.Separator)) || !strings.HasSuffix(p, "-"+strconv.Itoa(want)+".webp") {
			t.Errorf("width %d: %s", width, p)
		}
		data, err := os.ReadFile(p)
		if err != nil || !bytes.HasPrefix(data, []byte("RIFF webp")) {
			t.Errorf("width %d: served %q, %v; want media's re-encoding", width, data[:min(len(data), 16)], err)
		}
	}

	// A width the image lacks, as when it was encoded before its kind gained
	// that width, serves the widest width below it.
	p, _ := e.s.ImagePath(ctx, poster.ID, 960)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if p, err := e.s.ImagePath(ctx, poster.ID, 960); err != nil || !strings.HasSuffix(p, "-480.webp") {
		t.Fatalf("a missing width: %s, %v", p, err)
	}

	// When the cache lost the files, the result is not found and nothing is
	// written. The work's next refresh rebuilds them.
	e.s.deleteFiles(poster.ID, artworkPoster)
	if _, err := e.s.ImagePath(ctx, poster.ID, 480); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing file: %v", err)
	}
	if got := e.tasks.take(TaskRefreshMetadata); len(got) != 0 {
		t.Fatal("serving a missing file asked for a refresh")
	}
	e.mustRefresh(w)
	if _, err := e.s.ImagePath(ctx, e.images(w)[artworkPoster].ID, 480); err != nil {
		t.Fatalf("after the refresh: %v", err)
	}
}

func TestBlurhash(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 20))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:], []byte{255, 0, 0, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	h, err := blurhash(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	// 4×3 components, the 4 across the wider side, and a pure red average.
	var dc strings.Builder
	encode83(&dc, 0xff0000, 4)
	if len(h) != 28 || h[0] != base83[3+2*9] || h[2:6] != dc.String() {
		t.Errorf("blurhash %q", h)
	}
	if _, err := blurhash([]byte("not a png")); err == nil {
		t.Error("blurhash of garbage")
	}
}

// The daily prune deletes image files that nothing refers to, once they are
// old enough that no encode can still be recording them. It keeps artwork,
// previews of offered artwork, and the uploaded originals.
func TestPruneImages(t *testing.T) {
	e := newEnv(t)
	_, folder := e.b.Library(catalog.Movies)
	w := e.movie(folder, duneRel)
	e.mustRefresh(w)
	poster := e.images(w)[artworkPoster]
	artworks := e.artwork(w, artworkPoster)
	preview, err := e.s.ImagePath(ctx, artworks[len(artworks)-1].ID, 240)
	if err != nil {
		t.Fatal(err)
	}
	upload := filepath.Join(e.uploads, "kept.png")
	orphan := e.s.path(id.New(), 240)
	leftover := filepath.Join(e.cache, "images", cachedir.Temp, id.New().String()+".src")
	for _, p := range []string{upload, orphan, leftover} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * pruneAge)
	for _, dir := range []string{e.cache, e.uploads} {
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				os.Chtimes(p, old, old)
			}
			return nil
		})
	}
	young := e.s.path(id.New(), 240)
	if err := os.MkdirAll(filepath.Dir(young), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(young, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := e.s.pruneImages(ctx)
	if err != nil || n != 2 {
		t.Fatalf("swept %d, %v; want the orphan and the leftover", n, err)
	}
	for _, p := range []string{orphan, leftover} {
		if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s kept: %v", p, err)
		}
	}
	kept := []string{preview, upload, young}
	for _, width := range imageWidths[artworkPoster] {
		kept = append(kept, e.s.path(poster.ID, width))
	}
	for _, p := range kept {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed: %v", p, err)
		}
	}
}
