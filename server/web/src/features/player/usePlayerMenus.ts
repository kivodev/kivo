import { useEffect, useRef, useState } from "react";
import type { KeyDispatcher } from "./keyDispatcher";

/** What the player has open over the video: Audio & Subtitles, Playback Settings or the stream info. */
export type PlayerMenuName = "tracks" | "settings" | "info";

/**
 * usePlayerMenus tracks the menu open over the video and the subtitle
 * search. When a menu closes, focus goes back to whatever had it when the
 * menu opened.
 */
export function usePlayerMenus(keys: KeyDispatcher) {
  const [menu, setMenu] = useState<PlayerMenuName>();
  const [searching, setSearching] = useState(false);
  const openerRef = useRef<Element>(null);
  useEffect(() => {
    if (menu) return;
    const frame = requestAnimationFrame(() => {
      const opener = openerRef.current;
      if (document.activeElement === document.body && opener instanceof HTMLElement) opener.focus();
    });
    return () => cancelAnimationFrame(frame);
  }, [menu]);
  /** Opens a menu, or closes it with undefined. */
  const open = (opened: PlayerMenuName | undefined) => {
    if (opened) {
      // A menu takes focus, so the press that opened it stops there.
      keys.focusMoved();
      if (!menu) openerRef.current = document.activeElement;
    }
    setMenu(opened);
  };
  return {
    menu,
    searching,
    setSearching,
    open,
    /**
     * Handles a menu's own open state changing, as its button or Escape
     * changes it. Closing a menu leaves alone a menu opened from it, such as
     * Audio & Subtitles or the stream info opened from Playback Settings.
     */
    openChange: (name: PlayerMenuName) => (isOpen: boolean) => {
      if (isOpen) open(name);
      else setMenu((m) => (m === name ? undefined : m));
    },
    /** Closes the submenu that has focus, or else the menu and the subtitle search. */
    close: () => {
      if (closeSubmenu()) return;
      setMenu(undefined);
      setSearching(false);
    },
  };
}

/**
 * Closes the list that has focus, such as Playback Settings' Quality, as
 * the arrow back towards its menu does there, whether it opened beside the
 * menu or in its place, so Back goes back a list at a time. It returns
 * whether a list had focus.
 */
function closeSubmenu(): boolean {
  const focused = document.activeElement;
  if (!focused?.closest('[data-trigger="SubmenuTrigger"], [data-submenu]')) return false;
  const key = getComputedStyle(focused).direction === "rtl" ? "ArrowRight" : "ArrowLeft";
  for (const type of ["keydown", "keyup"]) {
    focused.dispatchEvent(new KeyboardEvent(type, { key, bubbles: true, cancelable: true }));
  }
  return true;
}
