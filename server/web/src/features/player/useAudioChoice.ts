import { useState } from "react";
import type { PlayStarted } from "@/api/schemas";
import type { PlayerController } from "./controller";

/**
 * useAudioChoice tracks the audio track the player plays: the one picked
 * during this play, or else the play's own. A track whose delivery is
 * `file` plays from the file at once, and the controller reports it in its
 * heartbeats. Any other calls `restart`, for a new play at the same
 * position with that track, since a stream carries one audio track.
 */
export function useAudioChoice(
  play: PlayStarted | undefined,
  run: (command: (controller: PlayerController) => void) => void,
  restart: (trackId: string) => void,
) {
  const [choice, setChoice] = useState<{ play: PlayStarted; trackId: string } | undefined>();
  const tracks = play?.tracks.filter((t) => t.type === "audio") ?? [];
  const picked = choice?.play === play ? choice?.trackId : undefined;
  const audioTrackId = picked ?? tracks.find((t) => t.selected)?.trackId;
  return {
    /** The chosen audio track's ID; undefined when the play has none. */
    audioTrackId,
    choose: (trackId: string) => {
      if (!play || trackId === audioTrackId) return;
      setChoice({ play, trackId });
      if (tracks.find((t) => t.trackId === trackId)?.delivery === "file") run((c) => c.selectAudio(trackId));
      else restart(trackId);
    },
  };
}
