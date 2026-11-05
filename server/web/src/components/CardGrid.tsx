import type { ReactNode } from "react";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import type { ArtworkShape } from "./Artwork";
import { tvCardClass } from "./tvLayout";

type Props<T> = {
  shape: ArtworkShape;
  items: readonly T[];
  itemKey: (item: T) => string;
  renderItem: (item: T) => ReactNode;
  /**
   * The grid's focus key on TV, unique on the screen. Back restores focus to
   * a card by it.
   */
  focusKey?: string;
  /** Whether the first card is the screen's initial focus on TV. */
  autoFocus?: boolean;
};

/**
 * A plain wrapping grid of cards, for short lists such as a collection's
 * members or search results. Library grids use the virtualized grid. On TV
 * it is six posters or three landscape cards across, as in a library, and
 * its cards take focus.
 */
export function CardGrid<T>({ shape, items, itemKey, renderItem, focusKey = "grid", autoFocus = false }: Props<T>) {
  const columns =
    shape !== "landscape"
      ? "grid-cols-(--layout-poster-grid) tv:tv-across-poster"
      : "grid-cols-(--layout-landscape-grid) tv:tv-across-landscape";
  return (
    <TvFocusGroup focusKey={`grid:${focusKey}`} part="grid-focus">
      <ul
        data-part="grid"
        className={`grid gap-x-(--layout-grid-gap) gap-y-6 tv:gap-x-(--layout-tv-row-gap) ${columns} tv:tv-grid-cols`}
      >
        {items.map((item, i) => (
          <li key={itemKey(item)}>
            <TvFocusable
              focusKey={`card:${focusKey}:${itemKey(item)}`}
              kind="card"
              autoFocus={autoFocus && i === 0}
              className={tvCardClass}
            >
              {renderItem(item)}
            </TvFocusable>
          </li>
        ))}
      </ul>
    </TvFocusGroup>
  );
}
