package container

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/mmcdole/kivo/internal/media"
)

// maxKeyframes bounds one index. Three hours of all-intra video at 60 fps
// is about 650,000 frames. Past this bound, the index is treated as absent.
const maxKeyframes = 1 << 20

var (
	ebmlMagic  = []byte{0x1a, 0x45, 0xdf, 0xa3}
	errTooMany = errors.New("too many keyframes")
)

// Keyframes reads the video keyframe index from an MKV or MP4 file's
// metadata, without reading the media data itself. The keyframes are sorted
// by time. complete reports whether the index lists every keyframe of the
// first video track. It is true for MKV Cues, for MP4 sync-sample tables,
// and for MP4 fragment flags read to the end of the file. A file with no
// index or no video, or in another container, returns no keyframes and
// false, so the caller can index the file another way.
func Keyframes(r io.ReaderAt, size int64) (keys []media.Keyframe, complete bool, err error) {
	defer func() {
		if p := recover(); p != nil {
			keys, complete, err = nil, false, corrupt("%v", p)
		}
	}()
	if size < 8 {
		return nil, false, nil
	}
	magic, err := readAt(r, 0, 8)
	if err != nil {
		return nil, false, err
	}
	var times []time.Duration
	switch {
	case bytes.Equal(magic[:4], ebmlMagic):
		times, complete, err = mkvKeyframes(r, size)
	case isMP4(magic[4:]):
		times, complete, err = mp4Keyframes(r, size)
	default:
		return nil, false, nil
	}
	if errors.Is(err, errTooMany) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	slices.Sort(times)
	times = slices.Compact(times)
	keys = make([]media.Keyframe, len(times))
	for i, t := range times {
		keys[i] = media.Keyframe{Time: t}
	}
	return keys, complete && len(keys) > 0, nil
}

// readAt reads n bytes at off. A short file is reported as
// media.ErrSourceCorrupt, and any other failure as unreadable.
func readAt(r io.ReaderAt, off, n int64) ([]byte, error) {
	b := make([]byte, n)
	if _, err := r.ReadAt(b, off); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, corrupt("truncated at %d", off)
		}
		return nil, unreadable(err)
	}
	return b, nil
}

// unreadable maps a read failure to media.ErrSourceUnreadable.
func unreadable(err error) error {
	if err == nil {
		err = io.ErrNoProgress
	}
	return fmt.Errorf("%w: %w", media.ErrSourceUnreadable, err)
}

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{media.ErrSourceCorrupt}, args...)...)
}

// ticks converts v units of 1/scale seconds to a duration without
// overflowing on large values.
func ticks(v int64, scale uint32) time.Duration {
	s := int64(scale)
	return time.Duration(v/s*int64(time.Second) + v%s*int64(time.Second)/s)
}
