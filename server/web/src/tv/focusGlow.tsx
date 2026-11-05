// One glow for a group of cards on TV, such as a library's grid or one of
// Home's rows, that lights the focused card and slides from card to card,
// rather than each card a glow of its own.
//
// A glow is three wide blurred shadows, which a TV rasterizes in software,
// and each card's own was rasterized again on every press, as the card it
// lit changed: about 190 ms of a 2025 Samsung's CPU a press. This one is
// rasterized once, on a layer of its own, and its moves are transitions of
// its transform, which the compositor runs. It's two layers: the glow and
// drop shadow beneath the focused card, which is raised above the rest, so
// its caption stays clear of the shadow, and the sheen over its artwork.
//
// The group's element holds them, and must be positioned. Each is placed by
// the focused card's artwork, its box as laid out, so a transform under
// way, such as the row's track sliding or the artwork scaling about its
// centre, doesn't throw it off; the group's own slide carries the glow with
// it. Focus leaving the group, or on a card still loading, hides it.

import { type FocusEvent, type RefObject, useRef } from "react";
import { isTv } from "./flag";

/** The artwork's box as laid out, relative to the group, or undefined if it isn't inside it. */
function layoutBox(ring: HTMLElement, group: HTMLElement) {
  let x = 0;
  let y = 0;
  for (let el: HTMLElement | null = ring; el !== group; el = el.offsetParent as HTMLElement | null) {
    if (!el) return undefined;
    x += el.offsetLeft;
    y += el.offsetTop;
  }
  return { x, y, width: ring.offsetWidth, height: ring.offsetHeight };
}

/**
 * The shared glow of a group of cards: its two layers, to render inside the
 * group's element, and the focus handlers for that element. On the web it
 * renders nothing.
 */
export function useFocusGlow(groupRef: RefObject<HTMLElement | null>) {
  const glowRef = useRef<HTMLDivElement>(null);
  const sheenRef = useRef<HTMLDivElement>(null);
  const layers = () => [glowRef.current, sheenRef.current].filter((g): g is HTMLDivElement => g !== null);
  const hide = () => {
    for (const layer of layers()) layer.removeAttribute("data-shown");
  };

  const onFocus = (e: FocusEvent) => {
    const group = groupRef.current;
    if (!isTv || !group) return;
    const ring = (e.target as Element).closest("[data-tv-focusable]")?.querySelector<HTMLElement>("[data-tv-ring]");
    const box = ring ? layoutBox(ring, group) : undefined;
    if (!ring || !box) {
      hide();
      return;
    }
    const radius = getComputedStyle(ring).borderRadius;
    for (const layer of layers()) {
      const shown = layer.hasAttribute("data-shown");
      // Shown afresh, it starts on the card rather than sliding from the last.
      if (!shown) layer.style.transition = "none";
      layer.style.width = `${box.width}px`;
      layer.style.height = `${box.height}px`;
      layer.style.borderRadius = radius;
      layer.style.transform = `translate(${box.x}px, ${box.y}px) scale(var(--tv-card-focus-scale))`;
      if (!shown) {
        void layer.offsetWidth;
        layer.style.transition = "";
        layer.setAttribute("data-shown", "");
      }
    }
  };

  const onBlur = (e: FocusEvent) => {
    if (e.relatedTarget instanceof Node && groupRef.current?.contains(e.relatedTarget)) return;
    hide();
  };

  const glows = isTv ? (
    <>
      <div ref={glowRef} aria-hidden data-part="tv-glow" />
      <div ref={sheenRef} aria-hidden data-part="tv-sheen" />
    </>
  ) : null;

  return { glows, onFocus, onBlur };
}
