package transcode

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mmcdole/kivo/internal/media"
	"github.com/mmcdole/kivo/internal/media/container"
)

// KeyframeSource gives a file's keyframes, sorted, and whether the list is
// complete. Only a remux reads them, because encoded video puts its
// keyframes on the grid. cmd/kivo wires in ContainerIndex.
type KeyframeSource interface {
	Keyframes(ctx context.Context, path string) ([]media.Keyframe, bool, error)
}

// ContainerIndex reads a file's keyframes from its container's own index:
// MKV Cues, or MP4 sync samples and fragments. It never reads the media
// data, so it's fast enough to run as each stream starts. A file with no
// index has no keyframes, so its video is encoded rather than copied.
type ContainerIndex struct{}

// Keyframes implements KeyframeSource.
func (ContainerIndex) Keyframes(_ context.Context, path string) ([]media.Keyframe, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", media.ErrSourceUnreadable, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", media.ErrSourceUnreadable, err)
	}
	return container.Keyframes(f, fi.Size())
}

// RemuxKeyframes returns a file's keyframes when they're close enough
// together to cut copied video into segments, and nil otherwise. The
// playback package decides by the result, and passes the keyframes back
// in the FileRef of a spec that copies video.
func (t *Transcoder) RemuxKeyframes(ctx context.Context, f FileRef) []media.Keyframe {
	keys, complete, err := t.deps.Keys.Keyframes(ctx, f.Path)
	if err != nil || !remuxable(f.Duration, keys, complete) {
		return nil
	}
	return keys
}

// remuxable reports whether keyframes are dense enough to remux. The index
// must be complete, and every gap, including the gap to the end, must be
// within maxKeyframeGap.
func remuxable(duration time.Duration, keys []media.Keyframe, complete bool) bool {
	if !complete || len(keys) == 0 {
		return false
	}
	prev := time.Duration(0)
	for _, k := range keys {
		if k.Time-prev > maxKeyframeGap {
			return false
		}
		prev = max(prev, k.Time)
	}
	return duration-prev <= maxKeyframeGap
}
