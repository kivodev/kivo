import { useLingui } from "@lingui/react/macro";
import tokens from "@shared/tokens.json";
import {
  type CSSProperties,
  type ReactNode,
  useCallback,
  useEffect,
  useEffectEvent,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { useFocusGlow } from "@/tv/focusGlow";
import type { ArtworkShape } from "./Artwork";
import { Button } from "./Button";
import { Icon } from "./Icon";
import { revealTvCard, revealTvRow, settleTvTrack, tvCardClass } from "./tvLayout";

const tvLayout = tokens.tv.web.layout;

type Props<T> = {
  /** The row's heading. Without one, `header` heads the row. */
  title?: ReactNode;
  /**
   * Heads the row in place of its title line, outside the row's focus group,
   * for a heading with controls of its own, such as a season's watched
   * check, which then take focus apart from the cards. On TV the page
   * scrolls the row to the top of the screen with it.
   */
  header?: ReactNode;
  /** A link beside the title, such as to the whole library. */
  action?: ReactNode;
  /** On TV, whether the action ends the row even when the row fits the screen. */
  endsInAction?: boolean;
  /** Landscape rows hold fewer, wider cards. Square cards are as wide as posters. */
  shape: ArtworkShape;
  items: readonly T[];
  itemKey: (item: T) => string;
  renderItem: (item: T) => ReactNode;
  /** Identifies the row for tests, such as `continueWatching`. */
  id?: string;
  /**
   * The row's focus key on TV, unique on the screen. Back restores focus to
   * a card by it. It is the id unless the screen has several rows of one, or
   * neither is given.
   */
  focusKey?: string;
  /** Whether the row's first card is the screen's initial focus on TV. */
  autoFocus?: boolean;
  /**
   * On TV, how many posters fit across the row, when it's narrower than the
   * content, so its cards stay the size they are elsewhere. Landscape cards
   * fit half as many.
   */
  tvAcross?: number;
  /**
   * On TV, the key of the card Up or Down enters the row on from outside it,
   * rather than the nearest, and which the row opens showing, such as a
   * series' Next Up.
   */
  enterOn?: string;
  /** On TV, what OK and the arrows do on a card, for a card that does more than open what it links to. */
  cardFocus?: (item: T) => { onPress?: () => void; onArrow?: (direction: string) => boolean };
};

/**
 * A titled, horizontally scrolling row of cards (Home, Cast, Extras). It
 * shows no scrollbar. With a pointer, previous and next buttons show
 * beside the title while the row overflows. Touch and trackpads swipe.
 *
 * On TV a row holds six posters or three landscape cards across, as many
 * as a library grid, with more room between them. Its cards take focus, and they move sideways only as
 * far as shows the focused card while the page scrolls the row to the top
 * of the screen. The action becomes a last card, since a remote has no
 * pointer for a link in the header, on a row longer than the screen.
 */
export function Row<T>({
  title,
  action,
  shape,
  items,
  itemKey,
  renderItem,
  id,
  focusKey: givenKey,
  autoFocus = false,
  tvAcross,
  header,
  enterOn,
  cardFocus,
  endsInAction = false,
}: Props<T>) {
  const { t } = useLingui();
  const generatedKey = useId();
  const focusKey = givenKey ?? id ?? generatedKey;
  const width = shape === "landscape" ? "w-(--layout-landscape-min-width)" : "w-(--layout-poster-min-width)";
  const across = shape === "landscape" ? "tv:tv-across-landscape" : "tv:tv-across-poster";
  const narrowAcross =
    tvAcross === undefined ? undefined : shape === "landscape" ? Math.max(1, Math.round(tvAcross / 2)) : tvAcross;
  const narrow = narrowAcross === undefined ? undefined : ({ "--tv-across": narrowAcross } as CSSProperties);
  // How many cards fit across on TV: the narrow count, or the ones the
  // stylesheet sizes cards by (--layout-tv-*-across, from the tokens).
  const tvFit = narrowAcross ?? Number(shape === "landscape" ? tvLayout.landscapeAcross : tvLayout.posterAcross);
  const section = useRef<HTMLElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const { glows, onFocus: onGlowFocus, onBlur: onGlowBlur } = useFocusGlow(list);
  const [edges, setEdges] = useState({ start: true, end: true });

  // The edges are for the pointer's buttons, which a TV hides, so a TV
  // doesn't measure them: each measure reads the row's layout and, with new
  // edges, renders the row again, on every scroll event of every arrow press.
  const measure = useCallback(() => {
    const el = list.current;
    if (!el) return;
    const start = el.scrollLeft <= 1;
    const end = el.scrollLeft + el.clientWidth >= el.scrollWidth - 1;
    setEdges((e) => (e.start === start && e.end === end ? e : { start, end }));
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: new items change the scroll width, which the observer doesn't see.
  useEffect(() => {
    const el = list.current;
    if (!el || isTv) return;
    measure();
    const resized = new ResizeObserver(measure);
    resized.observe(el);
    return () => resized.disconnect();
  }, [measure, items.length]);

  // A row whose first card changes goes back to its start. Left alone, the
  // browser keeps the card the row was snapped to in place, and cards put
  // ahead of it are off to its left. A TV's cards ride a track instead.
  const firstKey = items[0] === undefined ? undefined : itemKey(items[0]);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the first card's key is what changes.
  useLayoutEffect(() => {
    if (!isTv && list.current) list.current.scrollLeft = 0;
  }, [firstKey]);

  // On TV, See all ends only a row longer than the screen, since a shorter
  // row already shows all there is, unless the row always ends in it.
  const seeAll = isTv && Boolean(action) && (endsInAction || items.length > tvFit);

  // On TV the track keeps its place through new cards, so it's put back
  // where it belongs once they change, before they show, and as the row's
  // width changes with the window's. Without a focused card it shows the
  // one it enters on, which a screen still hidden as it opens can't lay out
  // until it shows and the row first takes its width.
  const cards = items.map(itemKey).join("\n");
  const settle = useEffectEvent(() => {
    const entry = enterOn
      ? list.current?.querySelector(`[data-tv-focus-key="${CSS.escape(`card:${focusKey}:${enterOn}`)}"]`)
      : null;
    settleTvTrack(list.current, entry?.closest("li"));
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: the track settles when its cards change.
  useLayoutEffect(() => {
    if (isTv) settle();
  }, [cards, seeAll, enterOn]);
  useEffect(() => {
    const el = list.current;
    if (!el || !isTv) return;
    const resized = new ResizeObserver(() => settle());
    resized.observe(el);
    return () => resized.disconnect();
  }, []);

  // One page is the visible width minus a card's gap, so the next card's
  // edge stays in view.
  const page = (direction: 1 | -1) => {
    const el = list.current;
    el?.scrollBy({ left: direction * (el.clientWidth - 16), behavior: "smooth" });
  };
  const overflows = !(edges.start && edges.end);
  const reveal = () => {
    revealTvCard(list.current, document.activeElement?.closest("li") ?? null);
    revealTvRow(section.current);
  };

  const cardsGroup = (
    <TvFocusGroup
      focusKey={`row:${focusKey}`}
      enterOn={enterOn && `card:${focusKey}:${enterOn}`}
      line
      className="flex min-w-0 flex-col gap-3"
    >
      {header === undefined && (
        // The buttons keep their place while hidden, so showing them never moves the title, its link or the row.
        <div className="flex min-h-10 items-center justify-between gap-4">
          <h2 className="min-w-0 truncate text-lg font-semibold text-text">{title}</h2>
          <div className="flex shrink-0 items-center gap-2 whitespace-nowrap">
            {!isTv && action}
            <div className={`hidden gap-1 sm:pointer-fine:flex tv:invisible ${overflows ? "" : "invisible"}`}>
              <Button
                variant="ghost"
                size="icon"
                aria-label={t`Previous`}
                isDisabled={edges.start}
                onPress={() => page(-1)}
              >
                <Icon name="chevronLeft" />
              </Button>
              <Button variant="ghost" size="icon" aria-label={t`Next`} isDisabled={edges.end} onPress={() => page(1)}>
                <Icon name="chevronRight" />
              </Button>
            </div>
          </div>
        </div>
      )}
      {/*
       * On TV the cards are clipped past the row, by the bleed above, below
       * and on the left, so the focused card's lift and glow aren't cut off
       * at its edges. On the right they run on to the screen's edge, past
       * the page's padding and the overscan, as TV apps' rows do, so the
       * next card shows as more to come. The cards move along on their
       * track (revealTvCard), which keeps the focused one clear of the
       * overscan; the part of the card before the first whole one that the
       * bleed would show beside the rail is hidden (data-tv-scrolled). The
       * cards are sized to fit `--tv-across` across the row, the row's gap
       * apart, which leaves a focused card's lift and glow clear of the
       * cards beside it.
       */}
      <div className="tv:-my-(--layout-tv-row-bleed) tv:-mr-(--layout-tv-row-end) tv:-ml-(--layout-tv-row-bleed) tv:overflow-clip tv:py-(--layout-tv-row-bleed) tv:pr-(--layout-tv-row-end) tv:pl-(--layout-tv-row-bleed)">
        {/* On TV one glow lights the focused card, riding the track as it slides (tv/focusGlow.tsx). */}
        <ul
          ref={list}
          data-part="row-cards"
          data-tv-glows={isTv || undefined}
          onScroll={isTv ? undefined : measure}
          onFocus={onGlowFocus}
          onBlur={onGlowBlur}
          style={narrow}
          className={`-mx-1 flex snap-x scroll-px-1 gap-4 overflow-x-auto px-1 pb-2 scrollbar-hidden tv:relative tv:mx-0 tv:snap-none tv:gap-(--layout-tv-row-gap) tv:overflow-visible tv:px-0 tv:transition-transform tv:duration-(--duration-glide) tv:ease-out ${across}`}
        >
          {items.map((item, i) => {
            const key = itemKey(item);
            return (
              <li key={key} className={`shrink-0 snap-start ${width} tv:tv-card-width`}>
                <TvFocusable
                  focusKey={`card:${focusKey}:${key}`}
                  kind="card"
                  autoFocus={autoFocus && i === 0}
                  scroll="none"
                  onFocus={reveal}
                  className={tvCardClass}
                  {...cardFocus?.(item)}
                >
                  {renderItem(item)}
                </TvFocusable>
              </li>
            );
          })}
          {seeAll && (
            <li className="shrink-0 snap-start tv:tv-card-width">
              <TvFocusable
                focusKey={`card:${focusKey}:see-all`}
                kind="card"
                scroll="none"
                onFocus={reveal}
                className={tvCardClass}
              >
                <div
                  data-part="see-all"
                  data-tv-ring
                  className={`flex items-center justify-center rounded-md border border-border bg-surface-raised text-text ${shape === "landscape" ? "aspect-landscape" : shape === "square" ? "aspect-square" : "aspect-poster"}`}
                >
                  {action}
                </div>
              </TvFocusable>
            </li>
          )}
          {glows && (
            <li aria-hidden data-tv-glow-host className="contents">
              {glows}
            </li>
          )}
        </ul>
      </div>
    </TvFocusGroup>
  );

  return (
    // Scrolling to the row on TV leaves the overscan above it.
    <section
      ref={section}
      data-part="row"
      data-row={id}
      className="flex min-w-0 flex-col gap-3 tv:scroll-mt-(--layout-tv-overscan-y)"
    >
      {/* A header and the cards are one block, which a TV measures as the row's content (revealTvRow). */}
      {header === undefined ? (
        cardsGroup
      ) : (
        <div className="flex min-w-0 flex-col gap-3">
          {header}
          {cardsGroup}
        </div>
      )}
    </section>
  );
}
