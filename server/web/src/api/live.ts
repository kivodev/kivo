import { type Query, type QueryClient, queryOptions, skipToken, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { isPackaged } from "@/tv/flag";
import { apiBaseUrl, sessionToken, unauthorized } from "./client";
import { savingLayout } from "./home";
import { type ApiPath, apiPaths, libraryGridPaths, listViewPaths, workViewPaths } from "./paths";
import type { EventMessage, EventStreamMessage, SnapshotMessage } from "./schemas";

// Live events, over one WebSocket per tab. Each event kind maps to one
// cache operation. The problem count updates in place. Other changes
// invalidate by path in one-second batches. Nothing polls. On reconnect,
// the new snapshot is applied and the queries kept live are refetched.

/** What the live events said about one play. */
export type PlayNews = {
  /** Stopped by an admin, another client or expiry. */
  ended: boolean;
  /** The admin's words to the viewer, if an admin stopped it with some. */
  message?: string;
};

export const liveQueries = {
  /** The latest snapshot. problemsChanged events keep its problem count current. */
  snapshot: () =>
    queryOptions<SnapshotMessage>({ queryKey: ["live", "snapshot"], queryFn: skipToken, staleTime: Infinity }),
  /** What playEnded said about a play, for the player to watch. */
  play: (id: string) =>
    queryOptions<PlayNews>({ queryKey: ["live", "play", id], queryFn: skipToken, staleTime: Infinity }),
};

/** Narrows an invalidation to queries whose path ID is one of ids. */
export type Scope = { ids: readonly string[] };

export type CacheOp =
  | { kind: "invalidate"; path: ApiPath; scope?: Scope }
  | { kind: "updateProblemCount"; problemCount: number }
  | { kind: "updatePlay"; playId: string; ended: true; message?: string }
  | { kind: "setSnapshot"; snapshot: SnapshotMessage }
  | { kind: "refetchAll" };

const invalidate = (path: ApiPath, scope?: Scope): CacheOp =>
  scope ? { kind: "invalidate", path, scope } : { kind: "invalidate", path };

/** Every view of works, with a work's own page narrowed to ids. */
const workViews = (ids?: readonly string[]): CacheOp[] =>
  workViewPaths.map((path) => invalidate(path, path === apiPaths.work && ids ? { ids } : undefined));

/** The cache operations for one event. Unknown kinds map to nothing. */
export function planEvent(event: EventMessage): CacheOp[] {
  switch (event.kind) {
    case "workChanged":
      return workViews(event.workId ? [event.workId] : undefined);
    case "libraryChanged": {
      // Home, the library list's counts, and the libraries' grids with their filters and letter rails.
      const ids = event.libraryIds ?? [];
      return [
        invalidate(apiPaths.home),
        invalidate(apiPaths.homeRows),
        invalidate(apiPaths.lists),
        invalidate(apiPaths.listWorks),
        invalidate(apiPaths.libraries),
        invalidate(apiPaths.library, { ids }),
        invalidate(apiPaths.libraryFacets, { ids }),
        ...libraryGridPaths.map((path) => invalidate(path, { ids })),
      ];
    }
    case "homeChanged":
      return [invalidate(apiPaths.home), invalidate(apiPaths.homeRows)];
    case "listsChanged":
      return listViewPaths.map((path) => invalidate(path));
    case "userStateChanged":
      // Cards everywhere show the state, and Home's rows follow it.
      return workViews(event.workIds);
    case "tasksChanged":
      // That work produces the tasks' runs and failures, library scans
      // and counts, works' files and missing files.
      return [
        invalidate(apiPaths.tasks),
        invalidate(apiPaths.taskFailures),
        invalidate(apiPaths.libraries),
        invalidate(apiPaths.library),
        invalidate(apiPaths.workFiles),
        invalidate(apiPaths.files),
      ];
    case "playsChanged":
      return [invalidate(apiPaths.plays), invalidate(apiPaths.playEvents), invalidate(apiPaths.currentUserPlayEvents)];
    case "logsChanged":
      return [invalidate(apiPaths.logs)];
    case "activityChanged":
      return [invalidate(apiPaths.activity)];
    case "problemsChanged":
      return [
        ...(event.problemCount === undefined
          ? []
          : [{ kind: "updateProblemCount", problemCount: event.problemCount } as const]),
        invalidate(apiPaths.problems),
      ];
    case "playEnded":
      return event.playId
        ? [
            {
              kind: "updatePlay",
              playId: event.playId,
              ended: true,
              message: event.message,
            },
          ]
        : [];
    default:
      return [];
  }
}

/** The cache operations for one frame, given whether this connection is a reconnect. */
export function planMessage(message: EventStreamMessage, reconnect: boolean): CacheOp[] {
  switch (message.type) {
    case "snapshot":
      return reconnect
        ? [{ kind: "setSnapshot", snapshot: message }, { kind: "refetchAll" }]
        : [{ kind: "setSnapshot", snapshot: message }];
    case "event":
      return planEvent(message);
    case "resync":
      return [{ kind: "refetchAll" }];
    default:
      // keepAlive, and a frame type this client doesn't know yet.
      return [];
  }
}

type KeyInit = { params?: { path?: { id?: string } } };

/** Whether an invalidation reaches a query. The query must have the same path and be inside its scope. */
export function matchesQuery(op: { path: ApiPath; scope?: Scope }, queryKey: readonly unknown[]): boolean {
  if (queryKey[0] !== "get" || queryKey[1] !== op.path) return false;
  if (!op.scope) return true;
  const id = ((queryKey[2] ?? {}) as KeyInit).params?.path?.id;
  return id !== undefined && op.scope.ids.includes(id);
}

/** Applies cache operations. Updates apply at once, and invalidations are batched. */
export function createCacheSync(queryClient: QueryClient, batchMs = 1000) {
  const pending = new Map<string, { path: ApiPath; scope?: Scope }>();
  let timer: ReturnType<typeof setTimeout> | undefined;

  const flush = () => {
    timer = undefined;
    // A save of the layout reads it when it's done; a read now would undo what's on screen.
    const saving = savingLayout(queryClient) > 0;
    const ops = [...pending.values()].filter((op) => !(saving && op.path === apiPaths.homeRows));
    pending.clear();
    void queryClient.invalidateQueries({
      predicate: (query: Query) => ops.some((op) => matchesQuery(op, query.queryKey)),
    });
  };

  const apply = (op: CacheOp) => {
    switch (op.kind) {
      case "invalidate":
        pending.set(JSON.stringify(op), op);
        timer ??= setTimeout(flush, batchMs);
        return;
      case "setSnapshot":
        queryClient.setQueryData(liveQueries.snapshot().queryKey, op.snapshot);
        return;
      case "updatePlay":
        queryClient.setQueryData(liveQueries.play(op.playId).queryKey, {
          ended: true,
          message: op.message,
        });
        return;
      case "updateProblemCount":
        queryClient.setQueryData(liveQueries.snapshot().queryKey, (s) =>
          s ? { ...s, problemCount: op.problemCount } : s,
        );
        return;
      case "refetchAll":
        pending.clear();
        if (timer) clearTimeout(timer);
        timer = undefined;
        void queryClient.invalidateQueries({ predicate: (q) => q.queryKey[0] !== "live" });
        return;
    }
  };

  return {
    apply,
    dispose: () => {
      if (timer) clearTimeout(timer);
      pending.clear();
    },
  };
}

/** A socket that has failed is retrying: `since` the first failure of this outage, with the next attempt `retryAt`. */
export type LiveStatus =
  | { state: "connecting" }
  | { state: "open" }
  | { state: "retrying"; since: number; retryAt: number };

type ConnectOptions = {
  onMessage: (message: EventStreamMessage, reconnect: boolean) => void;
  onStatus: (status: LiveStatus) => void;
};

/** WebSocket close code for a session that ended or was revoked. */
const closeUnauthorized = 4401;

const maxBackoffMs = 30_000;

// The server sends a keep-alive frame every 25 seconds (events.go). A
// socket that hears nothing for three of them is taken for dead: a TV in
// standby or a changed network address leaves a half-open connection that
// never fires onclose. Coming back after being hidden longer than
// staleAfterHiddenMs reconnects at once, rather than waiting out the
// silence, since the connection may have died while the page was frozen.
export const silenceMs = 75_000;
const watchEveryMs = 5_000;
export const staleAfterHiddenMs = 30_000;

/**
 * Opens the event socket. The packaged app's socket can't set a header, so
 * its token goes as a subprotocol beside `kivo`, which the server reads on
 * this upgrade alone and answers with `kivo`; the token never
 * goes in the URL. The web build offers none, as the cookie is its
 * credential.
 */
function openSocket(token: string | undefined): WebSocket {
  if (!isPackaged || token === undefined) return new WebSocket(eventsUrl());
  return new WebSocket(eventsUrl(), ["kivo", `kivo-bearer.${token}`]);
}

function eventsUrl(): string {
  const url = new URL(apiBaseUrl() + apiPaths.events);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.href;
}

/**
 * Opens the event socket and keeps it open, backing off between attempts.
 * A socket gone quiet, or one that may have died while the page was hidden
 * or the network changed, is replaced by a new one. That counts as a
 * reconnect, so its snapshot refetches what's live.
 */
export function connectLiveEvents({ onMessage, onStatus }: ConnectOptions) {
  let socket: WebSocket | undefined;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;
  let attempts = 0;
  let failedSince = 0;
  let opened = false;
  let closed = false;
  let lastFrameAt = Date.now();
  let hiddenAt: number | undefined;

  const open = () => {
    retryTimer = undefined;
    lastFrameAt = Date.now();
    // After a failure the status stays retrying through each attempt, so
    // what shows it doesn't come and go, moving the page, with every try.
    if (attempts === 0) onStatus({ state: "connecting" });
    const token = sessionToken();
    const ws = openSocket(token);
    socket = ws;
    // The first snapshot on a new socket after an earlier one means events
    // may have been missed. Later snapshots follow a resync, which already
    // refetched.
    let reconnect = opened;
    ws.onopen = () => {
      attempts = 0;
      lastFrameAt = Date.now();
      onStatus({ state: "open" });
    };
    ws.onmessage = (e) => {
      // Any frame, keep-alives included, shows the socket is alive.
      lastFrameAt = Date.now();
      if (typeof e.data !== "string") return;
      let message: EventStreamMessage;
      try {
        message = JSON.parse(e.data) as EventStreamMessage;
      } catch {
        return;
      }
      onMessage(message, reconnect);
      if (message.type === "snapshot") {
        opened = true;
        reconnect = false;
      }
    };
    ws.onclose = (e) => {
      if (closed) return;
      if (e.code === closeUnauthorized) {
        // A packaged app's socket opened with a token it has since replaced
        // says nothing about the session it has now.
        if (token === sessionToken()) unauthorized();
        return;
      }
      const delay = Math.min(1000 * 2 ** attempts, maxBackoffMs);
      if (attempts === 0) failedSince = Date.now();
      attempts++;
      onStatus({ state: "retrying", since: failedSince, retryAt: Date.now() + delay });
      retryTimer = setTimeout(open, delay);
    };
  };

  /**
   * Drops the current socket without waiting for it to close, which a
   * half-open connection may never do, and opens a new one. A wait between
   * attempts is cut short.
   */
  const replace = () => {
    if (closed) return;
    if (retryTimer !== undefined) {
      clearTimeout(retryTimer);
    } else if (socket) {
      socket.onopen = socket.onmessage = socket.onclose = null;
      socket.close();
    }
    open();
  };

  const watch = setInterval(() => {
    if (retryTimer === undefined && Date.now() - lastFrameAt > silenceMs) replace();
  }, watchEveryMs);

  const onVisibility = () => {
    if (document.visibilityState === "hidden") {
      hiddenAt = Date.now();
      return;
    }
    const away = hiddenAt === undefined ? 0 : Date.now() - hiddenAt;
    hiddenAt = undefined;
    if (away > staleAfterHiddenMs) replace();
  };
  // A new network, such as a new address after the TV wakes, strands the old connection.
  const onOnline = () => replace();
  document.addEventListener("visibilitychange", onVisibility);
  window.addEventListener("online", onOnline);

  open();

  return {
    // While retrying, an attempt still connecting may hang, so it's
    // replaced like a wait between attempts.
    retryNow: () => {
      if (attempts > 0) replace();
    },
    close: () => {
      closed = true;
      if (retryTimer) clearTimeout(retryTimer);
      clearInterval(watch);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("online", onOnline);
      socket?.close();
    },
  };
}

/** What the tab's event socket says about itself. */
export type LiveState = {
  status: LiveStatus;
  /** The server's version changed since this tab loaded, so offer a reload. */
  updateAvailable: boolean;
  retryNow: () => void;
};

/**
 * Keeps the tab's event socket open while mounted, applying each frame to
 * the query cache. A socket closed for an invalid session signs out, as a
 * 401 does.
 */
export function useLiveEvents(): LiveState {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<LiveStatus>({ state: "connecting" });
  const [updateAvailable, setUpdateAvailable] = useState(false);
  const connectionRef = useRef<{ retryNow: () => void } | undefined>(undefined);

  useEffect(() => {
    const sync = createCacheSync(queryClient);
    let firstVersion: string | undefined;
    const connection = connectLiveEvents({
      onMessage: (message, reconnect) => {
        if (message.type === "snapshot") {
          firstVersion ??= message.serverVersion;
          if (message.serverVersion !== firstVersion) setUpdateAvailable(true);
        }
        for (const op of planMessage(message, reconnect)) sync.apply(op);
      },
      onStatus: setStatus,
    });
    connectionRef.current = connection;
    return () => {
      connection.close();
      sync.dispose();
    };
  }, [queryClient]);

  return { status, updateAvailable, retryNow: () => connectionRef.current?.retryNow() };
}
