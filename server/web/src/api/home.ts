import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { type ApiPath, apiPaths, pathKey } from "./paths";
import type { HomeRow, HomeRowInfo } from "./schemas";

/** The row kinds this client shows; it skips kinds it doesn't know. */
export const homeRowKinds = new Set<HomeRow["kind"]>([
  "continueWatching",
  "continueListening",
  "recentlyAdded",
  "favorites",
]);

export const homeQueries = {
  rows: () => $api.queryOptions("get", "/api/v1/home"),
  layout: () => $api.queryOptions("get", "/api/v1/home/rows"),
};

/**
 * Save Home's row order and which rows show. The layout changes at once, so a
 * drag stays where it was dropped, and Home refetches once the server has
 * saved it. The layout refetches too if the server refused it. `retry` sends
 * a refused layout again.
 */
export function useSaveHomeRows() {
  const queryClient = useQueryClient();
  const refetch = (path: ApiPath) => queryClient.invalidateQueries({ queryKey: pathKey(path) });
  const mutation = $api.useMutation("patch", "/api/v1/home/rows", {
    onSuccess: () => refetch(apiPaths.home),
    onError: () => refetch(apiPaths.homeRows),
  });
  const save = (rows: HomeRowInfo[]) => {
    queryClient.setQueryData(homeQueries.layout().queryKey, { items: rows });
    mutation.mutate({ body: { items: rows.map(({ id, shown }) => ({ id, shown })) } });
  };
  const retry = (rows: HomeRowInfo[]) => {
    const wanted = mutation.variables?.body.items;
    if (!wanted) return;
    save(
      wanted.flatMap(({ id, shown }) => {
        const row = rows.find((r) => r.id === id);
        return row ? [{ ...row, shown }] : [];
      }),
    );
  };
  return { ...mutation, save, retry };
}

/** Forget Home's order and hidden rows. */
export function useResetHomeRows() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/home/rows/reset", {
    onSuccess: (data) => {
      queryClient.setQueryData(homeQueries.layout().queryKey, data);
      return queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.home) });
    },
  });
}
