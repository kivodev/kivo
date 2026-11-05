import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { workQueries } from "@/api/works";
import { DetailSkeleton, useSummary } from "@/features/detail/DetailSkeleton";
import { detailSearchSchema } from "@/features/detail/detailSearch";
import { authedRoute } from "./authed";

const WorkScreen = lazyRouteComponent(() => import("@/features/detail/WorkScreen"), "WorkScreen");

export const workRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/works/$workId",
  validateSearch: detailSearchSchema,
  loaderDeps: ({ search }) => ({ season: search.season }),
  loader: ({ context: { queryClient }, params, deps }) => {
    // The page's lists load side by side once the work is known, rather
    // than each after the one above it. For a series, these are its seasons
    // and its open season's episodes. For an episode, they are its season
    // and its series' seasons.
    void queryClient.fetchQuery(workQueries.detail(params.workId)).then(
      (work) => {
        if (work.kind === "series") {
          const open = deps.season ?? work.series?.nextUp?.episode?.seasonId;
          if (open) void queryClient.prefetchQuery(workQueries.children(open));
          void queryClient.prefetchQuery(workQueries.children(work.id));
        } else if (work.kind === "episode" && work.episode?.seasonId) {
          void queryClient.prefetchQuery(workQueries.children(work.episode.seasonId));
          void queryClient.prefetchQuery(workQueries.children(work.episode.seriesId));
        }
      },
      () => {},
    );
  },
  pendingComponent: WorkPending,
  component: WorkScreen,
});

/** The hero's shape while the page's code loads, with the card it was opened from. */
function WorkPending() {
  return <DetailSkeleton work={useSummary()} />;
}
