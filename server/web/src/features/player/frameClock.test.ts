import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { frameClock } from "./frameClock";

describe("frameClock", () => {
  let frames: FrameRequestCallback[] = [];
  const advance = (now: number) => {
    const due = frames;
    frames = [];
    for (const f of due) f(now);
  };
  beforeEach(() => {
    frames = [];
    vi.stubGlobal("requestAnimationFrame", (f: FrameRequestCallback) => frames.push(f));
    vi.stubGlobal("cancelAnimationFrame", () => {});
  });
  afterEach(() => vi.unstubAllGlobals());

  const video = (rvfc: boolean) => {
    const v = { currentTime: 12.5, videoWidth: 1920, videoHeight: 1080 } as HTMLVideoElement;
    if (rvfc) (v as { requestVideoFrameCallback?: unknown }).requestVideoFrameCallback = () => 0;
    return v;
  };

  it("draws at the video's time when the browser reports no frames, as Safari's HLS player", () => {
    const draw = vi.fn();
    const stop = frameClock(video(false), draw);
    advance(1000);
    expect(draw).toHaveBeenCalledWith({ mediaTime: 12.5, expectedDisplayTime: 1000, width: 1920, height: 1080 });
    stop();
    advance(1100);
    expect(draw).toHaveBeenCalledTimes(1);
  });

  it("stays out of the way while frame callbacks come", () => {
    const draw = vi.fn();
    let onFrame: VideoFrameRequestCallback | undefined;
    const v = video(true);
    v.requestVideoFrameCallback = (f) => {
      onFrame = f;
      return 0;
    };
    vi.spyOn(performance, "now").mockReturnValue(1000);
    frameClock(v, draw);
    onFrame?.(1000, {} as VideoFrameCallbackMetadata);
    advance(1100);
    expect(draw).not.toHaveBeenCalled();
    advance(1300);
    expect(draw).toHaveBeenCalledTimes(1);
  });
});
