import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Outlet, type ParsedLocation, useMatchRoute, useNavigate, useRouter } from "@tanstack/react-router";
import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { libraryQueries } from "@/api/libraries";
import { useLiveEvents } from "@/api/live";
import { workQueries } from "@/api/works";
import { ConnectionBanner } from "@/app/ConnectionBanner";
import { BookDock } from "@/features/player/BookPlayer";
import { useBookPlayer } from "@/features/player/useBookPlayer";
import { useTvShellBack } from "@/tv/back";
import { ExitDialog } from "@/tv/ExitDialog";
import { TvFocusGroup } from "@/tv/focus";
import { expectTvScreen, firstTvFocusIn, rememberTvFocus, settleTvFocus } from "@/tv/focusMemory";
import { currentTvFocus, focusTv, isTvFocusIn, leaveTvDock, pauseTv, TV_CONTENT, TV_RAIL } from "@/tv/spatial";
import { type RailKey, railKeys, TvRail, useGoToDestination } from "./TvRail";

// A history entry's key, which Back returns to with its focus.
function entryOf(location: ParsedLocation): string {
  return location.state.__TSR_key ?? location.href;
}

// The player's own element, unless its play failed. While it's on the
// screen the player has every key.
const playerSelector = '[data-part="player"]:not([data-failed])';

/**
 * Whether the player is on the screen and playing, rather than its route
 * alone: while the player loads, when its data fails and an error shows in
 * its place, or when a play fails and the player shows why, spatial
 * navigation and Back must stay on so the remote can reach Try again or go
 * back. It watches the page for the player's element and its failure
 * (data-failed), only on its route. The shell alone pauses spatial
 * navigation, so whichever it sees last, the player or its failure, the
 * remote ends up as the screen needs.
 */
function usePlayerOnScreen(watching: boolean): boolean {
  const subscribe = useCallback(
    (changed: () => void) => {
      if (!watching) return () => {};
      const observer = new MutationObserver(changed);
      observer.observe(document.body, {
        childList: true,
        subtree: true,
        attributes: true,
        attributeFilter: ["data-failed"],
      });
      return () => observer.disconnect();
    },
    [watching],
  );
  return useSyncExternalStore(subscribe, () => watching && document.querySelector(playerSelector) !== null);
}

// The rail entry of a library: its own if it's pinned, else More libraries.
function libraryKey(libraries: readonly { id: string; pinned?: boolean }[], libraryId: string): RailKey {
  return libraries.some((l) => l.id === libraryId && l.pinned) ? railKeys.library(libraryId) : railKeys.more;
}

/**
 * The rail destination the screen is, if it is one: Search, Home,
 * Lists, a library or Settings. A library that isn't pinned is under
 * More libraries.
 */
function useDestination(): RailKey | undefined {
  const matchRoute = useMatchRoute();
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  if (matchRoute({ to: "/" })) return railKeys.home;
  if (matchRoute({ to: "/search" })) return railKeys.search;
  if (matchRoute({ to: "/lists" })) return railKeys.lists;
  if (matchRoute({ to: "/settings" })) return railKeys.settings;
  const library = matchRoute({ to: "/libraries/$libraryId" });
  if (!library) return undefined;
  return libraryKey(libraries, library.libraryId);
}

/**
 * The rail entry of the library a work's page is in, once the work is
 * known. This only watches the work the page already reads, and asks for
 * nothing itself. A page not in a library has none, and keeps the entry
 * marked before it.
 */
function useWorkLibrary(): RailKey | undefined {
  const matchRoute = useMatchRoute();
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const work = matchRoute({ to: "/works/$workId" });
  const read = useQuery({ ...workQueries.detail(work ? work.workId : ""), enabled: false });
  const libraryId = work ? read.data?.libraryId : undefined;
  return libraryId === undefined ? undefined : libraryKey(libraries, libraryId);
}

/**
 * The TV build's signed-in layout: the rail at the left edge and
 * the screen beside it, clear of the edges a TV may crop. The player's
 * route has the whole screen, and once the player is on it, every key:
 * spatial navigation and Back wait.
 *
 * Back, when the screen doesn't handle it: on Home it moves focus
 * to the top-left card, then asks to exit; on another rail destination it
 * opens the rail on that destination, and Back in the rail then goes Home;
 * on a page above a destination it goes back to the page below. In the rail
 * over Home or such a page, it returns to the screen.
 *
 * A book that is playing docks its player along the bottom, below the
 * screen. The screen leaves room under its last row for it, and scrolls
 * focus clear of it.
 */
export function TvShell() {
  const router = useRouter();
  const navigate = useNavigate();
  const matchRoute = useMatchRoute();
  const live = useLiveEvents();
  const watching = matchRoute({ to: "/watch/$workId" }) !== false;
  const playing = usePlayerOnScreen(watching);
  const here = useDestination();
  const workLibrary = useWorkLibrary();
  const docked = useBookPlayer().book !== undefined && !watching;
  // The rail keeps the last destination marked on the pages above it, so a
  // person's page opened from Search keeps Search marked. A work's page
  // marks the work's library, so the rail shows where it is. Back still
  // treats it as a page above a destination, so `here` is not set to it.
  const [marked, setMarked] = useState<RailKey>(here ?? workLibrary ?? railKeys.home);
  const next = here ?? workLibrary;
  if (next !== undefined && next !== marked) setMarked(next);
  const [exiting, setExiting] = useState(false);
  const goToDestination = useGoToDestination();

  useEffect(() => {
    pauseTv(playing);
    // Back from the player arrived while focus was paused for it, so its focus is still to be restored.
    if (!playing) settleTvFocus();
    return () => pauseTv(false);
  }, [playing]);

  // The page scrolls focus clear of the dock, as far above it as the
  // overscan is above the screen's bottom edge.
  useEffect(() => {
    if (!docked) return;
    const root = document.documentElement.style;
    root.scrollPaddingBottom = "calc(var(--layout-mini-player-height) + var(--layout-tv-overscan-y))";
    return () => {
      root.scrollPaddingBottom = "";
    };
  }, [docked]);

  // Focus in the dock as it goes, when listening stops, goes back up.
  useEffect(() => {
    if (!docked) leaveTvDock();
  }, [docked]);

  // Focus is remembered per history entry as the path changes, and
  // restored when Back returns to it. A change of search alone, such as a
  // sort, stays on the screen and keeps its focus.
  useEffect(() => {
    expectTvScreen(entryOf(router.state.location));
    const leaving = router.subscribe("onBeforeNavigate", (e) => {
      if (e.pathChanged && e.fromLocation) rememberTvFocus(entryOf(e.fromLocation));
    });
    const arrived = router.subscribe("onResolved", (e) => {
      if (e.pathChanged) expectTvScreen(entryOf(e.toLocation), e.fromLocation?.pathname.startsWith("/watch/"));
    });
    return () => {
      leaving();
      arrived();
    };
  }, [router]);

  const screenBack = (fromScreen: boolean) => {
    if (here === railKeys.home) {
      const first = firstTvFocusIn(TV_CONTENT);
      if (fromScreen && first !== undefined && currentTvFocus() !== first) focusTv(first);
      else setExiting(true);
    } else if (here !== undefined) {
      focusTv(here);
    } else if (router.history.canGoBack()) {
      router.history.back();
    } else {
      void navigate({ to: "/", replace: true });
    }
  };
  useTvShellBack(() => {
    const focus = currentTvFocus();
    if (focus === undefined || !isTvFocusIn(focus, TV_RAIL)) return screenBack(true);
    // Home is a destination, gone to as the rail's own links go (TvRail).
    if (here !== undefined && here !== railKeys.home) goToDestination({ to: "/" });
    else if (firstTvFocusIn(TV_CONTENT) !== undefined) focusTv(TV_CONTENT);
    else screenBack(false);
  });

  return (
    <div data-part="tv-shell" className="min-h-dvh">
      {!watching && <TvRail marked={marked} />}
      <main
        id="main"
        className={
          playing
            ? ""
            : watching
              ? // Without the rail, what shows in the player's place keeps clear of the edges.
                "min-w-0 px-(--layout-tv-overscan-x) py-(--layout-tv-overscan-y)"
              : "min-w-0 py-(--layout-tv-overscan-y) pr-(--layout-tv-overscan-x) pl-(--layout-tv-content-left)"
        }
      >
        <TvFocusGroup focusKey={TV_CONTENT} part="tv-content">
          <ConnectionBanner {...live} docked={docked} />
          <Outlet />
        </TvFocusGroup>
        {/* The dock's room, so it never covers the last row. */}
        {docked && (
          <div aria-hidden className="box-content h-(--layout-mini-player-height) pb-(--layout-tv-overscan-y)" />
        )}
      </main>
      <BookDock />
      <ExitDialog isOpen={exiting} onClose={() => setExiting(false)} />
    </div>
  );
}
