import { createRoute } from "@tanstack/react-router";
import { LibrariesScreen } from "@/features/browse/LibrariesScreen";
import { authedRoute } from "./authed";

// The libraries belong to the signed-in routes and are already loaded.
export const librariesRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: "/libraries",
  component: LibrariesScreen,
});
