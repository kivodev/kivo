import type { QueryClient } from "@tanstack/react-query";
import {
  createHashHistory,
  createRouter,
  type NavigateOptions,
  type RouterHistory,
  type ToOptions,
} from "@tanstack/react-router";
import type { WorkSummary } from "@/api/schemas";
import { NotFoundState } from "@/components/LoadError";
import { BlankPending } from "@/components/Skeleton";
import { createAddressRoute } from "@/routes/address";
import { createAdminRouteTree } from "@/routes/admin";
import { authedRoute } from "@/routes/authed";
import { bookSeriesRoute, personRoute } from "@/routes/books";
import { collectionRoute } from "@/routes/collection";
import { historyRoute } from "@/routes/history";
import { homeRoute } from "@/routes/home";
import { inviteRoute } from "@/routes/invite";
import { librariesRoute } from "@/routes/libraries";
import { libraryRoute } from "@/routes/library";
import { linkRoute } from "@/routes/link";
import { listRoute, listsRoute } from "@/routes/lists";
import { rootRoute } from "@/routes/root";
import { searchRoute } from "@/routes/search";
import { settingsRoute } from "@/routes/settings";
import { setupRoute } from "@/routes/setup";
import { signInRoute } from "@/routes/signIn";
import { createUpdateRoute } from "@/routes/update";
import { watchRoute } from "@/routes/watch";
import { workRoute } from "@/routes/work";
import { isPackaged, isTv } from "@/tv/flag";
import { RouteError } from "./RouteStates";

// TV has no admin area; its routes and screens stay in the web build.
const adminRoutes = isTv ? [] : [createAdminRouteTree()];

// The packaged TV app's address and update screens. Only that build has
// them: elsewhere the factories are never called, so the build drops them.
const packagedRoutes = isPackaged ? [createAddressRoute(), createUpdateRoute()] : [];

const routeTree = rootRoute.addChildren([
  ...packagedRoutes,
  setupRoute,
  signInRoute,
  inviteRoute,
  linkRoute,
  authedRoute.addChildren([
    homeRoute,
    librariesRoute,
    libraryRoute,
    listsRoute,
    listRoute,
    workRoute,
    personRoute,
    bookSeriesRoute,
    collectionRoute,
    searchRoute,
    watchRoute,
    historyRoute,
    settingsRoute,
    ...adminRoutes,
  ]),
]);

/**
 * The base path the server serves the app under, from <base href>. The
 * packaged app's routes are in the URL's hash, from the root.
 */
function basePath(): string {
  if (isPackaged) return "/";
  return new URL(document.baseURI).pathname.replace(/\/$/, "") || "/";
}

export function createAppRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({
    routeTree,
    context: { queryClient },
    basepath: basePath(),
    // The packaged app's page is a file: page, where Chromium refuses
    // pushState to another path, so its routes go in the hash. Nothing
    // links into the app by URL, so no route changes.
    history: history ?? (isPackaged ? createHashHistory() : undefined),
    // The router loads a page's code. Loaders start the page's reads and
    // wait only for what decides where a URL leads. TanStack Query decides
    // freshness, and the page fills in as its data arrives.
    defaultPreload: "intent",
    defaultPreloadStaleTime: 0,
    // A page whose code is still loading after 100 ms shows its pending
    // component. A page whose code is already here resolves within that
    // time, so it never flashes the pending component. Pages show nothing
    // while they load. A content page names its own placeholder, shaped
    // like its layout.
    defaultPendingMs: 100,
    defaultPendingMinMs: 0,
    defaultPendingComponent: BlankPending,
    defaultErrorComponent: RouteError,
    defaultNotFoundComponent: NotFoundState,
    scrollRestoration: true,
  });
}

export type AppRouter = ReturnType<typeof createAppRouter>;

declare module "@tanstack/react-router" {
  interface Register {
    router: AppRouter;
  }
}

// React Aria's links, such as a menu item's href, take the router's own
// destinations (App.tsx's RouterProvider).
declare module "react-aria-components" {
  interface RouterConfig {
    href: ToOptions;
    routerOptions: Omit<NavigateOptions, keyof ToOptions>;
  }
}

declare module "@tanstack/history" {
  interface HistoryState {
    /** The card a work's page was opened from, for the page's pending hero. */
    work?: WorkSummary;
  }
}
