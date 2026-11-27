import { useEffect, useEffectEvent } from "react";
import { imageUrl } from "@/api/images";
import type { ImageRef } from "@/api/schemas";

type Metadata = { title: string; artist?: string; album?: string; artwork?: ImageRef };

type Actions = {
  play: () => void;
  pause: () => void;
  seekBy: (deltaMs: number) => void;
  seekTo: (positionMs: number) => void;
  previous?: () => void;
  next?: () => void;
};

type Position = { positionMs: number; durationMs: number; paused: boolean; rate?: number };

const artworkWidth = 512;

/**
 * useMediaSession wires up the Media Session API, so media keys, the lock
 * screen and the OS's media controls show what plays and drive the player.
 * Browsers without the API skip it. The video player and the audio player
 * each set it while they play. A player that goes away clears it only if it
 * is still the one shown.
 */
export function useMediaSession(metadata: Metadata | undefined, actions: Actions, position: Position | undefined) {
  const act = useEffectEvent((action: MediaSessionAction, details: MediaSessionActionDetails) => {
    switch (action) {
      case "play":
        return actions.play();
      case "pause":
        return actions.pause();
      case "seekbackward":
        return actions.seekBy(-(details.seekOffset ?? 10) * 1000);
      case "seekforward":
        return actions.seekBy((details.seekOffset ?? 30) * 1000);
      case "seekto":
        return details.seekTime !== undefined && actions.seekTo(details.seekTime * 1000);
      case "previoustrack":
        return actions.previous?.();
      case "nexttrack":
        return actions.next?.();
    }
  });
  const hasPrevious = actions.previous !== undefined;
  const hasNext = actions.next !== undefined;
  const { title, artist, album, artwork } = metadata ?? {};

  useEffect(() => {
    const session = navigator.mediaSession as MediaSession | undefined;
    if (!session || title === undefined) return;
    const shown = new MediaMetadata({
      title,
      artist,
      album,
      artwork: artwork && [{ src: imageUrl(artwork, artworkWidth) }],
    });
    session.metadata = shown;
    const actionNames: MediaSessionAction[] = ["play", "pause", "seekbackward", "seekforward", "seekto"];
    if (hasPrevious) actionNames.push("previoustrack");
    if (hasNext) actionNames.push("nexttrack");
    for (const action of actionNames) {
      try {
        session.setActionHandler(action, (details) => act(action, details));
      } catch {
        // The browser doesn't support this action.
      }
    }
    return () => {
      // Another player took over meanwhile, so its metadata and handlers stay.
      if (session.metadata !== shown) return;
      session.metadata = null;
      for (const action of actionNames) {
        try {
          session.setActionHandler(action, null);
        } catch {
          // As above.
        }
      }
    };
  }, [title, artist, album, artwork, hasPrevious, hasNext]);

  const { positionMs, durationMs, paused, rate = 1 } = position ?? {};
  useEffect(() => {
    const session = navigator.mediaSession as MediaSession | undefined;
    if (!session || positionMs === undefined || durationMs === undefined) return;
    session.playbackState = paused ? "paused" : "playing";
    if (durationMs > 0) {
      try {
        session.setPositionState({
          duration: durationMs / 1000,
          position: Math.min(positionMs, durationMs) / 1000,
          playbackRate: rate,
        });
      } catch {
        // The position is out of range while a seek settles. The next update fixes it.
      }
    }
  }, [positionMs, durationMs, paused, rate]);
}
