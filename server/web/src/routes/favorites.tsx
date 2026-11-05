import { createRoute, lazyRouteComponent, notFound } from "@tanstack/react-router";
import type { FavoriteKind } from "@/api/schemas";
import { favoriteKinds, workQueries } from "@/api/works";
import { LibrarySkeleton, PageSkeleton } from "@/components/Skeleton";
import { authedRoute } from "./authed";

const FavoritesScreen = lazyRouteComponent(() => import("@/features/browse/FavoritesScreen"), "FavoritesScreen");
const FavoritesGridScreen = lazyRouteComponent(
  () => import("@/features/browse/FavoritesScreen"),
  "FavoritesGridScreen",
);

export const favoritesRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/favorites",
  loader: ({ context: { queryClient } }) => {
    for (const kind of favoriteKinds) void queryClient.prefetchQuery(workQueries.favorites(kind));
  },
  pendingComponent: PageSkeleton,
  component: FavoritesScreen,
});

function isFavoriteKind(kind: string): kind is FavoriteKind {
  return (favoriteKinds as readonly string[]).includes(kind);
}

/**
 * One kind's favorites. The loader gives the screen its kind. Any other
 * kind in the URL doesn't exist.
 */
export const favoritesGridRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/favorites/$kind",
  loader: ({ context: { queryClient }, params: { kind } }) => {
    if (!isFavoriteKind(kind)) throw notFound();
    void queryClient.prefetchQuery(workQueries.favoritesPage(kind, 0));
    return kind;
  },
  pendingComponent: LibrarySkeleton,
  component: FavoritesGridScreen,
});
