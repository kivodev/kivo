// What a dialog does on TV: Back closes it, focus starts on its safe
// choice and goes back to what opened it.

import { type ReactNode, useEffect, useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import { buttonClass } from "@/components/Button";
import { useTvBack } from "@/tv/back";
import { isTv } from "@/tv/flag";
import { useTvFocusable } from "@/tv/focus";
import { currentTvFocus, focusTv } from "@/tv/spatial";

/**
 * Closes the dialog on Back while it's open, and puts focus back where it
 * was when it closes. Call it from the dialog's own component, so it
 * captures focus before the dialog's buttons take it. `isOpen` is whether
 * the dialog is open at all: focus to return to is taken as it goes from
 * closed to open, and never again while it stays open. `handlesBack` is
 * false while something above it, such as a confirmation it opened, has
 * Back.
 */
export function useTvDialog(isOpen: boolean, close: () => void, handlesBack = true) {
  const [returnTo, setReturnTo] = useState<string>();
  const [wasOpen, setWasOpen] = useState(false);
  if (isOpen !== wasOpen) {
    setWasOpen(isOpen);
    setReturnTo(isOpen && isTv ? currentTvFocus() : undefined);
  }
  useTvBack(() => {
    close();
    return true;
  }, isOpen && handlesBack);
  useEffect(() => {
    if (!isOpen || returnTo === undefined) return;
    return () => focusTv(returnTo);
  }, [isOpen, returnTo]);
}

type ButtonProps = {
  focusKey: string;
  onPress: () => void;
  /** The safe choice, which takes focus as the dialog opens. */
  autoFocus?: boolean;
  tone?: "default" | "danger";
  children: ReactNode;
};

/** A dialog's button on TV, large, which turns white and grows with focus. */
export function TvDialogButton({ focusKey, onPress, autoFocus = false, tone = "default", children }: ButtonProps) {
  const { ref, attributes } = useTvFocusable<HTMLButtonElement>({ focusKey });
  useEffect(() => {
    if (autoFocus) focusTv(focusKey);
  }, [autoFocus, focusKey]);
  return (
    <AriaButton
      ref={ref}
      {...attributes}
      data-tv-focus="button"
      data-tv-face=""
      onPress={onPress}
      className={`${buttonClass(tone === "danger" ? "danger" : "secondary")} h-auto px-8 py-3 text-base`}
    >
      {children}
    </AriaButton>
  );
}
