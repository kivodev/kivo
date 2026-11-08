import { fallbackPlay, startPlay, stopPlay, updatePlay } from "@/api/playback";
import { isProblem } from "@/api/problem";
import type { PlayCreate, PlayStarted, PlayTrack, PlayUpdate, QualityRequest } from "@/api/schemas";
import { lastBandwidth, measureBandwidth, rememberBandwidth, withBandwidth } from "./bandwidth";
import { capabilityReport } from "./capabilities";
import { attachEngine, type Engine, type EngineFailure } from "./engine";
import { pictureInPictureNeedsNewPlay } from "./pictureInPicture";
import type { PlayerAction, PlayerFailure } from "./reducer";

// PlayerController runs the play behind the player. It starts plays and
// attaches their media. It sends a heartbeat every 10 seconds and on pause,
// seek and end. When media fails, it retries as a transcode through the
// fallback request, up to maxFallbacks times. A play in picture-in-picture asks for no external
// subtitles, since nothing drawn over the video shows there, and entering or
// leaving it starts a new play when the subtitle needs one to show. It stops
// the play when the player goes away. It is plain
// TypeScript holding mutable instances, and the React side only sees the
// reducer's actions.

const heartbeatMs = 10_000;
/** How many times a play falls back: direct play to a transcode on an accelerator, then to one on the CPU. */
const maxFallbacks = 2;

/** What to play: the route's choices, or the player's own on a switch. */
export type PlayRequest = {
  workId: string;
  /** The version to play. Absent or empty lets the server pick, as absent tracks do. */
  versionId?: string;
  audioTrackId?: string;
  /** A subtitle track's ID, or `off`. */
  subtitleTrackId?: string;
  /** The quality on the server's local networks. */
  localQuality?: QualityRequest;
  /** The quality anywhere else, when it differs. The server picks between the two. */
  remoteQuality?: QualityRequest;
  /** Where to start on the version's timeline. Absent means the resume position. */
  positionMs?: number;
  /** Whether to boost dialogue, from this browser's setting or the switch in the player. */
  dialogueBoost?: boolean;
};

function failureOf(error: unknown): PlayerFailure {
  if (!isProblem(error)) return { kind: "problem", code: "network" };
  return {
    kind: "problem",
    code: error.code,
    reason: error.reason,
    mismatch: error.mismatch,
    plays: error.plays,
  };
}

const isGone = (error: unknown) => isProblem(error) && error.status === 404;

export class PlayerController {
  readonly #media: HTMLMediaElement;
  readonly #dispatch: (action: PlayerAction) => void;
  readonly #onEnded: () => void;
  readonly #removeListeners: () => void;
  #request: PlayRequest | undefined;
  #play: PlayStarted | undefined;
  #engine: Engine | undefined;
  /** Bumped by every start, fallback and teardown, so late answers to earlier ones are dropped. */
  #generation = 0;
  #timer: ReturnType<typeof setInterval> | undefined;
  #seekStartedAt: number | undefined;
  #waiting = false;
  #lastPositionMs = 0;
  /** The fallbacks since the play started. A failure after the last one shows the error. */
  #fallbacks = 0;
  /** Whether a failure is retried as a transcode. Books aren't, because they play directly or not at all. */
  readonly #canFallBack: boolean;
  /** The subtitle shown from the media itself, kept across fallbacks and reloads. */
  #mediaSubtitle: string | undefined;
  /** A subtitle shown without a new play, as a track's ID or `off`. The heartbeats report it. */
  #shownSubtitle: string | undefined;
  /** An audio track of the file played without a new play. The heartbeats report it. */
  #shownAudio: string | undefined;
  /** Whether the play was started in picture-in-picture, and so has no external subtitles. */
  #playInPictureInPicture = false;
  /** Whether the play starting should stay paused when ready, as one paused before a suspend does. */
  #startPaused = false;
  /** Whether suspend() stopped the play, and whether it was paused then. */
  #suspended: { paused: boolean } | undefined;

  constructor(
    media: HTMLMediaElement,
    dispatch: (action: PlayerAction) => void,
    onEnded: () => void,
    { fallBack = true }: { fallBack?: boolean } = {},
  ) {
    this.#canFallBack = fallBack;
    this.#media = media;
    this.#dispatch = dispatch;
    this.#onEnded = onEnded;
    const handlers: [keyof HTMLMediaElementEventMap, () => void][] = [
      ["play", () => dispatch({ type: "play" })],
      [
        "pause",
        () => {
          // Reaching the end of a part pauses on the way to the next one.
          if (media.ended) return;
          dispatch({ type: "pause" });
          this.#beat("paused");
        },
      ],
      [
        "waiting",
        () => {
          this.#waiting = true;
          dispatch({ type: "waiting" });
        },
      ],
      [
        "playing",
        () => {
          this.#waiting = false;
          dispatch({ type: "playing" });
        },
      ],
      [
        "seeked",
        () => {
          if (this.#seekStartedAt === undefined) return;
          const seekToFirstFrameMs = Math.round(performance.now() - this.#seekStartedAt);
          this.#seekStartedAt = undefined;
          dispatch({ type: "seeked" });
          this.#beat(undefined, { seekToFirstFrameMs });
        },
      ],
      ["timeupdate", () => this.#engine && dispatch({ type: "time", positionMs: this.positionMs() })],
    ];
    for (const [type, fn] of handlers) media.addEventListener(type, fn);
    const pictureInPicture = () => this.#followPictureInPicture();
    for (const type of ["enterpictureinpicture", "leavepictureinpicture"]) {
      media.addEventListener(type, pictureInPicture);
    }
    this.#removeListeners = () => {
      for (const [type, fn] of handlers) media.removeEventListener(type, fn);
      for (const type of ["enterpictureinpicture", "leavepictureinpicture"]) {
        media.removeEventListener(type, pictureInPicture);
      }
    };
  }

  /** The position on the version's timeline. */
  positionMs(): number {
    if (this.#engine) this.#lastPositionMs = this.#engine.positionMs();
    return this.#lastPositionMs;
  }

  /** The play the player has, once one started. */
  get started(): PlayStarted | undefined {
    return this.#play;
  }

  /** Starts a play, ending the current one first. A play started paused loads and waits. */
  async play(request: PlayRequest, { paused = false }: { paused?: boolean } = {}): Promise<void> {
    const generation = this.#end();
    this.#request = request;
    this.#startPaused = paused;
    this.#shownSubtitle = undefined;
    this.#shownAudio = undefined;
    this.#fallbacks = 0;
    const pictureInPicture = this.#inPictureInPicture();
    if (request.positionMs !== undefined) this.#lastPositionMs = request.positionMs;
    this.#dispatch({ type: "start" });
    try {
      const auto = request.localQuality?.mode === "auto" || request.remoteQuality?.mode === "auto";
      const bandwidthBps = auto ? (lastBandwidth() ?? (await measureBandwidth())) : undefined;
      const localQuality = withBandwidth(request.localQuality, bandwidthBps);
      const remoteQuality = withBandwidth(request.remoteQuality, bandwidthBps);
      const body: PlayCreate = {
        workId: request.workId,
        capabilities: await capabilityReport({ pictureInPicture }),
        settings: { localQuality, remoteQuality },
        // An empty version is the server's pick, as is one left out.
        versionId: request.versionId || undefined,
        audioTrackId: request.audioTrackId,
        subtitleTrackId: request.subtitleTrackId,
        positionMs: request.positionMs === undefined ? undefined : Math.round(request.positionMs),
        dialogueBoost: request.dialogueBoost,
      };
      const play = await startPlay(body);
      if (generation !== this.#generation) {
        // Superseded while starting, as StrictMode's double effects do in development.
        stopPlay(play.id, play.startMs).catch(() => {});
        return;
      }
      this.#playInPictureInPicture = pictureInPicture;
      await this.#load(play, generation);
    } catch (error) {
      if (generation === this.#generation) this.#dispatch({ type: "fail", failure: failureOf(error) });
    }
  }

  /** Plays the current request again from the current position, with changes such as another audio track. */
  switchTo(changes: Partial<PlayRequest>): Promise<void> {
    if (!this.#request) return Promise.resolve();
    return this.play({ ...this.#request, ...changes, positionMs: changes.positionMs ?? this.positionMs() });
  }

  /** Plays other tracks from the current position. It stays on the play's version, because a track belongs to one version. */
  switchTracks(tracks: Pick<PlayRequest, "audioTrackId" | "subtitleTrackId">): Promise<void> {
    return this.#onThisVersion(tracks);
  }

  /** Plays on from the current position with the dialogue boost on or off. The server applies the boost only to a new play. */
  boostDialogue(on: boolean): Promise<void> {
    return this.#onThisVersion({ dialogueBoost: on });
  }

  /** Plays the current request again from the current position, with changes, on the play's version. */
  #onThisVersion(changes: Partial<PlayRequest>): Promise<void> {
    const versionId = this.#play?.versionId ?? this.#request?.versionId;
    return this.switchTo({ ...changes, versionId });
  }

  /**
   * Records a subtitle the play shows without a new play, as its delivery
   * says. The heartbeats report it, so the server remembers it as a choice,
   * and a later switch or retry keeps it.
   */
  showSubtitle(trackId: string): void {
    this.#shownSubtitle = trackId;
    if (this.#request) this.#request = { ...this.#request, subtitleTrackId: trackId };
    this.#beat();
    this.#followPictureInPicture();
  }

  /**
   * Plays an audio track of the file without a new play, for a track whose
   * delivery is `file`. The heartbeats report it, as they do a subtitle.
   */
  selectAudio(trackId: string): void {
    this.#shownAudio = trackId;
    if (this.#request) this.#request = { ...this.#request, audioTrackId: trackId };
    this.#engine?.selectAudio?.(trackId);
    this.#beat();
  }

  /** Tries the last request again after a failure, from where it stopped. */
  retry(): Promise<void> {
    return this.switchTo({});
  }

  seek(positionMs: number): void {
    const engine = this.#engine;
    const duration = this.#play?.durationMs;
    if (!engine || duration === undefined) return;
    const target = Math.min(Math.max(0, positionMs), duration);
    this.#seekStartedAt = performance.now();
    this.#dispatch({ type: "seek", positionMs: target });
    engine.seek(target);
  }

  skip(deltaMs: number): void {
    this.seek(this.positionMs() + deltaMs);
  }

  togglePlay(): void {
    if (this.#media.paused) this.resume();
    else this.pause();
  }

  resume(): void {
    if (!this.#engine) return;
    if (this.#media.ended) this.seek(0);
    this.#media.play().catch(() => this.#dispatch({ type: "pause" }));
  }

  pause(): void {
    if (this.#engine) this.#media.pause();
  }

  /** Shows the subtitle track with this ID from the media itself, or none. */
  showMediaSubtitle(trackId: string | undefined): void {
    this.#mediaSubtitle = trackId;
    this.#engine?.showSubtitle(trackId);
  }

  /**
   * Handles a play that the live events say ended elsewhere. If it is this
   * play (an admin or another client stopped it, or it expired), the player
   * stops without a stop request and shows why. A play this controller ended
   * itself, such as the one a switch replaces, is no longer current and is
   * ignored.
   */
  playEnded(playId: string, message?: string): void {
    if (this.#play?.id !== playId) return;
    this.#generation++;
    this.#release();
    this.#play = undefined;
    this.#dispatch({ type: "fail", failure: { kind: "ended", message } });
  }

  /** Stops the play as the page goes away, with a request that outlives the page. */
  stopOnExit(): void {
    const play = this.#play;
    if (!play) return;
    stopPlay(play.id, this.positionMs(), { keepalive: true }).catch(() => {});
    this.#play = undefined;
    this.#generation++;
    this.#release();
  }

  /** Starts again where it was when the page comes back from the back/forward cache after stopOnExit. */
  restore(): void {
    if (this.#request && !this.#play) void this.retry();
  }

  /**
   * Stops the play as the app leaves the screen without leaving the page,
   * as a TV's standby, Home key or input switch does. A suspended app sends
   * no heartbeats, so the play would expire on the server after two
   * minutes and the player would show it ended elsewhere. Stopping it here
   * saves the position, as the stop carries it, and resumeFromSuspend
   * starts again from there.
   */
  suspend(): void {
    if (!this.#play) return;
    this.#suspended = { paused: this.#media.paused };
    this.#media.pause();
    this.stopOnExit();
  }

  /** Starts again where suspend stopped, paused if it was paused, as a play resumes. */
  resumeFromSuspend(): void {
    const suspended = this.#suspended;
    this.#suspended = undefined;
    if (!suspended || !this.#request || this.#play) return;
    void this.play({ ...this.#request, positionMs: this.positionMs() }, { paused: suspended.paused });
  }

  /** Stops the play and releases everything. */
  dispose(): void {
    this.#end();
    this.#removeListeners();
  }

  /** Ends the current play, if any, and returns the next generation. */
  #end(): number {
    const play = this.#play;
    // A stop is never awaited. If it fails, the play expires on the server.
    if (play) stopPlay(play.id, this.positionMs()).catch(() => {});
    this.#play = undefined;
    this.#release();
    return ++this.#generation;
  }

  /**
   * Ends the play after its last failure, with a stop that says what
   * failed, so the server logs the failure that ended playback and not just
   * a stop. The engine is already released, so the position is the one
   * taken as the failure happened.
   */
  #endFailed(failure: EngineFailure, positionMs: number): void {
    const play = this.#play;
    if (play) {
      stopPlay(play.id, positionMs, { failure: { code: failure.code, detail: failure.detail.slice(0, 1000) } }).catch(
        () => {},
      );
    }
    this.#play = undefined;
    ++this.#generation;
  }

  /** Releases the engine and the heartbeat timer, keeping the play. */
  #release(): void {
    this.positionMs();
    clearInterval(this.#timer);
    this.#timer = undefined;
    this.#engine?.destroy();
    this.#engine = undefined;
    this.#seekStartedAt = undefined;
    this.#waiting = false;
  }

  async #load(play: PlayStarted, generation: number): Promise<void> {
    this.#play = play;
    this.#lastPositionMs = play.startMs;
    this.#dispatch({ type: "started", play });
    const current = () => generation === this.#generation;
    const engine = await attachEngine(this.#media, play, {
      onReady: () => {
        if (!current()) return;
        this.#dispatch({ type: "ready" });
        if (this.#startPaused) {
          this.#startPaused = false;
          this.#dispatch({ type: "pause" });
          return;
        }
        this.#media.play().catch(() => this.#dispatch({ type: "pause" }));
      },
      onEnded: () => {
        if (!current()) return;
        this.#dispatch({ type: "ended" });
        this.#beat("ended");
        this.#onEnded();
      },
      onFailure: (failure) => {
        if (current()) void this.#fallBack(failure);
      },
    });
    if (!current()) {
      engine?.destroy();
      return;
    }
    this.#engine = engine;
    engine?.showSubtitle(this.#mediaSubtitle);
    this.#timer = setInterval(() => this.#beat(), heartbeatMs);
    // Picture-in-picture may have started or ended while the play was on its way.
    this.#followPictureInPicture();
  }

  #inPictureInPicture(): boolean {
    return document.pictureInPictureElement === this.#media;
  }

  /** The subtitle choice showing: the one shown since the play started, or the play's own. */
  #subtitleShowing(): PlayTrack | undefined {
    const tracks = this.#play?.tracks.filter((t) => t.type === "subtitle") ?? [];
    const id = this.#shownSubtitle ?? tracks.find((t) => t.selected)?.trackId;
    return tracks.find((t) => t.trackId === id);
  }

  /** Starts a new play at the position when picture-in-picture started or ended and the subtitle needs one to show. */
  #followPictureInPicture(): void {
    if (!this.#engine) return;
    const needed = pictureInPictureNeedsNewPlay(
      this.#inPictureInPicture(),
      this.#playInPictureInPicture,
      this.#subtitleShowing(),
    );
    if (needed) void this.#onThisVersion({ subtitleTrackId: this.#subtitleShowing()?.trackId });
  }

  /**
   * Handles a media failure. The server ends the play and retries it as a
   * transcode from the same position, first on an accelerator and then on
   * the CPU, until it has nothing left to try or the play has fallen back
   * maxFallbacks times. Then the player shows the error. A book's player has
   * no transcode to try, so it shows that the book can't play.
   */
  async #fallBack(failure: EngineFailure): Promise<void> {
    const play = this.#play;
    if (!play) return;
    const positionMs = Math.round(this.positionMs());
    this.#release();
    const generation = ++this.#generation;
    const retry = this.#canFallBack && this.#fallbacks < maxFallbacks;
    if (retry) this.#fallbacks++;
    if (retry) this.#dispatch({ type: "fallback", detail: failure.detail });
    try {
      if (!retry) {
        this.#endFailed(failure, positionMs);
        const code = this.#canFallBack ? "noFallback" : "notPlayable";
        this.#dispatch({ type: "fail", failure: { kind: "problem", code }, detail: failure.detail });
        return;
      }

      const next = await fallbackPlay(play.id, {
        positionMs,
        code: failure.code,
        detail: failure.detail.slice(0, 1000),
      });
      if (generation !== this.#generation) {
        stopPlay(next.id, next.startMs).catch(() => {});
        return;
      }
      await this.#load(next, generation);
    } catch (error) {
      if (generation !== this.#generation) return;
      if (isGone(error)) {
        // The server restarted and forgot the play, so start a new one.
        this.#play = undefined;
        void this.switchTo({ positionMs });
        return;
      }
      this.#end();
      this.#dispatch({ type: "fail", failure: failureOf(error) });
    }
  }

  #beat(state?: PlayUpdate["state"], extra: Partial<PlayUpdate> = {}): void {
    const play = this.#play;
    if (!play || !this.#engine) return;
    const bandwidthBps = this.#engine.bandwidthBps();
    if (bandwidthBps !== undefined && !play.local) rememberBandwidth(bandwidthBps);
    const body: PlayUpdate = {
      positionMs: Math.round(this.positionMs()),
      state: state ?? (this.#media.paused ? "paused" : this.#waiting ? "buffering" : "playing"),
      bandwidthBps,
      subtitleTrackId: this.#shownSubtitle,
      audioTrackId: this.#shownAudio,
      ...extra,
    };
    updatePlay(play.id, body).catch((error: unknown) => {
      // An unknown play means the server restarted, so start again at the saved position.
      if (isGone(error) && play === this.#play) {
        this.#play = undefined;
        void this.switchTo({ positionMs: body.positionMs });
      }
    });
  }
}
