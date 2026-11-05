import { I18nProvider } from "@lingui/react";
import { type QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, useRouterState } from "@tanstack/react-router";
import { type ReactNode, useEffect, useState } from "react";
import { RouterProvider as AriaRouterProvider } from "react-aria-components";
import { setUnauthorizedHandler } from "@/api/client";
import { signedOut } from "@/api/session";
import { Toasts } from "@/components/Toast";
import { i18n } from "@/i18n";
import { isTv } from "@/tv/flag";
import { TvSplash } from "@/tv/Splash";
import { startTv } from "@/tv/start";
import { createQueryClient } from "./queryClient";
import { type AppRouter, createAppRouter } from "./router";

if (isTv) startTv();

// Until the TV build's first screen shows, the splash that index.html
// starts with stays, while the router settles where the app opens, which
// shows nothing of its own.
function TvOpening({ router, children }: { router: AppRouter; children: ReactNode }) {
  const opening = useRouterState({ router, select: (s) => s.resolvedLocation === undefined });
  if (!isTv) return children;
  return (
    <>
      {children}
      {opening && <TvSplash />}
    </>
  );
}

/** The providers around the router. Tests render this with a memory history. */
export function AppProviders({ queryClient, router }: { queryClient: QueryClient; router: AppRouter }) {
  useEffect(() => {
    setUnauthorizedHandler(() => void signedOut(router, queryClient, true));
  }, [router, queryClient]);

  return (
    <I18nProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        {/* React Aria's links, such as a menu item with an href, go through the router as its Link does. */}
        <AriaRouterProvider
          navigate={(to, options) => router.navigate({ ...to, ...options })}
          useHref={(to) => router.history.createHref(router.buildLocation(to).publicHref)}
        >
          <TvOpening router={router}>
            <RouterProvider router={router} />
          </TvOpening>
        </AriaRouterProvider>
        <Toasts />
      </QueryClientProvider>
    </I18nProvider>
  );
}

export function App() {
  const [queryClient] = useState(createQueryClient);
  const [router] = useState(() => createAppRouter(queryClient));
  return <AppProviders queryClient={queryClient} router={router} />;
}
