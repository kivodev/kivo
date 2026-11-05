import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { historyQueries } from "@/api/history";
import { authedRoute } from "./authed";

const HistoryScreen = lazyRouteComponent(() => import("@/features/history/HistoryScreen"), "HistoryScreen");

/** Your watch history, from the user menu. */
export const historyRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/history",
  loader: ({ context }) => void context.queryClient.prefetchInfiniteQuery(historyQueries.mine()),
  component: HistoryScreen,
});
