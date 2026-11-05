import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link, Outlet, useMatchRoute, useParams } from "@tanstack/react-router";
import { type ReactElement, type ReactNode, useEffect } from "react";
import { Button as AriaButton, Focusable, Header, MenuSection, Tooltip, TooltipTrigger } from "react-aria-components";
import { libraryQueries } from "@/api/libraries";
import { liveQueries, useLiveEvents } from "@/api/live";
import { errorCode } from "@/api/problem";
import type { Library } from "@/api/schemas";
import { useSignOutHere } from "@/api/session";
import { setupQueries } from "@/api/setup";
import { currentUserQueries, useIsAdmin } from "@/api/users";
import { Banner } from "@/components/Banner";
import { Button } from "@/components/Button";
import { Icon, type IconName } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { Logo } from "@/components/Logo";
import { Menu, MenuItem } from "@/components/Menu";
import { useSidebarRail } from "@/components/phoneLayout";
import { favoritesKey } from "@/features/browse/LibrarySwitcher";
import { BookDock } from "@/features/player/BookPlayer";
import { useBookPlayer } from "@/features/player/useBookPlayer";
import { describeProblem } from "@/i18n/problems";
import { readStored, writeStored } from "@/lib/storage";
import { isTv } from "@/tv/flag";
import { ConnectionBanner } from "./ConnectionBanner";
import { TvShell } from "./tv/TvShell";

// On the phone's bottom bar, an entry's label sits small under its icon.
const navItem =
  "flex h-10 shrink-0 items-center gap-3 rounded-md px-3 text-sm font-medium text-text-muted hover:bg-surface-hover hover:text-text max-lg:justify-center max-lg:px-0 phone:h-12 phone:min-w-14 phone:max-w-20 phone:flex-col phone:gap-0.5 phone:px-1";
const navLabel = "truncate max-lg:sr-only phone:not-sr-only phone:max-w-full phone:text-xs";
const activeNav = { className: "bg-surface-raised text-text", "aria-current": "page" as const };

type NavLinkProps = {
  to: "/" | "/search" | "/favorites";
  icon: IconName;
  /** Whether the entry is also current on the pages under it, such as a kind's favorites. */
  under?: boolean;
  className?: string;
  children: ReactNode;
};

function NavLink({ to, icon, under = false, className = "", children }: NavLinkProps) {
  return (
    <RailTooltip label={children}>
      <Focusable>
        <Link to={to} className={`${navItem} ${className}`} activeProps={activeNav} activeOptions={{ exact: !under }}>
          <Icon name={icon} className="size-5 shrink-0" />
          <span className={navLabel}>{children}</span>
        </Link>
      </Focusable>
    </RailTooltip>
  );
}

/**
 * Shows an entry's name beside it on the sidebar collapsed to its icons,
 * when the pointer rests on it or the keyboard focuses it.
 */
function RailTooltip({ label, children }: { label: ReactNode; children: ReactElement }) {
  const rail = useSidebarRail();
  return (
    <TooltipTrigger isDisabled={!rail} delay={500}>
      {children}
      {/* Clear of the rail: past its padding, then a gap. */}
      <Tooltip
        placement="end"
        offset={20}
        className="rounded-md border border-border bg-surface-raised px-2 py-1 text-sm font-medium text-text shadow-overlay"
      >
        {label}
      </Tooltip>
    </TooltipTrigger>
  );
}

/**
 * The signed-in layout. A left sidebar holds Home, Search and Favorites,
 * then a line, your pinned libraries, More libraries for the rest, and
 * your avatar. Below 1024 px it collapses to icons, each named in a
 * tooltip. On phones it becomes a bottom bar with Home, Libraries and
 * Search instead of Favorites and the libraries. On the bottom bar each
 * icon has a small label under it. A book that is playing docks its mini
 * player along the bottom, above the bottom bar on phones. The layout
 * holds the tab's event socket, whose state the connection banner shows.
 * The TV build has its own shell, with the rail.
 */
export function AppShell() {
  return isTv ? <TvShell /> : <WebShell />;
}

function WebShell() {
  const { t } = useLingui();
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const others = libraries.filter((l) => !l.pinned);
  const signOut = useSignOutHere();
  const signOutProblem = signOut.isError ? describeProblem(errorCode(signOut.error)) : undefined;
  const name = useSuspenseQuery(currentUserQueries.current()).data.username;
  const isAdmin = useIsAdmin();
  const live = useLiveEvents();
  const listening = useBookPlayer().book !== undefined;
  const server = useSuspenseQuery(setupQueries.state()).data.serverName;

  return (
    <div className="min-h-dvh wide:max-lg:pl-(--layout-sidebar-collapsed-width) lg:pl-(--layout-sidebar-width) phone:pb-(--layout-bottom-bar-inset)">
      <nav
        data-part="sidebar"
        aria-label={t`Main`}
        className="fixed z-40 flex gap-1 border-border bg-surface phone:inset-x-0 phone:bottom-0 phone:h-(--layout-bottom-bar-inset) phone:items-center phone:overflow-x-auto phone:border-t phone:pr-(--layout-bottom-bar-padding-right) phone:pb-(--layout-safe-area-bottom) phone:pl-(--layout-bottom-bar-padding-left) wide:inset-y-0 wide:left-0 wide:max-lg:w-(--layout-sidebar-collapsed-width) wide:flex-col wide:overflow-y-auto wide:border-r wide:p-3 lg:w-(--layout-sidebar-width)"
      >
        <Link to="/" className="mb-4 flex px-3 pt-2 max-lg:justify-center max-lg:px-0 phone:hidden">
          <Logo className="h-6 max-lg:hidden" />
          <Logo mark className="h-8 lg:hidden" />
        </Link>
        <NavLink to="/" icon="home">
          <Trans>Home</Trans>
        </NavLink>
        <LibrariesTab libraries={libraries} />
        <NavLink to="/search" icon="search">
          <Trans>Search</Trans>
        </NavLink>
        <NavLink to="/favorites" icon="heart" under className="phone:hidden">
          <Trans>Favorites</Trans>
        </NavLink>
        <hr className="my-2 border-border phone:hidden" />
        {libraries
          .filter((l) => l.pinned)
          .map((library) => (
            <RailTooltip key={library.id} label={library.name}>
              <Focusable>
                <Link
                  to="/libraries/$libraryId"
                  params={{ libraryId: library.id }}
                  className={`${navItem} phone:hidden`}
                  activeProps={activeNav}
                  data-part="sidebar-library"
                >
                  <LibraryKindIcon kind={library.kind} className="size-5 shrink-0" />
                  <span className={navLabel}>{library.name}</span>
                </Link>
              </Focusable>
            </RailTooltip>
          ))}
        {others.length > 0 && <MoreLibraries libraries={others} />}
        <div className="wide:mt-auto phone:ml-auto phone:flex phone:items-center phone:gap-1" data-part="account-menu">
          <hr className="my-2 border-border phone:hidden" />
          <Menu>
            {/* Laid out as the entries above it: the avatar in the icons' column, the name in the text's. */}
            <RailTooltip label={name}>
              <AriaButton
                className={`${navItem} w-full cursor-pointer outline-none phone:w-auto`}
                aria-label={t`Account menu for ${name}`}
              >
                <span className="-mx-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-accent text-xs font-bold text-on-accent phone:size-5">
                  {name.charAt(0).toUpperCase()}
                </span>
                <span className={navLabel}>{name}</span>
              </AriaButton>
            </RailTooltip>
            <MenuSection>
              <Header className="flex flex-col px-3 pt-2 pb-2">
                <span className="truncate text-sm font-semibold text-text">{name}</span>
                {server && <span className="truncate text-xs text-text-muted">{server}</span>}
              </Header>
              <MenuItem href={{ to: "/history" }}>
                <Icon name="clock" className="size-4" />
                <Trans>Watch history</Trans>
              </MenuItem>
              <MenuItem href={{ to: "/link" }}>
                <Icon name="screen" className="size-4" />
                <Trans>Link a device</Trans>
              </MenuItem>
              <MenuItem href={{ to: "/settings" }}>
                <Icon name="settings" className="size-4" />
                <Trans>Settings</Trans>
              </MenuItem>
              <MenuItem onAction={() => signOut.mutate({})}>
                <Icon name="signOut" className="size-4" />
                <Trans>Sign out</Trans>
              </MenuItem>
            </MenuSection>
          </Menu>
          {isAdmin && <AdminEntry />}
        </div>
      </nav>
      <main
        id="main"
        data-listening={listening || undefined}
        className={`group/main min-w-0 pr-(--layout-safe-area-right) pl-(--layout-safe-area-left) ${listening ? "pb-(--layout-mini-player-inset) phone:pb-(--layout-mini-player-height)" : ""}`}
      >
        <ConnectionBanner {...live} />
        {signOutProblem && (
          <div className="px-4 pt-4 sm:px-8">
            <Banner
              tone="warning"
              action={
                <Button size="sm" onPress={() => signOut.mutate({})}>
                  <Trans>Retry</Trans>
                </Button>
              }
            >
              <Trans>Couldn't sign out. {signOutProblem}</Trans>
            </Banner>
          </div>
        )}
        <Outlet />
      </main>
      <BookDock />
    </div>
  );
}

// The library, or Favorites, that this browser last opened. It is cleared
// when the browser goes back to the list of libraries. Without storage, as
// in a private window, there is none.
const lastLibraryKey = "kivo.lastLibrary";

/**
 * Libraries on the phone's bottom bar. It is current while the list of
 * libraries, a library or Favorites is open. It opens the page this
 * browser last opened, whose Back goes to the list, or else the list
 * itself. Chosen again on one of those pages, it goes back to the list.
 */
function LibrariesTab({ libraries }: { libraries: Library[] }) {
  const matchRoute = useMatchRoute();
  const libraryId = useParams({ strict: false }).libraryId;
  const favorites = matchRoute({ to: "/favorites", fuzzy: true }) !== false;
  const list = matchRoute({ to: "/libraries" }) !== false;
  const page = libraryId ?? (favorites ? favoritesKey : undefined);
  useEffect(() => {
    if (page !== undefined) writeStored(lastLibraryKey, page);
    else if (list) writeStored(lastLibraryKey, undefined);
  }, [page, list]);
  const open = page !== undefined || list;
  const last = open ? undefined : readStored(lastLibraryKey);
  const link =
    last === favoritesKey
      ? ({ to: "/favorites" } as const)
      : last !== undefined && libraries.some((l) => l.id === last)
        ? ({ to: "/libraries/$libraryId", params: { libraryId: last } } as const)
        : ({ to: "/libraries" } as const);
  return (
    <Link
      {...link}
      className={`${navItem} wide:hidden ${open ? activeNav.className : ""}`}
      aria-current={open ? activeNav["aria-current"] : undefined}
      data-part="libraries-tab"
    >
      <Icon name="libraries" className="size-5 shrink-0" />
      <span className={navLabel}>
        <Trans>Libraries</Trans>
      </span>
    </Link>
  );
}

/**
 * More libraries, a menu of the libraries you haven't pinned. It is marked
 * as the current page while one of them is open.
 */
function MoreLibraries({ libraries }: { libraries: Library[] }) {
  const { t } = useLingui();
  const current = useParams({ strict: false }).libraryId;
  const open = libraries.some((l) => l.id === current);
  return (
    <Menu>
      <RailTooltip label={t`More libraries`}>
        <AriaButton
          className={`${navItem} cursor-pointer outline-none phone:hidden ${open ? activeNav.className : ""}`}
          aria-label={t`More libraries`}
          data-part="more-libraries"
        >
          <Icon name="more" className="size-5 shrink-0" />
          <span className={navLabel}>
            <Trans>More libraries</Trans>
          </span>
        </AriaButton>
      </RailTooltip>
      {libraries.map((library) => (
        <MenuItem
          key={library.id}
          id={library.id}
          textValue={library.name}
          href={{ to: "/libraries/$libraryId", params: { libraryId: library.id } }}
        >
          <LibraryKindIcon kind={library.kind} className="size-4" />
          {library.name}
        </MenuItem>
      ))}
    </Menu>
  );
}

/**
 * Admin, for admins, with a dot while the problems page has open problems.
 */
function AdminEntry() {
  const { t } = useLingui();
  const problemCount = useQuery(liveQueries.snapshot()).data?.problemCount ?? 0;
  return (
    <RailTooltip label={t`Admin`}>
      <Focusable>
        <Link
          to="/admin"
          className={navItem}
          activeProps={activeNav}
          aria-label={problemCount > 0 ? t`Admin, with open problems` : t`Admin`}
          data-part="admin-entry"
        >
          <span className="relative shrink-0">
            <Icon name="shield" className="size-5" />
            {problemCount > 0 && (
              <span data-part="problems-dot" className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-danger" />
            )}
          </span>
          <span className={navLabel}>
            <Trans>Admin</Trans>
          </span>
        </Link>
      </Focusable>
    </RailTooltip>
  );
}
