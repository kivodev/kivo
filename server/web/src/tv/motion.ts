// How the TV build moves what's on screen as focus moves: a row's cards,
// the page and the grid glide to the focused item, unless a key is held,
// when each repeat jumps, as a glide would still be under way at the next.
// Focus that moves without an arrow, as Back restores it, jumps too, and so
// does everything with reduced motion.

import { isTv } from "./flag";

// The window in which a move counts as the arrow's, from its keydown. It's
// longer than a slow TV takes to move focus, and far shorter than a person
// takes to press again.
const arrowMs = 300;

const arrowKeys = new Set(["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"]);
const arrowCodes = new Set([37, 38, 39, 40]);

let arrow = { at: Number.NEGATIVE_INFINITY, repeat: false };

// Read first, in the capture phase, so a move the key makes knows of it.
if (isTv && typeof window !== "undefined") {
  window.addEventListener(
    "keydown",
    (e) => {
      if (arrowKeys.has(e.key) || arrowCodes.has(e.keyCode)) arrow = { at: performance.now(), repeat: e.repeat };
    },
    { capture: true },
  );
}

/** How a move made now goes: a glide for an arrow just pressed, else at once. */
export function tvMotion(): "smooth" | "instant" {
  if (arrow.repeat || performance.now() - arrow.at > arrowMs) return "instant";
  return globalThis.matchMedia?.("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth";
}
