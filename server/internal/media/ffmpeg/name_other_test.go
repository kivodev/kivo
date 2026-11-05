//go:build !windows

package ffmpeg

// escapedName is a file name that needs escaping at both levels of a
// filter graph.
const escapedName = `Don't panic: [part 1], a;b\c.srt`
