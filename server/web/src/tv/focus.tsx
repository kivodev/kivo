// The TV build's focus building blocks, for screens to wrap their cards,
// buttons and rows in. Outside the TV build they render only their
// children, ignoring their own props, className and part among them, so a
// screen uses them on every build and styles its own elements.

import type { Direction, FocusableComponent } from "@noriginmedia/norigin-spatial-navigation";
import { type FocusEvent, type ReactNode, type RefObject, useEffect, useEffectEvent } from "react";
import { isTv } from "./flag";
import { cancelTvFocus, settleTvFocus } from "./focusMemory";
import { norigin } from "./norigin";
import { currentTvFocus, focusTv, TV_CONTENT, TV_RAIL } from "./spatial";

type TvDirection = Direction;

type FocusableOptions = {
  /**
   * A key for the item, stable across visits and unique on the screen, such
   * as `card:continue:${work.id}`. Back restores focus by it, so an item
   * without one can't get focus back.
   */
  focusKey?: string;
  /** OK on it. Without it, OK presses the first link or button inside. */
  onPress?: () => void;
  /** Runs as it takes focus, as a header follows the focused card. */
  onFocus?: () => void;
  /** An arrow pressed on it. Return false to stop focus moving. */
  onArrow?: (direction: TvDirection) => boolean;
};

// OK on an element that isn't a link or button itself presses the first
// one inside, as clicking a card opens it.
function press(node: Element | null) {
  const control = node?.matches("a[href], button, [role=button]")
    ? node
    : node?.querySelector("a[href], button, [role=button]");
  if (control instanceof HTMLElement) control.click();
}

/**
 * Registers the caller's element for spatial navigation, for TV-only parts
 * such as the shell's rail; a screen uses TvFocusable instead. Spread
 * `attributes` on the element with `ref`. Call it only in the TV build.
 */
export function useTvFocusable<E extends HTMLElement>({ focusKey, onPress, onFocus, onArrow }: FocusableOptions = {}) {
  // OK goes to the focused item, which has the page's focus too.
  const {
    ref,
    focused,
    focusKey: key,
  } = norigin().useFocusable<object, E>({
    focusKey,
    onEnterPress: () => (onPress ? onPress() : press(document.activeElement)),
    onFocus: () => onFocus?.(),
    onArrowPress: (direction) => onArrow?.(direction as TvDirection) ?? true,
  });
  // A screen's items arrive with its data, and the one to focus may be
  // among them.
  useEffect(() => settleTvFocus(), []);
  return {
    ref: ref as RefObject<E>,
    focused,
    focusKey: key,
    attributes: { "data-tv-focusable": "", "data-tv-focus-key": key },
  };
}

type TvFocusableProps = FocusableOptions & {
  /**
   * How it shows focus (index.css). A card lifts the element inside marked
   * `data-tv-ring`, its artwork, by 1.1 on a glow. A button turns white and
   * grows by 1.05: the control it wraps does, its first child, not the
   * wrapper. A field's input turns white, without its label. A row as wide
   * as the page, such as an episode or a setting, turns white whole without
   * scaling, and the page scrolls to keep a row's worth of its list in view
   * past it.
   */
  kind?: "card" | "button" | "field" | "row";
  /**
   * For a button, which element lights up when the first child isn't the
   * control: `inside`, the one marked `data-tv-face` within it, or `none`,
   * for an item that shows focus itself, as the letter rail does.
   */
  face?: "inside" | "none";
  /** The screen's initial focus when it opens anew, such as a detail page's Play. */
  autoFocus?: boolean;
  /**
   * `none` keeps the page from scrolling to it as it takes focus, for an
   * item whose row or grid scrolls itself.
   */
  scroll?: "nearest" | "none";
  /**
   * Down passes over it to what's below, and only Up lands on it, for a
   * small control between a line of items and the row it heads, such as a
   * season's watched check.
   */
  skipDown?: boolean;
  /** Its data-part, on TV only. */
  part?: string;
  /** Its classes, on TV only. */
  className?: string;
  children: ReactNode;
};

/**
 * Something focus can move to with the arrows: a card, a button, a chip.
 * On TV it wraps the card or control in a div that takes focus and shows
 * it on what it wraps (see `kind`), and OK presses the link or button
 * inside it, or calls `onPress`. Outside the TV build it renders only its
 * children.
 *
 * ```tsx
 * <TvFocusGroup focusKey={`row:${row.id}`} part="row">
 *   {row.items.map((work) => (
 *     <TvFocusable key={work.id} focusKey={`card:${row.id}:${work.id}`} kind="card">
 *       <WorkCard work={work} />
 *     </TvFocusable>
 *   ))}
 * </TvFocusGroup>
 * ```
 */
export function TvFocusable(props: TvFocusableProps) {
  return isTv ? <TvFocusableItem {...props} /> : props.children;
}

function TvFocusableItem({
  kind = "button",
  face,
  autoFocus,
  scroll,
  skipDown,
  part,
  className,
  children,
  ...options
}: TvFocusableProps) {
  const { ref, focusKey, attributes } = useTvFocusable<HTMLDivElement>(options);
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: it only watches where focus lands, for the control inside.
    <div
      ref={ref}
      tabIndex={-1}
      {...attributes}
      onFocus={(e) => handBack(e, focusKey)}
      data-tv-focus={kind}
      data-tv-wrap=""
      data-tv-face={face}
      data-tv-autofocus={autoFocus || undefined}
      data-tv-scroll={scroll}
      data-tv-skip-down={skipDown || undefined}
      data-part={part}
      className={className}
    >
      {children}
    </div>
  );
}

// Focus that lands on the item from outside Norigin goes on to the item as
// Norigin's focus: react-aria gives focus back to a popover's button when
// the popover closes, and the arrows there would act on the button, which
// opens the popover again. Focus moved inside on purpose, as OK on a field
// does, or onto text being typed, stays. React passes focus in a portal,
// such as a popover the item opened, up through the item too, but that
// focus isn't the item's.
function handBack(e: FocusEvent<HTMLDivElement>, focusKey: string) {
  const target = e.target as Element;
  if (!e.currentTarget.contains(target)) return;
  const inside = target !== e.currentTarget;
  if (inside && target.closest("input, textarea, [contenteditable=true]")) return;
  if (inside ? !e.currentTarget.contains(e.relatedTarget) : currentTvFocus() !== focusKey) focusTv(focusKey);
}

type TvFocusGroupProps = {
  /** A key for the group, stable across visits and unique on the screen, such as `row:${row.id}`. */
  focusKey: string;
  /**
   * Whether focus coming back into the group returns to the item it left
   * from, as moving Up into a row does (default). Otherwise it goes to
   * `preferredChild`, or the item nearest the top left.
   */
  remember?: boolean;
  /** The focus key of the item focus enters on, before one is remembered. */
  preferredChild?: string;
  /**
   * The focus key of the item Up or Down enters the group on from outside
   * it, rather than the one nearest, such as the season a series' page
   * shows. Without it, or while it isn't there, the nearest.
   */
  enterOn?: string;
  /**
   * Whether Up or Down coming back into the group returns to the item focus
   * last left it from, and `enterOn` until there's been one, as Up from a
   * row returns to the hero's button you came from. The key is kept on the
   * element, so moving along the group renders nothing.
   */
  returns?: boolean;
  /** Keeps focus inside in these directions, or all of them, as in a dialog. */
  trap?: boolean | TvDirection[];
  /** Runs as focus moves into the group or out of it. */
  onFocusWithin?: (within: boolean) => void;
  /** Takes focus when an arrow is pressed with nothing focused, as the rail does. */
  takesStrayFocus?: boolean;
  /**
   * Whether it moves focus among its items with its own arrow keys, as the
   * library's virtual grid does. Spatial navigation then has the keys only
   * after it, the ones it didn't take.
   */
  ownKeys?: boolean;
  /**
   * Whether its items are in one line across, in order, as a row's cards
   * are. Left and Right then go to the item before or after, without
   * measuring every item in the line on each press.
   */
  line?: boolean;
  /**
   * Whether Up and Down keep to its items, by position as on the rest of
   * the screen, rather than going on to what's beside it, as Search's
   * results do beside the keyboard. Past its last row Down still goes to
   * the dock; otherwise focus stays at its ends.
   */
  column?: boolean;
  /**
   * Whether it scrolls its items itself, as Search's results do beside the
   * keyboard, so a row inside scrolls it rather than the page.
   */
  scroller?: boolean;
  /** Its element on TV: a row is a section, the rail a nav. */
  as?: "div" | "section" | "nav";
  /** Its aria-label, data-part and classes, on TV only. */
  label?: string;
  part?: string;
  className?: string;
  children: ReactNode;
};

/**
 * A group of focusables, such as a row, a grid, a hero's buttons or a
 * dialog. Arrows move between its items first, then out of it. Up and Down
 * on a screen go to the item directly above or below, whatever its group.
 * Focus coming back another way, such as from the rail, returns to the item
 * it left from. Outside the TV build it renders only its children.
 */
export function TvFocusGroup(props: TvFocusGroupProps) {
  return isTv ? <TvFocusGroupItem {...props} /> : props.children;
}

function TvFocusGroupItem({
  focusKey,
  remember = true,
  preferredChild,
  enterOn,
  returns = false,
  trap = false,
  onFocusWithin,
  takesStrayFocus = false,
  ownKeys = false,
  line = false,
  column = false,
  scroller = false,
  as: Element = "div",
  label,
  part,
  className,
  children,
}: TvFocusGroupProps) {
  const { FocusContext } = norigin();
  const { ref, hasFocusedChild } = norigin().useFocusable<object, HTMLElement>({
    focusKey,
    saveLastFocusedChild: remember,
    preferredChildFocusKey: preferredChild,
    trackChildren: onFocusWithin !== undefined,
    isFocusBoundary: trap !== false,
    forceFocus: takesStrayFocus,
    focusBoundaryDirections: Array.isArray(trap) ? trap : undefined,
    // A row whose focused card goes away focuses another of its cards. The
    // shell's content gets focus back by the screen's rules instead, as
    // its screens change.
    autoRestoreFocus: focusKey !== TV_CONTENT,
    nextFocusResolver: line ? nextInLine : undefined,
    measureChildrenLayout: !line,
  });
  const within = useEffectEvent((has: boolean) => onFocusWithin?.(has));
  useEffect(() => within(hasFocusedChild), [hasFocusedChild]);
  return (
    <FocusContext.Provider value={focusKey}>
      <Element
        ref={ref as RefObject<HTMLDivElement>}
        onFocus={
          returns
            ? (e) => {
                const key = (e.target as Element).closest<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey;
                if (key) e.currentTarget.dataset.tvEnter = key;
              }
            : undefined
        }
        aria-label={label}
        data-tv-group={focusKey}
        data-tv-enter={enterOn}
        data-tv-trap={trap !== false || undefined}
        data-tv-own-keys={ownKeys || undefined}
        data-tv-line={line || undefined}
        data-tv-column={column || undefined}
        data-tv-scroller={scroller || undefined}
        data-part={part}
        className={className}
      >
        {children}
      </Element>
    </FocusContext.Provider>
  );
}

// Norigin measures every item in a group on each arrow press, which for a
// row of thirty cards is thirty layout reads on a slow TV. In a line, Left
// and Right go to the item before or after in the page's order instead. At
// either end, and Up or Down, Norigin moves on from the group as it would.
function nextInLine(direction: string, focusKey: string, siblings: FocusableComponent[]): FocusableComponent | null {
  if (direction !== "left" && direction !== "right") return null;
  const inOrder = [...siblings].sort((a, b) =>
    a.node.compareDocumentPosition(b.node) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1,
  );
  const at = inOrder.findIndex((c) => c.focusKey === focusKey);
  if (at < 0) return null;
  return inOrder[direction === "left" ? at - 1 : at + 1] ?? null;
}

/**
 * For a screen with nothing to focus, such as an empty Home or Lists:
 * while `empty`, focus goes to the rail.
 */
export function useTvEmptyScreen(empty: boolean) {
  useEffect(() => {
    if (!isTv || !empty) return;
    cancelTvFocus();
    focusTv(TV_RAIL);
  }, [empty]);
}
