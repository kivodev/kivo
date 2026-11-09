import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { invalidatePaths, listViewPaths } from "./paths";
import type { WorkListList } from "./schemas";

/** Skips the list kinds this client doesn't know. */
const knownKinds = (page: WorkListList): WorkListList => ({
  items: page.items.filter((list) => list.kind === "manual" || list.kind === "smart"),
});

export const listQueries = {
  /** The user's lists, My List first, then by name. */
  all: () => $api.queryOptions("get", "/api/v1/lists", undefined, { select: knownKinds }),
  /** The same with each smart list's count, which runs its rules: for the Lists screen alone. */
  counted: () =>
    $api.queryOptions("get", "/api/v1/lists", { params: { query: { counts: true } } }, { select: knownKinds }),
  detail: (id: string) => $api.queryOptions("get", "/api/v1/lists/{id}", { params: { path: { id } } }),
};

function useListsChanged() {
  const queryClient = useQueryClient();
  return () => invalidatePaths(queryClient, listViewPaths);
}

/** Make a list, manual or smart. A name in use is refused with `nameTaken`. */
export function useCreateList() {
  const onSuccess = useListsChanged();
  return $api.useMutation("post", "/api/v1/lists", { onSuccess });
}

/** Change a list's name, or a smart list's query. A name in use is refused with `nameTaken`. */
export function useUpdateList() {
  const onSuccess = useListsChanged();
  return $api.useMutation("patch", "/api/v1/lists/{id}", { onSuccess });
}

/** Delete a list, leaving its works. */
export function useDeleteList() {
  const onSuccess = useListsChanged();
  return $api.useMutation("delete", "/api/v1/lists/{id}", { onSuccess });
}

type Membership = { listId: string; workId: string; inList: boolean };

/** Puts a work on a list or takes it off. */
export function useSetInList() {
  const onSuccess = useListsChanged();
  const add = $api.useMutation("post", "/api/v1/lists/{id}/works", { onSuccess });
  const remove = $api.useMutation("delete", "/api/v1/lists/{id}/works/{workId}", { onSuccess });
  return {
    error: add.error ?? remove.error,
    mutate: ({ listId, workId, inList }: Membership) =>
      inList
        ? add.mutate({ params: { path: { id: listId } }, body: { workId } })
        : remove.mutate({ params: { path: { id: listId, workId } } }),
  };
}
