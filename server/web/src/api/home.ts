import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { type ApiPath, apiPaths, pathKey } from "./paths";
import type { HomeRow, HomeRowInfo } from "./schemas";

/** The row kinds this client shows; it skips kinds it doesn't know. */
export const homeRowKinds = new Set<HomeRow["kind"]>([
  "continueWatching",
  "continueListening",
  "recentlyAdded",
  "list",
]);

export const homeQueries = {
  rows: () => $api.queryOptions("get", "/api/v1/home"),
  layout: () => $api.queryOptions("get", "/api/v1/home/rows"),
};

/**
 * Save Home's row order and which rows show, given every row of the layout,
 * including kinds this client doesn't show. The layout changes at once, so a
 * drag stays where it was dropped, and Home and the layout refetch once the
 * server has answered. `retry` sends a refused layout again.
 */
export function useSaveHomeRows() {
  const queryClient = useQueryClient();
  const refetch = (path: ApiPath) => queryClient.invalidateQueries({ queryKey: pathKey(path) });
  const mutation = $api.useMutation("patch", "/api/v1/home/rows", {
    onMutate: async ({ body }) => {
      await queryClient.cancelQueries({ queryKey: pathKey(apiPaths.homeRows) });
      const key = homeQueries.layout().queryKey;
      const page = queryClient.getQueryData<{ items: HomeRowInfo[] }>(key);
      if (!page) return;
      const byId = new Map(page.items.map((r) => [r.id, r]));
      queryClient.setQueryData(key, {
        items: body.items.flatMap(({ id, shown }) => {
          const row = byId.get(id);
          return row ? [{ ...row, shown }] : [];
        }),
      });
    },
    onSettled: () => Promise.all([refetch(apiPaths.home), refetch(apiPaths.homeRows)]),
  });
  const save = (rows: HomeRowInfo[], onSuccess?: () => void) =>
    mutation.mutate({ body: { items: rows.map(({ id, shown }) => ({ id, shown })) } }, { onSuccess });
  const retry = (onSuccess?: () => void) => {
    const wanted = mutation.variables;
    if (wanted) mutation.mutate(wanted, { onSuccess });
  };
  return { ...mutation, save, retry };
}

/** Forget Home's order and hidden rows. */
export function useResetHomeRows() {
  const queryClient = useQueryClient();
  const refetch = (path: ApiPath) => queryClient.invalidateQueries({ queryKey: pathKey(path) });
  return $api.useMutation("post", "/api/v1/home/rows/reset", {
    onMutate: () => queryClient.cancelQueries({ queryKey: pathKey(apiPaths.homeRows) }),
    onSuccess: (data) => {
      queryClient.setQueryData(homeQueries.layout().queryKey, data);
    },
    onSettled: () => Promise.all([refetch(apiPaths.home), refetch(apiPaths.homeRows)]),
  });
}
