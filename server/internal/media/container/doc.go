// Package container reads what Kivo needs from media containers in
// process. It reads keyframe indexes from MKV and MP4 metadata, and the
// files an MKV carries besides its tracks, such as fonts for its styled
// subtitles. It detects a font's media type from its first bytes. It also
// splits the fragmented MP4 that ffmpeg writes into segments.
//
// Parsers cap every size they read, so a hostile file costs at most its own
// size in memory. They turn panics into media.ErrSourceCorrupt.
package container
