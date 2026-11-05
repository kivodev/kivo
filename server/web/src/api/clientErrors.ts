import type { Middleware } from "openapi-fetch";
import { browserOf } from "@/lib/browser";
import { isTv } from "@/tv/flag";
import { client } from "./client";
import { buildVersion } from "./identity";
import { isProblem } from "./problem";
import type { components } from "./schema";

// The TV build's uncaught errors go to the server's log, where an admin
// reads them on the Logs page. On a TV that is the only way to see one
// without Developer Mode and the remote inspector. The reports go to the
// household's own server and nowhere else. A computer's browser has its
// own console, so the web build reports nothing.

export type ClientErrorReport = components["schemas"]["ClientErrorReport"];
type ClientErrorKind = ClientErrorReport["kind"];
type ClientEngine = components["schemas"]["ClientEngine"];

/** How many reports a page sends a minute, under the server's own limit of 10. */
export const reportsPerMinute = 3;
const minute = 60_000;
// The messages already sent, so an error repeated in a loop is sent once.
const maxRemembered = 50;
const maxMessage = 500;
const maxFrames = 10;
const maxFrame = 300;

/** A URL's query and fragment, cut from anything reported: they can hold a search or a token. */
const urlQuery = /([a-z][a-z0-9+.-]*:\/\/[^\s?#)]*)[?#][^\s)]*/gi;

/**
 * Text that is safe to report: URLs lose their queries, and what the user
 * typed (the page's query values, and the focused field's value) is
 * replaced by an ellipsis.
 */
export function scrub(text: string, typed: readonly string[] = typedText()): string {
  let out = text.replace(urlQuery, "$1");
  for (const value of typed) {
    if (value.length >= 2) out = out.split(value).join("…");
  }
  return out;
}

// What the user typed that an error's text might repeat: the values in the
// page's query and hash, such as a search, and the focused field's value.
function typedText(): string[] {
  const out: string[] = [];
  const add = (query: string) => {
    for (const [, value] of new URLSearchParams(query)) out.push(value);
  };
  add(location.search);
  add(location.hash.replace(/^#/, ""));
  const field = document.activeElement;
  if (field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement) out.push(field.value);
  return out;
}

/**
 * The route's template for a path: each segment that isn't a plain word,
 * such as an ID, becomes `$id`, and the app's base path is left out.
 */
export function routeTemplate(pathname: string, base = new URL(document.baseURI).pathname): string {
  const rest = pathname.startsWith(base) ? pathname.slice(base.length) : pathname;
  const segments = rest.split("/").filter(Boolean);
  return `/${segments.map((s) => (/^[a-z][a-z-]*$/.test(s) ? s : "$id")).join("/")}`;
}

/** The engine a user agent names: the platform, Tizen's version, and Chromium's major version. */
export function engineOf(ua: string): ClientEngine {
  const browser = browserOf(ua, "");
  const engine: ClientEngine = { platform: browser.tizen ? "tizen" : browser.webos ? "webos" : "browser" };
  if (browser.tizen) engine.tizenVersion = browser.tizen.toFixed(1);
  const chromium = browser.chromium?.split(".")[0];
  if (chromium) engine.chromiumVersion = chromium;
  return engine;
}

// The messages Chromium, Firefox and Safari give a dynamic import that
// failed to load, and the one Vite gives a failed preload.
const chunkFailure =
  /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed|Unable to preload CSS/i;

/** Whether an error is a code chunk failing to load. */
export function isChunkError(error: unknown): boolean {
  return chunkFailure.test(error instanceof Error ? error.message : String(error));
}

/**
 * Whether an error is the app's own. A problem is the server's answer,
 * which the server logged itself, and a request that got no answer, timed
 * out or was cancelled says the server can't be reached, which a report
 * can't change.
 */
export function worthReporting(error: unknown): boolean {
  if (isProblem(error)) return false;
  if (isChunkError(error)) return true;
  // A DOMException isn't an Error on every engine, so it's told by its name.
  const name = typeof error === "object" && error !== null ? (error as { name?: unknown }).name : undefined;
  if (name === "AbortError" || name === "TimeoutError") return false;
  return !(error instanceof TypeError && /fetch|network|load failed/i.test(error.message));
}

/** A report built from what was thrown. */
export function reportOf(
  kind: ClientErrorKind,
  error: unknown,
  route: string,
  typed?: readonly string[],
): ClientErrorReport {
  const message =
    error instanceof Error
      ? `${error.name}: ${error.message}`
      : typeof error === "string"
        ? error
        : "a non-Error value was thrown";
  const lines = error instanceof Error && error.stack ? error.stack.split("\n") : [];
  // V8 starts a stack with the message, which is sent once already.
  const frames = lines.filter((line, i) => !(i === 0 && message.startsWith(line.trim())) && line.trim() !== "");
  return {
    kind,
    message: scrub(message, typed).slice(0, maxMessage),
    stack: frames.slice(0, maxFrames).map((f) => scrub(f.trim(), typed).slice(0, maxFrame)),
    route,
  };
}

interface ReporterDeps {
  send: (report: ClientErrorReport) => Promise<unknown>;
  signedIn: () => boolean;
  now?: () => number;
}

/**
 * A reporter that sends at most `reportsPerMinute` reports, each message
 * once, and only while signed in. It never throws: a failure while
 * reporting, or an error thrown while it reports, is dropped.
 */
export function createReporter({ send, signedIn, now = Date.now }: ReporterDeps) {
  const sentAt: number[] = [];
  const seen = new Set<string>();
  let busy = false;
  return function report(build: () => ClientErrorReport): void {
    if (busy) return;
    busy = true;
    try {
      if (!signedIn()) return;
      const t = now();
      while (sentAt.length > 0 && t - (sentAt[0] ?? 0) >= minute) sentAt.shift();
      if (sentAt.length >= reportsPerMinute) return;
      const r = build();
      const key = `${r.kind} ${r.message}`;
      if (seen.has(key)) return;
      if (seen.size >= maxRemembered) seen.clear();
      seen.add(key);
      sentAt.push(t);
      send(r).catch(() => {});
    } catch {
      // Reporting must never be the next error.
    } finally {
      busy = false;
    }
  };
}

// Whether this page is signed in, as the API's answers say: who is signed
// in answers 200 only to a session, any 401 means none, and signing out
// ends it.
let isSignedIn = false;
const sessionWatch: Middleware = {
  onResponse({ request, response, schemaPath }) {
    if (response.status === 401) isSignedIn = false;
    else if (request.method === "GET" && schemaPath === "/api/v1/users/me" && response.ok) isSignedIn = true;
    else if (request.method === "DELETE" && schemaPath === "/api/v1/sessions/current" && response.ok)
      isSignedIn = false;
    return undefined;
  },
};

const reporter = createReporter({
  send: (body) => client.POST("/api/v1/client-errors", { body }),
  signedIn: () => isSignedIn,
});

/**
 * Reports an error to the server's log. `route` is the route's template
 * when the caller knows it, as a route's error component does.
 */
export function reportClientError(kind: ClientErrorKind, error: unknown, route?: string): void {
  if (!isTv || !worthReporting(error)) return;
  reporter(() => ({
    ...reportOf(isChunkError(error) ? "chunk" : kind, error, route ?? routeTemplate(location.pathname)),
    version: buildVersion,
    engine: engineOf(navigator.userAgent),
  }));
}

let started = false;

/**
 * Reports every uncaught error and unhandled rejection, and a script or
 * stylesheet of the app's own that fails to load. The TV build calls it
 * once, at start.
 */
export function startClientErrorReports(): void {
  if (started) return;
  started = true;
  client.use(sessionWatch);
  window.addEventListener("unhandledrejection", (event) => reportClientError("unhandledRejection", event.reason));
  // In the capture phase, which also sees a script or stylesheet failing to
  // load: that error doesn't bubble, and has no error object.
  window.addEventListener(
    "error",
    (event) => {
      const el = event.target;
      if (el instanceof HTMLScriptElement || el instanceof HTMLLinkElement) {
        const url = el instanceof HTMLScriptElement ? el.src : el.href;
        reportClientError("chunk", new Error(`failed to load ${new URL(url, document.baseURI).pathname}`));
      } else if (event instanceof ErrorEvent) {
        reportClientError("error", event.error ?? event.message);
      }
    },
    true,
  );
  // Vite's event for a lazy route's chunk, or its CSS, failing to preload.
  window.addEventListener("vite:preloadError", (event) => {
    reportClientError("chunk", (event as Event & { payload?: unknown }).payload ?? "preload failed");
  });
}
