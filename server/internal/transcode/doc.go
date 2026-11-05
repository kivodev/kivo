// Package transcode writes playlists, runs one ffmpeg job per play's
// stream, handles seeks, paces jobs, stores each stream's segments, and
// runs download conversions. It never reads users, sessions or plays, and
// never chooses an accelerator.
package transcode
