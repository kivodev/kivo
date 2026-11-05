import { type RefObject, useEffect, useState } from "react";
import { subtitlesOff } from "@/api/playback";
import type { PlayStarted, PlayTrack, Version } from "@/api/schemas";
import type { PlayerController } from "./controller";
import { isMediaTrack } from "./engine";

/** A choice made during one play: a track's ID, or `subtitlesOff`. */
type Choice = { play: PlayStarted; trackId: string };

/**
 * useSubtitleChoice tracks the subtitle the player shows: the one picked
 * during this play, or else the play's own. The play's tracks say how each
 * choice shows. One without a delivery calls `restart`, for a new play at
 * the same position that shows it. Any other shows as its delivery says:
 * from the media (a stream rendition or the file's own track), from its own
 * file as a text track of the video or on the renderers' canvas, burned in,
 * or not at all. The controller reports such a choice in its heartbeats.
 */
export function useSubtitleChoice(
  play: PlayStarted | undefined,
  version: Version | undefined,
  videoRef: RefObject<HTMLVideoElement | null>,
  run: (command: (controller: PlayerController) => void) => void,
  restart: (trackId: string) => void,
) {
  const [choice, setChoice] = useState<Choice | undefined>();
  const subtitles = play?.tracks.filter((t) => t.type === "subtitle") ?? [];
  const picked = choice?.play === play ? choice?.trackId : undefined;
  const subtitle = picked ?? subtitles.find((t) => t.selected)?.trackId ?? subtitlesOff;
  const shown = subtitles.find((t) => t.trackId === subtitle);
  const textTrackUrl = shown?.delivery === "external" && !canvasRenderer(shown) ? shown.url : undefined;

  const fromMedia = shown?.delivery === "stream" || shown?.delivery === "file" ? shown.trackId : undefined;
  useEffect(() => run((c) => c.showMediaSubtitle(fromMedia)), [run, fromMedia]);
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    for (const track of Array.from(video.textTracks)) {
      if (!isMediaTrack(track)) track.mode = textTrackUrl ? "showing" : "disabled";
    }
  }, [videoRef, textTrackUrl]);

  return {
    /** The chosen track's ID, or `subtitlesOff`. */
    subtitle,
    /** How the chosen subtitle shows, as the play's tracks say. */
    shown,
    textTrackUrl,
    /** The chosen track, when it is one of the version's own. */
    track: version?.tracks.find((tr) => tr.type === "subtitle" && tr.id === subtitle),
    choose: (trackId: string) => {
      if (!play) return;
      setChoice({ play, trackId });
      if (subtitles.find((t) => t.trackId === trackId)?.delivery) run((c) => c.showSubtitle(trackId));
      else restart(trackId);
    },
    /** Records a track that the caller starts a new play for itself. */
    pick: (trackId: string) => play && setChoice({ play, trackId }),
  };
}

/** The renderer that draws a subtitle delivered external over the video, for the codecs a text track can't show. */
export function canvasRenderer(track: PlayTrack | undefined): "ass" | "pgs" | undefined {
  if (track?.delivery !== "external") return undefined;
  if (track.codec === "ass") return "ass";
  return track.codec === "hdmv_pgs_subtitle" ? "pgs" : undefined;
}
