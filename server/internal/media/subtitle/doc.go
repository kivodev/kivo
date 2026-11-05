// Package subtitle converts text subtitle files to WebVTT in
// process, so a subtitle file next to the media is served without running
// ffmpeg. It also reads the SubRip a transcode writes as it goes, and
// writes the WebVTT segments of an HLS subtitle rendition.
package subtitle
