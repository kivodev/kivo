// Spatial navigation for the TV build (Norigin). A remote's arrows move
// focus to the nearest focusable on screen in that direction, and OK
// presses it. Every focusable is registered by TvFocusable or TvFocusGroup
// under a focus key, which is how focus is remembered and restored.
//
// One service, started once by startTv (start.ts). Until then the
// functions here do nothing. The shell pauses it while the player is open
// and playing, as the player's KeyDispatcher owns every key there.

import type {
  AddEventListenersOptions,
  FocusableComponent,
  LayoutAdapter,
} from "@noriginmedia/norigin-spatial-navigation";
import { focusHandles } from "@/features/player/keys";
import { tvMotion } from "./motion";
import { norigin } from "./norigin";

/** The focus key of the shell's content, around the screen's own focusables. */
export const TV_CONTENT = "tv:content";
/** The focus key of the shell's rail. Focusing it opens the rail on the screen's destination. */
export const TV_RAIL = "tv:rail";
/** The focus key of the audiobook player's dock, along the bottom of the screen below the content. */
export const TV_DOCK = "tv:dock";

type Key = "left" | "right" | "up" | "down" | "enter";

const keys: Record<string, Key> = {
  ArrowLeft: "left",
  ArrowRight: "right",
  ArrowUp: "up",
  ArrowDown: "down",
  Enter: "enter",
};
// Some remotes send only the key code.
const keyCodes: Record<number, Key> = { 37: "left", 38: "up", 39: "right", 40: "down", 13: "enter" };
// Each key by its KeyboardEvent.key name, which focusHandles reads.
const keyNames: Record<Key, string> = {
  left: "ArrowLeft",
  right: "ArrowRight",
  up: "ArrowUp",
  down: "ArrowDown",
  enter: "Enter",
};

let paused = false;
let started = false;
// Until when a key pressed with nothing focused goes nowhere (holdStrayTvKeys).
let strayHeldUntil = 0;
// The content item focus went down to the dock from, which Up from the dock
// returns to.
let dockReturn: string | undefined;

/**
 * Whether Norigin acts on a key pressed on this element: the element is, or
 * is inside, the focusable Norigin has focused, or nothing is focused, when
 * an arrow focuses the rail. Focus elsewhere, such as in a popover Norigin
 * doesn't know, keeps its keys.
 */
function noriginActsOn(target: EventTarget | null): boolean {
  if (!(target instanceof Element) || target === document.body || target === document.documentElement) return true;
  const key = currentTvFocus();
  return key !== undefined && target.closest<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey === key;
}

function keyOf(e: KeyboardEvent): Key | undefined {
  return keys[e.key] ?? keyCodes[e.keyCode];
}

function groupOf(el: Element, groupKey: string): Element | null {
  return el.closest(`[data-tv-group="${groupKey}"]`);
}

type Candidate = { key: string; dy: number; dx: number; stops?: boolean };

// The focusables in a group that show, by how far below (Down) or above
// (Up) the focused item they are, and how far from its column. The focused
// item is scaled up a little, so it may overlap the rows next to it by that
// much.
//
// Each position read costs a TV's slow CPU, and Home has hundreds of cards,
// so they're measured a row at a time. Each row or grid, the group nearest
// an item, is measured first, and its items only if some may be on the
// nearest line: nearest keeps only the items within half the focused item's
// height of the nearest one, so a row further than that from the nearest
// item found so far has none, and nor has a row on the wrong side. A row's
// items lie inside it, give or take a card scaling back down from focus,
// which the overlap allows for. A group with no box of its own, as with
// display: contents, has its items measured one by one. The focused item's
// own row, when it's one line (data-tv-line), is neither above nor below it.
function candidatesIn(group: Element | null, from: HTMLElement, r: DOMRect, key: "up" | "down"): Candidate[] {
  const overlap = r.height * 0.1;
  const line = r.height / 2;
  const centre = r.left + r.width / 2;
  const candidates: Candidate[] = [];
  let best = Number.POSITIVE_INFINITY;
  const measure = (el: HTMLElement) => {
    if (key === "down" && el.hasAttribute("data-tv-skip-down")) return;
    const e = el.getBoundingClientRect();
    if (e.width === 0 && e.height === 0) return;
    const dy = key === "down" ? e.top - r.bottom : r.top - e.bottom;
    if (dy < -overlap) return;
    const below = Math.max(dy, 0);
    best = Math.min(best, below);
    candidates.push({
      key: el.dataset.tvFocusKey as string,
      dy: below,
      dx: Math.abs(e.left + e.width / 2 - centre),
      // Up lands on it from anywhere along the row below, as Down passes over it.
      stops: key === "up" && el.hasAttribute("data-tv-skip-down"),
    });
  };

  const rows = new Map<Element, HTMLElement[]>();
  for (const el of group?.querySelectorAll<HTMLElement>("[data-tv-focusable]") ?? []) {
    if (el === from || !el.dataset.tvFocusKey) continue;
    const row = el.parentElement?.closest("[data-tv-group]");
    if (!row || row === group) measure(el);
    else if (rows.has(row)) rows.get(row)?.push(el);
    else rows.set(row, [el]);
  }

  const near: { gap: number; items: HTMLElement[] }[] = [];
  for (const [row, items] of rows) {
    if (row.hasAttribute("data-tv-line") && row.contains(from)) continue;
    const b = row.getBoundingClientRect();
    if (b.width === 0 && b.height === 0) {
      for (const el of items) measure(el);
      continue;
    }
    const top = b.top - overlap;
    const bottom = b.bottom + overlap;
    // The furthest an item in the row can be past the focused one, and the nearest.
    const furthest = key === "down" ? bottom - r.bottom : r.top - top;
    if (furthest < -overlap) continue;
    near.push({ gap: Math.max(key === "down" ? top - r.bottom : r.top - bottom, 0), items });
  }
  near.sort((a, b) => a.gap - b.gap);
  for (const { gap, items } of near) {
    if (gap > best + line) break;
    for (const el of items) measure(el);
  }
  return candidates;
}

// The nearest line, then the item in it nearest the focused one's column.
function nearest(candidates: Candidate[], r: DOMRect): string | undefined {
  if (candidates.length === 0) return undefined;
  const closest = Math.min(...candidates.map((c) => c.dy));
  const stop = candidates.find((c) => c.stops && c.dy === closest);
  if (stop) return stop.key;
  const line = closest + r.height / 2;
  return candidates.filter((c) => c.dy <= line).sort((a, b) => a.dx - b.dx)[0]?.key;
}

/**
 * The item Up or Down goes to on a screen: the one on screen directly above
 * or below, nearest by position, in its row as it is scrolled,
 * whichever row or group it's in, or the item that group enters on, if it
 * names one. Norigin would go to the row's remembered item instead. In a
 * column (TvFocusGroup's column) it's the item above or below in the
 * column, and past its ends focus stays where it is. The audiobook dock is
 * below the whole screen: Down from the screen's last row goes to it, and
 * Up from it goes back to the item focus came from. Outside the screen, as in the rail or a dialog that keeps focus
 * in, Norigin decides.
 */
function verticalTarget(target: EventTarget | null, key: Key): string | undefined {
  if ((key !== "up" && key !== "down") || !(target instanceof Element)) return undefined;
  const from = target.closest<HTMLElement>("[data-tv-focusable]");
  if (!from || from.closest("[data-tv-trap]")) return undefined;
  const content = document.querySelector(`[data-tv-group="${TV_CONTENT}"]`);
  const dock = document.querySelector(`[data-tv-group="${TV_DOCK}"]`);
  const r = from.getBoundingClientRect();
  if (groupOf(from, TV_CONTENT)) {
    const column = from.closest("[data-tv-column]");
    const onScreen = entryOf(nearest(candidatesIn(column ?? content, from, r, key), r), from);
    if (onScreen === undefined && key === "down" && dock) {
      const below = nearest(candidatesIn(dock, from, r, key), r) ?? firstIn(dock);
      if (below !== undefined) {
        dockReturn = from.dataset.tvFocusKey;
        return below;
      }
    }
    return onScreen ?? (column ? from.dataset.tvFocusKey : undefined);
  }
  if (groupOf(from, TV_DOCK) && key === "up") {
    const back = dockReturn;
    dockReturn = undefined;
    if (back !== undefined && norigin().SpatialNavigation.doesFocusableExist(back)) return back;
    return nearest(candidatesIn(content, from, r, key), r);
  }
  return undefined;
}

/**
 * Measures the focused item's group afresh, before Norigin moves focus
 * within it by position. Norigin trusts a layout it measured in the last
 * 16 ms, as when the items registered, but a screen that has just mounted
 * may still be moving then: back from the player, the hero's Play becomes
 * Resume as the work's progress arrives, and the first Right went by where
 * the buttons had been, finding nothing to the right. Norigin then trusts
 * these layouts rather than measuring the items again, so a press measures
 * each item once, as Norigin would by itself once its layouts are older. A
 * line goes by its order and isn't measured.
 */
function measureGroupOf(from: Element) {
  const group = from.parentElement?.closest("[data-tv-group]");
  if (!group || group.hasAttribute("data-tv-line")) return;
  const service = norigin().SpatialNavigation;
  for (const el of group.querySelectorAll<HTMLElement>("[data-tv-focusable], [data-tv-group]")) {
    if (el.parentElement?.closest("[data-tv-group]") !== group) continue;
    const key = el.dataset.tvFocusKey ?? el.dataset.tvGroup;
    if (key) void service.updateLayout(key);
  }
}

/**
 * The item focus enters a group on from outside it, when the group names
 * one (TvFocusGroup's enterOn) and it's there, in place of the nearest.
 */
function entryOf(nearestKey: string | undefined, from: Element): string | undefined {
  if (nearestKey === undefined) return undefined;
  const target = document.querySelector(`[data-tv-focus-key="${CSS.escape(nearestKey)}"]`);
  const group = target?.closest<HTMLElement>("[data-tv-enter]");
  const entry = group?.dataset.tvEnter;
  if (!group || !entry || group.contains(from) || !tvFocusExists(entry)) return nearestKey;
  return entry;
}

function firstIn(group: Element | null): string | undefined {
  return group?.querySelector<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey;
}

/**
 * Norigin's layout adapter, measured with getBoundingClientRect so a row
 * scrolled sideways counts where it shows.
 *
 * Keys are read on the window in the capture phase, before the focused
 * element: a React Aria widget such as a radio group stops the arrows it
 * handles from going further, which would leave the remote stuck in it.
 * Only a key Norigin acts on is taken from the element, and only while
 * Norigin isn't paused: a key a focused field or list handles itself
 * reaches it, and on the player its KeyDispatcher has every key. Something
 * that moves focus with its own arrows, such as the library's grid, marks
 * itself data-tv-own-keys, and its keys reach Norigin after it, unless it
 * took them.
 */
class TvLayoutAdapter implements LayoutAdapter {
  private captureDown?: (e: KeyboardEvent) => void;
  private bubbleDown?: (e: KeyboardEvent) => void;
  private up?: (e: KeyboardEvent) => void;

  addEventListeners({ keyDown, keyUp }: AddEventListenersOptions) {
    const handle = (e: KeyboardEvent, capture: boolean) => {
      const key = keyOf(e);
      if (paused || e.defaultPrevented || !key) return;
      if (strayHeld(e.target)) {
        e.preventDefault();
        e.stopPropagation();
        return;
      }
      const own = e.target instanceof Element && e.target.closest("[data-tv-own-keys]") !== null;
      if (own === capture || focusHandles(e.target, keyNames[key]) || !noriginActsOn(e.target)) return;
      e.preventDefault();
      e.stopPropagation();
      const vertical = verticalTarget(e.target, key);
      if (vertical === undefined) {
        const from = e.target instanceof Element ? e.target.closest("[data-tv-focusable]") : null;
        if (from && key !== "enter") measureGroupOf(from);
        keyDown?.(key, e);
      }
      // The focused item's own arrow handler may still stop the move.
      else if (norigin().SpatialNavigation.onArrowPress(key, { pressedKeys: {} }) !== false) focusTv(vertical);
    };
    this.captureDown = (e) => handle(e, true);
    this.bubbleDown = (e) => handle(e, false);
    this.up = (e) => {
      const key = keyOf(e);
      if (paused || !key) return;
      keyUp?.(key);
    };
    window.addEventListener("keydown", this.captureDown, { capture: true });
    window.addEventListener("keydown", this.bubbleDown);
    window.addEventListener("keyup", this.up, { capture: true });
  }

  removeEventListeners() {
    if (this.captureDown) window.removeEventListener("keydown", this.captureDown, { capture: true });
    if (this.bubbleDown) window.removeEventListener("keydown", this.bubbleDown);
    if (this.up) window.removeEventListener("keyup", this.up, { capture: true });
  }

  async measureLayout(component: FocusableComponent) {
    const node = component.node;
    const rect = node.getBoundingClientRect();
    return {
      x: rect.x,
      y: rect.y,
      width: rect.width,
      height: rect.height,
      left: rect.left,
      top: rect.top,
      right: rect.right,
      bottom: rect.bottom,
      node,
    };
  }

  blurNode(component: FocusableComponent) {
    component.node.removeAttribute("data-focused");
  }

  // The page scrolls only as far as shows the focused item, gliding after an
  // arrow (motion.ts). An item that keeps its own place, such as a row
  // pinned near the top, says so with data-tv-scroll="none".
  focusNode(component: FocusableComponent) {
    const node = component.node;
    node.setAttribute("data-focused", "true");
    node.focus({ preventScroll: true });
    if (node.dataset.tvScroll !== "none")
      node.scrollIntoView?.({ block: "nearest", inline: "nearest", behavior: tvMotion() });
  }
}

/**
 * Starts spatial navigation. The page is laid out on a 1080p canvas, 1920
 * pixels wide, which a TV whose browser is 1280 wide scales down. A pause
 * asked for before it started holds.
 */
export function startSpatial() {
  started = true;
  document.querySelector('meta[name="viewport"]')?.setAttribute("content", "width=1920");
  norigin().init({
    layoutAdapter: TvLayoutAdapter,
    shouldFocusDOMNode: true,
    domNodeFocusOptions: { preventScroll: true },
    distanceCalculationMethod: "center",
  });
  if (paused) norigin().pause();
}

export function stopSpatial() {
  if (started) norigin().destroy();
  started = false;
  paused = false;
  dockReturn = undefined;
}

// Norigin's service, once spatial navigation has started.
function service() {
  return started ? norigin().SpatialNavigation : undefined;
}

/**
 * Pauses spatial navigation and the Back keys, while the player owns every
 * key, or resumes them.
 */
export function pauseTv(pausing: boolean) {
  paused = pausing;
  if (!started) return;
  if (pausing) norigin().pause();
  else norigin().resume();
}

export function isTvPaused(): boolean {
  return paused;
}

/** Moves focus to a focus key, or into the group it names. */
export function focusTv(focusKey: string) {
  void service()?.setFocus(focusKey);
}

/**
 * Holds keys pressed with nothing focused for a while, or stops holding
 * them (0), as a screen opening anew waits for its initial item to arrive
 * with its data (focusMemory). Without it an arrow there would focus the
 * rail, which would then keep focus from the screen, and OK would press the
 * first link on the page. The hold is bounded, so a screen that never gets
 * an item still lets an arrow reach the rail.
 */
export function holdStrayTvKeys(ms: number) {
  strayHeldUntil = ms > 0 ? Date.now() + ms : 0;
}

function strayHeld(target: EventTarget | null): boolean {
  if (Date.now() >= strayHeldUntil) return false;
  return (target === document.body || target === document.documentElement) && currentTvFocus() === undefined;
}

/** Whether a focusable or group with this key is registered. */
export function tvFocusExists(focusKey: string): boolean {
  return service()?.doesFocusableExist(focusKey) ?? false;
}

/** The focus key that has focus, if it still exists. */
export function currentTvFocus(): string | undefined {
  const key = service()?.getCurrentFocusKey();
  return key && tvFocusExists(key) ? key : undefined;
}

/** Whether a focus key is inside a group, such as the content or the rail. */
export function isTvFocusIn(focusKey: string, groupKey: string): boolean {
  return service()?.isDescendantOf(focusKey, groupKey) ?? false;
}

/**
 * The dock has gone, as listening stopped: focus that was in it goes back
 * to the item it came down from, or into the screen.
 */
export function leaveTvDock() {
  const back = dockReturn;
  dockReturn = undefined;
  if (back === undefined || currentTvFocus() !== undefined) return;
  focusTv(tvFocusExists(back) ? back : TV_CONTENT);
}
