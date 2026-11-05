import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { watchSearchSchema } from "@/features/player/watchSearch";
import { authedRoute } from "./authed";

const WatchScreen = lazyRouteComponent(() => import("@/features/player/WatchScreen"), "WatchScreen");

/**
 * The player's route. Its URL holds the work, and the version and tracks
 * chosen on its detail page. It's never preloaded, because hovering a Play link
 * must not start a play.
 */
export const watchRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/watch/$workId",
  validateSearch: watchSearchSchema,
  preload: false,
  component: WatchScreen,
});
