// What the LG TV app (clients/webos) asks of webOS. Its page has the
// PalmSystem global, which a browser, a Samsung TV and tests don't, and
// everything here does nothing without it. Leaving the app needs nothing of
// webOS: closing the window ends it (exit.ts). The remote's Back and media
// keys arrive unasked (features/player/keys.ts), and webOS keeps its
// screensaver off while a video element plays.

type WebosGlobals = { PalmSystem?: object };

/**
 * Whether the TV's panel shows HDR, by the display's dynamic range, which
 * the capability report declares HDR by. Undefined off an LG TV, or where
 * the display can't be asked.
 */
export function webosHdrPanel(): boolean | undefined {
  if (!(window as unknown as WebosGlobals).PalmSystem) return undefined;
  try {
    return matchMedia("(dynamic-range: high)").matches;
  } catch {
    return undefined;
  }
}
