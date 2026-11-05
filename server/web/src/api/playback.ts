import { queryOptions, useQueryClient } from "@tanstack/react-query";
import { $api, apiBaseUrl, client } from "./client";
import { apiPaths, pathKey } from "./paths";
import type { PlayCreate, PlayFallback, PlayStarted, PlayUpdate, PreviewManifest } from "./schemas";

// Plays. The player drives these imperatively, from media events and timers
// rather than from renders. So they are plain calls on the shared client
// instead of mutation hooks. Each throws the server's problem document, as
// query functions do.

/** Start playback. The server plays the version named, or the work's default one, and decides each track. */
export async function startPlay(body: PlayCreate): Promise<PlayStarted> {
  const { data, error } = await client.POST("/api/v1/plays", { body });
  if (error) throw error;
  return data;
}

/** The heartbeat. It reports the position, which also keeps the play alive. */
export async function updatePlay(id: string, body: PlayUpdate): Promise<void> {
  const { error } = await client.PATCH("/api/v1/plays/{id}", { params: { path: { id } }, body });
  if (error) throw error;
}

/**
 * Save the position and end the play. `keepalive` lets the request
 * outlive the page, for a stop that is sent as the tab closes. A
 * `failure` says the play ends because playback failed with nothing left
 * to try, for the server's log.
 */
export async function stopPlay(
  id: string,
  positionMs: number,
  { keepalive = false, failure }: { keepalive?: boolean; failure?: Pick<PlayFallback, "code" | "detail"> } = {},
): Promise<void> {
  const { error } = await client.POST("/api/v1/plays/{id}/stop", {
    params: { path: { id } },
    body: { positionMs: Math.round(positionMs), ...failure },
    keepalive,
  });
  if (error) throw error;
}

/**
 * Report a playback failure. The server ends the play and returns a new
 * play that transcodes everything.
 */
export async function fallbackPlay(id: string, body: PlayFallback): Promise<PlayStarted> {
  const { data, error } = await client.POST("/api/v1/plays/{id}/fallback", {
    params: { path: { id } },
    body,
  });
  if (error) throw error;
  return data;
}

/**
 * Fetches `size` bytes as a stream, to time a download. It is undefined
 * when the server counts this client as local, because a local client
 * needs no measurement.
 */
export async function bandwidthTest(
  size: number,
  signal: AbortSignal,
): Promise<ReadableStream<Uint8Array> | undefined> {
  const { data, error } = await client.GET("/api/v1/bandwidth-test", {
    params: { query: { size } },
    parseAs: "stream",
    signal,
    cache: "no-store",
  });
  if (error) throw error;
  return data ?? undefined;
}

/**
 * The absolute URL of a play's media. The play's URLs are relative to the
 * API root and carry the play ID, which is a credential. They go to the
 * media element and nowhere else.
 */
export function mediaUrl(relative: string): string {
  return new URL(relative, `${apiBaseUrl()}/api/v1/`).href;
}

/** The subtitle choice that shows no subtitles, as play requests and the URL write it. */
export const subtitlesOff = "off";

/** The last path segment of a media URL, which is the opaque name the server gave it. */
function mediaName(relative: string): string {
  return relative.slice(relative.lastIndexOf("/") + 1);
}

/** Stop anyone's play from Now playing. The play keeps the position its owner last reported. */
export function useStopPlay() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/plays/{id}/stop", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.plays) }),
  });
}

export const playQueries = {
  /** Every user's active plays with their decisions, kept current by playsChanged events. */
  active: () => $api.queryOptions("get", "/api/v1/plays"),
};

export const acceleratorQueries = {
  /** What the startup accelerator test found: the accelerators that passed, then those that can't be used. */
  list: () => $api.queryOptions("get", "/api/v1/accelerators"),
};

export const playbackQueries = {
  /**
   * A play's scrub-preview manifest, from `PlayStarted.previews.url`.
   * Its sheets' URLs are relative to `mediaUrl(manifestUrl)`.
   */
  previews: (playId: string, manifestUrl: string) => {
    const params = { path: { id: playId, name: mediaName(manifestUrl) } };
    return queryOptions({
      queryKey: ["get", "/api/v1/plays/{id}/previews/{name}", { params }] as const,
      queryFn: async ({ signal }): Promise<PreviewManifest> => {
        const { data, error } = await client.GET("/api/v1/plays/{id}/previews/{name}", { signal, params });
        if (error) throw error;
        // The route also serves the sheets, so its type includes their bytes.
        if (typeof data !== "object") throw new Error("previews: the manifest isn't JSON");
        return data;
      },
      staleTime: Number.POSITIVE_INFINITY,
    });
  },
};
