import type { SkipMarker } from "./markers";

// Up next follows the shared core's rules for an episode that has a next
// episode. Its card appears a fixed time before the end, or at the final
// credits. At the final credits it counts down 10 seconds and plays the next
// episode. Otherwise it counts the time left, and the next episode plays when
// this one ends. A run of episodes played on without input brings
// "Still watching?". The card is a state machine, `upNextReducer`. The watch
// route keeps the run across the players of its episodes.

/** Seconds up next counts down at the final credits before the next episode plays. */
export const upNextCountdownSeconds = 10;
/** An episode shorter than this has no up next card. */
const upNextMinEpisodeMs = 10 * 60_000;
/** Up next appears only with at least this long left, so a seek to the last moments doesn't bring it. */
const upNextMinLeftMs = 20_000;
/** Up next plays on by itself this many times in a row before asking "Still watching?". */
const autoplayLimit = 3;

/** The credits up next appears at. These are the last credits, when they are final credits. */
export function upNextCredits(markers: readonly SkipMarker[]): SkipMarker | undefined {
  const credits = markers.findLast((m) => m.kind === "credits");
  return credits?.final ? credits : undefined;
}

/**
 * Where up next appears in an episode of `durationMs`. It appears 30 seconds
 * before the end, 35 for an episode of 40 minutes or more, and 40 for 50
 * minutes or more. It appears where the final credits start
 * (`creditsStartMs`) if that is sooner. An episode shorter than 10 minutes
 * returns undefined.
 */
export function upNextAtMs(durationMs: number, creditsStartMs?: number): number | undefined {
  if (durationMs < upNextMinEpisodeMs) return undefined;
  const leadMs = durationMs >= 50 * 60_000 ? 40_000 : durationMs >= 40 * 60_000 ? 35_000 : 30_000;
  return Math.min(durationMs - leadMs, creditsStartMs ?? Infinity);
}

/**
 * The state of up next's card.
 *
 * - `hidden`: before its time.
 * - `countingDown`: counting down to the next episode. At the credits it
 *   counts 10 seconds, and otherwise the seconds left in the episode.
 * - `waiting`: waiting for Play now, because the user's autoplay is off.
 * - `stillWatching`: asking "Still watching?" after the run.
 * - `dismissed`: hidden by Hide for the rest of the episode. The episode
 *   still plays on at its end as autoplay allows.
 *
 * At the credits, the card's second button is "Watch credits" rather than "Hide".
 * The card is at the credits only when it appeared at their start, not when
 * credits shorter than its lead start after it.
 */
export type UpNextState =
  | { phase: "hidden" }
  | { phase: "countingDown"; secondsLeft: number; atCredits: boolean }
  | { phase: "waiting"; atCredits: boolean }
  | { phase: "stillWatching"; atCredits: boolean }
  | { phase: "dismissed" };

/** The card while it shows. */
export type UpNextShown = Extract<UpNextState, { atCredits: boolean }>;

/** How the card appears: counting down, waiting, or asking "Still watching?". */
type UpNextAppears = UpNextShown["phase"];

/**
 * An event that changes the card.
 *
 * - `position`: playback moved, either reaching the card's time or going back before it.
 * - `tick`: a second passed. It counts down at the credits only while `running`.
 * - `hide`: the user chose Hide or Watch credits.
 * - `ended`: the episode ended without playing on.
 */
export type UpNextEvent =
  | {
      type: "position";
      positionMs: number;
      durationMs: number;
      /** The final credits the card appears at, when the episode has them. */
      credits: SkipMarker | undefined;
      appears: UpNextAppears;
    }
  | { type: "tick"; running: boolean }
  | { type: "hide" }
  | { type: "ended"; appears: Exclude<UpNextAppears, "countingDown"> };

export const upNextHidden: UpNextState = { phase: "hidden" };
const upNextDismissed: UpNextState = { phase: "dismissed" };

/**
 * The card after `event`. It returns the same object when nothing changes.
 *
 * - `position`: the card appears at `upNextAtMs` as `appears`, if 20 seconds
 *   or more are left. A position before its time hides it, and the countdown
 *   starts over the next time it appears. Away from the credits, a counting
 *   down card follows the seconds left in the episode.
 * - `tick`: a second off the countdown at the credits.
 * - `hide`: the card stays away for the rest of the episode.
 * - `ended`: the card waits for an answer, as it appears.
 */
export function upNextReducer(state: UpNextState, event: UpNextEvent): UpNextState {
  switch (event.type) {
    case "position": {
      const { positionMs, durationMs, credits, appears } = event;
      const showAt = upNextAtMs(durationMs, credits?.startMs);
      if (positionMs < (showAt ?? durationMs - upNextMinLeftMs)) return upNextHidden;
      if (state.phase === "countingDown" && !state.atCredits) {
        const secondsLeft = secondsToEnd(positionMs, durationMs);
        return secondsLeft === state.secondsLeft ? state : { ...state, secondsLeft };
      }
      if (state.phase !== "hidden" || showAt === undefined || durationMs - positionMs < upNextMinLeftMs) return state;
      const atCredits = credits !== undefined && showAt === credits.startMs;
      if (appears !== "countingDown") return { phase: appears, atCredits };
      const secondsLeft = atCredits ? upNextCountdownSeconds : secondsToEnd(positionMs, durationMs);
      return { phase: "countingDown", secondsLeft, atCredits };
    }
    case "tick":
      return state.phase === "countingDown" && state.atCredits && event.running && state.secondsLeft > 0
        ? { ...state, secondsLeft: state.secondsLeft - 1 }
        : state;
    case "hide":
      return upNextShows(state) ? upNextDismissed : state;
    case "ended":
      return { phase: event.appears, atCredits: false };
  }
}

/** The whole seconds left in an episode of `durationMs` at `positionMs`. */
function secondsToEnd(positionMs: number, durationMs: number): number {
  return Math.max(0, Math.ceil((durationMs - positionMs) / 1000));
}

/** Whether the card shows. */
export function upNextShows(state: UpNextState): state is UpNextShown {
  return state.phase === "countingDown" || state.phase === "waiting" || state.phase === "stillWatching";
}

/** Whether the countdown at the credits ran out, so the next episode plays now. Away from the credits, the episode's end plays it. */
export function upNextPlaysNext(state: UpNextState): boolean {
  return state.phase === "countingDown" && state.atCredits && state.secondsLeft === 0;
}

/**
 * How the card appears after `run` episodes played on without input. It
 * waits when the user's `autoplayNext` is off. Otherwise it counts down and
 * plays on by itself, until after three it asks "Still watching?".
 */
export function upNextAppears(run: number, autoplayNext: boolean): UpNextAppears {
  if (!autoplayNext) return "waiting";
  return run < autoplayLimit ? "countingDown" : "stillWatching";
}

/** The run once the next episode starts. It grows by one when the episode played on by itself, and resets when someone chose it. */
export function runAfterPlay(run: number, automatic: boolean): number {
  return automatic ? run + 1 : 0;
}
