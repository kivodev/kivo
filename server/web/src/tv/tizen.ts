// The Tizen APIs the packaged TV app uses. The packaged page
// has the tizen and webapis globals; the TV flag's dev server, a browser and
// tests have neither, and everything here does nothing without them. Exit is
// in exit.ts.

/** AVPlay's states (webapis.avplay.getState). */
export type AvplayState = "NONE" | "IDLE" | "READY" | "PLAYING" | "PAUSED";

/** One of the media's tracks as AVPlay lists them. extra_info is JSON. */
export type AvplayTrack = { index: number; type: "VIDEO" | "AUDIO" | "TEXT"; extra_info: string };

export type AvplayListener = {
  onbufferingstart?: () => void;
  onbufferingprogress?: (percent: number) => void;
  onbufferingcomplete?: () => void;
  oncurrentplaytime?: (ms: number) => void;
  onstreamcompleted?: () => void;
  onevent?: (event: string, data: string) => void;
  onerror?: (error: string) => void;
  onerrormsg?: (error: string, message: string) => void;
  onsubtitlechange?: (durationMs: string, text: string, type: string) => void;
  onresourceconflicted?: () => void;
};

/**
 * Samsung's native player (webapis.avplay), which decodes on the TV's own
 * pipeline and draws on a plane under the page. Only the calls Kivo makes.
 */
export type Avplay = {
  open(url: string): void;
  close(): void;
  prepareAsync(success?: () => void, error?: (e: unknown) => void): void;
  setDisplayRect(x: number, y: number, width: number, height: number): void;
  setDisplayMethod(mode: "PLAYER_DISPLAY_MODE_LETTER_BOX" | "PLAYER_DISPLAY_MODE_FULL_SCREEN"): void;
  play(): void;
  pause(): void;
  stop(): void;
  seekTo(ms: number, success?: () => void, error?: (e: unknown) => void): void;
  getCurrentTime(): number;
  getDuration(): number;
  getState(): AvplayState;
  setListener(listener: AvplayListener): void;
  getTotalTrackInfo(): AvplayTrack[];
  setSelectTrack(type: "AUDIO" | "TEXT", index: number): void;
  setSilentSubtitle(silent: boolean): void;
  setBufferingParam(
    option: "PLAYER_BUFFER_FOR_PLAY" | "PLAYER_BUFFER_FOR_RESUME",
    unit: "PLAYER_BUFFER_SIZE_IN_SECOND",
    seconds: number,
  ): void;
  setTimeoutForBuffering(seconds: number): void;
  getStreamingProperty(property: "CURRENT_BANDWIDTH"): string;
};

type TizenGlobals = {
  tizen?: {
    tvinputdevice?: { registerKey(name: string): void };
  };
  webapis?: {
    productinfo?: { isUdPanelSupported(): boolean };
    avplay?: Avplay;
    avinfo?: { isHdrTvSupport(): boolean };
    appcommon?: {
      setScreenSaver(state: number, onsuccess?: () => void, onerror?: (e: unknown) => void): void;
      AppCommonScreenSaverState: { SCREEN_SAVER_OFF: number; SCREEN_SAVER_ON: number };
    };
  };
};

/**
 * Samsung's native player, on the packaged app's page with the webapis
 * script, or undefined anywhere else.
 */
export function avplay(): Avplay | undefined {
  return (window as unknown as TizenGlobals).webapis?.avplay;
}

/**
 * The remote's keys the app takes, by Tizen's names. Back and the arrows
 * arrive unasked; these reach the page only once registered.
 */
export const mediaKeys = [
  "MediaPlayPause",
  "MediaPlay",
  "MediaPause",
  "MediaStop",
  "MediaFastForward",
  "MediaRewind",
  "MediaTrackPrevious",
  "MediaTrackNext",
  "Info",
];

/**
 * Registers the media keys with the TV, so their presses reach the page
 * rather than the TV, for the video player and the docked book alike.
 * startTv calls it once. A key this model's remote lacks is skipped.
 * Without Tizen's input device API it does nothing.
 */
export function registerMediaKeys(): void {
  const input = (window as unknown as TizenGlobals).tizen?.tvinputdevice;
  if (!input) return;
  for (const key of mediaKeys) {
    try {
      input.registerKey(key);
    } catch {
      // A key this model's remote lacks.
    }
  }
}

/**
 * Whether the TV says its panel is 1080p, which the capability report caps
 * video's bitrate for, as Jellyfin's Tizen profile does. False without
 * productinfo (the webapis script didn't load, or this isn't a Samsung TV).
 */
export function fullHdPanel(): boolean {
  const info = (window as unknown as TizenGlobals).webapis?.productinfo;
  try {
    return info ? !info.isUdPanelSupported() : false;
  } catch {
    return false;
  }
}

/**
 * Whether the TV says its panel shows HDR (webapis.avinfo), which the
 * capability report declares HDR by. Undefined without avinfo, or when it
 * fails, so the report then keeps its rule for Samsung TVs.
 */
export function hdrPanel(): boolean | undefined {
  const info = (window as unknown as TizenGlobals).webapis?.avinfo;
  try {
    return info ? info.isHdrTvSupport() : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Keeps the TV's screensaver off while on is true, as a video plays, and
 * lets it come back once on is false. A TV otherwise dims to its
 * screensaver partway through a film, since a remote that isn't pressed
 * looks like nobody watching. Without appcommon it does nothing.
 */
export function keepScreenOn(on: boolean): void {
  const common = (window as unknown as TizenGlobals).webapis?.appcommon;
  if (!common) return;
  const state = common.AppCommonScreenSaverState;
  try {
    common.setScreenSaver(on ? state.SCREEN_SAVER_OFF : state.SCREEN_SAVER_ON);
  } catch {
    // An older firmware without it; the screensaver keeps its own timer.
  }
}
