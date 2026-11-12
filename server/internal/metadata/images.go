package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // DecodeConfig for sniffed GIFs
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/cachedir"
	"github.com/mmcdole/kivo/internal/egress"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/metadata/dbq"
)

// Image kinds: a work's artwork, and a person's photo.
const (
	artworkPoster   = "poster"
	artworkBackdrop = "backdrop"
	artworkLogo     = "logo"
	artworkBanner   = "banner"
	artworkThumb    = "thumb" // landscape artwork with the title on it
	artworkStill    = "still" // a frame from an episode
	personPhoto     = "photo"
)

// imageWidths lists the WebP widths each kind is re-encoded to, so clients
// only ever load sizes the server made. Backdrops go up to a 4K TV's 3840.
// Posters, including square book covers, go up to 960.
var imageWidths = map[string][]int{
	artworkPoster:   {240, 480, 960},
	artworkBackdrop: {640, 1280, 1920, 3840},
	artworkLogo:     {400, 800},
	artworkBanner:   {500, 1000},
	artworkThumb:    {640, 1280},
	artworkStill:    {320, 640},
	personPhoto:     {185, 370},
}

// Image sources.
const (
	sourceProvider = "provider"
	sourceLocal    = "local"
	sourceUpload   = "upload"
)

const (
	maxImageBytes = egress.MaxBody
	maxPixels     = 40_000_000
)

// ImagePath returns the file to serve for an image about width pixels wide.
// It picks the smallest stored width at least that wide, else the largest.
//
// Every stored file was re-encoded by the server, so no provider URL and no
// original file ever reaches a client. If the image is missing a width its
// kind now has, the widest stored width below it is served until the work's
// next refresh encodes it again. If the cache lost the file, ImagePath
// returns ErrNotFound and writes nothing; the work's next refresh rebuilds
// it.
//
// For the ID of an image that Artwork offers as a choice, ImagePath serves
// its preview.
func (s *Service) ImagePath(ctx context.Context, img id.ID, width int) (string, error) {
	row, err := s.q.GetImage(ctx, img)
	if errors.Is(err, sql.ErrNoRows) {
		return s.previewPath(ctx, img)
	}
	if err != nil {
		return "", err
	}
	widths := imageWidths[row.Kind]
	i := len(widths) - 1
	for j, x := range widths {
		if x >= width {
			i = j
			break
		}
	}
	for ; i >= 0; i-- {
		p := s.path(img, widths[i])
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: image %s", ErrNotFound, img)
}

func (s *Service) path(img id.ID, width int) string {
	return cachedir.Path(s.imageCache, img, fmt.Sprintf("%s-%d.webp", img, width))
}

// present reports whether an image's files are all in the cache.
func (s *Service) present(row dbq.Image) bool {
	for _, w := range imageWidths[row.Kind] {
		if _, err := os.Stat(s.path(row.ID, w)); err != nil {
			return false
		}
	}
	return true
}

func (s *Service) encodeLocal(ctx context.Context, kind string, f localFile) (dbq.InsertImageParams, error) {
	data, err := f.read()
	if err != nil {
		return dbq.InsertImageParams{}, err
	}
	p, err := s.encode(ctx, kind, data)
	if err != nil {
		return p, fmt.Errorf("encode %s: %w", f.origin(), err)
	}
	return p, nil
}

func (s *Service) encodeRemote(ctx context.Context, kind string, r remoteImage) (dbq.InsertImageParams, error) {
	data, err := s.fetcher.fetch(ctx, request{provider: r.provider, url: r.url})
	if err != nil {
		return dbq.InsertImageParams{}, err
	}
	p, err := s.encode(ctx, kind, data)
	if err != nil {
		return p, fmt.Errorf("encode %s: %w", r.url, err)
	}
	return p, nil
}

// encodeCover encodes the cover picture embedded in a book's file, as encode
// does for an image file. It reads the picture from the book's file
// itself.
func (s *Service) encodeCover(ctx context.Context, c embeddedCover) (dbq.InsertImageParams, error) {
	if c.width*c.height > maxPixels {
		return dbq.InsertImageParams{}, fmt.Errorf("%w: %dx%d is over %d pixels", ErrBadImage, c.width, c.height, maxPixels)
	}
	if _, err := s.encodeTemp(); err != nil {
		return dbq.InsertImageParams{}, err
	}
	p, err := s.encodeFile(ctx, id.New(), artworkPoster, imageWidths[artworkPoster], c.file.origin())
	if err != nil {
		return p, fmt.Errorf("encode the cover of %s: %w", c.file.origin(), err)
	}
	p.Width, p.Height = int64(c.width), int64(c.height)
	return p, nil
}

// encode checks an image's type and size before anything decodes it. It
// then re-encodes the image through media to WebP at the kind's widths,
// computes its blurhash, and renames the files into the cache under a new
// image ID.
func (s *Service) encode(ctx context.Context, kind string, data []byte) (dbq.InsertImageParams, error) {
	return s.encodeAs(ctx, id.New(), kind, imageWidths[kind], data)
}

// encodeAs encodes an image at the given widths into the cache under img.
func (s *Service) encodeAs(ctx context.Context, img id.ID, kind string, widths []int, data []byte) (dbq.InsertImageParams, error) {
	w, h, err := checkImage(data)
	if err != nil {
		return dbq.InsertImageParams{}, err
	}
	tmp, err := s.encodeTemp()
	if err != nil {
		return dbq.InsertImageParams{}, err
	}
	src := filepath.Join(tmp, id.New().String()+".src") // a unique name, since two requests may encode one preview at once
	if err := os.WriteFile(src, data, 0o600); err != nil {
		return dbq.InsertImageParams{}, err
	}
	defer os.Remove(src)
	p, err := s.encodeFile(ctx, img, kind, widths, src)
	p.Width, p.Height = int64(w), int64(h)
	return p, err
}

// encodeTemp creates cache/images/.tmp, the folder images are encoded in,
// and returns its path.
func (s *Service) encodeTemp() (string, error) {
	tmp := filepath.Join(s.imageCache, cachedir.Temp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", err
	}
	return tmp, nil
}

// encodeFile re-encodes the picture in the file at src to WebP at the given
// widths. src is an image, or a media file with an embedded cover. It
// computes the blurhash and renames the files into the cache under img. It
// encodes in the folder encodeTemp creates, which the caller must already
// have created.
func (s *Service) encodeFile(ctx context.Context, img id.ID, kind string, widths []int, src string) (dbq.InsertImageParams, error) {
	tmp := filepath.Join(s.imageCache, cachedir.Temp)
	name := id.New().String()

	var outs []media.ImageOut
	for _, width := range widths {
		outs = append(outs, media.ImageOut{Width: width, Path: filepath.Join(tmp, fmt.Sprintf("%s-%d.webp", name, width))})
	}
	removeOuts := func() {
		for _, o := range outs {
			os.Remove(o.Path)
		}
	}
	tiny, err := s.deps.Imager.Image(ctx, src, outs)
	if err != nil {
		removeOuts()
		if errors.Is(err, media.ErrSourceCorrupt) || errors.Is(err, media.ErrUnsupported) {
			return dbq.InsertImageParams{}, fmt.Errorf("%w: %w", ErrBadImage, err)
		}
		return dbq.InsertImageParams{}, err
	}
	hash, err := blurhash(tiny)
	if err != nil {
		removeOuts()
		return dbq.InsertImageParams{}, fmt.Errorf("%w: blurhash: %w", ErrBadImage, err)
	}
	for _, o := range outs {
		dst := s.path(img, o.Width)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			removeOuts()
			return dbq.InsertImageParams{}, err
		}
		if err := os.Rename(o.Path, dst); err != nil {
			removeOuts()
			s.deleteFiles(img, kind)
			return dbq.InsertImageParams{}, err
		}
	}
	return dbq.InsertImageParams{ID: img, Kind: kind, Blurhash: hash}, nil
}

// checkImage sniffs an image's type and reads its dimensions from its
// header, without decoding it. It must not decode, because a small file can
// claim enough pixels to exhaust memory.
func checkImage(data []byte) (w, h int, err error) {
	if len(data) > maxImageBytes {
		return 0, 0, fmt.Errorf("%w: over %d bytes", ErrBadImage, maxImageBytes)
	}
	switch typ := http.DetectContentType(data); typ {
	case "image/jpeg", "image/png", "image/gif":
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return 0, 0, fmt.Errorf("%w: %w", ErrBadImage, err)
		}
		w, h = cfg.Width, cfg.Height
	case "image/webp":
		if w, h, err = webpSize(data); err != nil {
			return 0, 0, err
		}
	default:
		return 0, 0, fmt.Errorf("%w: type %s", ErrBadImage, typ) // SVG sniffs as text or XML
	}
	if w <= 0 || h <= 0 || int64(w)*int64(h) > maxPixels {
		return 0, 0, fmt.Errorf("%w: %d×%d pixels", ErrBadImage, w, h)
	}
	return w, h, nil
}

// webpSize reads a WebP's canvas size from its first chunk.
func webpSize(b []byte) (w, h int, err error) {
	bad := fmt.Errorf("%w: truncated WebP header", ErrBadImage)
	if len(b) < 30 {
		return 0, 0, bad
	}
	switch string(b[12:16]) {
	case "VP8X":
		w = int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h = int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1, nil
	case "VP8 ":
		if b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, bad
		}
		return int(binary.LittleEndian.Uint16(b[26:]) & 0x3fff), int(binary.LittleEndian.Uint16(b[28:]) & 0x3fff), nil
	case "VP8L":
		if b[20] != 0x2f {
			return 0, 0, bad
		}
		bits := binary.LittleEndian.Uint32(b[21:])
		return int(bits&0x3fff) + 1, int(bits>>14&0x3fff) + 1, nil
	}
	return 0, 0, bad
}

// deleteImage removes a deleted image's files, and the original of an
// upload.
func (s *Service) deleteImage(row dbq.Image) {
	s.deleteFiles(row.ID, row.Kind)
	if row.Source == sourceUpload {
		if err := os.Remove(s.uploadPath(row.Origin)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("metadata: remove upload", "image", row.ID, "err", err)
		}
	}
}

func (s *Service) deleteFiles(img id.ID, kind string) {
	for _, w := range imageWidths[kind] {
		if err := os.Remove(s.path(img, w)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("metadata: remove image", "image", img, "err", err)
		}
	}
}

// pruneAge is how old a file in cache/images must be before the prune may
// remove it. The delay exists because an encode puts its files in place
// before its row is recorded.
const pruneAge = time.Hour

// pruneImages removes files in cache/images that no image and no offered
// artwork refers to, such as the files of deleted works. It also removes
// whatever an interrupted encode left in .tmp. The uploaded originals in
// data/images are never touched.
func (s *Service) pruneImages(ctx context.Context) (int, error) {
	paths, err := filepath.Glob(filepath.Join(s.imageCache, "*", "*"))
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-pruneAge)
	byImage := map[id.ID][]string{}
	var orphans []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.ModTime().After(cutoff) {
			continue
		}
		name := filepath.Base(p)
		img, err := id.Parse(name[:max(strings.LastIndexByte(name, '-'), 0)])
		if err != nil || filepath.Base(filepath.Dir(p)) == cachedir.Temp {
			orphans = append(orphans, p)
			continue
		}
		byImage[img] = append(byImage[img], p)
	}
	ids := id.Set(slices.Collect(maps.Keys(byImage))...)
	err = s.db.Read(ctx, func(tx *sql.Tx) error {
		q := dbq.New(tx)
		images, err := q.KnownImages(ctx, ids)
		if err != nil {
			return err
		}
		offered, err := q.KnownArtwork(ctx, ids)
		if err != nil {
			return err
		}
		for _, img := range slices.Concat(images, offered) {
			delete(byImage, img)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, ps := range byImage {
		orphans = append(orphans, ps...)
	}
	for _, p := range orphans {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, err
		}
	}
	return len(orphans), nil
}
