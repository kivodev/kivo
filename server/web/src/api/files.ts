import { useQueryClient } from "@tanstack/react-query";
import { $api, client } from "./client";
import { keysetPages } from "./pages";
import { apiPaths, invalidatePaths, pathKey } from "./paths";

const pageSize = 100;

export const fileQueries = {
  /** Every version and part of a work, with paths, sizes and tracks (Show files). */
  forWork: (workId: string) =>
    $api.queryOptions("get", "/api/v1/works/{id}/files", { params: { path: { id: workId } } }),

  /** How many of a library's files are missing, read as the first page's total. */
  missingCount: (libraryId: string) =>
    $api.queryOptions(
      "get",
      "/api/v1/files",
      { params: { query: { status: "gone", libraryId, limit: 1 } } },
      { select: (page) => page.total ?? page.items.length },
    ),

  /** The Missing files list, newest first, one keyset page at a time. */
  missing: () => {
    const query = { status: "gone" as const, limit: pageSize };
    return keysetPages(["get", apiPaths.files, { params: { query } }, "pages"] as const, async (after, signal) => {
      const { data, error } = await client.GET("/api/v1/files", {
        signal,
        params: { query: { ...query, after } },
      });
      if (error) throw error;
      return data;
    });
  },
};

export const subtitleQueries = {
  /**
   * OpenSubtitles' subtitles for a file, best first, in the user's subtitle
   * language. A failure is shown to the user and not retried.
   */
  results: (fileId: string) => ({
    ...$api.queryOptions("get", "/api/v1/files/{id}/subtitle-results", { params: { path: { id: fileId } } }),
    retry: false,
  }),
};

/** Add a subtitle from OpenSubtitles. The new track belongs to the file from then on. */
export function useAddFileSubtitle() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/files/{id}/subtitles", {
    onSuccess: () => invalidatePaths(queryClient, [apiPaths.work, apiPaths.workFiles]),
  });
}

/** Delete every missing file at once rather than after 30 days. */
export function useDeleteMissingFiles() {
  const queryClient = useQueryClient();
  return $api.useMutation("delete", "/api/v1/files", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.files) }),
  });
}

/** Delete a missing file at once rather than after 30 days. */
export function useDeleteFile() {
  const queryClient = useQueryClient();
  return $api.useMutation("delete", "/api/v1/files/{id}", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.files) }),
  });
}
