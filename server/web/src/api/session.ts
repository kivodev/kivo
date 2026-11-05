import { type QueryClient, useMutation, useQueryClient } from "@tanstack/react-query";
import { type AnyRouter, useRouter } from "@tanstack/react-router";
import { useEffect, useEffectEvent, useState } from "react";
import { isPackaged } from "@/tv/flag";
import { packagedClientId, packagedServer, packagedToken, setPackagedServer, setPackagedToken } from "@/tv/packaged";
import { $api, client } from "./client";
import { apiPaths, pathKey } from "./paths";
import type { Session } from "./schemas";
import { signedIn } from "./users";

export const sessionQueries = {
  /** Where you're signed in: your browsers and apps, most recently used first. */
  list: () => $api.queryOptions("get", "/api/v1/sessions"),
  /** A user's browsers and apps, as an admin sees them. */
  forUser: (userId: string) => $api.queryOptions("get", "/api/v1/sessions", { params: { query: { userId } } }),
};

/**
 * What the packaged TV app asks for as it signs in: an app
 * session under the client ID it keeps, so that signing in again replaces
 * the TV's session rather than adding one.
 */
export function packagedSession() {
  return { kind: "app", clientId: packagedClientId() } as const;
}

/**
 * Keeps a new session's token in the packaged app, which sends it in place
 * of a cookie. A browser's session is its cookie, so this does nothing
 * there.
 */
function keepToken(session: Session) {
  if (isPackaged && session.token !== undefined) setPackagedToken(session.token);
}

/**
 * Sign in with a password. The server sets the session cookie, or, for the
 * packaged app, answers with the token.
 */
export function useSignIn() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/sessions", {
    onSuccess: (session) => {
      keepToken(session);
      return signedIn(queryClient);
    },
  });
}

// What a screen keeps in the page, outside the query cache, that must not
// outlive the session: on a shared TV the next person would see it.
const signOutResets: (() => void)[] = [];

/** Runs `reset` whenever this browser is signed out, after the cache is cleared. */
export function onSignedOut(reset: () => void) {
  signOutResets.push(reset);
}

// Requests in flight and the event socket can each find the session gone
// at the same moment. Only one of them leaves the page.
let leaving = false;

/**
 * Runs once this browser is signed out. It leaves for the sign-in page and
 * clears the cache, so nothing from the session survives. An expired
 * session (a 401, or the event socket closing with 4401) returns to the
 * same page after signing in. The packaged app, once it has forgotten its
 * server, leaves for the address screen instead.
 */
export async function signedOut(router: AnyRouter, queryClient: QueryClient, expired: boolean): Promise<void> {
  if (leaving) return;
  leaving = true;
  try {
    const here = router.state.location.href;
    if (isPackaged && packagedServer() === undefined) {
      await router.navigate({ to: "/server", replace: true });
    } else {
      await router.navigate({
        to: "/sign-in",
        search: { redirect: expired && !here.startsWith("/sign-in") ? here : undefined },
        replace: expired,
      });
    }
    queryClient.clear();
    for (const reset of signOutResets) reset();
  } finally {
    leaving = false;
  }
}

/**
 * Sign this browser out. Once the server has signed it out, it leaves for
 * the sign-in page with the cache cleared. The packaged app forgets its
 * token first and keeps its server's address. On a failure it stays put,
 * with the error on the mutation.
 */
export function useSignOutHere() {
  const queryClient = useQueryClient();
  const router = useRouter();
  return $api.useMutation("delete", "/api/v1/sessions/current", {
    onSuccess: () => {
      if (isPackaged) setPackagedToken(undefined);
      return signedOut(router, queryClient, false);
    },
  });
}

/**
 * The packaged app's Change server. When signed in, it signs the TV out of
 * the server first, and forgets the token even when the server doesn't
 * answer, leaving the session on the account's Where you're signed in to
 * revoke there. Then it forgets the server and leaves for the address
 * screen with the cache cleared.
 */
export function useChangeServer() {
  const queryClient = useQueryClient();
  const router = useRouter();
  return useMutation({
    mutationFn: async () => {
      if (packagedToken() !== undefined) {
        await client.DELETE("/api/v1/sessions/current").catch(() => undefined);
      }
      setPackagedServer(undefined);
      await signedOut(router, queryClient, false);
      // A 401 to the revoke, for a session already gone, may have left for
      // sign-in by itself, before the server was forgotten.
      if (router.state.location.pathname !== "/server") await router.navigate({ to: "/server", replace: true });
    },
  });
}

/**
 * The address screen's leaving of the kept server for another address: it
 * revokes the session there, as the app starts again on the other. It
 * doesn't wait, as that server may be the one that no longer answers, and
 * the request outlives the page. It carries the token itself, since the
 * server it goes to is no longer kept once it's sent, and its answer ends
 * nothing here.
 */
export function revokeKeptSession(): void {
  const server = packagedServer();
  const token = packagedToken();
  if (server === undefined || token === undefined) return;
  void client
    .DELETE("/api/v1/sessions/current", {
      baseUrl: server.address,
      headers: { Authorization: `Bearer ${token}` },
      keepalive: true,
    })
    .catch(() => undefined);
}

/**
 * Sign another browser or app out. It can be yours, or, as an admin,
 * another user's. The lists refetch.
 */
export function useRevokeSession() {
  const queryClient = useQueryClient();
  return $api.useMutation("delete", "/api/v1/sessions/{id}", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.sessions) }),
  });
}

/**
 * Rename a browser or app. It can be yours, or, as an admin, another
 * user's. The lists refetch.
 */
export function useRenameSession() {
  const queryClient = useQueryClient();
  return $api.useMutation("patch", "/api/v1/sessions/{id}", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.sessions) }),
  });
}

/**
 * Whom Sign out everywhere signs out. That is every browser and app of
 * yours except this one, or, as an admin, each of a user's sessions by ID.
 */
type Everywhere = { except: "current" } | { ids: string[] };

/** Sign out everywhere. The lists refetch, even after a partial failure. */
export function useSignOutEverywhere() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (whom: Everywhere) => {
      if ("except" in whom) {
        const { error } = await client.DELETE("/api/v1/sessions", { params: { query: whom } });
        if (error) throw error;
        return;
      }
      for (const id of whom.ids) {
        const { error } = await client.DELETE("/api/v1/sessions/{id}", { params: { path: { id } } });
        if (error) throw error;
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.sessions) }),
  });
}

// Device codes. The /link page shows the device waiting on a code, then
// lets it sign in as you, or refuses it.
export const deviceCodeQueries = {
  waiting: (code: string) =>
    $api.queryOptions("get", "/api/v1/device-codes/{code}", { params: { path: { code } } }, { retry: false }),
};

export function useApproveDeviceCode() {
  return $api.useMutation("post", "/api/v1/device-codes/{code}/approve");
}

export function useDenyDeviceCode() {
  return $api.useMutation("post", "/api/v1/device-codes/{code}/deny");
}

// How long to wait to start a code again when the server doesn't say.
const defaultRetrySeconds = 10;

/** A code this browser shows to be signed in by another device. */
type BrowserLink = { code: string; linkUrl: string };

/** Where signing this browser in by code is. */
export type BrowserLinkState = {
  /** The code to show. It is absent while a code is being started. */
  link?: BrowserLink;
  /** Whether the last code was denied on the other device. A new code shows. */
  denied: boolean;
  /** Why no code could be started. It stays until Retry. */
  error?: unknown;
  retry: () => void;
};

/**
 * Signs this browser in by code. It starts a browser device code named
 * [name] and polls it as often as the server says. It starts a new code
 * when one expires or is denied. Once the code is approved, the poll sets
 * the session cookie, and this fills the cache and calls onSignedIn. The
 * packaged app starts an app code instead, and keeps the token the poll
 * answers with.
 */
export function useBrowserLink(name: string, onSignedIn: () => void): BrowserLinkState {
  const queryClient = useQueryClient();
  const [attempt, setAttempt] = useState(0);
  const [link, setLink] = useState<BrowserLink>();
  const [denied, setDenied] = useState(false);
  const [error, setError] = useState<unknown>();
  const done = useEffectEvent(onSignedIn);

  // biome-ignore lint/correctness/useExhaustiveDependencies: each attempt starts a new code.
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let expiry: ReturnType<typeof setTimeout> | undefined;
    const wait = (ms: number) =>
      new Promise<void>((resolve) => {
        timer = setTimeout(resolve, ms);
      });
    void (async () => {
      const started = await client.POST("/api/v1/device-codes", {
        body: isPackaged ? { name, ...packagedSession() } : { name, kind: "browser" },
      });
      if (stopped) return;
      if (!started.data) {
        setError(started.error ?? new Error("no answer"));
        // Too many codes pending from this address: the server says when
        // it will take another, and the panel asks again then by itself.
        // Retry stays for asking sooner.
        if (started.response?.status === 429) {
          await wait(Math.max(1, started.error?.retryAfterSeconds ?? defaultRetrySeconds) * 1000);
          if (stopped) return;
          setError(undefined);
          setAttempt((n) => n + 1);
        }
        return;
      }
      const { code, linkUrl, expiresAt, interval, secret } = started.data;
      // An expired code is replaced. Its life is measured on the server's
      // clock, from the answer's Date, because a TV's own clock can be
      // minutes out and would expire every code as it starts.
      const serverNow = Date.parse(started.response.headers.get("Date") ?? "");
      const life = Math.max(0, Date.parse(expiresAt) - (Number.isNaN(serverNow) ? Date.now() : serverNow));
      setLink({ code, linkUrl });
      const replace = () => {
        stopped = true;
        clearTimeout(timer);
        setLink(undefined);
        setAttempt((n) => n + 1);
      };
      // While the poll waits, the code is replaced at once. A poll in flight
      // as it expires, as on a slow TV, may have reached the server in time
      // and been approved, and its answer sets the session cookie: the code
      // is replaced once that answer is in, unless it signed in.
      let polling = false;
      let expired = false;
      expiry = setTimeout(() => {
        if (polling) expired = true;
        else replace();
      }, life);
      for (;;) {
        await wait(interval * 1000);
        if (stopped) return;
        polling = true;
        const poll = await client.POST("/api/v1/device-codes/poll", { body: { secret } }).catch(() => undefined);
        polling = false;
        if (stopped) return;
        const status = poll?.response.status;
        if (status === 201) {
          // The code is spent either way, so nothing replaces it on expiry.
          clearTimeout(expiry);
          try {
            if (poll?.data) keepToken(poll.data);
            await signedIn(queryClient);
          } catch (e) {
            // Approved, but there is no session: the cookie may have been
            // dropped. The code is spent, so Retry starts a new one.
            if (stopped) return;
            setLink(undefined);
            setError(e);
            return;
          }
          done();
          return;
        }
        if (expired) return replace();
        if (status === 403 || status === 410) {
          setLink(undefined);
          setDenied(status === 403);
          setAttempt((n) => n + 1);
          return;
        }
        if (status === 429 && poll?.error) await wait((poll.error.retryAfterSeconds ?? 0) * 1000);
      }
    })();
    return () => {
      stopped = true;
      clearTimeout(timer);
      clearTimeout(expiry);
    };
  }, [attempt, name, queryClient]);

  const retry = () => {
    setError(undefined);
    setAttempt((n) => n + 1);
  };
  return { link, denied, error, retry };
}
