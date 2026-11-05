// Makes a form control or a button a stop for the remote on TV, and leaves
// it as it is on the web, so a screen wraps its controls without a branch.

import type { ReactNode } from "react";
import { isTv } from "@/tv/flag";
import { TvFocusable } from "@/tv/focus";

// OK on a control presses it: a switch toggles, a button or a select opens
// or acts, and a text field takes focus, which opens the TV's keyboard.
function activate(root: Element | null) {
  const control = root?.querySelector("input, textarea, button, a[href], [role=button]");
  if (control instanceof HTMLInputElement && control.type !== "checkbox") control.focus();
  else if (control instanceof HTMLTextAreaElement) control.focus();
  else if (control instanceof HTMLElement) control.click();
}

type Props = {
  /** Stable if Back should restore focus to it. Without one the control gets a key of its own. */
  focusKey?: string;
  /** The screen's initial focus when it opens anew. */
  autoFocus?: boolean;
  /** An arrow pressed on it. Return false to stop focus moving. */
  onArrow?: (direction: "left" | "right" | "up" | "down") => boolean;
  /** Replaces OK, which otherwise presses the control inside. */
  onPress?: () => void;
  /**
   * `field` for a text field, whose input alone lights up, and `row` for one
   * as wide as the page, which lights up whole rather than scaling (TvFocusable).
   */
  kind?: "button" | "field" | "row";
  /** Its data-part. */
  part?: string;
  /** The wrapper's classes, for a control that fills a grid cell or a row. */
  className?: string;
  children: ReactNode;
};

export function TvControl({ children, onPress, ...props }: Props) {
  if (!isTv) return children;
  return (
    <TvFocusable kind="button" {...props} onPress={onPress ?? (() => activate(document.activeElement))}>
      {children}
    </TvFocusable>
  );
}
