import { useEffect, useRef } from "react";
import type { KeyDispatcher } from "./keyDispatcher";

/** Buttons a remote's arrows can move focus to. */
const focusable = "button:not([disabled])";

type Shown = {
  /** Whether a play has arrived, so the controls have something to focus. */
  ready: boolean;
  visible: boolean;
  live: boolean;
  /** The button over the picture: Skip, or Up next's. */
  overlay: "skip" | "upNext" | undefined;
  stillWatching: boolean;
  /** A menu, the stream info or a dialog is open over the player and keeps focus. */
  menuOpen: boolean;
};

/**
 * useRemoteFocus moves a remote's focus.
 *
 * - Play or pause takes focus whenever the controls appear, unless the key
 *   that showed them scrubs, which puts it on the scrub bar. While the media
 *   loads, play or pause is disabled, so it takes focus as the media goes
 *   live under the controls, if focus has nowhere else to be by then.
 * - Skip or Up next takes focus when it appears with the controls hidden, or
 *   when the controls hide under it.
 * - "Still watching?" takes focus as it appears, whatever the controls.
 *
 * Nothing moves focus out of an open menu, even when a new play that the
 * menu started goes live. Without a remote it moves nothing.
 */
export function useRemoteFocus(
  remote: boolean,
  { ready, visible, live, overlay, stillWatching, menuOpen }: Shown,
  keys: KeyDispatcher,
) {
  const thumbRef = useRef<HTMLInputElement>(null);
  const playRef = useRef<HTMLButtonElement>(null);
  const transportRef = useRef<HTMLDivElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  /** Whether the key that shows the controls scrubs, so the scrub bar takes focus rather than play or pause. */
  const toBarRef = useRef(false);
  const shownRef = useRef({ visible: false, live: false, overlay: undefined as typeof overlay, stillWatching: false });
  useEffect(() => {
    if (!remote || !ready) return;
    const was = shownRef.current;
    shownRef.current = { visible, live, overlay, stillWatching };
    const toBar = toBarRef.current;
    toBarRef.current = false;
    const overlayButton = () => overlayRef.current?.querySelector<HTMLElement>(focusable);
    const controls = (toBar ? thumbRef : playRef).current;
    // Focus that a disabled button dropped is on the page itself.
    const lost = document.activeElement === null || document.activeElement === document.body;
    let target: HTMLElement | null | undefined;
    if (stillWatching && !was.stillWatching) target = overlayButton();
    else if (visible && (!was.visible || (live && !was.live && lost))) target = controls;
    else if (!visible && overlay && (was.visible || was.overlay !== overlay)) target = overlayButton();
    if (!target || menuOpen) return;
    keys.focusMoved();
    target.focus();
  }, [remote, ready, visible, live, overlay, stillWatching, menuOpen, keys]);

  return {
    thumbRef,
    playRef,
    transportRef,
    overlayRef,
    /** Puts focus on the scrub bar, rather than play or pause, the next time the controls appear. */
    revealOnBar: () => {
      toBarRef.current = true;
    },
    onScrubBar: () => thumbRef.current !== null && document.activeElement === thumbRef.current,
    /** Focus the next button along the focused one's row. At either end focus stays. */
    along: (step: -1 | 1) => {
      const active = document.activeElement;
      const row = [transportRef, overlayRef].find((r) => active && r.current?.contains(active))?.current;
      if (!row || !active) return;
      const buttons = [...row.querySelectorAll<HTMLElement>(focusable)];
      buttons[buttons.indexOf(active as HTMLElement) + step]?.focus();
    },
    /** Focus the row above or below. The rows are the scrub bar, then the transport starting from play or pause. */
    across: (step: -1 | 1) => {
      const active = document.activeElement;
      const onBar = active !== null && active === thumbRef.current;
      if (step > 0 && onBar) playRef.current?.focus();
      else if (step < 0 && !onBar) thumbRef.current?.focus();
    },
  };
}
