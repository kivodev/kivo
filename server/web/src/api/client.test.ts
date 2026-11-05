import { HttpResponse, http } from "msw";
import { afterEach, expect, it, vi } from "vitest";
import { server } from "@/test/server";
import { setPackagedServer, setPackagedToken } from "@/tv/packaged";
import { apiBaseUrl, client } from "./client";

it("asks the server for names, such as track titles, in the app's language", async () => {
  let language: string | null = null;
  server.use(
    http.get("*/api/v1/server", ({ request }) => {
      language = request.headers.get("Accept-Language");
      return HttpResponse.json({ qualityLadder: [] });
    }),
  );
  await client.GET("/api/v1/server");
  expect(language).toBe("en");
});

it("signs in by the session cookie alone, with no Authorization header, even with a packaged token kept", async () => {
  setPackagedServer({ address: "http://elsewhere.test", serverId: "server-1" });
  setPackagedToken("token-1");
  server.use(http.get("*/api/v1/server", () => HttpResponse.json({ qualityLadder: [] })));
  const fetch = vi.spyOn(globalThis, "fetch");
  await client.GET("/api/v1/server");
  const request = fetch.mock.calls[0]?.[0] as Request;
  expect(new URL(request.url).origin).toBe(window.location.origin);
  expect(request.headers.has("Authorization")).toBe(false);
  expect(request.credentials).toBe("same-origin");
});

it("opens the events socket with no subprotocol, as the cookie is its credential", async () => {
  setPackagedToken("token-1");
  const opened: unknown[][] = [];
  vi.stubGlobal(
    "WebSocket",
    class {
      constructor(...args: unknown[]) {
        opened.push(args);
      }
      close() {}
    },
  );
  try {
    const { connectLiveEvents } = await import("./live");
    connectLiveEvents({ onMessage: () => {}, onStatus: () => {} }).close();
    expect(opened).toHaveLength(1);
    expect(opened[0]).toHaveLength(1);
  } finally {
    vi.unstubAllGlobals();
  }
});

afterEach(() => {
  document.head.querySelector("base")?.remove();
});

it("reaches the API beside the page", () => {
  const base = document.createElement("base");
  base.href = "http://kivo.test/kivo/";
  document.head.append(base);
  expect(apiBaseUrl()).toBe("http://kivo.test/kivo");
});

it("gives each request a time limit, longer for starting a play and none for media or uploads", async () => {
  const { requestTimeoutMs, slowRequestTimeoutMs, timeoutOf } = await import("./client");
  expect(timeoutOf("POST", "/api/v1/plays/{id}")).toBe(requestTimeoutMs);
  expect(timeoutOf("PATCH", "/api/v1/plays/{id}")).toBe(requestTimeoutMs);
  expect(timeoutOf("POST", "/api/v1/device-codes/poll")).toBe(requestTimeoutMs);
  expect(timeoutOf("POST", "/api/v1/plays")).toBe(slowRequestTimeoutMs);
  expect(timeoutOf("POST", "/api/v1/plays/{id}/fallback")).toBe(slowRequestTimeoutMs);
  expect(timeoutOf("GET", "/api/v1/plays/{id}/previews/{name}")).toBeUndefined();
  expect(timeoutOf("GET", "/api/v1/downloads/{id}/files/{part}")).toBeUndefined();
  expect(timeoutOf("GET", "/api/v1/bandwidth-test")).toBeUndefined();
  expect(timeoutOf("POST", "/api/v1/works/{id}/artwork")).toBeUndefined();
});

it("fails a request the server doesn't answer in time as one that got no answer", async () => {
  const timeout = new AbortController();
  const asked: number[] = [];
  vi.spyOn(AbortSignal, "timeout").mockImplementation((ms) => {
    asked.push(ms);
    return timeout.signal;
  });
  server.use(http.get("*/api/v1/server", () => new Promise<never>(() => {})));
  const answer = client.GET("/api/v1/server");
  await vi.waitFor(() => expect(asked).toEqual([15_000]));
  timeout.abort(new DOMException("The operation timed out.", "TimeoutError"));
  await expect(answer).rejects.toMatchObject({ name: "TimeoutError" });
});

it("still lets the caller cancel a request", async () => {
  server.use(http.get("*/api/v1/server", () => new Promise<never>(() => {})));
  const caller = new AbortController();
  const answer = client.GET("/api/v1/server", { signal: caller.signal });
  caller.abort();
  await expect(answer).rejects.toMatchObject({ name: "AbortError" });
});

it("times requests out on engines without AbortSignal.any, as the TV build's Chromium 108", async () => {
  const any = AbortSignal.any;
  Object.defineProperty(AbortSignal, "any", { value: undefined, configurable: true });
  try {
    const timeout = new AbortController();
    vi.spyOn(AbortSignal, "timeout").mockReturnValue(timeout.signal);
    server.use(http.get("*/api/v1/server", () => new Promise<never>(() => {})));
    const answer = client.GET("/api/v1/server");
    await vi.waitFor(() => expect(AbortSignal.timeout).toHaveBeenCalled());
    timeout.abort(new DOMException("The operation timed out.", "TimeoutError"));
    await expect(answer).rejects.toMatchObject({ name: "TimeoutError" });
  } finally {
    Object.defineProperty(AbortSignal, "any", { value: any, configurable: true });
  }
});
