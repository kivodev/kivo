import { type RefObject, useEffect, useState } from "react";

/** A video in Safari on the iPhone, which can only put a video full screen, in its own player. */
type IPhoneVideo = HTMLVideoElement & { webkitEnterFullscreen?: () => void };

/**
 * useFullscreen returns whether the page is full screen, and the toggle. The
 * whole page goes full screen, so the menus' popovers show over the video.
 * Leaving the player leaves full screen. Where only a video can go full
 * screen, the video does, with the browser's own controls.
 */
export function useFullscreen(videoRef: RefObject<HTMLVideoElement | null>) {
  const [fullscreen, setFullscreen] = useState(false);
  useEffect(() => {
    const onChange = () => setFullscreen(document.fullscreenElement !== null);
    document.addEventListener("fullscreenchange", onChange);
    return () => {
      document.removeEventListener("fullscreenchange", onChange);
      if (document.fullscreenElement) void document.exitFullscreen();
    };
  }, []);
  const toggle = () => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else if (document.documentElement.requestFullscreen) void document.documentElement.requestFullscreen();
    else (videoRef.current as IPhoneVideo | null)?.webkitEnterFullscreen?.();
  };
  return { fullscreen, toggle };
}

/** Puts the video in picture in picture, or takes it out. */
export function togglePictureInPicture(videoRef: RefObject<HTMLVideoElement | null>) {
  const video = videoRef.current;
  if (!video) return;
  void (document.pictureInPictureElement ? document.exitPictureInPicture() : video.requestPictureInPicture());
}
