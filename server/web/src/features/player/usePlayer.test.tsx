import { act, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { PlayerAction } from "./reducer";
import { usePlayer } from "./usePlayer";

// The controller drives the player's state through dispatch, which the test does in its place.
let dispatch: ((action: PlayerAction) => void) | undefined;
vi.mock("./controller", () => ({
  PlayerController: class {
    constructor(_media: unknown, d: (action: PlayerAction) => void) {
      dispatch = d;
    }
    play = vi.fn(async () => {});
    dispose = vi.fn();
  },
}));

afterEach(() => {
  vi.unstubAllGlobals();
  dispatch = undefined;
});

function Player({ workId }: { workId: string }) {
  const { videoRef } = usePlayer(workId, { from: "start" }, () => {});
  // biome-ignore lint/a11y/useMediaCaption: a test's player has nothing to caption.
  return <video ref={videoRef} />;
}

const send = (action: PlayerAction) => act(() => dispatch?.(action));

it("keeps the TV's screensaver off while the video plays, and lets it come back when paused or gone", () => {
  const appcommon = {
    setScreenSaver: vi.fn(),
    AppCommonScreenSaverState: { SCREEN_SAVER_OFF: 0, SCREEN_SAVER_ON: 1 },
  };
  vi.stubGlobal("webapis", { appcommon });
  const view = render(<Player workId="a" />);
  // Loading, nothing plays yet.
  expect(appcommon.setScreenSaver).not.toHaveBeenCalled();
  send({ type: "start" });
  send({ type: "ready" });
  send({ type: "play" });
  expect(appcommon.setScreenSaver.mock.calls).toEqual([[0]]);
  send({ type: "pause" });
  expect(appcommon.setScreenSaver.mock.calls).toEqual([[0], [1]]);
  send({ type: "play" });
  expect(appcommon.setScreenSaver.mock.calls).toEqual([[0], [1], [0]]);
  view.unmount();
  expect(appcommon.setScreenSaver).toHaveBeenLastCalledWith(1);
});
