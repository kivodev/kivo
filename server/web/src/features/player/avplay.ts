import { mediaUrl } from "@/api/playback";
import type { PlayStarted } from "@/api/schemas";
import type { Avplay, AvplayListener, AvplayTrack } from "@/tv/tizen";
import { type Engine, type EngineEvents, type EngineFailure, isMediaTrack } from "./engine";

// Direct play on a Samsung TV through Samsung's own player (webapis.avplay)
// rather than the video element. It demuxes and decodes the original file
// on the TV's media pipeline, as the TV's own apps do: MKV as well as MP4
// and MPEG-TS, HEVC Main 10 and HDR at 4K, every audio track of the file to
// switch between, and E-AC-3 out to a soundbar. It is the packaged app's
// engine for file delivery; streams stay on hls.js.
//
// AVPlay draws the picture on a plane of its own beneath the page, so the
// page shows it only where the page is transparent, and it has no media
// element. The rest of the player works the video element, so while AVPlay
// plays, the element stands in for it: its paused, ended, currentTime,
// duration, play() and pause() are AVPlay's, and it fires the media events
// the controller and the controls listen for, as AVPlay reports them. The
// element itself loads nothing. Destroying the engine gives the element
// back as it was, for a fallback to a stream.

/** AVPlay lays the screen out at 1920×1080, whatever the panel or the page. */
const screen = { width: 1920, height: 1080 };

// How much AVPlay buffers before it starts, and again after it runs dry.
// AVPlay holds its buffer in the media pipeline's memory, not the page's,
// so these are seconds of any bitrate. The start is short so a play starts
// quickly; after a stall it buffers longer, so a 4K remux's peak doesn't
// stall it again at once. Samsung's minimum is 4.
const bufferSeconds = { play: 5, resume: 10 };

/** How long AVPlay may buffer before it gives up with an error. */
const bufferTimeoutSeconds = 30;

/** How often, at most, the element fires timeupdate, as a browser does. */
const timeUpdateMs = 250;

/** How often the subtitle showing is checked against the position. */
const cueTickMs = 100;

const failureCodes: Record<string, EngineFailure["code"]> = {
  PLAYER_ERROR_NOT_SUPPORTED_FILE: "unsupported",
  PLAYER_ERROR_NOT_SUPPORTED_FORMAT: "unsupported",
  PLAYER_ERROR_INVALID_URI: "networkError",
  PLAYER_ERROR_CONNECTION_FAILED: "networkError",
  PLAYER_ERROR_NO_SUCH_FILE: "networkError",
};

/** A failure from AVPlay's error name. A decoder fault is anything it doesn't name otherwise. */
export function avplayFailure(error: string, message?: string): EngineFailure {
  return {
    code: failureCodes[error] ?? "mediaDecodeError",
    detail: `AVPlay ${error}${message ? `: ${message}` : ""}`,
  };
}

/** The picture's size, from AVPlay's video track. */
function videoSize(tracks: readonly AvplayTrack[]): { width: number; height: number } {
  const video = tracks.find((t) => t.type === "VIDEO");
  try {
    const info = JSON.parse(video?.extra_info ?? "{}") as { Width?: string; Height?: string };
    return { width: Number(info.Width) || 0, height: Number(info.Height) || 0 };
  } catch {
    return { width: 0, height: 0 };
  }
}

/** Overrides properties of one element, and returns a function that gives it back its own. */
function standIn(media: HTMLMediaElement, props: Record<string, PropertyDescriptor>): () => void {
  for (const [name, descriptor] of Object.entries(props)) {
    Object.defineProperty(media, name, { configurable: true, ...descriptor });
  }
  return () => {
    for (const name of Object.keys(props)) delete (media as unknown as Record<string, unknown>)[name];
  };
}

/**
 * The play's own text subtitle, its WebVTT file as a text track of the
 * element (PlayerVideo), drawn over the picture by position. The browser
 * draws a track's cues by the element's own clock, which doesn't run, so
 * the track is kept hidden, which still loads its cues, and they're drawn
 * here into the subtitle layer, in the player's subtitle style.
 */
function cueLayer(media: HTMLMediaElement, positionMs: () => number) {
  const box = document.createElement("div");
  box.dataset.part = "avplay-cues";
  box.setAttribute("aria-live", "off");
  (media.parentElement?.querySelector('[data-part="subtitle-layer"]') ?? media.parentElement)?.append(box);
  let shown: readonly TextTrackCue[] = [];
  const tick = () => {
    const ms = positionMs();
    const now: TextTrackCue[] = [];
    for (const track of Array.from(media.textTracks)) {
      if (isMediaTrack(track) || track.mode === "disabled") continue;
      // Showing would have the browser draw it as well, at the element's time.
      if (track.mode === "showing") track.mode = "hidden";
      for (const cue of Array.from(track.cues ?? [])) {
        if (cue.startTime * 1000 <= ms && ms < cue.endTime * 1000) now.push(cue);
      }
    }
    if (now.length === shown.length && now.every((c, i) => c === shown[i])) return;
    shown = now;
    box.replaceChildren(
      ...now.map((cue) => {
        const line = document.createElement("span");
        line.dataset.part = "avplay-cue";
        // getCueAsHTML builds the cue's own markup (italics, bold, voices) as nodes; it parses no HTML.
        if (typeof VTTCue !== "undefined" && cue instanceof VTTCue) line.append(cue.getCueAsHTML());
        else line.textContent = (cue as { text?: string }).text ?? "";
        return line;
      }),
    );
  };
  const timer = setInterval(tick, cueTickMs);
  return {
    tick,
    destroy: () => {
      clearInterval(timer);
      box.remove();
    },
  };
}

/**
 * Plays each part's file in turn with AVPlay. A position is on the
 * version's timeline, offset by the parts before its part, as the file
 * engine's is.
 */
export function avplayEngine(media: HTMLMediaElement, play: PlayStarted, events: EngineEvents, player: Avplay): Engine {
  const parts = play.parts.map((p) => ({ url: mediaUrl(p.url ?? play.url), durationMs: p.durationMs }));
  const offsets = parts.map((_, i) => parts.slice(0, i).reduce((sum, p) => sum + p.durationMs, 0));
  const totalMs = parts.reduce((sum, p) => sum + p.durationMs, 0);
  const partAt = (ms: number) =>
    Math.max(
      0,
      offsets.findLastIndex((offset) => offset <= ms),
    );

  let index = 0;
  let destroyed = false;
  // Bumped by every part loaded, so a late answer for an earlier one is dropped.
  let loads = 0;
  let ready = false;
  let paused = true;
  let ended = false;
  let buffering = false;
  // Where AVPlay last said it was, on the version's timeline, and when.
  let clock = { ms: play.startMs, at: performance.now() };
  // A seek under way, and the one asked for while it was, which follows it.
  let seeking: number | undefined;
  let queued: number | undefined;
  let size = { width: 0, height: 0 };
  let lastTimeUpdate = 0;
  let audio = play.tracks.find((t) => t.type === "audio" && t.selected)?.trackId;

  const fire = (type: string) => media.dispatchEvent(new Event(type));
  const fail = (failure: EngineFailure) => {
    if (!destroyed) events.onFailure(failure);
  };

  // Between AVPlay's reports the position runs on with the clock while it plays.
  const positionMs = () => {
    if (seeking !== undefined) return queued ?? seeking;
    const running = !paused && !buffering && ready ? Math.min(performance.now() - clock.at, 1000) : 0;
    return Math.min(clock.ms + running, totalMs);
  };

  // AVPlay selects a file's audio track only while it plays or is paused.
  // It lists only the tracks the TV decodes, in the file's order, leaving
  // out the rest, such as TrueHD and DTS, which are the play's tracks the
  // server didn't deliver in the file. So the chosen track is found by its
  // place among those delivered; if AVPlay lists another number of them,
  // the track it plays is left alone rather than guessed at.
  const applyAudio = () => {
    if (audio === undefined) return;
    const state = player.getState();
    if (state !== "PLAYING" && state !== "PAUSED") return;
    const delivered = play.tracks
      .filter((t) => t.type === "audio" && t.delivery === "file")
      .sort((a, b) => (a.index ?? 0) - (b.index ?? 0));
    const i = delivered.findIndex((t) => t.trackId === audio);
    const tracks = player
      .getTotalTrackInfo()
      .filter((t) => t.type === "AUDIO")
      .sort((a, b) => a.index - b.index);
    const track = tracks[i];
    if (i < 0 || tracks.length !== delivered.length || !track) return;
    try {
      player.setSelectTrack("AUDIO", track.index);
    } catch {
      // The file's track can't be switched to; the one playing stays.
    }
  };

  const start = () => {
    try {
      player.play();
    } catch (e) {
      fail(avplayFailure("PLAYER_ERROR_INVALID_STATE", String(e)));
      return;
    }
    paused = false;
    clock = { ms: clock.ms, at: performance.now() };
    fire("play");
    applyAudio();
    if (!buffering) fire("playing");
  };

  const seekDone = (load: number) => () => {
    if (destroyed || load !== loads) return;
    const next = queued;
    queued = undefined;
    if (next !== undefined && next !== seeking) {
      seekTo(next);
      return;
    }
    clock = { ms: seeking ?? clock.ms, at: performance.now() };
    seeking = undefined;
    fire("seeked");
    if (!paused && !buffering) fire("playing");
  };

  function seekTo(ms: number) {
    const i = partAt(ms);
    if (i !== index) {
      load(i, ms, !paused);
      return;
    }
    seeking = ms;
    fire("seeking");
    const done = seekDone(loads);
    player.seekTo(Math.max(0, ms - (offsets[i] ?? 0)), done, done);
  }

  const listener: AvplayListener = {
    onbufferingstart: () => {
      buffering = true;
      if (ready) fire("waiting");
    },
    onbufferingcomplete: () => {
      buffering = false;
      clock = { ms: clock.ms, at: performance.now() };
      if (ready && !paused) fire("playing");
    },
    oncurrentplaytime: (ms) => {
      if (seeking !== undefined) return;
      const now = performance.now();
      clock = { ms: (offsets[index] ?? 0) + ms, at: now };
      if (now - lastTimeUpdate >= timeUpdateMs) {
        lastTimeUpdate = now;
        fire("timeupdate");
      }
    },
    onstreamcompleted: () => {
      if (index < parts.length - 1) {
        load(index + 1, offsets[index + 1] ?? 0, true);
        return;
      }
      ended = true;
      paused = true;
      clock = { ms: totalMs, at: performance.now() };
      fire("pause");
      fire("ended");
      events.onEnded();
    },
    onerror: (error) => fail(avplayFailure(error)),
    onerrormsg: (error, message) => fail(avplayFailure(error, message)),
    // Another app took the decoder, as the TV's own does at an input switch.
    onresourceconflicted: () => fail(avplayFailure("PLAYER_ERROR_RESOURCE_CONFLICTED")),
  };

  function close() {
    try {
      if (player.getState() !== "NONE") {
        player.stop();
        player.close();
      }
    } catch {
      // Already closed.
    }
  }

  function load(i: number, ms: number, autoplay: boolean) {
    const load = ++loads;
    index = i;
    seeking = undefined;
    queued = undefined;
    clock = { ms, at: performance.now() };
    close();
    try {
      player.open(parts[i]?.url ?? "");
      player.setListener(listener);
      player.setDisplayRect(0, 0, screen.width, screen.height);
      player.setDisplayMethod("PLAYER_DISPLAY_MODE_LETTER_BOX");
      player.setBufferingParam("PLAYER_BUFFER_FOR_PLAY", "PLAYER_BUFFER_SIZE_IN_SECOND", bufferSeconds.play);
      player.setBufferingParam("PLAYER_BUFFER_FOR_RESUME", "PLAYER_BUFFER_SIZE_IN_SECOND", bufferSeconds.resume);
      player.setTimeoutForBuffering(bufferTimeoutSeconds);
      // The play's subtitles come as their own files; the file's own aren't drawn.
      player.setSilentSubtitle(true);
    } catch (e) {
      fail(avplayFailure("PLAYER_ERROR_INVALID_OPERATION", String(e)));
      return;
    }
    const prepared = () => {
      if (destroyed || load !== loads) return;
      size = videoSize(player.getTotalTrackInfo());
      if (!ready) {
        ready = true;
        fire("loadedmetadata");
        fire("durationchange");
        events.onReady();
        return;
      }
      // A seek into another part loads that part.
      fire("seeked");
      if (autoplay) start();
    };
    player.prepareAsync(
      () => {
        if (destroyed || load !== loads) return;
        const local = ms - (offsets[i] ?? 0);
        if (local <= 0) {
          prepared();
          return;
        }
        player.seekTo(local, prepared, prepared);
      },
      (e) => {
        if (load === loads) fail(avplayFailure("PLAYER_ERROR_NOT_SUPPORTED_FILE", String(e)));
      },
    );
  }

  // The plane AVPlay draws on, beneath the page, which the page must let
  // show through: the root's data-avplay makes it transparent and hides all
  // but the player (index.css).
  const plane = document.createElement("object");
  plane.type = "application/avplayer";
  plane.dataset.part = "avplay";
  plane.className = "pointer-events-none fixed inset-0 size-full";
  document.body.prepend(plane);
  document.documentElement.setAttribute("data-avplay", "");

  const giveBack = standIn(media, {
    paused: { get: () => paused },
    ended: { get: () => ended },
    seeking: { get: () => seeking !== undefined },
    duration: { get: () => (ready ? totalMs / 1000 : Number.NaN) },
    currentTime: {
      get: () => positionMs() / 1000,
      set: (seconds: number) => engine.seek(seconds * 1000),
    },
    videoWidth: { get: () => size.width },
    videoHeight: { get: () => size.height },
    readyState: { get: () => (ready ? HTMLMediaElement.HAVE_ENOUGH_DATA : HTMLMediaElement.HAVE_NOTHING) },
    play: {
      value: () => {
        if (!ready) return Promise.reject(new DOMException("not loaded", "InvalidStateError"));
        if (ended) {
          ended = false;
          engine.seek(0);
        }
        if (paused) start();
        return Promise.resolve();
      },
    },
    pause: {
      value: () => {
        if (paused || !ready) return;
        try {
          player.pause();
        } catch {
          // Not playing yet.
        }
        paused = true;
        clock = { ms: positionMs(), at: performance.now() };
        fire("pause");
      },
    },
  });
  const cues = cueLayer(media, positionMs);

  const engine: Engine = {
    kind: "avplay",
    positionMs,
    seek: (ms) => {
      ended = false;
      const target = Math.min(Math.max(ms, 0), totalMs);
      if (seeking !== undefined) {
        queued = target;
        return;
      }
      seekTo(target);
      cues.tick();
    },
    bandwidthBps: () => {
      try {
        const bps = Number(player.getStreamingProperty("CURRENT_BANDWIDTH"));
        return bps > 0 ? bps : undefined;
      } catch {
        return undefined;
      }
    },
    // The play's subtitles reach the TV as their own files, drawn by the cue layer.
    showSubtitle: () => {},
    selectAudio: (trackId) => {
      audio = trackId;
      applyAudio();
    },
    destroy: () => {
      destroyed = true;
      close();
      cues.destroy();
      giveBack();
      plane.remove();
      document.documentElement.removeAttribute("data-avplay");
    },
  };

  load(partAt(play.startMs), play.startMs, false);
  return engine;
}
