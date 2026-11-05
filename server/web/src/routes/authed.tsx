import type { QueryClient } from "@tanstack/react-query";
import { createRoute, redirect } from "@tanstack/react-router";
import { useEffect } from "react";
import { libraryQueries } from "@/api/libraries";
import { isProblem } from "@/api/problem";
import { currentUserQueries } from "@/api/users";
import { AppShell } from "@/app/AppShell";
import { BookPlayerProvider } from "@/features/player/BookPlayer";
import { rootRoute } from "./root";

/**
 * Who is signed in, for a page behind sign-in. When signed out, it goes to
 * sign-in, which returns to href afterwards.
 */
export async function requireSignIn(queryClient: QueryClient, href: string) {
  try {
    const user = await queryClient.ensureQueryData(currentUserQueries.current());
    return { user };
  } catch (error) {
    if (isProblem(error) && error.status === 401) {
      throw redirect({ to: "/sign-in", search: { redirect: href } });
    }
    throw error;
  }
}

/**
 * Everything behind sign-in, inside the app's shell. The shell waits for
 * the sidebar's libraries, so they never fill in after it shows.
 */
export const authedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "authed",
  // This waits, because who is signed in decides whether the page or
  // sign-in shows.
  beforeLoad: ({ context, location }) => requireSignIn(context.queryClient, location.href),
  // This waits, because the shell shows with the sidebar's libraries.
  loader: ({ context }) => context.queryClient.ensureQueryData(libraryQueries.list()),
  pendingMs: Number.POSITIVE_INFINITY,
  component: AuthedLayout,
});

function AuthedLayout() {
  // What this browser can play is measured once it's idle after sign-in,
  // so Play doesn't wait for it.
  useEffect(() => {
    void import("@/features/player/capabilities").then((m) => m.reportWhenIdle());
  }, []);
  return (
    <BookPlayerProvider>
      <AppShell />
    </BookPlayerProvider>
  );
}
