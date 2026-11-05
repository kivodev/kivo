import { type MouseEvent, type PointerEvent, useRef } from "react";

type Options = {
  /** Whether the controls show as the press starts. */
  visible: boolean;
  /** Whether a menu is open as the press starts. */
  menuOpen: boolean;
  /** Whether the player shows its error. A click leaves the error alone. */
  failed: boolean;
  paused: boolean;
  /** Called on any input, since it means someone is watching. */
  onInput: () => void;
  onTogglePlay: () => void;
  onHideControls: () => void;
  onFullscreen: () => void;
};

/** Whether the event hit the picture or the empty middle of the controls, rather than a control in the bars. */
const onSurface = (e: MouseEvent) => e.target instanceof Element && e.target.hasAttribute("data-surface");

/**
 * useSurfacePointer handles the pointer on the picture. A click plays or
 * pauses, and a double-click goes full screen. A double-click's two clicks
 * toggle twice, so play is left as it was. A tap shows or hides the
 * controls. A click that closes a menu does only that.
 */
export function useSurfacePointer({
  visible,
  menuOpen,
  failed,
  paused,
  onInput,
  onTogglePlay,
  onHideControls,
  onFullscreen,
}: Options) {
  const pressedRef = useRef<{ touch: boolean; shown: boolean; menu: boolean } | undefined>(undefined);
  return {
    onPointerMove: onInput,
    onPointerDown: (e: PointerEvent) => {
      pressedRef.current = { touch: e.pointerType !== "mouse", shown: visible, menu: menuOpen };
      onInput();
    },
    onClick: (e: MouseEvent) => {
      const pressed = pressedRef.current;
      if (!pressed || pressed.menu || !onSurface(e) || failed) return;
      if (!pressed.touch) return onTogglePlay();
      if (pressed.shown && !paused) onHideControls();
    },
    onDoubleClick: (e: MouseEvent) => {
      if (pressedRef.current?.touch || pressedRef.current?.menu || !onSurface(e)) return;
      onFullscreen();
    },
  };
}
