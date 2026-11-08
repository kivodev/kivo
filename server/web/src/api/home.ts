import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { apiPaths, pathKey } from "./paths";
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

const resetPath = "/api/v1/home/rows/reset";

/** Layout saves run one at a time, in order. */
const layoutScope = { id: "home-layout" };

/** How many layout saves are in flight. A read that lands during one would undo what's on screen. */
export function savingLayout(queryClient: QueryClient): number {
  return (
    queryClient.isMutating({ mutationKey: ["patch", apiPaths.homeRows] }) +
    queryClient.isMutating({ mutationKey: ["post", resetPath] })
  );
}

function useLayoutCache() {
  const queryClient = useQueryClient();
  return {
    queryClient,
    cancelReads: () => queryClient.cancelQueries({ queryKey: pathKey(apiPaths.homeRows) }),
    // The save that's settling still counts, so the last one to settle reads the layout.
    readWhenDone: () =>
      savingLayout(queryClient) <= 1
        ? queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.homeRows) })
        : undefined,
    readHome: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.home) }),
  };
}

/**
 * Save Home's row order and which rows show. The layout changes at once, so a
 * drag stays where it was dropped, and Home refetches once the server has
 * saved it. `retry` sends a refused layout again.
 */
export function useSaveHomeRows() {
  const { queryClient, cancelReads, readWhenDone, readHome } = useLayoutCache();
  const mutation = $api.useMutation("patch", "/api/v1/home/rows", {
    scope: layoutScope,
    onMutate: async ({ body }) => {
      await cancelReads();
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
    onSuccess: readHome,
    onSettled: readWhenDone,
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
  const { queryClient, cancelReads, readWhenDone, readHome } = useLayoutCache();
  return $api.useMutation("post", resetPath, {
    scope: layoutScope,
    onMutate: () => cancelReads(),
    onSuccess: (data) => {
      queryClient.setQueryData(homeQueries.layout().queryKey, data);
      return readHome();
    },
    onSettled: readWhenDone,
  });
}
