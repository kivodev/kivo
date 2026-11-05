# Safari manual checklist

Playwright covers Chromium; Safari plays HLS with its own player, draws a stream's subtitles itself and differs in small ways that break players. Run this on macOS Safari, and the rows marked iOS on an iPhone or iPad, before each release. Use a library with a direct-play MP4 (H.264 and AAC), an MKV with embedded SubRip and ASS tracks and chapters named Intro and Credits, an HEVC 10-bit HDR file, and a file with a PGS track.

## Playback

- [ ] The MP4 plays directly and resumes at its saved position (Safari ignores a seek before metadata loads, so this checks the start offset).
- [ ] The MKV plays as Direct Stream; Stream info says why.
- [ ] A file that needs a video transcode plays; Stream info says Transcode, the encoder and why.
- [ ] A far seek (an hour ahead) plays within a few seconds at the right place, picture and sound in step.
- [ ] Changing quality reloads at the same position, within the chosen cap.
- [ ] Preview thumbnails show while dragging; the seek happens on release.
- [ ] Skip Intro jumps to the intro's end; Skip Credits or Up Next shows at the credits.
- [ ] Picture in Picture, fullscreen, AirPlay and the keyboard's media keys work.
- [ ] Leaving the player stops the play (it's gone from the admin Now playing page within seconds).

## Subtitles

- [ ] An embedded SubRip track chosen before playing shows from the first seconds, through Safari's own HLS subtitle rendition, timed to the speech.
- [ ] After a far seek the same track keeps showing, still timed to the speech.
- [ ] Switching to another embedded text track that's extracted shows it at once, in the same play; one that isn't reloads at the same position with it.
- [ ] Off hides every track.
- [ ] An ASS track is drawn styled (positions, colours, fonts) over the video.
- [ ] A PGS track is drawn over the video, sized to the picture.
- [ ] iOS: ASS and PGS are burned in, and show in fullscreen.
- [ ] iOS: the streamed SubRip track shows in fullscreen.

## Admin

- [ ] Now playing shows the play with each track's method and reason, and Stop stream ends it on the playing client with "Playback was stopped."
- [ ] Settings › Transcoding lists the accelerators under Hardware acceleration, and any that can't be used, with its fix.
