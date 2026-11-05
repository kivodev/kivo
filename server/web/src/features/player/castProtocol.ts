import { i18n } from "@lingui/core";
import { msg } from "@lingui/core/macro";
import type { CastToken, DialogueBoost, PlayTrack, QualityRequest } from "@/api/schemas";
import { describeProblem } from "@/i18n/problems";
import { type UpNextState, upNextHidden } from "./upNext";

// The contract with Kivo's Web Receiver, matching the native apps' core
// (clients/core cast/Cast.kt). It covers a LOAD that names the work, the
// server and a cast token, the receiver's status, and Kivo's messages on
// castNamespace. The token is sent only in the LOAD and in `kivo.token`.

/** The Google Cast app that senders launch, which is Kivo's own Web Receiver. */
export const castAppId = "2457CC30";

/** The namespace of Kivo's own messages between senders and the receiver. */
export const castNamespace = "urn:x-cast:com.mmcdole.kivo";

/** The version of the contract with the receiver and the server. */
const castProtocol = 3;

/** What a cast plays: the work, its version and tracks, where it starts, and the choices it carries to the TV. */
export type CastRequest = {
  workId: string;
  /** Absent or empty lets the server pick. */
  versionId?: string;
  audioTrackId?: string;
  /** A track's ID or `off`. Absent means the server's default. */
  subtitleTrackId?: string;
  /** Where to start. Absent means the server's resume position. */
  positionMs?: number;
  dialogueBoost?: boolean;
  localQuality?: QualityRequest;
  remoteQuality?: QualityRequest;
  /** Episodes played on without input before the cast, so "Still watching?" keeps counting on the TV. */
  autoplayCount?: number;
};

/** A LOAD for the receiver. It names the work as its content ID and entity, and gives the start in seconds and the custom data. */
export type CastLoad = {
  contentId: string;
  entity: string;
  currentTimeSec: number | undefined;
  customData: Record<string, unknown>;
};

/** The LOAD for `request`. It carries `token` and the token's server ID and addresses. */
export function castLoad(token: CastToken, request: CastRequest): CastLoad {
  const customData: Record<string, unknown> = {
    protocol: castProtocol,
    serverId: token.serverId,
    urls: token.urls,
    token: token.token,
    versionId: request.versionId || undefined,
    audioTrackId: request.audioTrackId,
    subtitleTrackId: request.subtitleTrackId,
    dialogueBoost: request.dialogueBoost ?? false,
    localQuality: request.localQuality ?? { mode: "auto" },
    remoteQuality: request.remoteQuality ?? { mode: "auto" },
    autoplayCount: request.autoplayCount ?? 0,
  };
  return {
    contentId: request.workId,
    entity: `kivo://work/${request.workId}`,
    currentTimeSec: request.positionMs === undefined ? undefined : request.positionMs / 1000,
    customData,
  };
}

/** The messages senders send the receiver on castNamespace. */
export const castMessages = {
  /** Another audio track, a subtitle track or `off`, or both. */
  setTracks: (tracks: { audioTrackId?: string; subtitleTrackId?: string }) => ({ type: "kivo.setTracks", ...tracks }),
  setDialogueBoost: (enabled: boolean) => ({ type: "kivo.setDialogueBoost", enabled }),
  setQuality: (localQuality: QualityRequest, remoteQuality: QualityRequest) => ({
    type: "kivo.setQuality",
    localQuality,
    remoteQuality,
  }),
  /** Another version, with its own default tracks. */
  setVersion: (versionId: string) => ({ type: "kivo.setVersion", versionId }),
  /** Up next's Play now and Hide. */
  playNext: () => ({ type: "kivo.playNext" }),
  hideUpNext: () => ({ type: "kivo.hideUpNext" }),
  /** A fresh cast token for the receiver's play, from a sender that reconnects. */
  token: (token: string) => ({ type: "kivo.token", token }),
  getStatus: () => ({ type: "kivo.getStatus" }),
};

/** What the receiver says of its cast, from `kivo.status` or a media status' custom data. */
export type CastStatus = {
  workId: string | undefined;
  nextWorkId: string | undefined;
  /** Set while the receiver has a play. */
  versionId: string | undefined;
  /** The receiver's play's tracks, as `PlayStarted.tracks` gives them. */
  tracks: PlayTrack[];
  quality: QualityRequest | undefined;
  limitBps: number | undefined;
  dialogueBoost: DialogueBoost | undefined;
  audioTrackId: string | undefined;
  /** The subtitle showing, or `off`. Absent when there is no play. */
  subtitleTrackId: string | undefined;
  /** Whether the receiver plays from the server's local network. */
  local: boolean | undefined;
  /** A mirror of up next's card on the TV. Play now and Hide act on it. */
  upNext: UpNextState;
  /** Why the cast stopped or can't go on, such as `serverUnreachable`. Absent while the cast is healthy. */
  error: string | undefined;
  /** An admin's words when they stopped the play. */
  message: string | undefined;
};

const str = (v: unknown) => (typeof v === "string" ? v : undefined);
const obj = (v: unknown) =>
  v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : undefined;

/**
 * The receiver's status in `data`. The data may be a `kivo.status` message
 * or a media status's custom data, each with the status under `status`, or
 * the status itself. It returns undefined when the data holds no status.
 */
export function castStatus(data: unknown): CastStatus | undefined {
  let parsed = data;
  if (typeof data === "string") {
    try {
      parsed = JSON.parse(data);
    } catch {
      return undefined;
    }
  }
  const outer = obj(parsed);
  if (!outer) return undefined;
  const s = obj(outer.status) ?? outer;
  const versionId = str(s.versionId);
  return {
    workId: str(s.workId),
    nextWorkId: str(s.nextWorkId),
    versionId,
    tracks: Array.isArray(s.tracks) ? (s.tracks as PlayTrack[]) : [],
    quality: obj(s.quality) as QualityRequest | undefined,
    limitBps: typeof s.limitBps === "number" ? s.limitBps : undefined,
    dialogueBoost: obj(s.dialogueBoost) as DialogueBoost | undefined,
    audioTrackId: str(s.audioTrackId),
    subtitleTrackId: str(s.subtitleTrackId) ?? (versionId ? "off" : undefined),
    local: typeof s.local === "boolean" ? s.local : undefined,
    upNext: castUpNext(obj(s.upNext)),
    error: str(s.error),
    message: str(s.message),
  };
}

/** The receiver's up next. */
function castUpNext(upNext: Record<string, unknown> | undefined): UpNextState {
  switch (str(upNext?.phase)) {
    case "countingDown":
      return {
        phase: "countingDown",
        secondsLeft: typeof upNext?.secondsLeft === "number" ? upNext.secondsLeft : 0,
        atCredits: false,
      };
    case "stillWatching":
      return { phase: "stillWatching", atCredits: false };
    case "dismissed":
      return { phase: "dismissed" };
    default:
      return upNextHidden;
  }
}

/** The receiver can't reach the server, and keeps trying. */
export const unreachable = "serverUnreachable";

/** The receiver's error in words. When an admin stopped the play, their own message is used. */
export function castFailure(code: string, message?: string): string {
  if (message) return message;
  switch (code) {
    case unreachable:
      return i18n._(msg`Can't reach the server.`);
    case "signedOut":
      return i18n._(msg`Signed out on the TV.`);
    case "playEnded":
      return i18n._(msg`Playback was stopped.`);
    case "serverOutdated":
      return i18n._(msg`Update the server to cast to this TV.`);
    case "receiverOutdated":
      return i18n._(msg`The TV's receiver is older than the server.`);
    case "notPlayable":
    case "noFallback":
    case "playbackFailed":
      return i18n._(msg`This can't be played on this TV.`);
    default:
      return describeProblem(code);
  }
}
