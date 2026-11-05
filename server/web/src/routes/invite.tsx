import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { rootRoute } from "./root";

const InviteScreen = lazyRouteComponent(() => import("@/features/auth/InviteScreen"), "InviteScreen");

/** An invite link. It's public, so whoever it was sent to can become a user. */
export const inviteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/invite/$token",
  component: InviteScreen,
});
