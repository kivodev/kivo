import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PlayStarted, PlayTrack } from "@/api/schemas";
import { playTo, playToEnd } from "@/test/media";
import { attachEngine, audioIndex, bufferConfig, chooseEngine, subtitleIndex, tvBufferSeconds } from "./engine";

describe("bufferConfig", () => {
  it("keeps the web's buffers", () => {
    expect(bufferConfig(false)).toEqual({ maxBufferLength: 30, backBufferLength: 30 });
  });

  it("holds a TV's buffer near its byte budget at any bitrate, within a few seconds and 15", async () => {
    expect(tvBufferSeconds(4_000_000)).toEqual({ forward: 15, back: 8 });
    expect(tvBufferSeconds(16_000_000)).toEqual({ forward: 15, back: 5 });
    expect(tvBufferSeconds(40_000_000)).toEqual({ forward: 9, back: 2 });
    expect(tvBufferSeconds(60_000_000)).toEqual({ forward: 6, back: 2 });
    expect(tvBufferSeconds(120_000_000)).toEqual({ forward: 6, back: 2 });
    // A playlist without a bitrate.
    expect(tvBufferSeconds(0)).toEqual({ forward: 15, back: 8 });
    // About 45 MB ahead wherever the budget, rather than a limit, sets the length.
    expect((tvBufferSeconds(24_000_000).forward * 24_000_000) / 8).toBe(45_000_000);

    // hls.js itself works out the same forward length from the TV's config.
    const { default: Hls } = await vi.importActual<typeof import("hls.js")>("hls.js");
    const hls = new Hls(bufferConfig(true));
    const stream = (hls as unknown as { streamController: { getMaxBufferLength(bitrate: number): number } })
      .streamController;
    for (const bitrate of [4_000_000, 16_000_000, 24_000_000, 40_000_000, 60_000_000, 120_000_000]) {
      expect(stream.getMaxBufferLength(bitrate)).toBe(tvBufferSeconds(bitrate).forward);
    }
    hls.destroy();
  });
});

describe("chooseEngine", () => {
  it("plays files directly, and HLS through Safari's player or hls.js on MSE", () => {
    const chrome = { mse: true, nativeHls: true, apple: false };
    const safari = { mse: true, nativeHls: true, apple: true };
    const iphone = { mse: false, nativeHls: true, apple: true };
    const firefox = { mse: true, nativeHls: false, apple: false };
    expect(chooseEngine("file", firefox)).toBe("file");
    expect(chooseEngine("stream", chrome)).toBe("hlsjs");
    expect(chooseEngine("stream", firefox)).toBe("hlsjs");
    expect(chooseEngine("stream", safari)).toBe("native");
    expect(chooseEngine("stream", iphone)).toBe("native");
    expect(chooseEngine("stream", { mse: false, nativeHls: false, apple: false })).toBeUndefined();
    expect(chooseEngine("dash", chrome)).toBeUndefined();
  });

  it("plays files through AVPlay on a Samsung TV, and streams through hls.js still", () => {
    const tizen = { mse: true, nativeHls: false, apple: false, avplay: true };
    expect(chooseEngine("file", tizen)).toBe("avplay");
    expect(chooseEngine("stream", tizen)).toBe("hlsjs");
  });
});

const minute = 60_000;

/** A two-part version: 10 minutes, then 5. */
const play: PlayStarted = {
  id: "s",
  workId: "w",
  versionId: "v",
  startMs: 12 * minute,
  durationMs: 15 * minute,
  local: true,
  delivery: "file",
  url: "plays/s/files/0",
  tracks: [
    { trackId: "v1", type: "video", selected: true, delivery: "file", index: 0 },
    { trackId: "a1", type: "audio", selected: true, delivery: "file", index: 1 },
    { trackId: "a2", type: "audio", delivery: "file", index: 2 },
    { trackId: "off", type: "subtitle", selected: true, delivery: "none" },
  ],
  parts: [
    { durationMs: 10 * minute, url: "plays/s/files/0" },
    { durationMs: 5 * minute, url: "plays/s/files/1" },
  ],
  chapters: [],
};

function attach(s: PlayStarted = play, tv = false) {
  const video = document.createElement("video");
  const events = {
    onReady: vi.fn(),
    onEnded: vi.fn(),
    onFailure: vi.fn(),
    onPlay: vi.fn(),
    onPause: vi.fn(),
    onWaiting: vi.fn(),
    onPlaying: vi.fn(),
    onSeeked: vi.fn(),
    onTime: vi.fn(),
  };
  return { video, events, engine: attachEngine(video, s, events, tv) };
}

describe("file engine", () => {
  it("starts in the part holding the start position, after its metadata loads", async () => {
    const { video, events, engine } = attach();
    const e = await engine;
    expect(video.getAttribute("src")).toMatch(/\/api\/v1\/plays\/s\/files\/1$/);
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    expect(video.currentTime).toBe(120);
    expect(e?.positionMs()).toBe(12 * minute);
  });

  it("reports positions on the joined timeline and seeks across parts", async () => {
    const { video, events, engine } = attach();
    const e = await engine;
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    playTo(video, 180);
    expect(e?.positionMs()).toBe(13 * minute);

    e?.seek(5 * minute);
    expect(video.getAttribute("src")).toMatch(/files\/0$/);
    // Until the part's metadata loads, the position is the seek's target.
    expect(e?.positionMs()).toBe(5 * minute);
    await waitFor(() => expect(video.currentTime).toBe(300));
    e?.seek(6 * minute);
    expect(video.currentTime).toBe(360);
  });

  it("plays the next part when one ends, and ends after the last", async () => {
    const { video, events, engine } = attach({ ...play, startMs: 0 });
    const e = await engine;
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    expect(video.currentTime).toBe(0);
    playToEnd(video);
    expect(video.getAttribute("src")).toMatch(/files\/1$/);
    await waitFor(() => expect(video.paused).toBe(false));
    expect(e?.positionMs()).toBe(10 * minute);
    expect(events.onEnded).not.toHaveBeenCalled();
    playToEnd(video);
    expect(events.onEnded).toHaveBeenCalledOnce();
  });

  it("plays and pauses the element, and passes on what the element does, whoever asked", async () => {
    const { video, events, engine } = attach({ ...play, startMs: 0 });
    const e = await engine;
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    expect(e?.paused()).toBe(true);
    await e?.play();
    expect(video.paused).toBe(false);
    expect(e?.paused()).toBe(false);
    // The element reports on a later task, as a browser's does.
    await waitFor(() => expect(events.onPlaying).toHaveBeenCalledOnce());
    expect(events.onPlay).toHaveBeenCalledOnce();
    // The browser's own controls pause the element behind the engine's back.
    video.pause();
    await waitFor(() => expect(events.onPause).toHaveBeenCalledOnce());
    playTo(video, 30);
    expect(events.onTime).toHaveBeenCalledOnce();
    video.dispatchEvent(new Event("waiting"));
    video.dispatchEvent(new Event("seeked"));
    expect(events.onWaiting).toHaveBeenCalledOnce();
    expect(events.onSeeked).toHaveBeenCalledOnce();
  });

  it("doesn't report the pause at the end of a part as a pause", async () => {
    const { video, events, engine } = attach({ ...play, startMs: 0 });
    const e = await engine;
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    await e?.play();
    playToEnd(video);
    await waitFor(() => expect(video.paused).toBe(false));
    playToEnd(video);
    expect(events.onEnded).toHaveBeenCalledOnce();
    expect(e?.ended()).toBe(true);
    expect(events.onPause).not.toHaveBeenCalled();
  });

  it("reports a media error once, by its fallback code", async () => {
    const { video, events, engine } = attach();
    await engine;
    Object.defineProperty(video, "error", { value: { code: 3, message: "PIPELINE_ERROR_DECODE" } });
    video.dispatchEvent(new Event("error"));
    video.dispatchEvent(new Event("error"));
    expect(events.onFailure).toHaveBeenCalledExactlyOnceWith({
      code: "mediaDecodeError",
      detail: "MediaError 3: PIPELINE_ERROR_DECODE",
    });
  });

  it("plays the chosen audio track of each part's file, where the element switches audio tracks", async () => {
    const tracks = [{ enabled: true }, { enabled: false }];
    const { video, events, engine } = attach({
      ...play,
      tracks: play.tracks.map((t) => (t.type === "audio" ? { ...t, selected: t.trackId === "a2" || undefined } : t)),
    });
    Object.defineProperty(video, "audioTracks", { value: tracks });
    const e = await engine;
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    expect(tracks.map((t) => t.enabled)).toEqual([false, true]);
    e?.selectAudio?.("a1");
    expect(tracks.map((t) => t.enabled)).toEqual([true, false]);
  });

  it("tears down fully: no source, a reload, and no more events", async () => {
    const { video, events, engine } = attach();
    const e = await engine;
    const load = vi.spyOn(video, "load");
    e?.destroy();
    expect(video.hasAttribute("src")).toBe(false);
    expect(load).toHaveBeenCalled();
    video.dispatchEvent(new Event("ended"));
    video.dispatchEvent(new Event("pause"));
    video.dispatchEvent(new Event("timeupdate"));
    expect(events.onEnded).not.toHaveBeenCalled();
    expect(events.onPause).not.toHaveBeenCalled();
    expect(events.onTime).not.toHaveBeenCalled();
  });
});

/** The same version streamed as HLS, from 12 minutes in, with three text subtitles in the stream and one burned in. */
const hlsTracks: PlayTrack[] = [
  { trackId: "v1", type: "video", selected: true, delivery: "stream" },
  { trackId: "a1", type: "audio", selected: true, delivery: "stream" },
  { trackId: "s1", type: "subtitle", delivery: "stream", codec: "webvtt" },
  { trackId: "s2", type: "subtitle", delivery: "external", codec: "ass", url: "plays/s/subtitles/s2.ass" },
  { trackId: "s3", type: "subtitle", delivery: "stream", codec: "webvtt" },
  { trackId: "s4", type: "subtitle" },
  { trackId: "s5", type: "subtitle", delivery: "stream", codec: "webvtt" },
  { trackId: "off", type: "subtitle", selected: true, delivery: "none" },
];
const hlsPlay: PlayStarted = { ...play, delivery: "stream", url: "plays/s/hls/index.m3u8", tracks: hlsTracks };

describe("subtitleIndex", () => {
  it("finds a rendition by its STABLE-RENDITION-ID", () => {
    expect(subtitleIndex(hlsPlay, "s3", ["s5", "s3", "s1"])).toBe(1);
  });

  it("without IDs, counts the renditions in the order of the play's streamed tracks", () => {
    const none = [undefined, undefined, undefined];
    expect(subtitleIndex(hlsPlay, "s1", none)).toBe(0);
    expect(subtitleIndex(hlsPlay, "s3", none)).toBe(1);
    expect(subtitleIndex(hlsPlay, "s5", none)).toBe(2);
    // Not in the media: an external subtitle, a choice that needs a new play, and off.
    expect(subtitleIndex(hlsPlay, "s2", none)).toBe(-1);
    expect(subtitleIndex(hlsPlay, "s4", none)).toBe(-1);
    expect(subtitleIndex(hlsPlay, "off", none)).toBe(-1);
    expect(subtitleIndex(hlsPlay, undefined, none)).toBe(-1);
    // A rendition the player hasn't added yet.
    expect(subtitleIndex(hlsPlay, "s5", [undefined])).toBe(-1);
  });

  it("counts a file's own text tracks by their stream index", () => {
    const filePlay: PlayStarted = {
      ...play,
      tracks: [
        { trackId: "t9", type: "subtitle", delivery: "file", index: 9 },
        { trackId: "t4", type: "subtitle", delivery: "file", index: 4 },
      ],
    };
    expect(subtitleIndex(filePlay, "t9", [undefined, undefined])).toBe(1);
  });
});

describe("audioIndex", () => {
  it("counts the file's audio tracks in the play's order", () => {
    expect(audioIndex(play, "a2")).toBe(1);
    expect(audioIndex(play, "v1")).toBe(-1);
  });
});

describe("Safari's own HLS", () => {
  function safari() {
    vi.spyOn(HTMLMediaElement.prototype, "canPlayType").mockImplementation((type) =>
      type === "application/vnd.apple.mpegurl" ? "maybe" : "",
    );
    vi.spyOn(navigator, "vendor", "get").mockReturnValue("Apple Computer, Inc.");
  }

  it("is at the start position while the metadata loads, then plays from there", async () => {
    safari();
    const { video, events, engine } = attach(hlsPlay);
    const e = await engine;
    expect(e?.kind).toBe("native");
    // The element reads 0 until then, and a switch now must not start over.
    expect(video.currentTime).toBe(0);
    expect(e?.positionMs()).toBe(12 * minute);
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    expect(video.currentTime).toBe(720);
    expect(e?.positionMs()).toBe(12 * minute);
  });

  it("starts from a seek made while the metadata loads", async () => {
    safari();
    const { video, events, engine } = attach(hlsPlay);
    const e = await engine;
    e?.seek(3 * minute);
    expect(e?.positionMs()).toBe(3 * minute);
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    expect(video.currentTime).toBe(180);
  });
});

/** A fake of the parts of hls.js the engine uses: the start position, which it seeks to once a segment is in. */
const fakeHls = vi.hoisted(() => ({
  startPosition: 0,
  media: undefined as HTMLMediaElement | undefined,
  config: {} as Record<string, unknown>,
  handlers: new Map<string, (event: string, data: unknown) => void>(),
  subtitleTracks: [] as { attrs: Record<string, string> }[],
  instance: undefined as { subtitleTrack: number; subtitleDisplay: boolean } | undefined,
  loads: 0,
}));
vi.mock("hls.js", () => ({
  default: class {
    static Events = {
      ERROR: "hlsError",
      FRAG_LOADED: "hlsFragLoaded",
      SUBTITLE_TRACKS_UPDATED: "hlsSubtitleTracksUpdated",
      LEVEL_SWITCHING: "hlsLevelSwitching",
      SUBTITLE_TRACK_SWITCH: "hlsSubtitleTrackSwitch",
    };
    static ErrorTypes = { NETWORK_ERROR: "networkError", MEDIA_ERROR: "mediaError" };
    subtitleTracks = fakeHls.subtitleTracks;
    subtitleTrack = -1;
    subtitleDisplay = true;
    bandwidthEstimate = Number.NaN;
    config: Record<string, unknown>;
    constructor(config: { startPosition: number }) {
      fakeHls.startPosition = config.startPosition;
      this.config = { ...config };
      fakeHls.config = this.config;
      fakeHls.handlers.clear();
      fakeHls.instance = this;
    }
    on(event: string, handler: (event: string, data: unknown) => void) {
      fakeHls.handlers.set(event, handler);
    }
    loadSource() {}
    startLoad() {
      fakeHls.loads++;
    }
    media: HTMLMediaElement | null = null;
    attachMedia(media: HTMLMediaElement) {
      fakeHls.media = media;
      this.media = media;
    }
    destroy() {
      this.media = null;
    }
  },
}));

describe("hls.js", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("is at the start position until hls.js seeks there", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    const { video, engine } = attach(hlsPlay);
    const e = await engine;
    expect(e?.kind).toBe("hlsjs");
    expect(fakeHls.startPosition).toBe(720);
    expect(video.currentTime).toBe(0);
    expect(e?.positionMs()).toBe(12 * minute);
    // The first segment is in.
    video.currentTime = fakeHls.startPosition;
    await waitFor(() => expect(e?.positionMs()).toBe(12 * minute));
    playTo(video, 750);
    expect(e?.positionMs()).toBe(12.5 * minute);
  });

  it("shows the chosen subtitle's rendition by its STABLE-RENDITION-ID, and none for a choice outside the stream", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    fakeHls.subtitleTracks = ["s1", "s3", "s5"].map((id) => ({ attrs: { "STABLE-RENDITION-ID": id } }));
    const e = await attach(hlsPlay).engine;
    e?.showSubtitle("s5");
    expect(fakeHls.instance).toMatchObject({ subtitleTrack: 2, subtitleDisplay: true });
    e?.showSubtitle("s2");
    expect(fakeHls.instance).toMatchObject({ subtitleTrack: -1, subtitleDisplay: false });
    fakeHls.subtitleTracks = [];
  });

  const networkError = (response?: { code: number }, fatal = true, details = "fragLoadError") =>
    fakeHls.handlers.get("hlsError")?.("hlsError", {
      fatal,
      type: "networkError",
      details,
      response,
    });

  it("keeps the chosen rendition when hls.js switches to another of its own accord", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    // Two English renditions, as a film with a commentary has.
    fakeHls.subtitleTracks = ["s-commentary", "s-en", "s-fr"].map((id) => ({ attrs: { "STABLE-RENDITION-ID": id } }));
    const e = await attach(hlsPlay).engine;
    e?.showSubtitle("s-en");
    // A TV's browser shows the other English track itself, and hls.js follows the track showing.
    const instance = fakeHls.instance;
    if (instance) instance.subtitleTrack = 0;
    fakeHls.handlers.get("hlsSubtitleTrackSwitch")?.("hlsSubtitleTrackSwitch", { id: 0 });
    await waitFor(() => expect(fakeHls.instance).toMatchObject({ subtitleTrack: 1, subtitleDisplay: true }));
    // With subtitles off, none it shows of its own accord stays.
    e?.showSubtitle(undefined);
    if (instance) instance.subtitleTrack = 2;
    fakeHls.handlers.get("hlsSubtitleTrackSwitch")?.("hlsSubtitleTrackSwitch", { id: 2 });
    await waitFor(() => expect(fakeHls.instance).toMatchObject({ subtitleTrack: -1, subtitleDisplay: false }));
    fakeHls.subtitleTracks = [];
  });

  it("loads again after a network error without an answer rather than falling back", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    const { events, engine } = attach(hlsPlay);
    await engine;
    fakeHls.loads = 0;
    networkError();
    networkError({ code: 0 });
    expect(fakeHls.loads).toBe(2);
    expect(events.onFailure).not.toHaveBeenCalled();
  });

  it("fails after three network errors in a row, counting afresh once a segment loads", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    const { events, engine } = attach(hlsPlay);
    await engine;
    fakeHls.loads = 0;
    networkError();
    networkError();
    fakeHls.handlers.get("hlsFragLoaded")?.("hlsFragLoaded", {});
    for (let i = 0; i < 3; i++) networkError();
    expect(fakeHls.loads).toBe(5);
    expect(events.onFailure).not.toHaveBeenCalled();
    networkError();
    expect(fakeHls.loads).toBe(5);
    expect(events.onFailure).toHaveBeenCalledWith(expect.objectContaining({ code: "networkError" }));
  });

  it("fails at once on an HTTP error status", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    const { events, engine } = attach(hlsPlay);
    await engine;
    fakeHls.loads = 0;
    networkError({ code: 404 });
    expect(fakeHls.loads).toBe(0);
    expect(events.onFailure).toHaveBeenCalledWith({
      code: "networkError",
      detail: "hls.js networkError fragLoadError (HTTP 404)",
    });
  });

  it("fails on a segment or playlist answered with a 500 without waiting for hls.js's retries", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    for (const details of ["fragLoadError", "levelLoadError"]) {
      const { events, engine } = attach(hlsPlay);
      await engine;
      networkError({ code: 500 }, false, details);
      expect(events.onFailure).toHaveBeenCalledWith({
        code: "networkError",
        detail: `hls.js networkError ${details} (HTTP 500)`,
      });
    }
  });

  it("leaves other errors hls.js retries to its retries", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    const { events, engine } = attach(hlsPlay);
    await engine;
    fakeHls.loads = 0;
    for (const code of [0, 502, 503, 504]) networkError({ code }, false);
    expect(fakeHls.loads).toBe(0);
    expect(events.onFailure).not.toHaveBeenCalled();
  });

  it("keeps a TV's back buffer by the bitrate of the level it switches to", async () => {
    vi.stubGlobal("MediaSource", { isTypeSupported: () => true });
    await attach(hlsPlay, true).engine;
    expect(fakeHls.config).toMatchObject(bufferConfig(true));
    fakeHls.handlers.get("hlsLevelSwitching")?.("hlsLevelSwitching", { maxBitrate: 40_000_000 });
    expect(fakeHls.config.backBufferLength).toBe(2);
    fakeHls.handlers.get("hlsLevelSwitching")?.("hlsLevelSwitching", { maxBitrate: 16_000_000 });
    expect(fakeHls.config.backBufferLength).toBe(5);
  });
});

describe("attachEngine", () => {
  it("reports unsupported when nothing here plays the delivery", async () => {
    // jsdom has neither MSE nor native HLS.
    const { events, engine } = attach({ ...play, delivery: "stream", url: "plays/s/hls/main.m3u8" });
    expect(await engine).toBeUndefined();
    expect(events.onFailure).toHaveBeenCalledWith(expect.objectContaining({ code: "unsupported" }));
  });
});
