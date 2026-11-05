import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet, redirect } from "@tanstack/react-router";
import { serverQueries } from "@/api/server";
import { setupQueries } from "@/api/setup";
import { isPackaged, isTv } from "@/tv/flag";
import { packagedServer, packagedToken } from "@/tv/packaged";

export type RouterContext = { queryClient: QueryClient };

/**
 * Until the admin user exists, every page is the setup wizard. The
 * packaged TV app first needs its server: until it has one, and while the
 * one it has doesn't answer, isn't Kivo, is too old or isn't set up, every
 * page is the address screen, and until it has a token, sign-in.
 */
export const rootRoute = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ context, location }) => {
    // There is no admin area on TV, so its pages go Home.
    if (isTv && (location.pathname === "/admin" || location.pathname.startsWith("/admin/"))) {
      throw redirect({ to: "/", replace: true });
    }
    if (isPackaged) {
      // The address screen asks the server nothing but its probe.
      if (location.pathname === "/server") return;
      const server = packagedServer();
      if (server === undefined) throw redirect({ to: "/server", replace: true });
      // A server that doesn't answer, or isn't Kivo, is asked nothing more:
      // the address screen says so, with its address to open again or
      // change.
      const probe = await context.queryClient.fetchQuery(serverQueries.probe(server.address));
      if (probe.result !== "kivo") throw redirect({ to: "/server", search: { failed: probe.result }, replace: true });
    }
    // This waits, because whether setup is needed decides where every URL
    // leads.
    const setup = await context.queryClient.ensureQueryData(setupQueries.state());
    if (isPackaged) {
      // Setup signs in with a browser's cookie, which the app can't use; the
      // address screen says to set the server up in a browser.
      if (setup.required) throw redirect({ to: "/server", search: { unready: true }, replace: true });
      if (packagedToken() === undefined && location.pathname !== "/sign-in") {
        throw redirect({ to: "/sign-in", search: { redirect: undefined }, replace: true });
      }
      return;
    }
    if (setup.required && location.pathname !== "/setup") {
      throw redirect({ to: "/setup" });
    }
  },
  // Nothing shows before the shell. A page's own skeleton shows inside it.
  pendingMs: Number.POSITIVE_INFINITY,
  component: Outlet,
});
