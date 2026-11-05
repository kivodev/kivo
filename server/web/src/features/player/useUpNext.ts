import { useEffect, useEffectEvent, useMemo, useReducer } from "react";
import type { PlayStarted } from "@/api/schemas";
import { markersOf } from "./markers";
import {
  runAfterPlay,
  type UpNextEvent,
  upNextAppears,
  upNextCredits,
  upNextHidden,
  upNextPlaysNext,
  upNextReducer,
  upNextShows,
} from "./upNext";
import { useWatchNavigation } from "./watchSearch";

type Options = {
  nextId: string | undefined;
  play: PlayStarted | undefined;
  live: boolean;
  positionMs: number;
  /** Whether the countdown at the credits runs. It runs while playing with no menu or panel open. */
  counting: boolean;
  /** The user's `autoplayNext`. When it is off, the card waits for Play now. */
  autoplayNext: boolean;
  /** How many episodes have played on in a row without input. The watch page keeps it across episodes. */
  run: number;
  onRunChange: (run: number) => void;
};

/**
 * useUpNext runs up next for an episode. Its card follows the position and
 * appears at the final credits or a fixed time before the end. At the
 * credits it counts down 10 seconds while `counting` holds, and plays the
 * next episode when the countdown ends. Otherwise it counts the seconds left
 * in the episode. The next episode plays when the episode ends, as the run
 * allows.
 */
export function useUpNext({ nextId, play, live, positionMs, counting, autoplayNext, run, onRunChange }: Options) {
  const { playWork } = useWatchNavigation();
  const [state, dispatch] = useReducer(upNextReducer, upNextHidden);
  const credits = useMemo(() => (play ? upNextCredits(markersOf(play)) : undefined), [play]);
  const follows = nextId !== undefined && play !== undefined && live;
  const appears = upNextAppears(run, autoplayNext);
  // The card follows the position as the player renders, so it takes Skip
  // Credits' place in the same frame.
  if (follows) {
    const position: UpNextEvent = { type: "position", positionMs, durationMs: play.durationMs, credits, appears };
    if (upNextReducer(state, position) !== state) dispatch(position);
  }

  const tick = useEffectEvent(() => dispatch({ type: "tick", running: counting }));
  const countingDown = state.phase === "countingDown" && state.atCredits;
  useEffect(() => {
    if (!countingDown) return;
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [countingDown]);

  const playNext = (automatic: boolean) => {
    if (nextId === undefined) return;
    onRunChange(runAfterPlay(run, automatic));
    playWork(nextId);
  };
  const playsNext = upNextPlaysNext(state);
  const playOnItsOwn = useEffectEvent(() => playNext(true));
  useEffect(() => {
    if (playsNext) playOnItsOwn();
  }, [playsNext]);

  const card = follows && upNextShows(state) ? state : undefined;
  return {
    /** The card, while it shows. */
    card,
    shows: card !== undefined,
    /** Whether it asks "Still watching?". */
    stillWatching: card?.phase === "stillWatching",
    /** Play now, Next, or the next track key. Someone chose the next episode, which ends the run. */
    playNow: () => playNext(false),
    /** Hide, or Watch credits: someone is watching, so up next may play on again. */
    hide: () => {
      onRunChange(0);
      dispatch({ type: "hide" });
    },
    /** Any input means someone is watching, so up next may play on again. */
    input: () => onRunChange(0),
    /** The episode played to its end. The next one plays on, or the card waits for an answer. */
    ended: () => (appears === "countingDown" ? playNext(true) : dispatch({ type: "ended", appears })),
  };
}
