import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { workQueries } from "@/api/works";
import { PageSkeleton } from "@/components/Skeleton";
import { authedRoute } from "./authed";

const CollectionScreen = lazyRouteComponent(() => import("@/features/detail/CollectionScreen"), "CollectionScreen");

/** The page of a movie collection or of a videos library's folder. */
export const collectionRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/collections/$collectionId",
  loader: ({ context, params }) => void context.queryClient.prefetchQuery(workQueries.collection(params.collectionId)),
  pendingComponent: PageSkeleton,
  component: CollectionScreen,
});
