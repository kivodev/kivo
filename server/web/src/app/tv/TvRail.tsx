import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { type ReactNode, useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import { libraryQueries } from "@/api/libraries";
import type { Library } from "@/api/schemas";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { TvFocusGroup, useTvFocusable } from "@/tv/focus";
import { firstTvFocusIn } from "@/tv/focusMemory";
import { focusTv, TV_CONTENT, TV_RAIL } from "@/tv/spatial";

/** The focus key of a rail entry: `rail:search`, `rail:home`, `rail:library:<id>`, `rail:more`. */
export type RailKey = `rail:${string}`;

export const railKeys = {
  search: "rail:search",
  home: "rail:home",
  favorites: "rail:favorites",
  more: "rail:more",
  settings: "rail:settings",
  library: (id: string): RailKey => `rail:library:${id}`,
} as const;

export type RailLinkTarget =
  | { to: "/" | "/search" | "/favorites" | "/settings" }
  | { to: "/libraries/$libraryId"; params: { libraryId: string } };

/**
 * Goes to a rail destination in place of the app's first history entry:
 * back to that entry, then replacing it once there. History then holds the
 * destination and the pages opened from it, however many times a person
 * goes from a page to the rail. Replacing only the page it's chosen from
 * would leave one entry more each time. The entry's index is the router's
 * own count of this app's entries, as its canGoBack uses.
 */
export function useGoToDestination(): (link: RailLinkTarget) => void {
  const router = useRouter();
  return (link) => {
    const index = router.history.location.state.__TSR_index ?? 0;
    if (index <= 0) {
      void router.navigate({ ...link, replace: true });
      return;
    }
    const arrived = router.history.subscribe(() => {
      arrived();
      void router.navigate({ ...link, replace: true });
    });
    router.history.go(-index);
  };
}

// Leaves the rail for the screen, if the screen has anything to focus.
function leaveRail() {
  if (firstTvFocusIn(TV_CONTENT) !== undefined) focusTv(TV_CONTENT);
}

// Right leaves the rail for the screen. The open rail covers the screen's
// left edge, so the nearest item to the right isn't the one to go back to.
function railArrow(direction: string) {
  if (direction !== "right") return true;
  leaveRail();
  return false;
}

// A rail entry's part in the rail as it opens and closes: whether the rail
// is open, and `onArrow` for the entry's arrows, which show a rail closed
// on a choice again.
type RailState = { open: boolean; onArrow: (direction: string) => boolean };

// An entry's icon stays where it is as the rail opens, with its name beside
// it. Closed, the rail recedes behind the screen: its icons are dimmed but
// for the current screen's, which keeps the accent so the rail says where
// the screen is without competing with its artwork. The entry just chosen
// keeps focus in the closed rail while its screen loads, and shows it in
// the accent, as the screen's own entry does, rather than filled.
function itemClass(current: boolean, open: boolean): string {
  return [
    "flex h-(--layout-tv-rail-item-size) shrink-0 items-center gap-6 rounded-md px-4 text-lg font-medium",
    current ? "text-accent" : "text-text-muted",
    open ? "w-full focus:bg-text focus:text-bg" : "w-(--layout-tv-rail-item-size) focus:text-accent focus:opacity-100",
    !open && !current ? "opacity-40" : "",
  ].join(" ");
}

type RailEntryProps = {
  railKey: RailKey;
  current: boolean;
  rail: RailState;
  icon: ReactNode;
  label: ReactNode;
};

/** An entry's name, beside its icon in the open rail, and for screen readers in the closed one. */
function RailLabel({ open, children }: { open: boolean; children: ReactNode }) {
  return (
    <span data-part={open ? "tv-rail-label" : undefined} className={open ? "truncate" : "sr-only"}>
      {children}
    </span>
  );
}

function RailLink({
  railKey,
  current,
  rail,
  icon,
  label,
  link,
  onChoose,
}: RailEntryProps & { link: RailLinkTarget; onChoose: (key: RailKey) => void }) {
  const { open } = rail;
  const { ref, attributes } = useTvFocusable<HTMLAnchorElement>({ focusKey: railKey, onArrow: rail.onArrow });
  const goTo = useGoToDestination();
  return (
    <Link
      ref={ref}
      {...attributes}
      {...link}
      // Choosing the screen that's showing goes back to it. Another
      // destination takes the place of the app's first history entry.
      onClick={(e) => {
        e.preventDefault();
        if (current) leaveRail();
        else {
          onChoose(railKey);
          goTo(link);
        }
      }}
      data-tv-focus="button"
      data-tv-fill=""
      data-part="tv-rail-entry"
      aria-current={current ? "page" : undefined}
      className={itemClass(current, open)}
    >
      {icon}
      <RailLabel open={open}>{label}</RailLabel>
    </Link>
  );
}

function MoreLibraries({
  current,
  rail,
  expanded,
  onExpand,
}: Omit<RailEntryProps, "railKey" | "icon" | "label"> & { expanded: boolean; onExpand: () => void }) {
  const { t } = useLingui();
  const { open } = rail;
  const { ref, attributes } = useTvFocusable<HTMLButtonElement>({ focusKey: railKeys.more, onArrow: rail.onArrow });
  return (
    <AriaButton
      ref={ref}
      {...attributes}
      onPress={onExpand}
      aria-expanded={expanded}
      aria-current={current ? "page" : undefined}
      aria-label={t`More libraries`}
      data-tv-focus="button"
      data-tv-fill=""
      data-part="more-libraries"
      className={itemClass(current, open)}
    >
      <Icon name="more" className="size-8 shrink-0" />
      <RailLabel open={open}>
        <Trans>More libraries</Trans>
      </RailLabel>
    </AriaButton>
  );
}

function LibraryEntry({
  library,
  current,
  rail,
  onChoose,
}: {
  library: Library;
  current: boolean;
  rail: RailState;
  onChoose: (key: RailKey) => void;
}) {
  return (
    <RailLink
      railKey={railKeys.library(library.id)}
      link={{ to: "/libraries/$libraryId", params: { libraryId: library.id } }}
      current={current}
      rail={rail}
      onChoose={onChoose}
      icon={<LibraryKindIcon kind={library.kind} className="size-8 shrink-0" />}
      label={library.name}
    />
  );
}

/**
 * The TV's left rail: Search, Home, Favorites, a line, the pinned
 * libraries, More libraries, then Settings. It is a quiet column of icons at
 * the screen's left edge until focus moves onto it, then a panel with their
 * names, floating over the screen dimmed behind it. Closed, it shows only
 * Search, Home, Favorites and Settings, and the library the screen is in:
 * every library's icon is one of a few kinds, so a column of them told the
 * libraries apart no better than it cluttered the edge. Search, Home and
 * Favorites stay at the top and Settings at the bottom; the libraries
 * between them scroll. `marked` is the destination the screen sits on,
 * where focus enters the rail; a library that isn't pinned marks More
 * libraries, which opens the rest of the libraries under it. The
 * libraries are read again each time the rail opens.
 *
 * It opens by widening from its icons as the screen dims (index.css).
 * Choosing a destination closes it at once, while focus stays on the
 * chosen entry until the screen has something to focus.
 */
export function TvRail({ marked }: { marked: RailKey }) {
  const { t } = useLingui();
  const query = useSuspenseQuery(libraryQueries.list());
  const libraries = query.data.items;
  const pinned = libraries.filter((l) => l.pinned);
  const others = libraries.filter((l) => !l.pinned);
  const [within, setWithin] = useState(false);
  const [expanded, setExpanded] = useState(false);
  // The destination chosen, whose screen is on its way. The rail shows
  // closed meanwhile, with the chosen entry, until focus leaves it or an
  // arrow moves in it, as on a screen that hands focus back to the rail.
  const [chosen, setChosen] = useState<RailKey>();
  const { refetch } = query;
  const onFocusWithin = (has: boolean) => {
    setWithin(has);
    setChosen(undefined);
    if (has) void refetch();
    // A closed rail shows only icons, so More libraries' list folds away.
    else setExpanded(false);
  };
  const open = within && chosen === undefined;
  const rail: RailState = {
    open,
    onArrow: (direction) => {
      setChosen(undefined);
      return railArrow(direction);
    },
  };
  const shows = (key: RailKey) => open || marked === key || chosen === key;

  return (
    <>
      {open && <div aria-hidden className="fixed inset-0 z-30 bg-scrim" data-part="tv-rail-scrim" />}
      <TvFocusGroup
        as="nav"
        focusKey={TV_RAIL}
        label={t`Main`}
        part="tv-rail"
        remember={false}
        preferredChild={marked}
        trap={["left", "up", "down"]}
        onFocusWithin={onFocusWithin}
        takesStrayFocus
        className={`fixed z-40 flex flex-col gap-2 ${
          open
            ? "inset-y-(--layout-tv-rail-inset) left-(--layout-tv-rail-inset) w-(--layout-tv-rail-open-width) px-(--layout-tv-rail-open-padding-x) py-(--layout-tv-rail-open-padding-y)"
            : "inset-y-0 left-0 w-(--layout-tv-rail-collapsed-width) py-(--layout-tv-overscan-y) pl-(--layout-tv-rail-left)"
        }`}
      >
        {/* The open rail's panel, behind its entries, which widens as it opens. */}
        {open && (
          <span
            aria-hidden
            data-part="tv-rail-panel"
            className="absolute inset-0 -z-10 rounded-lg bg-surface-raised shadow-overlay"
          />
        )}
        <RailLink
          railKey={railKeys.search}
          link={{ to: "/search" }}
          current={marked === railKeys.search}
          rail={rail}
          onChoose={setChosen}
          icon={<Icon name="search" className="size-8 shrink-0" />}
          label={<Trans>Search</Trans>}
        />
        <RailLink
          railKey={railKeys.home}
          link={{ to: "/" }}
          current={marked === railKeys.home}
          rail={rail}
          onChoose={setChosen}
          icon={<Icon name="home" className="size-8 shrink-0" />}
          label={<Trans>Home</Trans>}
        />
        <RailLink
          railKey={railKeys.favorites}
          link={{ to: "/favorites" }}
          current={marked === railKeys.favorites}
          rail={rail}
          onChoose={setChosen}
          icon={<Icon name="heart" className="size-8 shrink-0" />}
          label={<Trans>Favorites</Trans>}
        />
        {open && <hr className="my-2 border-border" />}
        <TvFocusGroup
          focusKey="tv:rail:libraries"
          part="tv-rail-libraries"
          className="-mx-2 flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 scrollbar-hidden"
        >
          {pinned
            .filter((library) => shows(railKeys.library(library.id)))
            .map((library) => (
              <LibraryEntry
                key={library.id}
                library={library}
                current={marked === railKeys.library(library.id)}
                rail={rail}
                onChoose={setChosen}
              />
            ))}
          {others.length > 0 && shows(railKeys.more) && (
            <MoreLibraries
              current={marked === railKeys.more}
              rail={rail}
              expanded={expanded}
              onExpand={() => setExpanded((e) => !e)}
            />
          )}
          {others
            .filter((library) => (expanded && open) || chosen === railKeys.library(library.id))
            .map((library) => (
              <LibraryEntry key={library.id} library={library} current={false} rail={rail} onChoose={setChosen} />
            ))}
        </TvFocusGroup>
        <RailLink
          railKey={railKeys.settings}
          link={{ to: "/settings" }}
          current={marked === railKeys.settings}
          rail={rail}
          onChoose={setChosen}
          icon={<Icon name="settings" className="size-8 shrink-0" />}
          label={<Trans>Settings</Trans>}
        />
      </TvFocusGroup>
    </>
  );
}
