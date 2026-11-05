import { queryOptions, useQuery, useQueryClient } from "@tanstack/react-query";
import { $api, client } from "./client";
import { gatewayDown } from "./problem";
import type { QualityRung } from "./schemas";

// The server's own information, which doesn't change while the app runs.
export const serverQueries = {
  info: () => $api.queryOptions("get", "/api/v1/server", undefined, { staleTime: Number.POSITIVE_INFINITY }),
  /**
   * The packaged app's probe of its kept server, once each time it starts,
   * so a server below the floor is found before anything else is asked of
   * it. Only a Kivo server's answer is kept: an
   * address that didn't answer, or answered as something else, is asked
   * again, and a server that has come back has its version checked then.
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
 * with its ID and version; something that isn't one, such as a router's
 * admin page on a reused address; or no answer in time.
 */
export type ServerProbe = { result: "kivo"; id: string; version: string } | { result: "other" } | { result: "none" };

/**
 * Asks the server at a base URL, such as "https://nas:8484/kivo", what it
 * is, by its signed-out GET /api/v1/server. The packaged app's address
 * screen asks before it keeps the address, so the request names its base
 * URL rather than the kept one. The answer is read as text, because a web
 * page answering with HTML is something other than Kivo, not a failure to
 * connect. A gateway saying the server is down counts as no answer, as a
 * request that got none or ran out of time does.
 */
export async function probeServer(baseUrl: string): Promise<ServerProbe> {
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
  const { id, name, version } = info as Record<string, unknown>;
  if (typeof id !== "string" || typeof name !== "string" || typeof version !== "string") return { result: "other" };
  return { result: "kivo", id, version };
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
