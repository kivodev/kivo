import { display, lookup } from "./display";
import { describeReason } from "./playback";

// The server sends errors as stable codes. The shared display table has a
// sentence for each code, which the client shows in its language.
// `network` is the client's own code for a request that got no answer.

/**
 * A sentence for a problem code. A code this version
 * doesn't know says that something went wrong.
 */
export function describeProblem(code: string): string {
  return lookup(display.problems, code) ?? display.unknownProblem;
}

/**
 * A sentence for a failed subtitle download, from its problem code. When
 * the quota is used up, it says when more is available, at `time` if the
 * server knows.
 */
export function describeSubtitleFailure(code: string, time?: string): string {
  const words = display.subtitleSearch;
  switch (code) {
    case "notConfigured":
      return words.notConfigured;
    case "quotaUsed":
      return time ? words.quotaUsedUntil.replace("{time}", time) : words.quotaUsed;
    case "notFound":
      return words.notFound;
    case "providerUnavailable":
      return words.providerUnavailable;
    default:
      return describeProblem(code);
  }
}

/** A play stopped elsewhere, by another device or an admin, with the admin's message if there is one. */
export function describePlayStopped(message?: string): string {
  return message ? display.adminStopped.replace("{message}", message) : display.playStopped;
}

/** Why a play failed. It is the server's reason for a refused play, or else the play's problem. */
export function describePlayFailure({ code, reason }: { code: string; reason?: string }): string {
  return reason ? describeReason(reason) : describeProblem(code);
}
