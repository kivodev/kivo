import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { serverQueries } from "@/api/server";
import { sessionQueries } from "@/api/session";
import { authedRoute } from "./authed";

const SettingsScreen = lazyRouteComponent(() => import("@/features/user/SettingsScreen"), "SettingsScreen");

/** Your own and this device's settings, from the user menu. */
export const settingsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/settings",
  // The user and the list of libraries belong to the signed-in routes and
  // are already loaded.
  loader: ({ context: { queryClient } }) => {
    void queryClient.prefetchQuery(sessionQueries.list());
    void queryClient.prefetchQuery(serverQueries.info());
  },
  component: SettingsScreen,
});
