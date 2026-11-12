package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/transcode"
)

// authorize checks a media request. The play must exist and not be
// expired, the session or API key that started it must still be valid, and
// its owner, rebuilt from current permissions, must still see the work.
// So signing the session out, revoking the key, disabling the owner, or
// removing a grant takes effect at the next request.
func (s *Service) authorize(ctx context.Context, sid id.ID) (*play, error) {
	s.mu.Lock()
	pl := s.plays[sid]
	if pl == nil || s.expiredLocked(pl, time.Now()) {
		s.mu.Unlock()
		return nil, ErrNoPlay
	}
	s.mu.Unlock()
	if alive, err := s.deps.Auth.Alive(ctx, pl.caller); err != nil || !alive {
		return nil, ErrNoPlay
	}
	v, err := s.deps.Users.Viewer(ctx, pl.user)
	if err != nil {
		return nil, ErrNoPlay
	}
	if ok, err := s.visible(ctx, v, pl.work); err != nil || !ok {
		return nil, ErrNoPlay
	}
	s.mu.Lock()
	pl.seen = time.Now()
	s.mu.Unlock()
	return pl, nil
}

// Media is the answer to a media request. It's either a file to serve
// with range requests or bytes made for the request, with a MIME type.
type Media struct {
	Path string // the file to serve from disk, when set
	Data []byte // the bytes to serve when Path is empty
	Type string
	// ETag is a download part's strong ETag, so a transfer can resume by
	// range. It's empty for a play's media.
	ETag string
}

// File returns a direct play's part, for api to serve with range requests.
func (s *Service) File(ctx context.Context, sid id.ID, part int) (Media, error) {
	pl, err := s.authorize(ctx, sid)
	if err != nil {
		return Media{}, err
	}
	if part < 0 || part >= len(pl.files) {
		return Media{}, fmt.Errorf("%w: part %d of play %s", ErrNotFound, part, sid)
	}
	f := pl.files[part]
	return Media{Path: f.Path(), Type: mimeType(f.Probe.Container, f.RelPath)}, nil
}

// mimeType is a file's MIME type by its probed container. ffprobe names
// MKV and WebM alike, and MP4 video and audio alike, so the extension tells
// them apart.
func mimeType(container, name string) string {
	first, _, _ := strings.Cut(container, ",")
	ext := strings.ToLower(filepath.Ext(name))
	switch first {
	case "matroska":
		if ext == ".webm" {
			return "video/webm"
		}
		return "video/x-matroska"
	case "mov":
		if ext == ".m4b" || ext == ".m4a" {
			return "audio/mp4"
		}
		return "video/mp4"
	case "mp3":
		return "audio/mpeg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	case "mpegts":
		return "video/mp2t"
	case "avi":
		return "video/x-msvideo"
	}
	return "application/octet-stream"
}

// HLS returns an HLS play's playlist, for a name ending in .m3u8, or the
// path of a segment once it's ready. A failed transcode returns
// ErrJobFailed, and the client answers that with Fallback.
func (s *Service) HLS(ctx context.Context, sid id.ID, name string) (Media, error) {
	pl, err := s.authorize(ctx, sid)
	if err != nil {
		return Media{}, err
	}
	if pl.tc == nil {
		return Media{}, fmt.Errorf("%w: hls %s", ErrNotFound, name)
	}
	if strings.HasSuffix(name, ".m3u8") {
		b, err := pl.tc.Playlist(name)
		if errors.Is(err, transcode.ErrNotFound) {
			return Media{}, fmt.Errorf("%w: hls %s", ErrNotFound, name)
		}
		if err != nil {
			return Media{}, err
		}
		return Media{Data: b, Type: "application/vnd.apple.mpegurl"}, nil
	}
	p, err := pl.tc.Segment(ctx, name)
	switch {
	case errors.Is(err, transcode.ErrNotFound):
		return Media{}, fmt.Errorf("%w: hls %s", ErrNotFound, name)
	case errors.Is(err, transcode.ErrClosed):
		return Media{}, fmt.Errorf("%w: %s", ErrNoPlay, sid)
	case errors.Is(err, transcode.ErrJobFailed), errors.Is(err, transcode.ErrAcceleratorFailed):
		return Media{}, fmt.Errorf("%w: %w", ErrJobFailed, err)
	case err != nil:
		return Media{}, err
	}
	typ := "video/mp4"
	if filepath.Ext(p) == ".vtt" {
		typ = "text/vtt; charset=utf-8"
	}
	return Media{Path: p, Type: typ}, nil
}

// PreviewManifest is the media name of a play's scrub-preview manifest,
// which names the sheets.
const PreviewManifest = "manifest.json"

// manifest is a play's scrub-preview manifest. It gives the interval
// between frames, a frame's size, a sheet's grid, and the sheets' names
// relative to the manifest.
type manifest struct {
	Columns    int      `json:"columns"`
	Height     int      `json:"height"`
	IntervalMs int      `json:"intervalMs"`
	Rows       int      `json:"rows"`
	Sheets     []string `json:"sheets"`
	Width      int      `json:"width"`
}

// Preview returns a play's scrub-preview manifest (PreviewManifest) or one
// of its sheets, once they're made. Only single-file versions have
// previews. Previews that aren't made yet return ErrNotFound. found is
// false for a name that's neither the manifest nor a sheet.
func (s *Service) Preview(ctx context.Context, sid id.ID, name string) (m Media, found bool, err error) {
	pl, err := s.authorize(ctx, sid)
	if err != nil {
		return Media{}, false, err
	}
	if len(pl.files) != 1 {
		return Media{}, false, fmt.Errorf("%w: preview %s", ErrNotFound, name)
	}
	p, dir, ok := s.deps.Previews.Previews(ctx, pl.files[0].ID)
	if !ok {
		return Media{}, false, fmt.Errorf("%w: preview %s", ErrNotFound, name)
	}
	if name == PreviewManifest {
		b, err := json.Marshal(manifest{
			Columns: p.Columns, Height: p.Height, IntervalMs: int(p.Interval.Milliseconds()), Rows: p.Rows, Sheets: p.SheetNames(), Width: p.Width,
		})
		if err != nil {
			return Media{}, false, err
		}
		return Media{Data: b, Type: "application/json"}, true, nil
	}
	if !slices.Contains(p.SheetNames(), name) {
		return Media{}, false, nil
	}
	return Media{Path: filepath.Join(dir, name), Type: "image/webp"}, true, nil
}
