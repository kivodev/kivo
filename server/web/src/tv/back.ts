// Back on a TV remote. Every Back key is GoBack: the key codes 461
// (webOS) and 10009 (Tizen), XF86Back, BrowserBack and Escape, named by the
// player's keyName. A press goes to the newest Back handler first, such as
// an open dialog's, down to the shell's, which decides by the screen. Its
// listener is apart from spatial navigation's, which takes only the arrows
// and OK.

import { useEffect, useEffectEvent } from "react";
import { keyName } from "@/features/player/keys";
import { isTvPaused } from "./spatial";

/** Handles Back, returning false to leave it to the handler under it. */
export type BackHandler = () => boolean | undefined;

const handlers: { current: BackHandler }[] = [];
let fallback: BackHandler | undefined;

function onKeyDown(e: KeyboardEvent) {
  if (isTvPaused() || e.repeat || keyName(e, true) !== "GoBack") return;
  // The browser's own Back would leave the app's history behind its screens.
  e.preventDefault();
  goBack();
}

/** Does what Back does now. */
export function goBack() {
  for (const handler of [...handlers].reverse()) {
    if (handler.current() !== false) return;
  }
  fallback?.();
}

export function startBackKeys() {
  window.addEventListener("keydown", onKeyDown);
}

export function stopBackKeys() {
  window.removeEventListener("keydown", onKeyDown);
  handlers.length = 0;
  fallback = undefined;
}

/**
 * Handles Back while the caller is mounted and `enabled`, before every
 * handler registered earlier: a dialog closes, a settings section goes back
 * to its list. Return false to pass Back on. Outside the TV build it does
 * nothing.
 */
export function useTvBack(handler: BackHandler, enabled = true) {
  const handle = useEffectEvent(handler);
  useEffect(() => {
    if (!enabled) return;
    const entry = { current: () => handle() };
    handlers.push(entry);
    return () => {
      handlers.splice(handlers.indexOf(entry), 1);
    };
  }, [enabled]);
}

/** The shell's Back, for when no screen handles it. */
export function useTvShellBack(handler: () => void) {
  const handle = useEffectEvent(handler);
  useEffect(() => {
    fallback = () => {
      handle();
      return true;
    };
    return () => {
      fallback = undefined;
    };
  }, []);
}
