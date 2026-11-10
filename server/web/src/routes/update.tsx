import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { rootRoute } from "./root";

/**
 * The packaged TV app's update screen, which says the server or the app
 * needs an update while the two don't work together. Only the packaged
 * build has the route, so it's made by the router there, and the web
 * build drops it with the screen.
 */
export function createUpdateRoute() {
  if (import.meta.env.VITE_KIVO_PACKAGED !== "1") throw new Error("The update route is packaged-only");
  const UpdateScreen = lazyRouteComponent(() => import("@/features/auth/UpdateScreen"), "UpdateScreen");
  return createRoute({
    getParentRoute: () => rootRoute,
    path: "/update",
    component: UpdateScreen,
  });
}
