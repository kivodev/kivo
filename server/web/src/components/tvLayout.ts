// Shared pieces of the TV build's screens.

import { pageScrollTarget, scrollPage } from "@/tv/glide";
import { tvMotion } from "@/tv/motion";

/**
 * A focusable card's classes, for TvFocusable, which uses them on TV
 * only. The focused card is raised over its neighbours, which its scale
 * would overlap, and the page scrolls to it clear of the screen's top and
 * bottom edges.
 */
export const tvCardClass = "relative focus:z-10 scroll-my-(--layout-tv-overscan-y)";

// The overscan and the dock's scroll padding, read from the root's style.
// Reading it forces the root's style, which a slow TV feels on every card
// focus, so it's read once and again only as the window resizes or the
// shell sets the root's scroll padding for the dock, which it does on the
// root's own style.
let insets: { overscan: number; dock: number; padding: string } | undefined;
let watching = false;

function rootInsets() {
  const padding = document.documentElement.style.scrollPaddingBottom;
  if (insets?.padding !== padding) {
    if (!watching) {
      watching = true;
      window.addEventListener("resize", () => {
        insets = undefined;
      });
    }
    const root = getComputedStyle(document.documentElement);
    insets = {
      overscan: Number.parseFloat(root.getPropertyValue("--layout-tv-overscan-y")) || 0,
      dock: Number.parseFloat(root.scrollPaddingBottom) || 0,
      padding,
    };
  }
  return insets;
}

/**
 * Scrolls the page to put the row at the top of the screen, below the
 * overscan, so the focused row stays at one height while the rows move
 * under it. A row that is on the page's first screen stays where it is, so
 * the page's title and first rows aren't scrolled away for it; that goes by
 * the row's content, as Home's last row reaches on down the screen to give
 * the page room to scroll it to the top. The first screen ends above the
 * audiobook dock while it shows, which the shell keeps the page's
 * scrolling clear of with the root's scroll padding. After an arrow the
 * page glides there (tv/motion.ts), and a press during a glide re-aims it
 * (tv/glide.ts). A row in a group that scrolls its items itself
 * (TvFocusGroup's scroller), as Search's results do, scrolls that only as
 * far as shows the row.
 */
export function revealTvRow(row: Element | null | undefined) {
  if (!row) return;
  const scroller = row.closest("[data-tv-scroller]");
  if (scroller) {
    const s = scroller.getBoundingClientRect();
    const r = row.getBoundingClientRect();
    const by = r.top < s.top ? r.top - s.top : r.bottom > s.bottom ? Math.min(r.bottom - s.bottom, r.top - s.top) : 0;
    if (by !== 0) scroller.scrollBy({ top: by, behavior: tvMotion() });
    return;
  }
  const { overscan, dock } = rootInsets();
  const box = row.getBoundingClientRect();
  const top = box.top + window.scrollY;
  const height = (row.firstElementChild ?? row).getBoundingClientRect().height;
  const fits = top + height <= window.innerHeight - overscan - dock;
  const to = fits ? 0 : top - overscan;
  if (Math.abs(to - pageScrollTarget()) >= 1) scrollPage(to, tvMotion());
}

// How far each row's track has moved its cards along.
const offsets = new WeakMap<HTMLElement, number>();

/**
 * Moves a row's cards sideways only as far as shows the focused card,
 * clear of the row's edges. The cards move together on their track, by a
 * transform rather than by scrolling, which painted the row again on every
 * press. After an arrow the track glides there (tv/motion.ts), a
 * transition the TV composites, on a layer of the track's own only while
 * it moves: a layer kept for every row held many screens of the TV's
 * graphics memory. A held key's repeats jump. A
 * row moved from its start is marked on the element that clips it
 * (data-tv-scrolled), which hides the card before its first whole one.
 */
export function revealTvCard(track: HTMLElement | null, card: HTMLElement | null) {
  if (!track || !card) return;
  const was = offsets.get(track) ?? 0;
  const to = reach(track, card, was);
  if (to !== was) move(track, to, tvMotion() === "smooth");
}

/**
 * Puts a row's track back where it belongs after its cards or its width
 * change, as when live data removes or reorders cards or the window
 * resizes: the offset kept from before could leave the focused card off
 * the screen, or show empty track past the last card. The focused card,
 * kept through the change, shows as an arrow to it would show it; without
 * one the track moves back no further than its last card needs. It moves
 * at once, since nothing was pressed.
 */
export function settleTvTrack(track: HTMLElement | null) {
  if (!track) return;
  const was = offsets.get(track) ?? 0;
  const focused = document.activeElement;
  const card = focused instanceof HTMLElement && track.contains(focused) ? focused.closest("li") : null;
  const to = card ? reach(track, card, was) : Math.min(was, furthest(track));
  if (to !== was) move(track, to, false);
}

/** Where the track goes from `was` to show the card clear of its edges. */
function reach(track: HTMLElement, card: HTMLElement, was: number): number {
  const start = card.offsetLeft - firstLeft(track);
  const end = start + card.offsetWidth;
  const width = track.clientWidth;
  const to = start < was ? start : end > was + width ? end - width : was;
  return Math.max(0, Math.min(to, furthest(track)));
}

/** The track's cards, without the shared glow's holder (tv/focusGlow.tsx). */
const cardsOf = (track: HTMLElement) => track.querySelectorAll<HTMLElement>(":scope > li:not([data-tv-glow-host])");

/** How far the track can move: until its last card ends at its right edge. */
function furthest(track: HTMLElement): number {
  const cards = cardsOf(track);
  const last = cards[cards.length - 1] ?? null;
  if (!last) return 0;
  return Math.max(0, last.offsetLeft + last.offsetWidth - firstLeft(track) - track.clientWidth);
}

// Cards are measured from the first, so the track's own place doesn't matter.
function firstLeft(track: HTMLElement): number {
  return cardsOf(track)[0]?.offsetLeft ?? 0;
}

function move(track: HTMLElement, to: number, glide: boolean) {
  offsets.set(track, to);
  track.style.transitionDuration = glide ? "" : "0s";
  track.style.transform = `translateX(${-to}px)`;
  track.parentElement?.toggleAttribute("data-tv-scrolled", to > 0);
}
