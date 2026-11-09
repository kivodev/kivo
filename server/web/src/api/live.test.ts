import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  connectLiveEvents,
  createCacheSync,
  liveQueries,
  matchesQuery,
  planEvent,
  planMessage,
  silenceMs,
  staleAfterHiddenMs,
} from "./live";
import { apiPaths } from "./paths";
import type { EventMessage, SnapshotMessage } from "./schemas";

const lib = "0190a000-0000-7000-8000-000000000001";
const other = "0190a000-0000-7000-8000-000000000002";
const work = "0190a000-0000-7000-8000-000000000003";

const snapshot = (serverVersion = "1.0.0"): SnapshotMessage => ({
  type: "snapshot",
  serverVersion,
  problemCount: 2,
});

describe("planEvent", () => {
  it("maps workChanged to the work and every list that shows it", () => {
    const views = (ids: string[]) => [
      { kind: "invalidate", path: apiPaths.work, scope: { ids } },
      { kind: "invalidate", path: apiPaths.libraryWorks },
      { kind: "invalidate", path: apiPaths.libraryCollections },
      { kind: "invalidate", path: apiPaths.libraryAuthors },
      { kind: "invalidate", path: apiPaths.libraryBookSeries },
      { kind: "invalidate", path: apiPaths.workChildren },
      { kind: "invalidate", path: apiPaths.collection },
      { kind: "invalidate", path: apiPaths.unmatchedWorks },
      { kind: "invalidate", path: apiPaths.home },
      { kind: "invalidate", path: apiPaths.listWorks },
      { kind: "invalidate", path: apiPaths.search },
      { kind: "invalidate", path: apiPaths.person },
      { kind: "invalidate", path: apiPaths.bookSeries },
    ];
    expect(planEvent({ type: "event", kind: "workChanged", workId: work })).toEqual(views([work]));
  });

  it("maps libraryChanged to Home, the library list, and those libraries' grids, filters and letters", () => {
    expect(planEvent({ type: "event", kind: "libraryChanged", libraryIds: [lib] })).toEqual([
      { kind: "invalidate", path: apiPaths.home },
      { kind: "invalidate", path: apiPaths.homeRows },
      { kind: "invalidate", path: apiPaths.lists },
      { kind: "invalidate", path: apiPaths.listWorks },
      { kind: "invalidate", path: apiPaths.libraries },
      { kind: "invalidate", path: apiPaths.library, scope: { ids: [lib] } },
      { kind: "invalidate", path: apiPaths.libraryFacets, scope: { ids: [lib] } },
      { kind: "invalidate", path: apiPaths.libraryWorks, scope: { ids: [lib] } },
      { kind: "invalidate", path: apiPaths.libraryCollections, scope: { ids: [lib] } },
      { kind: "invalidate", path: apiPaths.libraryAuthors, scope: { ids: [lib] } },
      { kind: "invalidate", path: apiPaths.libraryBookSeries, scope: { ids: [lib] } },
    ]);
  });

  it("maps homeChanged to Home and its layout", () => {
    expect(planEvent({ type: "event", kind: "homeChanged" })).toEqual([
      { kind: "invalidate", path: apiPaths.home },
      { kind: "invalidate", path: apiPaths.homeRows },
    ]);
  });

  it("maps userStateChanged to the works and every list that shows them", () => {
    expect(planEvent({ type: "event", kind: "userStateChanged", workIds: [work, other] })).toEqual([
      { kind: "invalidate", path: apiPaths.work, scope: { ids: [work, other] } },
      { kind: "invalidate", path: apiPaths.libraryWorks },
      { kind: "invalidate", path: apiPaths.libraryCollections },
      { kind: "invalidate", path: apiPaths.libraryAuthors },
      { kind: "invalidate", path: apiPaths.libraryBookSeries },
      { kind: "invalidate", path: apiPaths.workChildren },
      { kind: "invalidate", path: apiPaths.collection },
      { kind: "invalidate", path: apiPaths.unmatchedWorks },
      { kind: "invalidate", path: apiPaths.home },
      { kind: "invalidate", path: apiPaths.listWorks },
      { kind: "invalidate", path: apiPaths.search },
      { kind: "invalidate", path: apiPaths.person },
      { kind: "invalidate", path: apiPaths.bookSeries },
    ]);
  });

  it("updates the problem count in place and refetches the problems", () => {
    expect(planEvent({ type: "event", kind: "problemsChanged", problemCount: 3 })).toEqual([
      { kind: "updateProblemCount", problemCount: 3 },
      { kind: "invalidate", path: apiPaths.problems },
    ]);
  });

  it("maps tasksChanged to the tasks and what their work changes: failures, libraries' scans and counts, works' files, missing files", () => {
    expect(planEvent({ type: "event", kind: "tasksChanged" })).toEqual([
      { kind: "invalidate", path: apiPaths.tasks },
      { kind: "invalidate", path: apiPaths.taskFailures },
      { kind: "invalidate", path: apiPaths.libraries },
      { kind: "invalidate", path: apiPaths.library },
      { kind: "invalidate", path: apiPaths.workFiles },
      { kind: "invalidate", path: apiPaths.files },
    ]);
  });

  it("maps playsChanged to the plays and the watch history", () => {
    expect(planEvent({ type: "event", kind: "playsChanged" })).toEqual([
      { kind: "invalidate", path: apiPaths.plays },
      { kind: "invalidate", path: apiPaths.playEvents },
      { kind: "invalidate", path: apiPaths.currentUserPlayEvents },
    ]);
  });

  it("maps logsChanged to the recent log", () => {
    expect(planEvent({ type: "event", kind: "logsChanged" })).toEqual([{ kind: "invalidate", path: apiPaths.logs }]);
  });

  it("maps activityChanged to the activity, the Overview's and the Activity page's", () => {
    expect(planEvent({ type: "event", kind: "activityChanged" })).toEqual([
      { kind: "invalidate", path: apiPaths.activity },
    ]);
  });

  it("passes a play's end to the player, with an admin's message", () => {
    expect(planEvent({ type: "event", kind: "playEnded", playId: work })).toEqual([
      { kind: "updatePlay", playId: work, ended: true },
    ]);
    expect(planEvent({ type: "event", kind: "playEnded", playId: work, message: "Bedtime" })).toEqual([
      { kind: "updatePlay", playId: work, ended: true, message: "Bedtime" },
    ]);
  });

  it("skips kinds it doesn't know, as the spec asks", () => {
    expect(planEvent({ type: "event", kind: "somethingNew" as EventMessage["kind"], workId: work })).toEqual([]);
  });
});

describe("planMessage", () => {
  it("applies the first snapshot without refetching", () => {
    expect(planMessage(snapshot(), false)).toEqual([{ kind: "setSnapshot", snapshot: snapshot() }]);
  });

  it("refetches live queries after a reconnect's snapshot and after resync", () => {
    expect(planMessage(snapshot(), true)).toContainEqual({ kind: "refetchAll" });
    expect(planMessage({ type: "resync" }, false)).toEqual([{ kind: "refetchAll" }]);
  });

  it("does nothing for a keep-alive", () => {
    expect(planMessage({ type: "keepAlive" }, true)).toEqual([]);
  });
});

describe("matchesQuery", () => {
  const detail = (id: string) => ["get", apiPaths.work, { params: { path: { id } } }];

  it("matches by path prefix, as query keys start with the method and path", () => {
    expect(matchesQuery({ path: apiPaths.home }, ["get", apiPaths.home])).toBe(true);
    expect(matchesQuery({ path: apiPaths.home }, ["get", apiPaths.libraryWorks])).toBe(false);
    expect(matchesQuery({ path: apiPaths.home }, ["live", "snapshot"])).toBe(false);
  });

  it("narrows details to the listed IDs", () => {
    const op = { path: apiPaths.work, scope: { ids: [work] } } as const;
    expect(matchesQuery(op, detail(work))).toBe(true);
    expect(matchesQuery(op, detail(other))).toBe(false);
  });

  it("narrows grids to the listed libraries, every page of them", () => {
    const page = (id: string) => ["get", apiPaths.libraryWorks, { params: { path: { id }, query: {} } }, "page", 3];
    const op = { path: apiPaths.libraryWorks, scope: { ids: [lib] } } as const;
    expect(matchesQuery(op, page(lib))).toBe(true);
    expect(matchesQuery(op, page(other))).toBe(false);
  });
});

describe("createCacheSync", () => {
  let queryClient: QueryClient;
  beforeEach(() => {
    vi.useFakeTimers();
    queryClient = new QueryClient();
  });
  afterEach(() => vi.useRealTimers());

  it("batches invalidations into one pass per second", () => {
    const spy = vi.spyOn(queryClient, "invalidateQueries");
    const sync = createCacheSync(queryClient);
    for (const op of planEvent({ type: "event", kind: "workChanged", workId: work })) sync.apply(op);
    for (const op of planEvent({ type: "event", kind: "workChanged", workId: other })) sync.apply(op);
    expect(spy).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1000);
    expect(spy).toHaveBeenCalledTimes(1);
  });

  it("invalidates only the matching queries", () => {
    queryClient.setQueryData(["get", apiPaths.work, { params: { path: { id: work } } }], { id: work });
    queryClient.setQueryData(["get", apiPaths.work, { params: { path: { id: other } } }], { id: other });
    const sync = createCacheSync(queryClient);
    for (const op of planEvent({ type: "event", kind: "workChanged", workId: work })) sync.apply(op);
    vi.advanceTimersByTime(1000);
    const state = (id: string) => queryClient.getQueryState(["get", apiPaths.work, { params: { path: { id } } }]);
    expect(state(work)?.isInvalidated).toBe(true);
    expect(state(other)?.isInvalidated).toBe(false);
  });

  it("updates the problem count in place", () => {
    queryClient.setQueryData(liveQueries.snapshot().queryKey, snapshot());
    createCacheSync(queryClient).apply({ kind: "updateProblemCount", problemCount: 9 });
    expect(queryClient.getQueryData(liveQueries.snapshot().queryKey)?.problemCount).toBe(9);
  });

  it("records a play's news in place", () => {
    createCacheSync(queryClient).apply({ kind: "updatePlay", playId: work, ended: true });
    expect(queryClient.getQueryData(liveQueries.play(work).queryKey)).toEqual({ ended: true });
    expect(queryClient.getQueryData(liveQueries.play(other).queryKey)).toBeUndefined();
  });

  it("refetches everything but the snapshot on refetchAll, dropping pending batches", () => {
    queryClient.setQueryData(["get", apiPaths.home], { rows: [] });
    queryClient.setQueryData(liveQueries.snapshot().queryKey, snapshot());
    const sync = createCacheSync(queryClient);
    sync.apply({ kind: "invalidate", path: apiPaths.search });
    sync.apply({ kind: "refetchAll" });
    expect(queryClient.getQueryState(["get", apiPaths.home])?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(liveQueries.snapshot().queryKey)?.isInvalidated).toBe(false);
  });
});

/** A WebSocket that tests drive by hand. */
class FakeSocket {
  static all: FakeSocket[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: unknown }) => void) | null = null;
  onclose: ((e: { code: number }) => void) | null = null;
  closed = false;
  readonly url: string;
  constructor(url: string) {
    this.url = url;
    FakeSocket.all.push(this);
  }
  close() {
    this.closed = true;
  }
  receive(frame: object) {
    this.onmessage?.({ data: JSON.stringify(frame) });
  }
}

describe("connectLiveEvents", () => {
  let visibility: DocumentVisibilityState = "visible";

  beforeEach(() => {
    vi.useFakeTimers();
    FakeSocket.all = [];
    visibility = "visible";
    vi.stubGlobal("WebSocket", FakeSocket);
    vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  const connect = () => {
    const messages: [string, boolean][] = [];
    const connection = connectLiveEvents({
      onMessage: (m, reconnect) => messages.push([m.type, reconnect]),
      onStatus: () => {},
    });
    const first = FakeSocket.all[0] as FakeSocket;
    first.onopen?.();
    first.receive(snapshot());
    return { connection, messages, latest: () => FakeSocket.all.at(-1) as FakeSocket };
  };

  const setVisibility = (state: DocumentVisibilityState) => {
    visibility = state;
    document.dispatchEvent(new Event("visibilitychange"));
  };

  it("reconnects on coming back after a long time hidden, and the new snapshot refetches", () => {
    const { connection, messages, latest } = connect();
    setVisibility("hidden");
    vi.advanceTimersByTime(staleAfterHiddenMs + 1000);
    setVisibility("visible");
    expect(FakeSocket.all).toHaveLength(2);
    expect(FakeSocket.all[0]?.closed).toBe(true);
    latest().receive(snapshot());
    expect(messages).toEqual([
      ["snapshot", false],
      ["snapshot", true],
    ]);
    connection.close();
  });

  it("keeps the socket after a short time hidden", () => {
    const { connection } = connect();
    setVisibility("hidden");
    vi.advanceTimersByTime(5000);
    setVisibility("visible");
    expect(FakeSocket.all).toHaveLength(1);
    connection.close();
  });

  it("takes a socket that hears nothing, not even a keep-alive, for dead", () => {
    const { connection, latest } = connect();
    for (let i = 0; i < 6; i++) {
      vi.advanceTimersByTime(25_000);
      latest().receive({ type: "keepAlive" });
    }
    expect(FakeSocket.all).toHaveLength(1);
    vi.advanceTimersByTime(silenceMs + 5000);
    expect(FakeSocket.all).toHaveLength(2);
    expect(FakeSocket.all[0]?.closed).toBe(true);
    connection.close();
  });

  it("reconnects when the network comes back", () => {
    const { connection } = connect();
    window.dispatchEvent(new Event("online"));
    expect(FakeSocket.all).toHaveLength(2);
    connection.close();
  });

  it("keeps saying it's retrying through each attempt, until a socket opens", () => {
    const states: string[] = [];
    const connection = connectLiveEvents({ onMessage: () => {}, onStatus: (s) => states.push(s.state) });
    FakeSocket.all[0]?.onclose?.({ code: 1006 });
    vi.advanceTimersByTime(1000);
    FakeSocket.all[1]?.onclose?.({ code: 1006 });
    vi.advanceTimersByTime(2000);
    FakeSocket.all[2]?.onopen?.();
    expect(states).toEqual(["connecting", "retrying", "retrying", "open"]);
    connection.close();
  });

  it("replaces an attempt still connecting when asked to retry now, as one may hang", () => {
    const connection = connectLiveEvents({ onMessage: () => {}, onStatus: () => {} });
    FakeSocket.all[0]?.onclose?.({ code: 1006 });
    vi.advanceTimersByTime(1000);
    expect(FakeSocket.all).toHaveLength(2);
    connection.retryNow();
    expect(FakeSocket.all).toHaveLength(3);
    expect(FakeSocket.all[1]?.closed).toBe(true);
    connection.close();
  });

  it("leaves an open socket be when asked to retry now", () => {
    const { connection } = connect();
    connection.retryNow();
    expect(FakeSocket.all).toHaveLength(1);
    connection.close();
  });

  it("stops watching once closed", () => {
    const { connection } = connect();
    connection.close();
    window.dispatchEvent(new Event("online"));
    vi.advanceTimersByTime(silenceMs * 2);
    expect(FakeSocket.all).toHaveLength(1);
  });
});
