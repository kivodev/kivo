import { i18n } from "@lingui/core";
import createFetchClient, { type Middleware } from "openapi-fetch";
import createQueryClient from "openapi-react-query";
import { isPackaged } from "@/tv/flag";
import { packagedServer, packagedToken, setPackagedToken } from "@/tv/packaged";
import { identityHeaders } from "./identity";
import { gatewayDown, isProblem } from "./problem";
import type { paths } from "./schema";
import type { Problem } from "./schemas";

/**
 * The URL every API path is relative to. It is the page's base without the
 * trailing slash, because the spec's paths start with one. The server
 * sets <base href> to its base path. The packaged app's page is in
 * its package, so its API is at the kept server's address, and there is
 * none, an empty string, before one is chosen.
 */
export function apiBaseUrl(): string {
  if (isPackaged) return packagedServer()?.address ?? "";
  return new URL(".", document.baseURI).href.replace(/\/$/, "");
}

type UnauthorizedHandler = () => void;
let onUnauthorized: UnauthorizedHandler = () => {};

/** The app registers what a 401 does, which is to clear the cache and go to sign-in. */
export function setUnauthorizedHandler(handler: UnauthorizedHandler): void {
  onUnauthorized = handler;
}

/**
 * The session's token, which the packaged app's requests and events socket
 * carry, or undefined when signed out. The web build has none:
 * the session cookie is their credential.
 */
export function sessionToken(): string | undefined {
  if (!isPackaged) return undefined;
  return packagedToken();
}

/**
 * Called when the session ended, on a 401 or when the event socket closed
 * for it. The packaged app forgets its token, which no longer works, and
 * keeps the server's address, so it signs in to the same server again.
 */
export function unauthorized(): void {
  if (isPackaged) setPackagedToken(undefined);
  onUnauthorized();
}

// Requests whose 401 doesn't mean the session ended. Signing in and
// first-run setup answer 401 for a wrong password, which is a form error.
// Asking who is signed in is how the router decides to show sign-in in the
// first place. They are keyed by method and path, because the same paths
// answer other requests with a 401 that does mean signed out.
const selfHandled = new Set<string>(["POST /api/v1/sessions", "POST /api/v1/setup", "GET /api/v1/users/me"]);

const sessionExpiry: Middleware = {
  onResponse({ request, response, schemaPath }) {
    if (response.status !== 401 || selfHandled.has(`${request.method} ${schemaPath}`)) return;
    if (!madeWithCurrentToken(request)) return;
    unauthorized();
  },
};

/**
 * Whether a request was made with the session the app has now. A packaged
 * app's request made with a token it has since replaced, such as one still
 * in flight from before it signed in again, says nothing about it, and nor
 * does one to another address, such as the address screen's probe of an
 * address behind a proxy that asks for its own sign-in.
 */
function madeWithCurrentToken(request: Request): boolean {
  if (!isPackaged) return true;
  return toKeptServer(request) && request.headers.get("Authorization") === bearerOf(sessionToken());
}

/** Whether a packaged app's request goes to the kept server's API. */
function toKeptServer(request: Request): boolean {
  const server = packagedServer();
  // The request's URL is parsed, so the address is too: its host's case
  // or a default port may differ from how it was typed.
  return server !== undefined && request.url.startsWith(new URL(`${server.address}/`).href);
}

/** The Authorization header for a token, or null without one. */
function bearerOf(token: string | undefined): string | null {
  return token === undefined ? null : `Bearer ${token}`;
}

// The packaged app's credential. Its page is a file: page, so
// the session cookie would be a third-party cookie there and never sent: its
// requests carry the session's token instead, and no cookies. The token goes
// only to the kept server, never to an address the app is still probing.
const bearer: Middleware = {
  onRequest({ request }) {
    const authorization = bearerOf(sessionToken());
    if (authorization === null || !toKeptServer(request)) return undefined;
    request.headers.set("Authorization", authorization);
    return request;
  },
};

// An answer that isn't a problem document, such as a proxy's error page,
// becomes one with code `unknown`. So any answer from the server is a
// problem. Only a request that got no answer stays a plain error. That is
// a request that failed to send, or a gateway's 502 to 504 saying the
// server is down.
const answers: Middleware = {
  async onResponse({ response }) {
    if (response.ok) return undefined;
    const text = await response.clone().text();
    let body: unknown = text;
    try {
      body = JSON.parse(text);
    } catch {
      // Not JSON, so it's an error page.
    }
    if (isProblem(body) || gatewayDown(response.status, body)) return undefined;
    const problem: Problem = {
      type: "about:blank",
      title: response.statusText,
      status: response.status,
      // A code no client knows, which every client shows as something
      // going wrong.
      code: "unknown" as Problem["code"],
      requestId: response.headers.get("X-Request-Id") ?? "",
    };
    const headers = new Headers(response.headers);
    headers.set("Content-Type", "application/problem+json");
    headers.delete("Content-Length");
    return new Response(JSON.stringify(problem), { status: response.status, statusText: response.statusText, headers });
  },
};

// What the server names, such as track titles, comes in the language the
// app shows, which isn't always the browser's.
const language: Middleware = {
  onRequest({ request }) {
    request.headers.set("Accept-Language", i18n.locale);
    return request;
  },
};

// Which app and device sends each request, which the server keeps with
// the session and its plays.
const identity: Middleware = {
  onRequest({ request }) {
    for (const [name, value] of Object.entries(identityHeaders())) request.headers.set(name, value);
    return request;
  },
};

// How long a request waits for the server before it counts as no answer,
// as the native apps' requests do. A dead connection, such as one left by a
// TV's standby or a changed network, otherwise hangs a request for good.
// A browser has no connect or idle timeout to set, so the whole answer's
// 15 s stands for all three. Starting a play and its fallback, which the
// server answers once the transcode has its first segment, wait 60 s.
// Media, the bandwidth test's stream, which times itself, and an artwork
// upload, which may be large, have no limit.
export const requestTimeoutMs = 15_000;
export const slowRequestTimeoutMs = 60_000;

const slow = new Set(["POST /api/v1/plays", "POST /api/v1/plays/{id}/fallback"]);
const unlimited = new Set([
  "GET /api/v1/bandwidth-test",
  "POST /api/v1/works/{id}/artwork",
  "GET /api/v1/images/{id}",
  "GET /api/v1/fonts/{sha256}",
]);
const media = /^\/api\/v1\/(plays|downloads)\/\{id\}\/[a-z]+\/\{[a-z]+\}$/;

/** How long a request may take, or undefined for no limit. */
export function timeoutOf(method: string, schemaPath: string): number | undefined {
  const key = `${method} ${schemaPath}`;
  if (unlimited.has(key) || (method === "GET" && media.test(schemaPath))) return undefined;
  return slow.has(key) ? slowRequestTimeoutMs : requestTimeoutMs;
}

/** A signal that aborts when either does. AbortSignal.any is Chromium 116, newer than the TV build's 108. */
function either(a: AbortSignal, b: AbortSignal): AbortSignal {
  if (typeof AbortSignal.any === "function") return AbortSignal.any([a, b]);
  const controller = new AbortController();
  for (const signal of [a, b]) {
    if (signal.aborted) controller.abort(signal.reason);
    else signal.addEventListener("abort", () => controller.abort(signal.reason), { once: true });
  }
  return controller.signal;
}

// A timed-out request fails as one that got no answer, a TimeoutError
// rather than a problem, so it shows as the server being unreachable.
const timeouts: Middleware = {
  onRequest({ request, schemaPath }) {
    const ms = timeoutOf(request.method, schemaPath);
    if (ms === undefined) return undefined;
    return new Request(request, { signal: either(request.signal, AbortSignal.timeout(ms)) });
  },
};

/**
 * The one HTTP client. Nothing else calls fetch for the API. Its address is
 * the page's as it loads: the packaged app starts again when it keeps
 * another server (src/tv/packaged.ts). A call that names its own address
 * keeps it, as the address screen's probe of a typed address does.
 */
export const client = createFetchClient<paths>({
  baseUrl: apiBaseUrl(),
  credentials: isPackaged ? "omit" : "same-origin",
  // Looked up per request so test doubles installed later are used.
  fetch: (request) => globalThis.fetch(request),
});
client.use(timeouts, sessionExpiry, answers, language, identity, ...(isPackaged ? [bearer] : []));

/** TanStack Query bindings over the client: query options and hooks per path. */
export const $api = createQueryClient(client);
