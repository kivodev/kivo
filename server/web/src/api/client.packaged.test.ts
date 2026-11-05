import { HttpResponse, http } from "msw";
import { expect, it, vi } from "vitest";
import { testServer } from "@/test/packagedSetup";
import { problem, server } from "@/test/server";
import { packagedServer, packagedToken, setPackagedServer, setPackagedToken } from "@/tv/packaged";
import { restartApp } from "@/tv/restart";
import { apiBaseUrl, client, setUnauthorizedHandler } from "./client";

/** The requests the client sends while run does, as fetch sees them. */
async function sent(run: () => Promise<unknown>): Promise<Request[]> {
  const fetch = vi.spyOn(globalThis, "fetch");
  await run();
  return fetch.mock.calls.map(([request]) => request as Request);
}

it("reaches the API at the kept server's address", () => {
  expect(apiBaseUrl()).toBe("http://kivo.test");
  setPackagedServer({ address: "https://media.example/kivo/", serverId: "server-2" });
  expect(apiBaseUrl()).toBe("https://media.example/kivo");
});

it("sends requests to the server the app started on, as keeping another starts the app again", async () => {
  server.use(http.get("*/api/v1/server", () => HttpResponse.json({ qualityLadder: [] })));
  setPackagedServer({ address: "https://media.example/kivo", serverId: "server-2" });
  expect(restartApp).toHaveBeenCalledOnce();
  const [request] = await sent(() => client.GET("/api/v1/server"));
  expect(request?.url).toBe("http://kivo.test/api/v1/server");
});

it("sends the session's token as a bearer, and no cookies", async () => {
  server.use(http.get("*/api/v1/server", () => HttpResponse.json({ qualityLadder: [] })));
  setPackagedToken("token-1");
  const [request] = await sent(() => client.GET("/api/v1/server"));
  expect(request?.url).toBe("http://kivo.test/api/v1/server");
  expect(request?.headers.get("Authorization")).toBe("Bearer token-1");
  expect(request?.credentials).toBe("omit");
});

it("says it's the Tizen app, at its release's version", async () => {
  server.use(http.get("*/api/v1/server", () => HttpResponse.json({ qualityLadder: [] })));
  const [request] = await sent(() => client.GET("/api/v1/server"));
  expect(request?.headers.get("Kivo-App")).toBe("kivo-tizen");
  expect(request?.headers.get("Kivo-Platform")).toBe("tizen");
  expect(request?.headers.get("Kivo-App-Version")).toBeTruthy();
});

it("sends no Authorization header when signed out", async () => {
  server.use(http.get("*/api/v1/server", () => HttpResponse.json({ qualityLadder: [] })));
  const [request] = await sent(() => client.GET("/api/v1/server"));
  expect(request?.headers.has("Authorization")).toBe(false);
  expect(request?.credentials).toBe("omit");
});

it("keeps the token from an address it's probing that isn't the kept server", async () => {
  server.use(http.get("*/api/v1/server", () => HttpResponse.json({ qualityLadder: [] })));
  setPackagedToken("token-1");
  const [request] = await sent(() => client.GET("/api/v1/server", { baseUrl: "http://elsewhere.test" }));
  expect(request?.url).toBe("http://elsewhere.test/api/v1/server");
  expect(request?.headers.has("Authorization")).toBe(false);
});

it("stays signed in when an address it's probing answers 401, as behind an auth proxy", async () => {
  server.use(http.get("*/api/v1/server", () => new HttpResponse("Unauthorized", { status: 401 })));
  const signedOut = vi.fn();
  setUnauthorizedHandler(signedOut);
  await client.GET("/api/v1/server", { baseUrl: "http://elsewhere.test" });
  setPackagedToken("token-1");
  await client.GET("/api/v1/server", { baseUrl: "http://elsewhere.test" });
  expect(signedOut).not.toHaveBeenCalled();
  expect(packagedToken()).toBe("token-1");
});

it("forgets the token on a 401 and keeps the address, then signs out", async () => {
  server.use(http.get("*/api/v1/libraries", () => problem(401, "unauthenticated")));
  const signedOut = vi.fn();
  setUnauthorizedHandler(signedOut);
  setPackagedToken("token-1");
  await client.GET("/api/v1/libraries");
  expect(signedOut).toHaveBeenCalledOnce();
  expect(packagedToken()).toBeUndefined();
  expect(packagedServer()).toEqual(testServer);
});

it("keeps a new token when a request made with the one before answers 401", async () => {
  let answer: ((response: Response) => void) | undefined;
  server.use(http.get("*/api/v1/libraries", () => new Promise<Response>((resolve) => (answer = resolve))));
  const signedOut = vi.fn();
  setUnauthorizedHandler(signedOut);
  setPackagedToken("token-1");
  const pending = client.GET("/api/v1/libraries");
  await vi.waitFor(() => expect(answer).toBeDefined());
  setPackagedToken("token-2");
  answer?.(problem(401, "unauthenticated"));
  await pending;
  expect(signedOut).not.toHaveBeenCalled();
  expect(packagedToken()).toBe("token-2");
});
