import { createCastToken } from "@/api/cast";
import { isProblem } from "@/api/problem";
import { describeProblem } from "@/i18n/problems";
import {
  type CastRequest,
  type CastStatus,
  castAppId,
  castFailure,
  castLoad,
  castMessages,
  castNamespace,
  castStatus,
  unreachable,
} from "./castProtocol";
import {
  type CastFramework,
  type CastLabel,
  type CastSession,
  loadCastSdk,
  type RemotePlayer,
  type RemotePlayerController,
} from "./castSdk";

// CastLink is the Cast session as the player sees it. It tracks whether
// Cast devices are found, the session with one, and what Kivo's receiver
// plays there. A LOAD carries a cast token, so the TV plays as this user.
// The receiver starts its play, sends its heartbeats and changes the play
// itself. CastLink only mirrors the receiver's status and sends it messages.
// There is one per page, outside React, and React reads its snapshots.

const mediaNamespace = "urn:x-cast:com.google.cast.media";

export type CastConnection = "unavailable" | "disconnected" | "connecting" | "connected";

/** What plays on the TV, where and how. */
type CastMedia = {
  workId: string;
  state: "loading" | "buffering" | "playing" | "paused";
  positionMs: number;
  durationMs: number;
  rate: number;
};

export type CastSnapshot = {
  connection: CastConnection;
  /** The Cast device's name, such as "Living Room TV". */
  device: string | undefined;
  /** The TV's volume, 0 to 1. */
  volume: number;
  media: CastMedia | undefined;
  status: CastStatus | undefined;
  /** The work of a LOAD on its way, until the receiver's media names it. */
  loading: string | undefined;
  /** Why the cast stopped or couldn't start, in words. */
  failure: string | undefined;
};

/** What plays in this browser. A session that starts carries it on to the TV. */
export type LocalPlay = Omit<CastRequest, "positionMs"> & { positionMs: number };

export const unavailableCast: CastSnapshot = {
  connection: "unavailable",
  device: undefined,
  volume: 0,
  media: undefined,
  status: undefined,
  loading: undefined,
  failure: undefined,
};

export class CastLink {
  #snapshot = unavailableCast;
  readonly #listeners = new Set<() => void>();
  #sdk: CastFramework | undefined;
  #player: RemotePlayer | undefined;
  #controller: RemotePlayerController | undefined;
  #session: CastSession | undefined;
  #local: (() => LocalPlay | undefined) | undefined;
  /** What played in this browser when the session started, for the TV to carry on with. */
  #handoff: LocalPlay | undefined;
  /** The last cast asked for. Try again uses it before the TV has a play. */
  #last: { request: CastRequest; label: CastLabel } | undefined;
  #started = false;

  readonly #onMessage = (_namespace: string, message: string) => {
    const status = castStatus(message);
    if (status) this.#statusChanged(status);
  };

  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  getSnapshot = () => this.#snapshot;

  /** Loads the sender library and follows the Cast state, once. */
  start(): void {
    if (this.#started) return;
    this.#started = true;
    void loadCastSdk(castAppId).then((sdk) => {
      if (!sdk) return;
      this.#sdk = sdk;
      const { player, controller } = sdk.newPlayer();
      this.#player = player;
      this.#controller = controller;
      controller.addEventListener("anyChanged", () => this.#mediaChanged());
      sdk.context.addEventListener("caststatechanged", () => this.#castStateChanged());
      sdk.context.addEventListener("sessionstatechanged", (e) => {
        if (e.sessionState === "SESSION_STARTED") this.#connected(false);
        if (e.sessionState === "SESSION_RESUMED") this.#connected(true);
      });
      this.#castStateChanged();
      if (sdk.context.getCurrentSession()) this.#connected(true);
    });
  }

  /** Sets how to read what the player plays here, for a session that starts during a play. */
  setLocal(local: (() => LocalPlay | undefined) | undefined): void {
    this.#local = local;
  }

  /** Returns what played here when the session started. The remote that carries it on takes it, once. */
  takeHandoff(workId: string): LocalPlay | undefined {
    const handoff = this.#handoff?.workId === workId ? this.#handoff : undefined;
    this.#handoff = undefined;
    return handoff;
  }

  /** Shows the list of Cast devices to connect to. */
  showDevices(): void {
    this.#sdk?.context.requestSession().catch(() => {});
  }

  /** Casts `request` with a new cast token. `label` names it in Google's own notifications. */
  async cast(request: CastRequest, label: CastLabel): Promise<void> {
    const session = this.#session;
    const device = this.#snapshot.device;
    if (!session || !this.#sdk || device === undefined) return;
    this.#last = { request, label };
    this.#set({ loading: request.workId, failure: undefined, status: undefined });
    try {
      const load = castLoad(await createCastToken(device), request);
      await session.loadMedia(this.#sdk.loadRequest({ ...load, label }));
    } catch (error) {
      if (this.#snapshot.loading !== request.workId) return;
      // A LOAD the TV refused fails without a problem document. Its reason comes in the status.
      this.#set({ loading: undefined, failure: describeProblem(isProblem(error) ? error.code : "network") });
    }
  }

  /** Casts the work again with a fresh token, from where the TV was. This is Try again. */
  retry(fallback: { request: CastRequest; label: CastLabel }): void {
    const { media, status } = this.#snapshot;
    const workId = media?.workId ?? status?.workId;
    const last = this.#last ?? fallback;
    if (workId && status?.workId === workId) {
      const positionMs = media?.positionMs;
      void this.cast(
        {
          ...last.request,
          workId,
          versionId: status.versionId,
          audioTrackId: status.audioTrackId,
          subtitleTrackId: status.subtitleTrackId,
          positionMs: positionMs || last.request.positionMs,
        },
        last.label,
      );
    } else {
      void this.cast(last.request, last.label);
    }
  }

  togglePlay(): void {
    this.#controller?.playOrPause();
  }

  seek(positionMs: number): void {
    const player = this.#player;
    const media = this.#snapshot.media;
    if (!player || !this.#controller || !media) return;
    const target = Math.min(Math.max(0, positionMs), Math.max(media.durationMs - 1000, 0));
    this.#set({ media: { ...media, positionMs: target } });
    player.currentTime = target / 1000;
    this.#controller.seek();
  }

  setVolume(volume: number): void {
    if (!this.#player || !this.#controller) return;
    this.#set({ volume });
    this.#player.volumeLevel = volume;
    this.#controller.setVolumeLevel();
  }

  setRate(rate: number): void {
    const media = this.#session?.getMediaSession();
    if (!media) return;
    void this.#session
      ?.sendMessage(mediaNamespace, {
        type: "SET_PLAYBACK_RATE",
        requestId: Date.now() % 1e9,
        mediaSessionId: media.mediaSessionId,
        playbackRate: rate,
      })
      .catch(() => {});
  }

  /** Sends a message on Kivo's namespace. */
  send(message: object): void {
    void this.#session?.sendMessage(castNamespace, message).catch(() => {});
  }

  /**
   * Ends the session. Stop casting stops the TV and returns what played
   * there, at the TV's position, so this browser can carry on with it.
   * Disconnect lets go, and the TV plays on.
   */
  end(stopCasting: boolean): CastRequest | undefined {
    const { media, status } = this.#snapshot;
    const workId = media?.workId ?? status?.workId;
    const same = status?.workId === workId ? status : undefined;
    const carry =
      stopCasting && workId
        ? {
            workId,
            versionId: same?.versionId,
            audioTrackId: same?.audioTrackId,
            subtitleTrackId: same?.subtitleTrackId,
            positionMs: media?.positionMs,
          }
        : undefined;
    this.#session?.endSession(stopCasting);
    this.#disconnected();
    return carry;
  }

  #set(changes: Partial<CastSnapshot>): void {
    this.#snapshot = { ...this.#snapshot, ...changes };
    for (const listener of this.#listeners) listener();
  }

  #castStateChanged(): void {
    const state = this.#sdk?.context.getCastState();
    const was = this.#snapshot.connection;
    switch (state) {
      case "CONNECTING":
        this.#set({ connection: "connecting" });
        return;
      case "CONNECTED":
        // The session's own events say whether it started or was joined again.
        return;
      case "NOT_CONNECTED":
        if (was === "connected" || was === "connecting") this.#disconnected();
        this.#set({ connection: "disconnected" });
        return;
      default:
        if (was === "connected") this.#disconnected();
        this.#set({ connection: "unavailable" });
    }
  }

  /** Handles a session that started here, or was joined again as the page came back. The receiver plays on with a fresh token. */
  #connected(resumed: boolean): void {
    const session = this.#sdk?.context.getCurrentSession() ?? undefined;
    if (!session) return;
    if (session !== this.#session) {
      this.#session?.removeMessageListener(castNamespace, this.#onMessage);
      session.addMessageListener(castNamespace, this.#onMessage);
      this.#session = session;
    }
    const device = session.getCastDevice().friendlyName;
    if (this.#snapshot.connection !== "connected") this.#handoff = resumed ? undefined : this.#local?.();
    this.#set({ connection: "connected", device, volume: session.getVolume() ?? this.#snapshot.volume });
    if (resumed) {
      createCastToken(device)
        .then((token) => this.send(castMessages.token(token.token)))
        .catch(() => {});
    }
    this.send(castMessages.getStatus());
    this.#mediaChanged();
  }

  #disconnected(): void {
    this.#session?.removeMessageListener(castNamespace, this.#onMessage);
    this.#session = undefined;
    this.#handoff = undefined;
    this.#set({
      connection: this.#snapshot.connection === "unavailable" ? "unavailable" : "disconnected",
      media: undefined,
      status: undefined,
      loading: undefined,
      failure: undefined,
    });
  }

  #mediaChanged(): void {
    const player = this.#player;
    const session = this.#session;
    if (!player || !session) return;
    const ms = session.getMediaSession();
    const workId = ms?.media?.contentId;
    let media: CastMedia | undefined;
    if (ms && workId) {
      const state = mediaState(ms.playerState, ms.idleReason);
      if (state) {
        media = {
          workId,
          state,
          positionMs: Math.round((player.isMediaLoaded ? player.currentTime : ms.getEstimatedTime()) * 1000),
          durationMs: Math.round(Math.max(0, player.duration || ms.media?.duration || 0) * 1000),
          rate: ms.playbackRate || 1,
        };
      }
    }
    const loading = media && media.workId === this.#snapshot.loading ? undefined : this.#snapshot.loading;
    this.#set({
      media,
      loading,
      volume: player.isMuted ? 0 : player.volumeLevel,
      ...(media ? { failure: undefined } : {}),
    });
    const status = ms?.customData ? castStatus(ms.customData) : undefined;
    if (status) this.#statusChanged(status);
  }

  #statusChanged(status: CastStatus): void {
    const failing = status.error !== undefined && status.error !== unreachable;
    this.#set({
      status,
      ...(failing ? { loading: undefined, failure: castFailure(status.error ?? "", status.message) } : {}),
    });
  }
}

/**
 * The receiver's player state in Kivo's terms. Idle with no reason is a load
 * under way. Idle because it finished, stopped or failed is undefined.
 */
function mediaState(playerState: string, idleReason: string | null | undefined): CastMedia["state"] | undefined {
  switch (playerState) {
    case "PLAYING":
      return "playing";
    case "PAUSED":
      return "paused";
    case "BUFFERING":
      return "buffering";
    case "IDLE":
      return idleReason ? undefined : "loading";
    default:
      return "loading";
  }
}

/** The page's one Cast session. */
export const castLink = /* @__PURE__ */ new CastLink();
