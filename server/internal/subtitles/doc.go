// Package subtitles prepares embedded subtitles and keeps downloaded ones.
//
// A file's embedded tracks are extracted ahead of its plays when it has an
// ASS or PGS track, and also when a download needs them. The result lives
// in cache/subtitles/ under the file's ID and generation, so moving the
// file keeps the result, and a changed file never reads a stale one. Each
// font the subtitles use is stored once, by its content, and shared by
// every file that uses it. A finished job is only a hint. The result on
// disk is what counts.
//
// Subtitles found and downloaded from OpenSubtitles through the egress
// client, or obtained elsewhere, are kept in data/subtitles/ as the file's
// downloaded tracks.
//
// The package never serves HTTP. The api and playback packages serve what
// it returns.
package subtitles
