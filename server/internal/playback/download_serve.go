package playback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/mmcdole/kivo/internal/catalog"
	"github.com/mmcdole/kivo/internal/id"
	"github.com/mmcdole/kivo/internal/playback/dbq"
	"github.com/mmcdole/kivo/internal/users"
)

// File returns a part of one of the user's downloads, as long as
// the user may still have it (see Available). An original's part is
// returned at once. A conversion's part is returned once it's ready, and
// ErrNotReady before then.
func (s *Downloads) File(ctx context.Context, v users.Viewer, download id.ID, part int) (Media, error) {
	d, dl, files, err := s.servable(ctx, v, download)
	if err != nil {
		return Media{}, err
	}
	if part < 0 || part >= len(files) {
		return Media{}, fmt.Errorf("%w: part %d of download %s", ErrNotFound, part, download)
	}
	if dl.Delivery == DeliverFile {
		f := files[part]
		fi, err := os.Stat(f.Path())
		if err != nil {
			return Media{}, err
		}
		return Media{Path: f.Path(), Type: mimeType(f.Probe.Container, f.RelPath), ETag: fileETag(fi)}, nil
	}
	made := dl.Parts[part]
	if DownloadState(d.State) != DownloadReady || made.Path == "" {
		return Media{}, fmt.Errorf("%w: download %s", ErrNotReady, download)
	}
	return Media{Path: made.Path, Type: "video/mp4", ETag: `"` + made.SHA256 + `"`}, nil
}

// Subtitle returns a subtitle of one of the user's downloads by
// the name in its delivery, the same way Service.Subtitle does for a play.
func (s *Downloads) Subtitle(ctx context.Context, v users.Viewer, download id.ID, name string) (Media, error) {
	_, _, files, err := s.servable(ctx, v, download)
	if err != nil {
		return Media{}, err
	}
	return s.subtitleFile(ctx, files[0], name)
}

// servable returns one of the viewer's downloads, what it delivers and its
// files, as long as the user may still have it.
func (s *Downloads) servable(ctx context.Context, v users.Viewer, download id.ID) (dbq.Download, delivered, []catalog.File, error) {
	d, err := s.own(ctx, v.User(), download)
	if err != nil {
		return d, delivered{}, nil, err
	}
	if _, err := s.downloader(ctx, v, v.User(), d.WorkID); err != nil {
		return d, delivered{}, nil, fmt.Errorf("%w: download %s", ErrNotFound, download)
	}
	dl, files, err := s.kept(ctx, d)
	if errors.Is(err, catalog.ErrNotFound) {
		return d, dl, nil, fmt.Errorf("%w: download %s", ErrNotFound, download)
	}
	return d, dl, files, err
}

// fileETag is an original file's strong ETag, built from its current size
// and modification time on disk. A file replaced since it was scanned is
// then fetched again rather than resumed.
func fileETag(fi os.FileInfo) string {
	return `"` + strconv.FormatInt(fi.Size(), 10) + "-" + strconv.FormatInt(fi.ModTime().UnixNano(), 36) + `"`
}

// KeptVersion is a version of a work a client keeps downloaded, for the
// availability check.
type KeptVersion struct{ Work, Version id.ID }

// Available says whether the user may still have each download. The user
// must be allowed downloads, the work must be visible to them, and the
// version must still exist as a version of that work. A missing work and
// a hidden work give the same answer.
func (s *Downloads) Available(ctx context.Context, v users.Viewer, downloads []KeptVersion) ([]bool, error) {
	out := make([]bool, len(downloads))
	user, err := s.deps.Users.User(ctx, v.User())
	if err != nil {
		return nil, err
	}
	if !user.Downloads {
		return out, nil
	}
	for i, d := range downloads {
		ok, err := s.visible(ctx, v, d.Work)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		ver, err := s.deps.Catalog.Version(ctx, d.Version)
		if errors.Is(err, catalog.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[i] = slices.Contains(ver.Works, d.Work)
	}
	return out, nil
}
