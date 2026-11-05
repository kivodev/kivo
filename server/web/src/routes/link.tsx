import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { z } from "zod";
import { requireSignIn } from "./authed";
import { rootRoute } from "./root";

const LinkScreen = lazyRouteComponent(() => import("@/features/auth/LinkScreen"), "LinkScreen");

/**
 * The /link page, Link a device, where another device's code is answered.
 * It opens from the device's QR code, with the code and its server filled
 * in, or from the account menu. It's behind sign-in but outside the app's
 * shell. A signed-out visitor signs in and comes back.
 */
export const linkRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/link",
  validateSearch: z.object({
    code: z.string().optional().catch(undefined),
    server: z.string().optional().catch(undefined),
  }),
  // This waits, because who is signed in decides whether the page or
  // sign-in shows.
  beforeLoad: ({ context, location }) => requireSignIn(context.queryClient, location.href),
  // Another code is a new page, with fresh state.
  remountDeps: ({ search }) => search.code ?? "",
  component: LinkScreen,
});
