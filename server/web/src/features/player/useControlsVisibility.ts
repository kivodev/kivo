import { useEffect, useState } from "react";

/** Controls hide this long after the last input while playing. */
const hideControlsMs = 3000;

/**
 * With a remote, controls hide this long after the last press instead. A
 * remote steps along the controls a press at a time, and the next press
 * after they hide scrubs rather than moving focus.
 */
const remoteHideControlsMs = 5000;

/**
 * useControlsVisibility tracks whether the player's controls show. They show
 * on any input. While `autoHide` holds (playing, with no menu open), they
 * hide 3 seconds after the last input, or 5 with a remote.
 */
export function useControlsVisibility(autoHide: boolean, { remote = false }: { remote?: boolean } = {}) {
  const [hidden, setHidden] = useState(false);
  const [lastInputAt, setLastInputAt] = useState(Date.now);
  const reveal = () => {
    setHidden(false);
    setLastInputAt(Date.now());
  };
  const hideMs = remote ? remoteHideControlsMs : hideControlsMs;
  useEffect(() => {
    if (!autoHide) return;
    const timer = setTimeout(() => setHidden(true), Math.max(0, lastInputAt + hideMs - Date.now()));
    return () => clearTimeout(timer);
  }, [autoHide, lastInputAt, hideMs]);

  return {
    visible: !hidden,
    reveal,
    hide: () => setHidden(true),
  };
}
