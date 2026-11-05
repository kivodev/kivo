import { createRoute } from "@tanstack/react-router";
import { homeQueries } from "@/api/home";
import { HomeScreen } from "@/features/home/HomeScreen";
import { authedRoute } from "./authed";

export const homeRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/",
  loader: ({ context }) => void context.queryClient.prefetchQuery(homeQueries.rows()),
  component: HomeScreen,
});
