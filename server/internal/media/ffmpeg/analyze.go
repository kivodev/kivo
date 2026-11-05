package ffmpeg

import (
	"context"
	"os"
	"path/filepath"

	"github.com/mmcdole/kivo/internal/media"
)

// Subtitles extracts the request's tracks in one run of ffmpeg. A failure
// that ffmpeg doesn't name is reported as unreadable, so the caller tries
// again.
func (r *Runner) Subtitles(ctx context.Context, req media.SubtitlesRequest) error {
	op := "extract subtitles " + req.Path
	if len(req.Tracks) == 0 {
		return nil
	}
	args, err := subtitlesArgs(req)
	if err != nil {
		return refuse(op, err)
	}
	_, err = run(ctx, op, media.ErrSourceUnreadable, r.ffmpeg, args, spawn{background: true})
	return err
}

// Previews writes the request's sheets and describes them. A failure that
// ffmpeg doesn't name is reported as unreadable, so the caller tries again.
// A video with no frame to show is reported as corrupt.
func (r *Runner) Previews(ctx context.Context, req media.PreviewsRequest) (media.Previews, error) {
	op := "make previews " + req.Path
	src := req.Source
	switch {
	case src.Width <= 0 || src.Height <= 0:
		return media.Previews{}, refuse(op, unsupported("the source's size is unknown"))
	case req.Interval <= 0 || req.Width <= 0 || req.Columns <= 0 || req.Rows <= 0 || req.Dir == "":
		return media.Previews{}, refuse(op, unsupported("incomplete request"))
	}
	w, h := fit(src.Width, src.Height, req.Width, 0)
	if _, err := run(ctx, op, media.ErrSourceUnreadable, r.ffmpeg, previewsArgs(req, w, h), spawn{background: true}); err != nil {
		return media.Previews{}, err
	}
	p := media.Previews{Interval: req.Interval, Width: w, Height: h, Columns: req.Columns, Rows: req.Rows}
	for {
		if _, err := os.Stat(filepath.Join(req.Dir, media.SheetName(p.Sheets))); err != nil {
			break
		}
		p.Sheets++
	}
	if p.Sheets == 0 {
		return p, &media.Error{Op: op, Kind: media.ErrSourceCorrupt, Detail: "no frames"}
	}
	return p, nil
}
