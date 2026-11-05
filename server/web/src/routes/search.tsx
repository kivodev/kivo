import { createRoute, lazyRouteComponent } from "@tanstack/react-router";
import { z } from "zod";
import { authedRoute } from "./authed";

const SearchScreen = lazyRouteComponent(() => import("@/features/search/SearchScreen"), "SearchScreen");

export const searchRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/search",
  validateSearch: z.object({ q: z.string().max(200).optional().catch(undefined) }),
  component: SearchScreen,
});
