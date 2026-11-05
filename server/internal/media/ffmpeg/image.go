package ffmpeg

import (
	"context"

	"github.com/mmcdole/kivo/internal/media"
)

// Image re-encodes the image at src to WebP at each output's width. It
// returns a PNG 32 pixels wide for the blurhash. The caller checks the
// source's size and type first, and renames the outputs into place.
func (r *Runner) Image(ctx context.Context, src string, outs []media.ImageOut) ([]byte, error) {
	op := "image " + src
	args, err := imageArgs(src, outs)
	if err != nil {
		return nil, refuse(op, err)
	}
	return run(ctx, op, media.ErrSourceCorrupt, r.ffmpeg, args, spawn{background: true})
}
