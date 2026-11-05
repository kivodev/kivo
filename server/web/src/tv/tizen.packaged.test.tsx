import { afterEach, expect, it, vi } from "vitest";
import { fullHdPanel, hdrPanel, keepScreenOn, mediaKeys, registerMediaKeys } from "./tizen";

afterEach(() => {
  vi.unstubAllGlobals();
});

function fakeInput(lacking: string[] = []) {
  const held = new Set<string>();
  const input = {
    registerKey: vi.fn((key: string) => {
      if (lacking.includes(key)) throw new Error("InvalidValuesError");
      held.add(key);
    }),
  };
  vi.stubGlobal("tizen", { tvinputdevice: input });
  return held;
}

it("registers the media keys, the track keys among them", () => {
  const held = fakeInput();
  registerMediaKeys();
  expect([...held]).toEqual(mediaKeys);
  expect(held.has("MediaTrackPrevious")).toBe(true);
  expect(held.has("MediaTrackNext")).toBe(true);
});

it("skips a key the remote lacks", () => {
  const held = fakeInput(["Info"]);
  registerMediaKeys();
  expect(held.has("Info")).toBe(false);
  expect(held.has("MediaPlayPause")).toBe(true);
});

it("does nothing without Tizen", () => {
  vi.stubGlobal("tizen", undefined);
  expect(() => registerMediaKeys()).not.toThrow();
});

it("says a panel is 1080p from productinfo", () => {
  const productinfo = { isUdPanelSupported: vi.fn(() => false) };
  vi.stubGlobal("webapis", { productinfo });
  expect(fullHdPanel()).toBe(true);
  productinfo.isUdPanelSupported.mockReturnValue(true);
  expect(fullHdPanel()).toBe(false);
});

it("says no without productinfo, or when it fails", () => {
  vi.stubGlobal("webapis", undefined);
  expect(fullHdPanel()).toBe(false);
  vi.stubGlobal("webapis", {
    productinfo: {
      isUdPanelSupported: () => {
        throw new Error("SecurityError");
      },
    },
  });
  expect(fullHdPanel()).toBe(false);
});

it("says whether the panel shows HDR from avinfo, and nothing without it or when it fails", () => {
  const avinfo = { isHdrTvSupport: vi.fn(() => true) };
  vi.stubGlobal("webapis", { avinfo });
  expect(hdrPanel()).toBe(true);
  avinfo.isHdrTvSupport.mockReturnValue(false);
  expect(hdrPanel()).toBe(false);
  avinfo.isHdrTvSupport.mockImplementation(() => {
    throw new Error("NotSupportedError");
  });
  expect(hdrPanel()).toBeUndefined();
  vi.stubGlobal("webapis", undefined);
  expect(hdrPanel()).toBeUndefined();
});

it("turns the screensaver off to keep the screen on, and back on after", () => {
  const appcommon = {
    setScreenSaver: vi.fn(),
    AppCommonScreenSaverState: { SCREEN_SAVER_OFF: 0, SCREEN_SAVER_ON: 1 },
  };
  vi.stubGlobal("webapis", { appcommon });
  keepScreenOn(true);
  keepScreenOn(false);
  expect(appcommon.setScreenSaver.mock.calls).toEqual([[0], [1]]);
  appcommon.setScreenSaver.mockImplementation(() => {
    throw new Error("TypeMismatchError");
  });
  expect(() => keepScreenOn(true)).not.toThrow();
  vi.stubGlobal("webapis", undefined);
  expect(() => keepScreenOn(true)).not.toThrow();
});
