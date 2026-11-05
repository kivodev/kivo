import type { CapabilityMismatch, PlayStarted, PlaySummary } from "@/api/schemas";

// The player's state is a small reducer over idle, loading, playing,
// buffering, seeking and error. Pause is a flag rather than a state. A
// paused player is still playing in the sense that matters here: its media
// is loaded and its controls are live.

export type PlayerStatus = "idle" | "loading" | "playing" | "buffering" | "seeking" | "error";

/** Why the player stopped. */
export type PlayerFailure =
  /** The server refused. It gives a problem code, and for a playback refusal a reason code and the capability that failed. */
  | { kind: "problem"; code: string; reason?: string; mismatch?: CapabilityMismatch; plays?: readonly PlaySummary[] }
  /** Stopped elsewhere: by an admin (with their message, if any), from another client, or by expiry. */
  | { kind: "ended"; message?: string };

export type PlayerState = {
  status: PlayerStatus;
  play?: PlayStarted;
  paused: boolean;
  /** The position on the version's timeline. While seeking, it is the target. */
  positionMs: number;
  ended: boolean;
  failure?: PlayerFailure;
  /** What the media element said when it failed, for diagnostics. */
  detail?: string;
};

export type PlayerAction =
  /** A play is being requested, in place of the one there is, if any. */
  | { type: "start" }
  /** The server answered with a play, from a start or a fallback, and its media loads. */
  | { type: "started"; play: PlayStarted }
  /** Media failed and a fallback was asked for. */
  | { type: "fallback"; detail?: string }
  /** The media is loaded at the start position and can play. */
  | { type: "ready" }
  | { type: "play" }
  | { type: "pause" }
  /** Playback stalled waiting for data. */
  | { type: "waiting" }
  /** Frames are moving again. */
  | { type: "playing" }
  /** The user moved the position. */
  | { type: "seek"; positionMs: number }
  | { type: "seeked" }
  | { type: "time"; positionMs: number }
  | { type: "ended" }
  | { type: "fail"; failure: PlayerFailure; detail?: string }
  | { type: "reset" };

export const initialPlayerState: PlayerState = {
  status: "idle",
  paused: true,
  positionMs: 0,
  ended: false,
};

/** Whether media is loaded, so controls act on it. */
export function isLive(status: PlayerStatus): boolean {
  return status === "playing" || status === "buffering" || status === "seeking";
}

export function playerReducer(state: PlayerState, action: PlayerAction): PlayerState {
  switch (action.type) {
    // A switch shows the play it replaces, menus and all, until the server answers.
    case "start":
      return {
        ...initialPlayerState,
        status: "loading",
        positionMs: state.positionMs,
        play: state.play,
      };
    case "started":
      return {
        ...state,
        status: "loading",
        play: action.play,
        positionMs: action.play.startMs,
        ended: false,
      };
    case "fallback":
      return {
        ...state,
        status: "loading",
        detail: action.detail ?? state.detail,
      };
    case "ready":
      return state.status === "loading" ? { ...state, status: "playing" } : state;
    case "play":
      return { ...state, paused: false, ended: false };
    case "pause":
      return { ...state, paused: true };
    case "waiting":
      return state.status === "playing" ? { ...state, status: "buffering" } : state;
    case "playing":
      if (state.status === "error" || state.status === "idle") return state;
      return {
        ...state,
        status: state.status === "seeking" ? "seeking" : "playing",
        paused: false,
      };
    case "seek":
      return isLive(state.status)
        ? { ...state, status: "seeking", positionMs: action.positionMs, ended: false }
        : state;
    case "seeked":
      return state.status === "seeking" ? { ...state, status: "playing" } : state;
    case "time":
      return isLive(state.status) && state.status !== "seeking" ? { ...state, positionMs: action.positionMs } : state;
    case "ended":
      return isLive(state.status) ? { ...state, status: "playing", paused: true, ended: true } : state;
    case "fail":
      return {
        ...state,
        status: "error",
        failure: action.failure,
        paused: true,
        detail: action.detail ?? state.detail,
      };
    case "reset":
      return initialPlayerState;
  }
}
