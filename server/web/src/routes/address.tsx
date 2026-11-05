import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { z } from "zod";
import { rootRoute } from "./root";

/**
 * The packaged TV app's address screen, where it's told its
 * server. Only the packaged build has the route, so it's made by the
 * router there, and the web build drops it with the screen.
 */
export function createAddressRoute() {
  if (import.meta.env.VITE_KIVO_PACKAGED !== "1") throw new Error("The address route is packaged-only");
  const AddressScreen = lazyRouteComponent(() => import("@/features/auth/AddressScreen"), "AddressScreen");
  return createRoute({
    getParentRoute: () => rootRoute,
    path: "/server",
    // A server not set up yet comes back here, which the screen says, and
    // so does one that didn't answer the app's probe as it started (none)
    // or answered as something other than Kivo (other).
    validateSearch: z.object({
      unready: z.boolean().optional().catch(undefined),
      failed: z.enum(["none", "other"]).optional().catch(undefined),
    }),
    component: AddressScreen,
  });
}
