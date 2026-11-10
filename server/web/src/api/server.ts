import { type QueryClient, queryOptions, useQuery, useQueryClient } from "@tanstack/react-query";
import type { AnyRouter } from "@tanstack/react-router";
import { type Compatibility, compatibility } from "@/tv/compatibility";
import { packagedServer } from "@/tv/packaged";
import { $api, client } from "./client";
import { gatewayDown } from "./problem";
import type { QualityRung } from "./schemas";

// The server's own information, which doesn't change while the app runs.
export const serverQueries = {
  info: () => $api.queryOptions("get", "/api/v1/server", undefined, { staleTime: Number.POSITIVE_INFINITY }),
  /**
   * The packaged app's probe of its kept server, once each time it starts,
   * so a server the app doesn't work with is found before anything else is
   * asked of it. Only a Kivo server's answer is kept: an address that
   * didn't answer, or answered as something else, is asked again, and a
   * server that has come back has its version checked then.
   */
  probe: (baseUrl: string) =>
    queryOptions({
      queryKey: ["server", "probe", baseUrl],
      queryFn: () => probeServer(baseUrl),
      staleTime: (query) => (query.state.data?.result === "kivo" ? Number.POSITIVE_INFINITY : 0),
    }),
};

const noRungs: QualityRung[] = [];

/** The quality menu's fixed ladder, highest first, as the server serves it. */
export function useQualityLadder(): QualityRung[] {
  return useQuery(serverQueries.info()).data?.qualityLadder ?? noRungs;
}

/**
 * What an address answered the packaged TV app's probe: a Kivo server,
 * with its ID, name and version and the oldest app it supports, when it
 * says; something that isn't one, such as a router's admin page on a
 * reused address; or no answer in time.
 */
export type ServerProbe =
  | { result: "kivo"; id: string; name: string; version: string; minAppVersion?: string }
  | { result: "other" }
  | { result: "none" };

/** How the packaged app and the server a probe found stand: whether they work together. */
export function probeCompatibility(probe: ServerProbe & { result: "kivo" }): Compatibility {
  return compatibility(probe.version, probe.minAppVersion);
}

/**
 * Asks the server at a base URL, such as "https://nas:8484/kivo", what it
 * is, by its signed-out GET /api/v1/server. The packaged app's address
 * screen asks before it keeps the address, so the request names its base
 * URL rather than the kept one. The answer is read as text, because a web
 * page answering with HTML is something other than Kivo, not a failure to
 * connect. A gateway saying the server is down counts as no answer, as a
 * request that got none or ran out of time does.
 */
async function probeServer(baseUrl: string): Promise<ServerProbe> {
  let answer: Awaited<ReturnType<typeof ask>>;
  try {
    answer = await ask(baseUrl);
  } catch {
    return { result: "none" };
  }
  const { data, error, response } = answer;
  if (gatewayDown(response.status, error)) return { result: "none" };
  if (!response.ok || typeof data !== "string") return { result: "other" };
  let info: unknown;
  try {
    info = JSON.parse(data);
  } catch {
    return { result: "other" };
  }
  if (typeof info !== "object" || info === null) return { result: "other" };
  const { id, name, version, minAppVersion } = info as Record<string, unknown>;
  if (typeof id !== "string" || typeof name !== "string" || typeof version !== "string") return { result: "other" };
  return { result: "kivo", id, name, version, ...(typeof minAppVersion === "string" ? { minAppVersion } : {}) };
}

function ask(baseUrl: string) {
  return client.GET("/api/v1/server", { baseUrl, parseAs: "text", cache: "no-store" });
}

/**
 * A probe that always asks afresh, and keeps the answer as the probe the
 * app makes as it starts: the address screen's, so it isn't asked twice,
 * and the check of a server whose version changed while the app ran, so
 * the router's check as each page opens agrees with it.
 */
export function useProbeServer(): (baseUrl: string) => Promise<ServerProbe> {
  const queryClient = useQueryClient();
  return (baseUrl) => queryClient.fetchQuery({ ...serverQueries.probe(baseUrl), staleTime: 0 });
}

/**
 * The packaged app's check of its server as the events socket reconnects,
 * which it does when the server restarts on another version: the server is
 * asked what it is again, and one the app no longer works with sends the
 * app to the screen that says which of them needs an update.
 */
export async function recheckServer(queryClient: QueryClient, router: AnyRouter): Promise<void> {
  const server = packagedServer();
  if (server === undefined) return;
  const probe = await queryClient.fetchQuery({ ...serverQueries.probe(server.address), staleTime: 0 });
  if (probe.result === "kivo" && probeCompatibility(probe).state !== "compatible") {
    await router.navigate({ to: "/update", replace: true });
  }
}

/**
 * Whether the server at a base URL still waits for its first-run setup, or
 * undefined when it doesn't say. Setup signs in with a browser's cookie,
 * which the packaged app can't use, so the app sends such a server back to
 * a browser to be set up.
 */
export async function awaitsSetup(baseUrl: string): Promise<boolean | undefined> {
  try {
    const { data } = await client.GET("/api/v1/setup", { baseUrl, cache: "no-store" });
    return data?.required;
  } catch {
    return undefined;
  }
}
