import { useEffect, useEffectEvent, useReducer, useRef } from "react";
import { deviceSettings } from "@/lib/deviceSettings";
import { isTv } from "@/tv/flag";
import { keepScreenOn } from "@/tv/tizen";
import { PlayerController } from "./controller";
import { initialPlayerState, isLive, playerReducer } from "./reducer";
import type { WatchSearch } from "./watchSearch";

/**
 * usePlayer holds the player's state and its controller for what the route
 * asks to play. The play uses this browser's home and remote quality, which
 * the server picks between, and this browser's dialogue boost. A different
 * work, version or track choice in the URL tears the old play down and
 * starts again. Switches inside the player go through `run`.
 */
export function usePlayer(workId: string, search: WatchSearch, onEnded: () => void) {
  const [state, dispatch] = useReducer(playerReducer, initialPlayerState);
  const videoRef = useRef<HTMLVideoElement>(null);
  const controllerRef = useRef<PlayerController | null>(null);
  const ended = useEffectEvent(onEnded);
  const { version: versionId, audio: audioTrackId, subtitle: subtitleTrackId } = search;
  const positionMs = search.from === "start" ? 0 : search.at;

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const controller = new PlayerController(video, dispatch, () => ended());
    controllerRef.current = controller;
    const { localQuality, remoteQuality, dialogueBoost } = deviceSettings();
    void controller.play({
      workId,
      localQuality: localQuality ?? { mode: "auto" },
      remoteQuality: remoteQuality ?? { mode: "auto" },
      dialogueBoost,
      versionId,
      audioTrackId,
      subtitleTrackId,
      positionMs,
    });
    // Leaving the page stops the play with a request that outlives the page.
    // A page restored from the back/forward cache starts again where it was.
    const onPageHide = () => controller.stopOnExit();
    const onPageShow = (e: PageTransitionEvent) => e.persisted && controller.restore();
    // A TV hides the page without leaving it for standby, its Home key or
    // another input, and may freeze it there for good. Stopping then saves
    // the position before the play expires unheard, which would show it
    // ended elsewhere on return. A browser tab keeps playing hidden, so
    // only the TV build stops.
    const onVisibility = () =>
      document.visibilityState === "hidden" ? controller.suspend() : controller.resumeFromSuspend();
    window.addEventListener("pagehide", onPageHide);
    window.addEventListener("pageshow", onPageShow);
    if (isTv) document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("pagehide", onPageHide);
      window.removeEventListener("pageshow", onPageShow);
      if (isTv) document.removeEventListener("visibilitychange", onVisibility);
      controller.dispose();
      controllerRef.current = null;
      dispatch({ type: "reset" });
    };
  }, [workId, versionId, audioTrackId, subtitleTrackId, positionMs]);

  // A TV's screensaver would come on partway through, since nobody presses
  // the remote while watching, so it's kept off while the video plays. It
  // comes back when paused, so a paused picture doesn't stay on the panel.
  const watching = isLive(state.status) && !state.paused;
  useEffect(() => {
    if (!watching) return;
    keepScreenOn(true);
    return () => keepScreenOn(false);
  }, [watching]);

  /** Runs a command on the current controller, from an event handler. */
  const run = (command: (controller: PlayerController) => void) => {
    const controller = controllerRef.current;
    if (controller) command(controller);
  };

  return { state, videoRef, run };
}
