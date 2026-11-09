import { useMutation, useQueryClient } from "@tanstack/react-query";
import { $api, client } from "./client";
import { invalidatePaths, listViewPaths } from "./paths";
import type { WorkDetail, WorkListList } from "./schemas";
import { workQueries } from "./works";

/** Skips the list kinds this client doesn't know. */
const knownKinds = (page: WorkListList): WorkListList => ({
  items: page.items.filter((list) => list.kind === "manual"),
});

export const listQueries = {
  /** The user's lists, My List first, then by name. */
  all: () => $api.queryOptions("get", "/api/v1/lists", undefined, { select: knownKinds }),
  detail: (id: string) => $api.queryOptions("get", "/api/v1/lists/{id}", { params: { path: { id } } }),
};

function useListsChanged() {
  const queryClient = useQueryClient();
  return () => invalidatePaths(queryClient, listViewPaths);
}

/** Make a list. A name in use is refused with `nameTaken`. */
export function useCreateList() {
  const onSuccess = useListsChanged();
  return $api.useMutation("post", "/api/v1/lists", { onSuccess });
}

/** Rename a list. A name in use is refused with `nameTaken`. */
export function useRenameList() {
  const onSuccess = useListsChanged();
  return $api.useMutation("patch", "/api/v1/lists/{id}", { onSuccess });
}

/** Delete a list, leaving its works. */
export function useDeleteList() {
  const onSuccess = useListsChanged();
  return $api.useMutation("delete", "/api/v1/lists/{id}", { onSuccess });
}

type Membership = { listId: string; workId: string; inList: boolean };

const membershipKey = ["lists", "membership"];

/**
 * Puts a work on a list or takes it off. The work's page changes at once,
 * and saves run one at a time, in order. The last save to settle reads what
 * the server holds, which also undoes a refused change.
 */
export function useSetInList() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: membershipKey,
    scope: { id: "list-membership" },
    mutationFn: async ({ listId, workId, inList }: Membership) => {
      const { error } = inList
        ? await client.POST("/api/v1/lists/{id}/works", { params: { path: { id: listId } }, body: { workId } })
        : await client.DELETE("/api/v1/lists/{id}/works/{workId}", { params: { path: { id: listId, workId } } });
      if (error) throw error;
    },
    onMutate: async ({ listId, workId, inList }) => {
      const { queryKey } = workQueries.detail(workId);
      await queryClient.cancelQueries({ queryKey });
      queryClient.setQueryData<WorkDetail>(queryKey, (work) => {
        if (!work) return work;
        const others = (work.listIds ?? []).filter((id) => id !== listId);
        return { ...work, listIds: inList ? [...others, listId] : others };
      });
    },
    onSettled: () =>
      queryClient.isMutating({ mutationKey: membershipKey }) <= 1
        ? invalidatePaths(queryClient, listViewPaths)
        : undefined,
  });
}
