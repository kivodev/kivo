import { createMemoryHistory } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AppProviders } from "@/app/App";
import { createQueryClient } from "@/app/queryClient";
import { createAppRouter } from "@/app/router";
import "./screens";

/** Renders the whole app at a URL, against whatever the mock server serves. */
export function renderApp(path: string) {
  const queryClient = createQueryClient();
  queryClient.setDefaultOptions({ queries: { ...queryClient.getDefaultOptions().queries, retry: false } });
  const router = createAppRouter(queryClient, createMemoryHistory({ initialEntries: [path] }));
  const user = userEvent.setup();
  const view = render(<AppProviders queryClient={queryClient} router={router} />);
  return { ...view, router, queryClient, user };
}

/** The current URL path and search, as the router sees it. */
export function locationOf(router: ReturnType<typeof renderApp>["router"]): string {
  return router.state.location.href;
}

export { screen };
