import tokens from "@shared/tokens.json";
import { observeWindowOffset, useWindowVirtualizer } from "@tanstack/react-virtual";
import {
  Fragment,
  type KeyboardEvent,
  memo,
  type ReactNode,
  type Ref,
  useCallback,
  useEffect,
  useEffectEvent,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { ArtworkLoads } from "@/components/Artwork";
import { CardSkeleton } from "@/components/Skeleton";
import { tvCardClass } from "@/components/tvLayout";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { useFocusGlow } from "@/tv/focusGlow";
import { pageScrollTarget, scrollPage } from "@/tv/glide";
import { tvMotion } from "@/tv/motion";
import { focusTv } from "@/tv/spatial";

// How long the grid counts as moving after its last fast motion, which is
// a fast scroll, a jump or an arrow key. It's longer than the gap between
// a held key's repeats or a scrub's jumps, so those keep the grid moving.
// It's short enough that the grid looks loaded as soon as it stops.
const settleMs = 150;

// The scroll speed, in window heights a second, above which the grid
// counts as moving. That is faster than anyone looks at what passes. A
// browsing scroll, at about half a window a second, loads its rows as they
// come into view. A fling, a held arrow key (about ten a second) or a
// scrub doesn't.
const fastScreensPerSecond = 2;

// On TV, how long after the grid asks for a card's focus a key repeat still steps on from that card.
const arrivalMs = 100;

// On TV, how long the page's glide to a row counts as under way, about as
// long as it takes to settle (tv/glide.ts).
const glideMs = 300;

export type GridHandle = { scrollToIndex: (index: number) => void };

// A phone is a touch screen narrower than this on its short side. This
// matches the stylesheet's phone layout, which applies below this width
// when upright.
const phoneScreenSide = "40rem";

// The narrowest poster or square on a phone. It matches the width of three
// across an upright phone, so turning the phone sideways fits more cards
// of the same size.
const phonePosterMinWidth = "6rem";

const rem = (value: string, rootFontPx: number) => Number.parseFloat(value) * rootFontPx;

/** A number the TV build sets as a custom property on the root, such as its overscan in pixels. */
const rootPx = (name: string) =>
  Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue(name)) || 0;

type Props<T> = {
  /** Every item in the result. An item is undefined where its page hasn't loaded. */
  items: readonly (T | undefined)[];
  shape: "poster" | "landscape" | "square";
  view: "grid" | "list";
  renderCard: (item: T, width: number) => ReactNode;
  renderListRow: (item: T) => ReactNode;
  /**
   * Called as the grid settles and as its top row changes, so the pages it
   * needs load, or load again.
   */
  onRangeChange: (range: GridRange) => void;
  /** While the letter rail is held, the grid counts as moving until it's let go. */
  scrubbing?: boolean;
  ref?: Ref<GridHandle>;
};

/**
 * The items to load.
 *  - start and end cover the items the grid rendered when it last settled,
 *    plus a screen past them.
 *  - top and topEnd cover the items of the top row in view.
 *  - settles counts how many times the grid has come to rest.
 */
export type GridRange = { start: number; end: number; top: number; topEnd: number; settles: number };

/**
 * A window-scrolled grid that renders only the rows in view. While it
 * moves fast, by a fling, a jump, the arrow keys or the letter rail, it
 * fetches no pages and loads no artwork, and its new rows show
 * placeholders. Otherwise, as when it's still or scrolled slowly, the
 * pages in view and a screen past them load, along with the rendered rows'
 * artwork. Each time it comes to rest, pages that failed load again. The
 * arrow keys move focus from card to card, scrolling the card into view.
 *
 * On TV it is six posters or three landscape cards across, and
 * only grid view. Up and Down step by a row of the whole list, not of the
 * cards on screen, so they work past the rows that are scrolled out and
 * not rendered. Left from the first column and Right from the last leave
 * the grid, for the rail and the letter rail.
 */
export function VirtualGrid<T extends { id: string }>({
  items,
  shape,
  view,
  renderCard,
  renderListRow,
  onRangeChange,
  scrubbing = false,
  ref,
}: Props<T>) {
  // The virtualizer holds mutable state the React Compiler must not memoize.
  "use no memo";
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(1024);
  const [rootFontPx, setRootFontPx] = useState(16);
  const [tvLinePx, setTvLinePx] = useState(0);
  const [tvOverscanPx, setTvOverscanPx] = useState(0);
  const [tvGapPx, setTvGapPx] = useState(0);
  const [scrollMargin, setScrollMargin] = useState(0);

  // The grid's offset changes with whatever is above it, such as a banner
  // or wrapping filter chips, so it's measured whenever the page resizes.
  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const measure = () => {
      if (el.clientWidth > 0) setWidth(el.clientWidth);
      setRootFontPx(Number.parseFloat(getComputedStyle(document.documentElement).fontSize) || 16);
      setScrollMargin(el.getBoundingClientRect().top + window.scrollY);
      // Read here rather than on each render, which every scroll frame and
      // arrow press is, as each read forces the root's style on a slow TV.
      if (isTv) {
        setTvLinePx(rootPx("--text-sm--line-height"));
        setTvOverscanPx(rootPx("--layout-tv-overscan-y"));
        setTvGapPx(rootPx("--layout-tv-row-gap"));
      }
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    observer.observe(document.body);
    window.addEventListener("resize", measure);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);

  // On TV the cards sit as far apart as a row's, so a focused card and its
  // glow clear the cards around it.
  const gapPx = isTv ? tvGapPx : rem(tokens.layout.gridGap, rootFontPx);
  // Title and subtitle lines under the artwork, plus the gap between them.
  // On TV the lines are taller, and the gap is the same half a rem.
  const captionPx = isTv ? 2 * tvLinePx + rem("0.5rem", rootFontPx) : rem(tokens.layout.cardCaptionHeight, rootFontPx);
  const listRowPx = rem(tokens.layout.listRowHeight, rootFontPx);
  const aspect = { poster: 1.5, landscape: 9 / 16, square: 1 }[shape];
  // The cards are sized by the grid's width alone, never the window's
  // height, so a phone's toolbar collapsing as the page scrolls doesn't
  // resize them. A phone's posters and squares keep their size when it's
  // turned sideways, and more fit across. The screen's short side, not the
  // window's, decides whether it's a phone, so turning it doesn't change
  // that.
  const phone =
    Math.min(screen.width, screen.height) < rem(phoneScreenSide, rootFontPx) &&
    (globalThis.matchMedia?.("(pointer: coarse)").matches ?? false);
  const minCard = rem(
    shape === "landscape"
      ? tokens.layout.landscapeMinWidth
      : phone
        ? phonePosterMinWidth
        : tokens.layout.posterMinWidth,
    rootFontPx,
  );
  // A phone fits three posters or squares across.
  const fewest = shape === "landscape" ? 1 : 3;
  // On TV, the same counts as a row's, which the stylesheet sizes its cards by.
  const columns = isTv
    ? Number(shape === "landscape" ? tokens.tv.web.layout.landscapeAcross : tokens.tv.web.layout.posterAcross)
    : view === "list"
      ? 1
      : Math.max(fewest, Math.floor((width + gapPx) / (minCard + gapPx)));
  const cardWidth = (width - gapPx * (columns - 1)) / columns;
  const rowHeight = view === "list" ? listRowPx : cardWidth * aspect + captionPx + gapPx;
  const rowCount = Math.ceil(items.length / columns);

  // The grid counts as moving while it scrolls fast, while the letter rail
  // is held, and for a moment after a jump or an arrow key, which may not
  // scroll it at all.
  const [nudged, setNudged] = useState(false);
  const nudgeTimer = useRef(0);
  // The card the last arrow key moved to, which is focused once its row
  // renders, and the card focused last. These are refs because a held
  // key's repeats can come before a render. The state re-renders the grid
  // for each move.
  const target = useRef<{ index: number; at: number; from: EventTarget }>(undefined);
  const focused = useRef<{ index: number; at: number; from: EventTarget }>(undefined);
  const [, setFocusTarget] = useState<{ index: number; at: number; from: EventTarget }>();
  useEffect(() => () => clearTimeout(nudgeTimer.current), []);
  // A nudge keeps the grid moving for `ms`, or until an earlier, longer one ends.
  const nudgedUntil = useRef(0);
  const nudge = useCallback((ms = settleMs) => {
    setNudged(true);
    nudgedUntil.current = Math.max(nudgedUntil.current, performance.now() + ms);
    clearTimeout(nudgeTimer.current);
    nudgeTimer.current = window.setTimeout(() => setNudged(false), nudgedUntil.current - performance.now());
  }, []);
  const virtualizer = useWindowVirtualizer({
    count: rowCount,
    estimateSize: () => rowHeight,
    overscan: 3,
    scrollMargin,
    // A card scrolled to on TV stops clear of the screen's edges.
    scrollPaddingStart: isTv ? tvOverscanPx : 0,
    scrollPaddingEnd: isTv ? tvOverscanPx : 0,
    // On TV, rows a scroll brings in render with React's next update rather
    // than inside the scroll event, which would hold up that frame on a slow
    // TV; the rows rendered past the screen cover the wait. A fast browser
    // keeps rendering them in the event, so a quick drag of the scrollbar
    // shows no blank rows.
    useFlushSync: !isTv,
    // A scroll's speed is the distance since the last scroll divided by
    // the time since, capped at settleMs, so a jump after a pause reads as
    // fast. It's measured before the virtualizer takes the offset, so the
    // rows a fast scroll brings in render as moving from the start.
    observeElementOffset: (instance, onOffset) => {
      let last = { y: window.scrollY, at: performance.now() };
      return observeWindowOffset(instance, (offset, isScrolling) => {
        const now = { y: offset, at: performance.now() };
        const seconds = Math.max(Math.min(now.at - last.at, settleMs), 1) / 1000;
        if (Math.abs(now.y - last.y) / seconds > fastScreensPerSecond * window.innerHeight) nudge();
        last = now;
        onOffset(offset, isScrolling);
      });
    },
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: re-measure when the row height changes.
  useEffect(() => {
    virtualizer.measure();
  }, [virtualizer, rowHeight]);

  const moving = nudged || scrubbing;

  useImperativeHandle(ref, () => ({
    scrollToIndex: (index: number) => {
      nudge();
      virtualizer.scrollToIndex(Math.floor(index / columns), { align: "start", behavior: "instant" });
    },
  }));

  const virtualRows = virtualizer.getVirtualItems();
  const start = (virtualRows[0]?.index ?? 0) * columns;
  const end = Math.min(((virtualRows.at(-1)?.index ?? 0) + 1) * columns, items.length);
  // The top row in view is the first row that ends below the top, by more
  // than the pixel the window rounds its scroll position to. On TV it's the
  // first whose artwork is mostly below the overscan, so a row scrolled up
  // until only its captions show doesn't count.
  const scrolled = (virtualizer.scrollOffset ?? 0) + 1;
  const inView = (row: { start: number; end: number }) =>
    isTv ? row.start + (cardWidth * aspect) / 2 >= scrolled + tvOverscanPx : row.end > scrolled;
  const top = (virtualRows.find(inView)?.index ?? 0) * columns;
  const topEnd = Math.min(top + columns, items.length);
  // Pages load for where the grid settles, not for every row it passes.
  const [settled, setSettled] = useState({ start, end });
  if (!moving && (settled.start !== start || settled.end !== end)) setSettled({ start, end });
  const loadEnd = Math.min(settled.end * 2 - settled.start, items.length);
  // Each time the grid comes to rest, after any scroll, however small, or
  // after a jump, the range is reported again, so failed pages load again.
  const still = !moving && !virtualizer.isScrolling;
  const [rests, setRests] = useState({ count: 0, still });
  if (rests.still !== still) setRests({ count: rests.count + (still ? 1 : 0), still });
  useEffect(() => {
    onRangeChange({ start: settled.start, end: loadEnd, top, topEnd, settles: rests.count });
  }, [settled.start, loadEnd, top, topEnd, rests.count, onRangeChange]);

  useEffect(() => {
    const next = target.current;
    if (!next || focused.current === next) return;
    // The wait ends if the card is no longer in the list, or if focus moved
    // elsewhere on the page.
    const active = document.activeElement;
    if (next.index >= items.length || (active && active !== document.body && !containerRef.current?.contains(active))) {
      target.current = undefined;
      return;
    }
    const cell = containerRef.current?.querySelector<HTMLElement>(`[data-index="${next.index}"]`);
    if (!cell) return;
    focused.current = next;
    focusCell(cell);
  });
  // On TV the page glides to an arrow's row and jumps for a held key
  // (tv/motion.ts). It scrolls only as far as shows the row, from where the
  // glide is going rather than where it has got to, so a press during a
  // glide re-aims it and the page keeps moving (tv/glide.ts). The page is
  // scrolled directly: the virtualizer's own scroll to an index keeps
  // correcting a glide toward a target it measures afresh each frame.
  const glideToRow = (row: number) => {
    const going = pageScrollTarget();
    const start = virtualizer.getOffsetForIndex(row, "start")?.[0];
    const end = virtualizer.getOffsetForIndex(row, "end")?.[0];
    if (start === undefined || end === undefined) return;
    const to = going > start ? start : going < end ? end : undefined;
    if (to === undefined) return;
    const motion = tvMotion();
    // The grid counts as moving for the whole glide, whose slow end would
    // otherwise read as settling and load the pages it passes.
    if (motion === "smooth") nudge(glideMs + settleMs);
    scrollPage(to, motion);
  };
  const steps: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -columns, ArrowDown: columns };
  const onKeyDown = (e: KeyboardEvent) => {
    const by = steps[e.key];
    if (by === undefined || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    // Repeats can come before the previous target's card renders, so they
    // step on from that target.
    // On TV, focus reaches the card a moment after it is asked for, so a
    // repeat in that moment, with focus still where it was, steps on from the
    // card asked for too.
    const arriving =
      isTv &&
      target.current !== undefined &&
      performance.now() - target.current.at < arrivalMs &&
      document.activeElement === target.current.from;
    const pending =
      target.current && (focused.current !== target.current || arriving) ? target.current.index : undefined;
    const from = pending ?? Number((e.target as HTMLElement).closest("[data-index]")?.getAttribute("data-index"));
    if (Number.isNaN(from)) return;
    // Past the first card or the last row, the page handles the key, for
    // example to move up into the header. Moving down from a row above a
    // shorter last row goes to the last card.
    const last = items.length - 1;
    const lastRow = Math.floor(last / columns);
    if (from + by < 0 || (from + by > last && Math.floor(from / columns) === lastRow)) return;
    // On TV the first and last columns' sideways keys are the rail's, and
    // the letter rail's, not the next row's.
    if (
      isTv &&
      ((e.key === "ArrowLeft" && from % columns === 0) || (e.key === "ArrowRight" && from % columns === columns - 1))
    )
      return;
    const to = Math.min(from + by, last);
    e.preventDefault();
    nudge();
    if (isTv) glideToRow(Math.floor(to / columns));
    else virtualizer.scrollToIndex(Math.floor(to / columns), { align: "auto", behavior: "instant" });
    target.current = { index: to, at: performance.now(), from: e.target };
    setFocusTarget(target.current);
  };

  // On TV one glow lights the focused card and slides from card to card (tv/focusGlow.tsx).
  const { glows, onFocus, onBlur } = useFocusGlow(containerRef);

  return (
    // The grid steps by its own rows, past the ones not rendered, so its
    // arrows come before spatial navigation's.
    <TvFocusGroup focusKey="grid" part="grid-focus" ownKeys>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: the arrow keys of the cards inside, which are the controls. */}
      <div
        ref={containerRef}
        data-part="grid"
        data-view={view}
        data-tv-glows={isTv || undefined}
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
        onKeyDown={onKeyDown}
        onFocus={onFocus}
        onBlur={onBlur}
      >
        {glows}
        <ArtworkLoads value={!moving}>
          {virtualRows.map((row) => {
            const start = row.index * columns;
            const rowItems = items.slice(start, start + columns);
            return (
              <div
                key={row.key}
                className={
                  view === "grid"
                    ? "absolute inset-x-0 top-0 grid gap-x-(--layout-grid-gap) tv:gap-x-(--layout-tv-row-gap)"
                    : "absolute inset-x-0 top-0"
                }
                style={{
                  // On TV a row is placed by its top rather than a transform, which
                  // would make each row a stacking context of its own: a focused
                  // card's raised z-index then lifts it above every row, not only
                  // its own, so while its focus animates no row painted after it
                  // overlaps it, which would put that row on a compositor layer
                  // of its own, which a TV rasterizes in software on each press.
                  ...(isTv
                    ? { top: row.start - scrollMargin }
                    : { transform: `translateY(${row.start - scrollMargin}px)` }),
                  gridTemplateColumns: view === "grid" ? `repeat(${columns}, minmax(0, 1fr))` : undefined,
                }}
              >
                {rowItems.map((item, i) => (
                  <Cell
                    // biome-ignore lint/suspicious/noArrayIndexKey: a cell is a place in the grid, so focus stays as a card replaces its placeholder.
                    key={start + i}
                    index={start + i}
                    item={item}
                    shape={shape}
                    view={view}
                    width={cardWidth}
                    renderCard={renderCard}
                    renderListRow={renderListRow}
                  />
                ))}
              </div>
            );
          })}
        </ArtworkLoads>
      </div>
    </TvFocusGroup>
  );
}

/** Fetches the grid's pages that failed again, each time it settles. */
export function useRefetchFailed(
  pages: readonly { isError: boolean; isFetching: boolean; refetch: () => unknown }[],
  settles: number,
) {
  const refetch = useEffectEvent(() => {
    for (const page of pages) if (page.isError && !page.isFetching) void page.refetch();
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: as the grid settles, not as each page fails.
  useEffect(() => refetch(), [settles]);
}

/** What an arrow key focuses in a cell: its card's link, or its first control. */
const focusable = "a[href], button";

/**
 * Focuses the card in a cell. On TV that is the card's focusable, which
 * spatial navigation knows by its key, so the arrows go on from there.
 */
function focusCell(cell: HTMLElement) {
  const key = cell.querySelector<HTMLElement>("[data-tv-focus-key]")?.dataset.tvFocusKey;
  if (isTv && key) focusTv(key);
  else (cell.querySelector<HTMLElement>(focusable) ?? cell).focus({ preventScroll: true });
}

const GridCard = memo(function GridCard<T extends { id: string }>({
  item,
  first,
  width,
  renderCard,
}: {
  item: T;
  first: boolean;
  width: number;
  renderCard: Props<T>["renderCard"];
}) {
  return (
    <TvFocusable focusKey={`card:grid:${item.id}`} kind="card" autoFocus={first} scroll="none" className={tvCardClass}>
      {renderCard(item, width)}
    </TvFocusable>
  );
}) as <T extends { id: string }>(props: {
  item: T;
  first: boolean;
  width: number;
  renderCard: Props<T>["renderCard"];
}) => ReactNode;

/**
 * One item's place in the grid. If it has focus while its page loads, as
 * when a held arrow key passes unloaded rows, it hands focus to the card
 * when the card arrives.
 */
const Cell = memo(function Cell<T extends { id: string }>({
  index,
  item,
  shape,
  view,
  width,
  renderCard,
  renderListRow,
}: Pick<Props<T>, "shape" | "view" | "renderCard" | "renderListRow"> & {
  index: number;
  item: T | undefined;
  width: number;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const loaded = item !== undefined;
  useEffect(() => {
    const cell = ref.current;
    if (loaded && cell && document.activeElement === cell) focusCell(cell);
  }, [loaded]);
  return (
    <div ref={ref} data-index={index} tabIndex={-1}>
      {/* The card is its item's, so none of one item's state passes to another. */}
      <Fragment key={item?.id}>
        {item === undefined ? (
          <CardSkeleton shape={shape} />
        ) : view === "grid" ? (
          <GridCard item={item} first={index === 0} width={width} renderCard={renderCard} />
        ) : (
          renderListRow(item)
        )}
      </Fragment>
    </div>
  );
}) as <T extends { id: string }>(
  props: Pick<Props<T>, "shape" | "view" | "renderCard" | "renderListRow"> & {
    index: number;
    item: T | undefined;
    width: number;
  },
) => ReactNode;
