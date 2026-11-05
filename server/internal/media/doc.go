// Package media declares the typed requests, results and errors for media
// work: probing files, reading keyframe indexes, re-encoding artwork, the
// transcode job that writes fragmented MP4 segments, and the conversion that
// writes one MP4 file. It knows nothing about users, sessions or the
// database, and never chooses an accelerator.
//
// The ffmpeg subpackage carries out these requests by running ffmpeg and
// ffprobe as child processes. The container and subtitle subpackages read
// containers and subtitle files in process. No ffmpeg flag, encoder name or
// filter string appears in this package.
package media
