import { http as untyped } from "msw";
import { setupServer } from "msw/node";
import type { MockApi } from "@/api/mocks/api";
import { createMockApi } from "@/api/mocks/fixtures";
import { createEventsHandler, createHandlers } from "@/api/mocks/handlers";

export { summaryOf } from "@/api/mocks/api";
export { mockId, mockIds, scenarios } from "@/api/mocks/fixtures";
export { closeEvents, http, problem, sendEvent } from "@/api/mocks/handlers";

export const server = setupServer();

/** A request the app sent, with its JSON body if it had one. */
export type SentRequest = { method: string; url: URL; body?: unknown };

const sentRequests: SentRequest[] = [];
server.events.on("request:start", ({ request }) => {
  const sent: SentRequest = { method: request.method, url: new URL(request.url) };
  sentRequests.push(sent);
  void request
    .clone()
    .text()
    .then((text) => {
      if (text) sent.body = JSON.parse(text);
    })
    .catch(() => {});
});

/**
 * Serves the sample server for this test, after applying tweaks to it.
 * Returns the server's answers for the test to change.
 */
export function serveMockApi(...tweaks: ((api: MockApi) => void)[]): MockApi {
  sentRequests.length = 0;
  const api = createMockApi();
  server.use(...createHandlers(api), createEventsHandler(api));
  for (const tweak of tweaks) tweak(api);
  return api;
}

const pattern = (path: string) => new RegExp(`/api/v1${path.replace(/\{\w+\}/g, "[^/]+")}$`);

/** The requests sent to a route, such as `sent("POST", "/plays/{id}/stop")`, oldest first. */
export function sent(method: string, path: string): SentRequest[] {
  return sentRequests.filter((r) => r.method === method && pattern(path).test(r.url.pathname));
}

/**
 * Makes the change that a request to a route stands for, before the mock
 * server answers it from api. The test says what the real server would do.
 */
export function onRequest(
  method: "get" | "post" | "patch" | "delete",
  path: string,
  change: (body: unknown, params: Record<string, string>) => void,
): void {
  server.use(
    untyped[method](`*/api/v1${path.replace(/\{(\w+)\}/g, ":$1")}`, async ({ request, params }) => {
      const text = await request.clone().text();
      change(text ? JSON.parse(text) : undefined, params as Record<string, string>);
    }),
  );
}
