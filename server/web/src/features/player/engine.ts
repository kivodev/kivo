import { mediaUrl } from "@/api/playback";
import type { PlayStarted } from "@/api/schemas";
import { currentBrowser } from "@/lib/browser";
import { isPackaged, isTv } from "@/tv/flag";
import { avplay } from "@/tv/tizen";

// The engine adapter is one interface over the ways a play's media reaches
// the screen. `file` plays each part's URL directly, in turn, and on the
// packaged Samsung TV app `avplay` does, through Samsung's own player
// (avplay.ts). HLS plays through hls.js on Media Source Extensions, or through
// Safari's own HLS. Positions are always on the version's timeline. Every
// text subtitle the play streams is a subtitle rendition in its HLS
// playlist, and both HLS engines draw the one chosen as a text track of the
// video. The controller plays, pauses and follows the play through the
// engine alone: three of the engines play through the media element and
// pass on what it does, and AVPlay has no element to play through.

export type EngineKind = "file" | "avplay" | "hlsjs" | "native";

/** What failed, in the fallback request's codes. */
type FailureCode = "mediaDecodeError" | "mediaSourceError" | "networkError" | "unsupported";
export type EngineFailure = { code: FailureCode; detail: string };

export type EngineEvents = {
  /** Loaded at the play's start position, so playback can begin. */
  onReady: () => void;
  /** The last part played to its end. */
  onEnded: () => void;
  /** Media failed, and the engine can't be used again. Called at most once. */
  onFailure: (failure: EngineFailure) => void;
  /** Playback was started, by the controller or by the browser itself, as its media keys do. */
  onPlay: () => void;
  /** Playback was paused, likewise. Not called for the pause at the end of a part or of the last. */
  onPause: () => void;
  /** Playback stalled waiting for data. */
  onWaiting: () => void;
  /** Frames are moving, at the start, after a stall or after a seek. */
  onPlaying: () => void;
  /** A seek landed. */
  onSeeked: () => void;
  /** The position moved on, a few times a second while it plays. */
  onTime: () => void;
};

export type Engine = {
  kind: EngineKind;
  positionMs: () => number;
  seek: (positionMs: number) => void;
  paused: () => boolean;
  /** Whether the last part played to its end, until a seek. */
  ended: () => boolean;
  /** Starts playback. It rejects where it can't start, as when a browser refuses to play unasked. */
  play: () => Promise<void>;
  pause: () => void;
  /** The measured download rate, where the engine measures one. */
  bandwidthBps: () => number | undefined;
  /**
   * Shows the play's subtitle track with this ID from the media itself, a
   * rendition of the stream or a text track of the file, and hides the
   * rest. Undefined hides them all.
   */
  showSubtitle: (trackId: string | undefined) => void;
  /** Plays the file's audio track with this ID, where the element switches audio tracks. A stream has none. */
  selectAudio?: (trackId: string) => void;
  /** Releases the decoder. A leaked decoder breaks the next play on a TV. */
  destroy: () => void;
};

export type EngineEnv = {
  mse: boolean;
  nativeHls: boolean;
  apple: boolean;
  /** Whether Samsung's AVPlay is there, on the packaged TV app. */
  avplay?: boolean;
};

/** Which engine plays a delivery here, or undefined when none can. */
export function chooseEngine(delivery: string, env: EngineEnv): EngineKind | undefined {
  // A Samsung TV's own player plays the original file at its full quality,
  // in more containers and with more of its tracks than the video element.
  if (delivery === "file") return env.avplay ? "avplay" : "file";
  if (delivery !== "stream") return undefined;
  // Safari plays HLS itself, which keeps AirPlay working. Chromium answers
  // canPlayType for HLS too, but hls.js on MSE is the tested path there.
  if (env.nativeHls && (env.apple || !env.mse)) return "native";
  return env.mse ? "hlsjs" : undefined;
}

/** Whether Samsung's AVPlay is there to play files: on the packaged TV app. */
const hasAvplay = () => isPackaged && avplay() !== undefined;

/**
 * Whether a delivery's engine plays at a chosen speed. AVPlay plays a film
 * only at normal speed: its own speeds are a fast-forward and rewind,
 * without sound.
 */
export function playsAtSpeed(delivery: string): boolean {
  return chooseEngine(delivery, { mse: true, nativeHls: false, apple: false, avplay: hasAvplay() }) !== "avplay";
}

function engineEnv(media: HTMLMediaElement): EngineEnv {
  const mse = globalThis.MediaSource ?? (globalThis as { ManagedMediaSource?: typeof MediaSource }).ManagedMediaSource;
  return {
    mse: mse?.isTypeSupported('video/mp4; codecs="avc1.42E01E,mp4a.40.2"') ?? false,
    nativeHls: media.canPlayType("application/vnd.apple.mpegurl") !== "",
    apple: currentBrowser().safari,
    avplay: hasAvplay(),
  };
}

const mediaErrorCodes: Record<number, FailureCode> = {
  2: "networkError", // MEDIA_ERR_NETWORK
  3: "mediaDecodeError", // MEDIA_ERR_DECODE
  4: "unsupported", // MEDIA_ERR_SRC_NOT_SUPPORTED
};

function mediaFailure(media: HTMLMediaElement): EngineFailure | undefined {
  const error = media.error;
  const code = error ? mediaErrorCodes[error.code] : undefined;
  return error && code ? { code, detail: `MediaError ${error.code}: ${error.message}` } : undefined;
}

/** Event listeners on the media element, removed together. */
function listeners(media: HTMLMediaElement) {
  const added: [string, () => void][] = [];
  return {
    on: (type: keyof HTMLMediaElementEventMap, fn: () => void) => {
      media.addEventListener(type, fn);
      added.push([type, fn]);
    },
    clear: () => {
      for (const [type, fn] of added) media.removeEventListener(type, fn);
    },
  };
}

/**
 * Playing and pausing through the media element, for the engines that play
 * through it, with what the element does passed on as the engine's events.
 * Call it after the engine's own listeners, which then run first.
 */
function elementTransport(
  media: HTMLMediaElement,
  on: ReturnType<typeof listeners>["on"],
  events: EngineEvents,
): Pick<Engine, "paused" | "ended" | "play" | "pause"> {
  on("play", events.onPlay);
  on("pause", () => {
    // Reaching the end of a part pauses on the way to the next one.
    if (!media.ended) events.onPause();
  });
  on("waiting", events.onWaiting);
  on("playing", events.onPlaying);
  on("seeked", events.onSeeked);
  on("timeupdate", events.onTime);
  return {
    paused: () => media.paused,
    ended: () => media.ended,
    play: () => media.play(),
    pause: () => media.pause(),
  };
}

/**
 * Whether a text track comes with the media: a subtitle rendition of the
 * HLS stream, or a subtitle track of the file. Safari gives a rendition
 * for the hard of hearing the kind `captions`, and a forced one `forced`.
 * The player's own text tracks have IDs that start with "subtitle-".
 */
export const isMediaTrack = (track: TextTrack) =>
  ["subtitles", "captions", "forced"].includes(track.kind) && !track.id.startsWith("subtitle-");

/**
 * Where the play's subtitle track `trackId` is among the media's text
 * tracks or renditions, or -1 when it isn't. `ids` are theirs, where the
 * player exposes them: a rendition's STABLE-RENDITION-ID is its track's
 * ID. Without one, the renditions come in the order of the play's tracks
 * streamed, and a file's text tracks in the order of their stream index.
 */
export function subtitleIndex(play: PlayStarted, trackId: string | undefined, ids: readonly (string | undefined)[]) {
  if (trackId === undefined) return -1;
  const byId = ids.indexOf(trackId);
  if (byId >= 0) return byId;
  const track = play.tracks.find((t) => t.type === "subtitle" && t.trackId === trackId);
  if (track?.delivery !== "stream" && track?.delivery !== "file") return -1;
  const order = play.tracks.filter((t) => t.type === "subtitle" && t.delivery === track.delivery);
  if (track.delivery === "file") order.sort((a, b) => (a.index ?? 0) - (b.index ?? 0));
  const i = order.indexOf(track);
  return i < ids.length ? i : -1;
}

/** Where the play's audio track `trackId` is among the file's audio tracks, which come in the play's order. */
export function audioIndex(play: PlayStarted, trackId: string): number {
  return play.tracks.filter((t) => t.type === "audio").findIndex((t) => t.trackId === trackId);
}

/** The media element's text tracks of the media, showing the chosen one. They're added as the media loads. */
function mediaSubtitles(media: HTMLMediaElement, play: PlayStarted) {
  let shown: string | undefined;
  const apply = () => {
    const tracks = Array.from(media.textTracks).filter(isMediaTrack);
    const i = subtitleIndex(
      play,
      shown,
      tracks.map((t) => t.id || undefined),
    );
    for (const [k, track] of tracks.entries()) track.mode = k === i ? "showing" : "disabled";
  };
  media.textTracks.addEventListener("addtrack", apply);
  return {
    show: (trackId: string | undefined) => {
      shown = trackId;
      apply();
    },
    destroy: () => media.textTracks.removeEventListener("addtrack", apply),
  };
}

/** A media element's audio tracks, which only Safari has without a flag. */
type AudioTracks = ArrayLike<{ enabled: boolean }>;

/** Stops the element and drops its source, so the decoder is released. */
function release(media: HTMLMediaElement) {
  media.pause();
  media.removeAttribute("src");
  media.load();
}

/** Calls onFailure once, ignoring what follows the first failure. */
function once(events: EngineEvents): EngineEvents {
  let failed = false;
  return {
    ...events,
    onFailure: (failure) => {
      if (failed) return;
      failed = true;
      events.onFailure(failure);
    },
  };
}

/** Direct play of each part's file in turn. A position is offset by the parts before its part. */
function fileEngine(media: HTMLMediaElement, play: PlayStarted, events: EngineEvents): Engine {
  const parts = play.parts.map((p) => ({ url: mediaUrl(p.url ?? play.url), durationMs: p.durationMs }));
  const offsets = parts.map((_, i) => parts.slice(0, i).reduce((sum, p) => sum + p.durationMs, 0));
  const { on, clear } = listeners(media);
  let index = 0;
  // A position in the loading part, applied once its metadata is in.
  let pendingMs: number | undefined;
  let resume = false;
  let ready = false;
  const subtitles = mediaSubtitles(media, play);
  // The audio track to play, which each part's file is told as it loads.
  let audio = play.tracks.find((t) => t.type === "audio" && t.selected)?.trackId;
  const applyAudio = () => {
    const tracks = (media as { audioTracks?: AudioTracks }).audioTracks;
    const i = audio === undefined ? -1 : audioIndex(play, audio);
    if (!tracks || i < 0 || i >= tracks.length) return;
    for (let k = 0; k < tracks.length; k++) {
      const track = tracks[k];
      if (track) track.enabled = k === i;
    }
  };

  const partAt = (ms: number) =>
    Math.max(
      0,
      offsets.findLastIndex((offset) => offset <= ms),
    );
  const load = (i: number, ms: number, play: boolean) => {
    index = i;
    pendingMs = Math.max(0, ms - (offsets[i] ?? 0));
    resume = play;
    media.src = parts[i]?.url ?? "";
  };

  on("loadedmetadata", () => {
    applyAudio();
    // Safari ignores currentTime set before metadata loads.
    if (pendingMs !== undefined && (ready || pendingMs > 0)) media.currentTime = pendingMs / 1000;
    pendingMs = undefined;
    if (!ready) {
      ready = true;
      events.onReady();
    } else if (resume) {
      // If the browser refuses play(), the media stays paused and the controls show that.
      media.play().catch(() => {});
    }
  });
  on("ended", () => {
    if (index < parts.length - 1) load(index + 1, offsets[index + 1] ?? 0, true);
    else events.onEnded();
  });
  on("error", () => {
    const failure = mediaFailure(media);
    if (failure) events.onFailure(failure);
  });

  const transport = elementTransport(media, on, events);

  load(partAt(play.startMs), play.startMs, false);
  return {
    kind: "file",
    ...transport,
    positionMs: () => (offsets[index] ?? 0) + (pendingMs ?? media.currentTime * 1000),
    seek: (ms) => {
      const i = partAt(ms);
      if (i === index && pendingMs === undefined) media.currentTime = (ms - (offsets[i] ?? 0)) / 1000;
      else load(i, ms, !media.paused);
    },
    bandwidthBps: () => undefined,
    showSubtitle: subtitles.show,
    selectAudio: (trackId) => {
      audio = trackId;
      applyAudio();
    },
    destroy: () => {
      clear();
      subtitles.destroy();
      release(media);
    },
  };
}

/** Safari's own HLS, with the playlist as the element's source. */
function nativeEngine(media: HTMLMediaElement, play: PlayStarted, events: EngineEvents): Engine {
  const { on, clear } = listeners(media);
  // Safari ignores currentTime set before metadata loads, and the element
  // reads 0 until then. The position waits here and is applied once
  // metadata loads.
  let pendingMs: number | undefined = play.startMs;
  on("loadedmetadata", () => {
    if (pendingMs === undefined) return;
    if (pendingMs > 0) media.currentTime = pendingMs / 1000;
    pendingMs = undefined;
    events.onReady();
  });
  on("ended", events.onEnded);
  on("error", () => {
    const failure = mediaFailure(media);
    if (failure) events.onFailure(failure);
  });
  // Safari adds the stream's subtitle renditions as text tracks once the
  // playlist loads. One shows only when the player picks it.
  const subtitles = mediaSubtitles(media, play);
  const transport = elementTransport(media, on, events);
  media.src = mediaUrl(play.url);
  return {
    kind: "native",
    ...transport,
    positionMs: () => pendingMs ?? media.currentTime * 1000,
    seek: (ms) => {
      if (pendingMs === undefined) media.currentTime = ms / 1000;
      else pendingMs = ms;
    },
    bandwidthBps: () => undefined,
    showSubtitle: subtitles.show,
    // A stream carries one audio track.
    destroy: () => {
      clear();
      subtitles.destroy();
      release(media);
    },
  };
}

// What a TV's hls.js may buffer of the playing level: about 45 MB ahead and
// 10 MB behind. A TV has about 1 to 1.5 GB of RAM for the whole browser and
// the app, and its decoder takes its share, so a buffer held in seconds is
// too much for a high-bitrate stream: 15 seconds ahead and 8 behind of a
// 60 Mb/s remux is about 170 MB. A 4K remux whose video is copied runs at
// 40 to 100 Mb/s, so the budget is enough for several seconds of one.
const tvForwardBytes = 45_000_000;
const tvBackBytes = 10_000_000;
// Ahead, at least six seconds, so a slow segment or a peak in a 4K remux
// doesn't stall playback, and at most 15 seconds, which rides out a slow
// second at any bitrate.
const tvForwardSeconds = { min: 6, max: 15 };
// Behind, enough for a short rewind.
const tvBackSeconds = { min: 2, max: 8 };

const clamp = (n: number, { min, max }: { min: number; max: number }) => Math.min(Math.max(n, min), max);

/**
 * The seconds a TV buffers of a level of this bitrate, in bits a second:
 * its byte budget at that rate, within the limits. hls.js works out the
 * forward length itself from bufferConfig, and the back length is set from
 * this as the level changes.
 */
export function tvBufferSeconds(bitrate: number): { forward: number; back: number } {
  const at = (bytes: number) => (bitrate > 0 ? (8 * bytes) / bitrate : Number.POSITIVE_INFINITY);
  return { forward: clamp(at(tvForwardBytes), tvForwardSeconds), back: clamp(at(tvBackBytes), tvBackSeconds) };
}

/**
 * How much hls.js buffers. hls.js (1.7) buffers ahead, in seconds of the
 * playing level, min(max(8 * maxBufferSize / bitrate, maxBufferLength),
 * maxMaxBufferLength), with the level's peak bitrate: maxBufferSize only
 * lengthens the buffer of a low bitrate, and doesn't cap a high one. So the
 * TV's maxBufferLength is its minimum and maxMaxBufferLength its maximum,
 * and maxBufferSize, its byte budget, sets the length between them, as
 * tvBufferSeconds does. hls.js loads a segment while the buffer is short of
 * that, so it can hold up to a segment more. It shortens maxMaxBufferLength
 * itself if the browser's buffer still fills. Behind, it keeps
 * backBufferLength seconds, but at least a segment, whatever the bitrate, so
 * the engine sets that by the level as it changes. The starting 8 seconds
 * hold until a level is chosen, before any segment loads. hls.js's own
 * defaults are 30 seconds ahead, 600 at most, a 60 MB budget and an
 * unlimited back buffer; the web build keeps 30 seconds each way.
 *
 * File delivery plays through the video element itself, or AVPlay on a
 * Samsung TV, which buffer as they decide, so none of this applies to it.
 */
export function bufferConfig(tv: boolean) {
  return tv
    ? {
        maxBufferLength: tvForwardSeconds.min,
        maxMaxBufferLength: tvForwardSeconds.max,
        maxBufferSize: tvForwardBytes,
        backBufferLength: tvBackSeconds.max,
      }
    : { maxBufferLength: 30, backBufferLength: 30 };
}

/** How many times in a row hls.js loads again after a network error without an HTTP error status. */
const maxNetworkRetries = 3;

/** hls.js on Media Source Extensions, loaded only when a play needs it. */
async function hlsJsEngine(
  media: HTMLMediaElement,
  play: PlayStarted,
  events: EngineEvents,
  tv: boolean,
): Promise<Engine> {
  const { default: Hls } = await import("hls.js");
  const hls = new Hls({
    startPosition: play.startMs / 1000,
    ...bufferConfig(tv),
    // Captions inside the video stream aren't Kivo's tracks. The menus list the file's tracks.
    enableCEA708Captions: false,
  });
  // hls.js reads its config as it trims the back buffer, after each segment.
  if (tv) {
    hls.on(Hls.Events.LEVEL_SWITCHING, (_event, level) => {
      hls.config.backBufferLength = tvBufferSeconds(level.maxBitrate).back;
    });
  }
  // A streamed subtitle shows only when the player picks it.
  let shown: string | undefined;
  const chosen = () =>
    subtitleIndex(
      play,
      shown,
      hls.subtitleTracks.map((t) => t.attrs["STABLE-RENDITION-ID"]),
    );
  const applySubtitles = () => {
    const i = chosen();
    hls.subtitleDisplay = i >= 0;
    hls.subtitleTrack = i;
  };
  hls.subtitleDisplay = false;
  // hls.js picks its own track right after announcing the tracks, so choose after it does.
  hls.on(Hls.Events.SUBTITLE_TRACKS_UPDATED, () => queueMicrotask(applySubtitles));
  // hls.js also follows whichever of its text tracks shows. A TV's browser
  // shows one of its own accord when a new play adds them, as Tizen does
  // after a track showed in the play before, picking another of the same
  // language, so a switch to any but the chosen one is undone. The switch
  // to none as the engine is destroyed is left alone.
  hls.on(Hls.Events.SUBTITLE_TRACK_SWITCH, (_event, data) => {
    if (data.id !== chosen()) queueMicrotask(() => hls.media && applySubtitles());
  });
  const { on, clear } = listeners(media);
  let recovered = false;
  let networkRetries = 0;
  hls.on(Hls.Events.FRAG_LOADED, () => {
    networkRetries = 0;
  });
  hls.on(Hls.Events.ERROR, (_event, data) => {
    const status = data.type === Hls.ErrorTypes.NETWORK_ERROR ? (data.response?.code ?? 0) : 0;
    // A 500 is the server's answer that the transcode failed, so it fails at once rather than after hls.js's own
    // retries, which would get the same answer.
    if (!data.fatal && status !== 500) return;
    // A network error that outlasted hls.js's own retries loads again where it stopped, up to three times in a row:
    // the connection dropped, and the media isn't at fault, so it doesn't fall back to a transcode. An answer from
    // the server with an HTTP error status ends the play, as loading again would get the same answer.
    if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
      if (status < 400 && networkRetries < maxNetworkRetries) {
        networkRetries++;
        hls.startLoad();
        return;
      }
      events.onFailure({
        code: "networkError",
        detail: `hls.js ${data.type} ${data.details}${status > 0 ? ` (HTTP ${status})` : ""}`,
      });
      return;
    }
    // A media error is often a transient decoder hiccup, so hls.js's own recovery gets one try.
    if (data.type === Hls.ErrorTypes.MEDIA_ERROR && !recovered) {
      recovered = true;
      hls.recoverMediaError();
      return;
    }
    const code: FailureCode = data.type === Hls.ErrorTypes.MEDIA_ERROR ? "mediaDecodeError" : "mediaSourceError";
    events.onFailure({
      code,
      detail: `hls.js ${data.type} ${data.details}${data.error ? `: ${data.error.message}` : ""}`,
    });
  });
  let ready = false;
  on("loadedmetadata", () => {
    if (ready) return;
    ready = true;
    events.onReady();
  });
  // The element reads 0 until hls.js seeks to the start position, which it
  // does once the first segment is in. Until then the position is the start.
  let pendingMs: number | undefined = play.startMs > 0 ? play.startMs : undefined;
  on("seeked", () => {
    pendingMs = undefined;
  });
  on("ended", events.onEnded);
  const transport = elementTransport(media, on, events);
  hls.loadSource(mediaUrl(play.url));
  hls.attachMedia(media);
  return {
    kind: "hlsjs",
    ...transport,
    positionMs: () => pendingMs ?? media.currentTime * 1000,
    seek: (ms) => {
      pendingMs = undefined;
      media.currentTime = ms / 1000;
    },
    bandwidthBps: () => (Number.isFinite(hls.bandwidthEstimate) ? Math.round(hls.bandwidthEstimate) : undefined),
    showSubtitle: (trackId) => {
      shown = trackId;
      applySubtitles();
    },
    destroy: () => {
      clear();
      hls.destroy();
      release(media);
    },
  };
}

/**
 * Attaches a play's media to the video or audio element, or on a Samsung TV
 * a file's to AVPlay, which leaves the element empty. When no engine here
 * plays the delivery, it reports `unsupported` and resolves to undefined.
 * A TV keeps smaller buffers.
 */
export async function attachEngine(
  media: HTMLMediaElement,
  play: PlayStarted,
  events: EngineEvents,
  tv = isTv,
): Promise<Engine | undefined> {
  const guarded = once(events);
  switch (chooseEngine(play.delivery, engineEnv(media))) {
    case "file":
      return fileEngine(media, play, guarded);
    case "avplay": {
      const player = avplay();
      if (!player) return fileEngine(media, play, guarded);
      const { avplayEngine } = await import("./avplay");
      return avplayEngine(media, play, guarded, player);
    }
    case "native":
      return nativeEngine(media, play, guarded);
    case "hlsjs":
      return hlsJsEngine(media, play, guarded, tv);
    default:
      guarded.onFailure({ code: "unsupported", detail: `no engine plays ${play.delivery} here` });
      return undefined;
  }
}
