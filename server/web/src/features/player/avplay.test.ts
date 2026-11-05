import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PlayStarted } from "@/api/schemas";
import type { Avplay, AvplayListener, AvplayState, AvplayTrack } from "@/tv/tizen";
import { avplayEngine, avplayFailure } from "./avplay";

const minute = 60_000;

/** A two-part version: 10 minutes, then 5, with two audio tracks. */
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

const tracks: AvplayTrack[] = [
  { index: 0, type: "VIDEO", extra_info: JSON.stringify({ fourCC: "HEVC", Width: "3840", Height: "2160" }) },
  { index: 1, type: "AUDIO", extra_info: JSON.stringify({ language: "eng", channels: "6", fourCC: "EAC3" }) },
  { index: 2, type: "AUDIO", extra_info: JSON.stringify({ language: "jpn", channels: "2", fourCC: "AAC" }) },
];

/** Samsung's player as it behaves on a TV, answering on a later task as it does. */
function fakeAvplay() {
  let state: AvplayState = "NONE";
  let listener: AvplayListener = {};
  const later = (fn?: () => void) => setTimeout(() => fn?.());
  const player = {
    opened: [] as string[],
    open: vi.fn((url: string) => {
      player.opened.push(url);
      state = "IDLE";
    }),
    close: vi.fn(() => {
      state = "NONE";
    }),
    stop: vi.fn(() => {
      state = "IDLE";
    }),
    prepareAsync: vi.fn((success?: () => void) => {
      later(() => {
        state = "READY";
        success?.();
      });
    }),
    setDisplayRect: vi.fn(),
    setDisplayMethod: vi.fn(),
    setBufferingParam: vi.fn(),
    setTimeoutForBuffering: vi.fn(),
    setSilentSubtitle: vi.fn(),
    setListener: vi.fn((l: AvplayListener) => {
      listener = l;
    }),
    play: vi.fn(() => {
      state = "PLAYING";
    }),
    pause: vi.fn(() => {
      state = "PAUSED";
    }),
    seekTo: vi.fn((_ms: number, success?: () => void) => later(success)),
    getCurrentTime: vi.fn(() => 0),
    getDuration: vi.fn(() => 0),
    getState: vi.fn(() => state),
    getTotalTrackInfo: vi.fn(() => tracks),
    setSelectTrack: vi.fn(),
    getStreamingProperty: vi.fn(() => "48000000"),
    /** What AVPlay tells its listener. */
    emit: () => listener,
  };
  return player;
}

const engines: { destroy: () => void }[] = [];

afterEach(() => {
  for (const e of engines.splice(0)) e.destroy();
});

function attach(s: PlayStarted = play) {
  const player = fakeAvplay();
  const video = document.createElement("video");
  const layer = document.createElement("div");
  layer.dataset.part = "subtitle-layer";
  const host = document.createElement("div");
  host.append(video, layer);
  document.body.append(host);
  const events = { onReady: vi.fn(), onEnded: vi.fn(), onFailure: vi.fn() };
  const engine = avplayEngine(video, s, events, player as unknown as Avplay);
  engines.push(engine);
  return { player, video, events, engine, layer };
}

describe("AVPlay engine", () => {
  it("finds the chosen audio track among those AVPlay lists, which leave out what the TV can't decode", async () => {
    // TrueHD, then two AC-3 tracks; AVPlay lists only the AC-3 ones, numbered on from the video.
    const remux: PlayStarted = {
      ...play,
      startMs: 0,
      tracks: [
        { trackId: "v1", type: "video", selected: true, delivery: "file", index: 0 },
        { trackId: "thd", type: "audio", index: 1, reason: "audioCodecUnsupported" },
        { trackId: "ac3", type: "audio", selected: true, delivery: "file", index: 2 },
        { trackId: "commentary", type: "audio", delivery: "file", index: 3 },
      ],
    };
    const { player, video, events, engine } = attach(remux);
    player.getTotalTrackInfo.mockReturnValue([
      tracks[0] as AvplayTrack,
      { index: 1, type: "AUDIO", extra_info: JSON.stringify({ fourCC: "audio/x-ac3" }) },
      { index: 2, type: "AUDIO", extra_info: JSON.stringify({ fourCC: "audio/x-ac3" }) },
    ]);
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    await video.play();
    expect(player.setSelectTrack).toHaveBeenLastCalledWith("AUDIO", 1);
    engine.selectAudio?.("commentary");
    expect(player.setSelectTrack).toHaveBeenLastCalledWith("AUDIO", 2);
    // A track AVPlay doesn't list isn't guessed at.
    player.setSelectTrack.mockClear();
    engine.selectAudio?.("thd");
    expect(player.setSelectTrack).not.toHaveBeenCalled();
  });

  it("opens the part holding the start, full screen, and is ready once there", async () => {
    const { player, video, events, engine } = attach();
    expect(player.opened).toEqual([expect.stringMatching(/\/api\/v1\/plays\/s\/files\/1$/)]);
    expect(player.setDisplayRect).toHaveBeenCalledWith(0, 0, 1920, 1080);
    expect(player.setBufferingParam).toHaveBeenCalledWith("PLAYER_BUFFER_FOR_PLAY", "PLAYER_BUFFER_SIZE_IN_SECOND", 5);
    expect(player.setBufferingParam).toHaveBeenCalledWith(
      "PLAYER_BUFFER_FOR_RESUME",
      "PLAYER_BUFFER_SIZE_IN_SECOND",
      10,
    );
    // The page lets AVPlay's plane show through, on which it draws.
    expect(document.documentElement.hasAttribute("data-avplay")).toBe(true);
    expect(document.querySelector('object[type="application/avplayer"]')).not.toBeNull();

    await waitFor(() => expect(events.onReady).toHaveBeenCalledOnce());
    // Two minutes into the second part.
    expect(player.seekTo).toHaveBeenCalledWith(2 * minute, expect.any(Function), expect.any(Function));
    expect(engine.positionMs()).toBe(12 * minute);
    expect(video.videoWidth).toBe(3840);
    expect(video.duration).toBe(15 * 60);
    expect(video.paused).toBe(true);
  });

  it("stands the video element in for AVPlay: play, pause and their events", async () => {
    const { player, video, events } = attach();
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    const seen: string[] = [];
    for (const type of ["play", "playing", "pause", "waiting"]) video.addEventListener(type, () => seen.push(type));

    await video.play();
    expect(player.play).toHaveBeenCalledOnce();
    expect(video.paused).toBe(false);
    player.emit().onbufferingstart?.();
    player.emit().onbufferingcomplete?.();
    video.pause();
    expect(player.pause).toHaveBeenCalledOnce();
    expect(video.paused).toBe(true);
    expect(seen).toEqual(["play", "playing", "waiting", "playing", "pause"]);
  });

  it("reports AVPlay's position on the version's timeline", async () => {
    const { player, video, events, engine } = attach();
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    const updates = vi.fn();
    video.addEventListener("timeupdate", updates);
    player.emit().oncurrentplaytime?.(3 * minute);
    expect(engine.positionMs()).toBe(13 * minute);
    expect(video.currentTime).toBe(13 * 60);
    expect(updates).toHaveBeenCalledOnce();
  });

  it("seeks in the part, and a seek asked for during one follows it", async () => {
    const { player, video, events, engine } = attach();
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    player.seekTo.mockClear();
    const seeked = vi.fn();
    video.addEventListener("seeked", seeked);

    engine.seek(11 * minute);
    engine.seek(14 * minute);
    expect(engine.positionMs()).toBe(14 * minute);
    expect(player.seekTo).toHaveBeenCalledOnce();
    await waitFor(() => expect(seeked).toHaveBeenCalledOnce());
    expect(player.seekTo.mock.calls.map(([ms]) => ms)).toEqual([1 * minute, 4 * minute]);
    expect(engine.positionMs()).toBe(14 * minute);
  });

  it("loads the other part for a seek into it", async () => {
    const { player, video, events, engine } = attach();
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    const seeked = vi.fn();
    video.addEventListener("seeked", seeked);
    engine.seek(4 * minute);
    expect(player.opened.at(-1)).toMatch(/files\/0$/);
    await waitFor(() => expect(seeked).toHaveBeenCalledOnce());
    expect(engine.positionMs()).toBe(4 * minute);
  });

  it("switches to the chosen audio track of the file, by AVPlay's own index", async () => {
    const { player, video, events, engine } = attach();
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    await video.play();
    expect(player.setSelectTrack).toHaveBeenLastCalledWith("AUDIO", 1);
    engine.selectAudio?.("a2");
    expect(player.setSelectTrack).toHaveBeenLastCalledWith("AUDIO", 2);
  });

  it("plays the next part when one ends, and ends after the last", async () => {
    const { player, video, events } = attach({ ...play, startMs: 0 });
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    await video.play();
    player.emit().onstreamcompleted?.();
    expect(player.opened.at(-1)).toMatch(/files\/1$/);
    await waitFor(() => expect(player.play).toHaveBeenCalledTimes(2));
    expect(events.onEnded).not.toHaveBeenCalled();
    player.emit().onstreamcompleted?.();
    expect(events.onEnded).toHaveBeenCalledOnce();
    expect(video.ended).toBe(true);
  });

  it("fails by the fallback request's codes, so a file it can't play falls back to a stream", () => {
    expect(avplayFailure("PLAYER_ERROR_NOT_SUPPORTED_FORMAT")).toEqual({
      code: "unsupported",
      detail: "AVPlay PLAYER_ERROR_NOT_SUPPORTED_FORMAT",
    });
    expect(avplayFailure("PLAYER_ERROR_CONNECTION_FAILED", "timeout").code).toBe("networkError");
    expect(avplayFailure("PLAYER_ERROR_GENEREIC").code).toBe("mediaDecodeError");
    const { player, events } = attach();
    player.emit().onerror?.("PLAYER_ERROR_NOT_SUPPORTED_FILE");
    expect(events.onFailure).toHaveBeenCalledWith(expect.objectContaining({ code: "unsupported" }));
  });

  it("draws the play's text subtitle by AVPlay's position, keeping the browser from drawing it", async () => {
    const { player, video, events, layer } = attach();
    const track = {
      id: "subtitle-s1",
      kind: "subtitles",
      mode: "showing",
      cues: [
        { startTime: 12 * 60, endTime: 12 * 60 + 2, text: "Hello" },
        { startTime: 13 * 60, endTime: 13 * 60 + 2, text: "Later" },
      ],
    };
    Object.defineProperty(video, "textTracks", { configurable: true, value: [track] });
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    player.emit().oncurrentplaytime?.(2 * minute + 500);
    await waitFor(() => expect(layer.textContent).toBe("Hello"));
    expect(track.mode).toBe("hidden");
    player.emit().oncurrentplaytime?.(3 * minute + 500);
    await waitFor(() => expect(layer.textContent).toBe("Later"));
  });

  it("gives the element back as it was, and closes AVPlay", async () => {
    const { player, video, events, engine } = attach();
    await waitFor(() => expect(events.onReady).toHaveBeenCalled());
    await video.play();
    engine.destroy();
    engines.length = 0;
    expect(player.stop).toHaveBeenCalled();
    expect(player.close).toHaveBeenCalled();
    expect(Object.getOwnPropertyNames(video)).not.toContain("paused");
    expect(video.paused).toBe(true);
    expect(document.documentElement.hasAttribute("data-avplay")).toBe(false);
    expect(document.querySelector('object[type="application/avplayer"]')).toBeNull();
  });
});
