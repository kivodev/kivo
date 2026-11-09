import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { listQueries } from "@/api/lists";
import { workQueries } from "@/api/works";
import { LibrarySkeleton, PageSkeleton } from "@/components/Skeleton";
import { authedRoute } from "./authed";

const ListsScreen = lazyRouteComponent(() => import("@/features/lists/ListsScreen"), "ListsScreen");
const ListScreen = lazyRouteComponent(() => import("@/features/lists/ListScreen"), "ListScreen");

export const listsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/lists",
  loader: ({ context: { queryClient } }) => {
    void queryClient.prefetchQuery(listQueries.all());
  },
  pendingComponent: PageSkeleton,
  component: ListsScreen,
});

export const listRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/lists/$listId",
  loader: ({ context: { queryClient }, params: { listId } }) => {
    void queryClient.prefetchQuery(listQueries.detail(listId));
    void queryClient.prefetchQuery(workQueries.listPage(listId, 0));
  },
  // Another list is a new page, with fresh state.
  remountDeps: ({ params }) => params.listId,
  pendingComponent: LibrarySkeleton,
  component: ListScreen,
});
