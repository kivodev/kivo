// Where focus goes as screens change. Leaving a screen
// remembers its focused item by the history entry, so Back to that entry
// focuses the same card, once it's on screen again. A screen opened anew
// focuses its initial item: the one marked autoFocus, else its first.

import {
  currentTvFocus,
  focusTv,
  holdStrayTvKeys,
  isTvFocusIn,
  isTvPaused,
  TV_CONTENT,
  TV_RAIL,
  tvFocusExists,
} from "./spatial";

// A remembered card that hasn't come back this long after its screen opened
// is gone, and the screen gets its initial focus instead.
const restoreWaitMs = 1000;
// Keys pressed with nothing focused go nowhere this long at most while a
// screen's focus is pending, as its data arrives on a TV's slow network.
const holdKeysMs = 5000;

const remembered = new Map<string, string>();
let pending: { key?: string; since: number; fromRail: boolean } | undefined;
let timer: ReturnType<typeof setTimeout> | undefined;

/** Remembers the focused item of the screen being left, if focus is on it rather than in the rail. */
export function rememberTvFocus(entry: string) {
  const key = currentTvFocus();
  if (key && isTvFocusIn(key, TV_CONTENT)) remembered.set(entry, key);
}

/** A screen is opening at a history entry: focus what was focused there, or the screen's initial item. */
export function expectTvScreen(entry: string) {
  const current = currentTvFocus();
  pending = {
    key: remembered.get(entry),
    since: Date.now(),
    fromRail: current !== undefined && isTvFocusIn(current, TV_RAIL),
  };
  holdStrayTvKeys(holdKeysMs);
  settleTvFocus();
}

/**
 * Tries the pending focus once the screen's focusables have registered,
 * after every focusable mounts, since a screen's cards arrive with its data.
 */
export function settleTvFocus() {
  if (!pending) return;
  clearTimeout(timer);
  timer = setTimeout(resolve, 0);
}

/** Drops the pending focus, as the screen focused something itself. */
export function cancelTvFocus() {
  settled();
  clearTimeout(timer);
}

/** The pending focus is done with, and keys go where they would again. */
function settled() {
  pending = undefined;
  holdStrayTvKeys(0);
}

export function forgetFocus() {
  cancelTvFocus();
  remembered.clear();
}

/** The first focusable in a group in the page's order, its top-left item. */
export function firstTvFocusIn(groupKey: string): string | undefined {
  const group = document.querySelector(`[data-tv-group="${CSS.escape(groupKey)}"]`);
  return group?.querySelector<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey;
}

function initialFocus(): string | undefined {
  const group = document.querySelector(`[data-tv-group="${CSS.escape(TV_CONTENT)}"]`);
  const marked = group?.querySelector<HTMLElement>("[data-tv-focusable][data-tv-autofocus]");
  return marked?.dataset.tvFocusKey ?? firstTvFocusIn(TV_CONTENT);
}

function resolve() {
  if (!pending || isTvPaused()) return;
  // The screen focused something itself, or the person moved focus to the
  // rail while it loaded, as Back does: a screen that arrives late doesn't
  // take focus back from the open rail. The rail keeps focus only while
  // it chose this screen, which then takes focus from it.
  const current = currentTvFocus();
  if (current && (!pending.fromRail || !isTvFocusIn(current, TV_RAIL))) {
    settled();
    return;
  }
  const { key, since } = pending;
  if (key && tvFocusExists(key)) {
    settled();
    focusTv(key);
    return;
  }
  const waited = Date.now() - since;
  if (key && waited < restoreWaitMs) {
    timer = setTimeout(resolve, restoreWaitMs - waited);
    return;
  }
  const initial = initialFocus();
  if (initial) {
    settled();
    focusTv(initial);
  }
}
