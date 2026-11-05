import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { bookQueries } from "@/api/books";
import { PageSkeleton } from "@/components/Skeleton";
import { authedRoute } from "./authed";

const PersonScreen = lazyRouteComponent(() => import("@/features/detail/PersonScreen"), "PersonScreen");
const BookSeriesScreen = lazyRouteComponent(() => import("@/features/detail/BookSeriesScreen"), "BookSeriesScreen");

/** An author's or narrator's page. */
export const personRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/people/$personId",
  loader: ({ context, params }) => void context.queryClient.prefetchQuery(bookQueries.person(params.personId)),
  pendingComponent: PageSkeleton,
  component: PersonScreen,
});

/** A book series' page. */
export const bookSeriesRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/book-series/$seriesId",
  loader: ({ context, params }) => void context.queryClient.prefetchQuery(bookQueries.series(params.seriesId)),
  pendingComponent: PageSkeleton,
  component: BookSeriesScreen,
});
