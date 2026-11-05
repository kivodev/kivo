import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { z } from "zod";
import { rootRoute } from "./root";

const SignInScreen = lazyRouteComponent(() => import("@/features/auth/SignInScreen"), "SignInScreen");

export const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sign-in",
  // Where to return after signing in, which is the page the expired
  // session was on.
  validateSearch: z.object({ redirect: z.string().optional().catch(undefined) }),
  component: SignInScreen,
});
