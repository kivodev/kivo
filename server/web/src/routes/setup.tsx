import { createRoute, lazyRouteComponent, redirect } from "@tanstack/react-router";
import { setupQueries } from "@/api/setup";
import { rootRoute } from "./root";

const SetupWizard = lazyRouteComponent(() => import("@/features/setup/SetupWizard"), "SetupWizard");

export const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/setup",
  beforeLoad: async ({ context }) => {
    // This waits, because whether setup is still needed decides whether
    // this page shows.
    const setup = await context.queryClient.ensureQueryData(setupQueries.state());
    if (!setup.required) throw redirect({ to: "/" });
  },
  component: SetupWizard,
});
