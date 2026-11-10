import { useSuspenseQuery } from "@tanstack/react-query";
import { createRoute, lazyRouteComponent, redirect } from "@tanstack/react-router";
import { libraryQueries } from "@/api/libraries";
import type { Library } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { LibrarySkeleton } from "@/components/Skeleton";
import { browseSearchSchema, gridRequest, offersCollections, storedBrowse } from "@/features/browse/browseSearch";
import { LibrarySwitcher } from "@/features/browse/LibrarySwitcher";
import { authedRoute } from "./authed";

const LibraryScreen = lazyRouteComponent(() => import("@/features/browse/LibraryScreen"), "LibraryScreen");

export const libraryRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/libraries/$libraryId",
  validateSearch: browseSearchSchema,
  // Opened with no URL state, a library restores what this browser last
  // used there. A link with its own state opens as linked.
  beforeLoad: ({ params, search, cause }) => {
    if (cause !== "enter" || Object.keys(search).length > 0) return;
    const stored = storedBrowse(params.libraryId);
    if (Object.keys(stored).length > 0) {
      throw redirect({ to: "/libraries/$libraryId", params, search: stored, replace: true });
    }
  },
  loaderDeps: ({ search }) => ({ search }),
  loader: ({ context: { queryClient, user }, params: { libraryId }, deps }) => {
    void queryClient.prefetchQuery(libraryQueries.detail(libraryId));
    // The first page of the tab's section.
    const tab = deps.search.tab ?? "library";
    if (tab === "collections") {
      void queryClient.prefetchQuery(workQueries.collectionsPage(libraryId, 0));
    } else if (tab === "authors") {
      void queryClient.prefetchQuery(workQueries.authorsPage(libraryId, 0));
    } else if (tab === "series") {
      void queryClient.prefetchQuery(workQueries.bookSeriesPage(libraryId, 0));
    } else {
      // The grid's query names the library's kind, which the list of
      // libraries has. The collections' first page says whether the
      // Collections tab shows.
      const libraries = libraryQueries.list();
      void queryClient.prefetchQuery(libraries).then(() => {
        const library = queryClient
          .getQueryData<{ items: Library[] }>(libraries.queryKey)
          ?.items.find((l) => l.id === libraryId);
        if (!library) return;
        void queryClient.prefetchQuery(workQueries.browsePage(gridRequest(library, deps.search, user.admin), 0));
        if (offersCollections(library)) void queryClient.prefetchQuery(workQueries.collectionsPage(libraryId, 0));
      });
    }
  },
  // Another library is a new page, with fresh state.
  remountDeps: ({ params }) => params.libraryId,
  pendingComponent: LibraryPending,
  component: LibraryScreen,
});

/** The library while its screen's code loads, with its title from the list of libraries. */
function LibraryPending() {
  const { libraryId } = libraryRoute.useParams();
  const library = useSuspenseQuery(libraryQueries.list()).data.items.find((l) => l.id === libraryId);
  return <LibrarySkeleton title={library && <LibrarySwitcher title={library.name} current={library.id} />} />;
}
