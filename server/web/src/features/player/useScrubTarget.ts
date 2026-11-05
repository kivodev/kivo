import { useEffect, useEffectEvent, useState } from "react";
import type { PlayStarted } from "@/api/schemas";
import type { PlayerController } from "./controller";
import { scrubStepMs } from "./keys";

/** A remote's scrub seeks to its target after this long without another step. */
const scrubCommitMs = 1000;

type Options = {
  play: PlayStarted | undefined;
  live: boolean;
  positionMs: number;
  /** Shows the controls. */
  reveal: () => void;
  run: (command: (controller: PlayerController) => void) => void;
};

/**
 * useScrubTarget tracks where a remote's Left and Right have moved the scrub
 * bar. Presses add up. The seek happens a moment after the last press, or at
 * once on OK.
 */
export function useScrubTarget({ play, live, positionMs, reveal, run }: Options) {
  const [target, setTarget] = useState<number>();
  const move = (deltaMs: number) => {
    reveal();
    if (!play || !live) return;
    const from = target ?? positionMs;
    setTarget(Math.min(play.durationMs, Math.max(0, from + deltaMs)));
  };
  /** Seeks to the target, if there is one, and returns whether there was. */
  const commit = () => {
    if (target === undefined) return false;
    setTarget(undefined);
    run((c) => c.seek(target));
    return true;
  };
  const commitLater = useEffectEvent(commit);
  useEffect(() => {
    if (target === undefined) return;
    const timer = setTimeout(commitLater, scrubCommitMs);
    return () => clearTimeout(timer);
  }, [target]);
  return {
    target,
    /** A step back or forward, longer as the key is held. */
    scrub: (direction: -1 | 1, repeatCount: number) => move(direction * scrubStepMs(repeatCount)),
    /** Back 10 or forward 30 seconds, from the target while there is one and otherwise from the position. */
    skipBy: (deltaMs: number) => {
      if (target !== undefined) return move(deltaMs);
      reveal();
      run((c) => c.skip(deltaMs));
    },
    commit,
    /** Drops the target, as hiding the controls does. */
    drop: () => setTarget(undefined),
  };
}
