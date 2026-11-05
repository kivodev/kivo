// Safari's own HLS player never calls requestVideoFrameCallback, which
// JASSUB draws on, so a renderer there would never draw. frameClock stands
// in. While no frame callback has come for a while, it hands the renderer
// the video's current time and size on each animation frame.

/** What a renderer needs to draw the frame on screen. */
export type Frame = { mediaTime: number; expectedDisplayTime: number; width: number; height: number };

/** How long without a frame callback before the clock takes over. */
const quietMs = 250;

/** Calls draw for each frame the browser doesn't report itself. It returns a function that stops it. */
export function frameClock(video: HTMLVideoElement, draw: (frame: Frame) => void): () => void {
  let lastFrame = 0;
  let stopped = false;
  let raf = 0;
  const onFrame = () => {
    if (stopped) return;
    lastFrame = performance.now();
    video.requestVideoFrameCallback(onFrame);
  };
  if ("requestVideoFrameCallback" in video) video.requestVideoFrameCallback(onFrame);
  const tick = (now: number) => {
    if (stopped) return;
    if (now - lastFrame > quietMs && video.videoWidth > 0) {
      draw({
        mediaTime: video.currentTime,
        expectedDisplayTime: now,
        width: video.videoWidth,
        height: video.videoHeight,
      });
    }
    raf = requestAnimationFrame(tick);
  };
  raf = requestAnimationFrame(tick);
  return () => {
    stopped = true;
    cancelAnimationFrame(raf);
  };
}
