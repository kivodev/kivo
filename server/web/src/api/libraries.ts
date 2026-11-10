import { useQuery, useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { type ApiPath, apiPaths, invalidatePaths, libraryGridPaths, pathKey } from "./paths";
import type { AdminLibrary, Library } from "./schemas";

export const libraryQueries = {
  /** Libraries you may see, in your own order, each marked whether you pinned it. */
  list: () => $api.queryOptions("get", "/api/v1/libraries"),
  /** The libraries as an admin gets them, with their settings. */
  admin: () => ({
    ...libraryQueries.list(),
    select: (page: { items: Library[] }) => page.items as AdminLibrary[],
  }),
  detail: (id: string) => $api.queryOptions("get", "/api/v1/libraries/{id}", { params: { path: { id } } }),
};

/**
 * A library. It shows at once from the list of libraries while its own
 * read is on its way.
 */
export function useLibrary(id: string) {
  const queryClient = useQueryClient();
  return useQuery({
    ...libraryQueries.detail(id),
    placeholderData: () =>
      queryClient.getQueryData<{ items: Library[] }>(libraryQueries.list().queryKey)?.items.find((l) => l.id === id),
  });
}

/**
 * Refetches what a library action changes. That is the libraries, their
 * filters, Home, the grids and the tasks.
 */
function useInvalidateLibraries() {
  const queryClient = useQueryClient();
  return () =>
    invalidatePaths(queryClient, [
      apiPaths.libraries,
      apiPaths.library,
      apiPaths.workFieldValues,
      apiPaths.home,
      ...libraryGridPaths,
      apiPaths.tasks,
    ]);
}

/**
 * Save the order of your libraries and which ones are pinned. The order
 * follows you wherever you sign in. The library list changes at once, so
 * the sidebar follows and a drag stays where it was dropped. The list
 * refetches if the server refused it. `retry` sends a refused order again.
 */
export function useSaveLibraries() {
  const queryClient = useQueryClient();
  const refetch = (path: ApiPath) => queryClient.invalidateQueries({ queryKey: pathKey(path) });
  const mutation = $api.useMutation("patch", "/api/v1/users/me", {
    onError: () => refetch(apiPaths.libraries),
  });
  const save = (libraries: Library[]) => {
    queryClient.setQueryData(libraryQueries.list().queryKey, (page) => page && { ...page, items: libraries });
    mutation.mutate({ body: { libraries: libraries.map(({ id, pinned }) => ({ id, pinned })) } });
  };
  // Puts the refused order back on the list of libraries and sends it again.
  const retry = (libraries: Library[]) => {
    const wanted = mutation.variables?.body.libraries;
    if (!wanted) return;
    save(
      wanted.flatMap(({ id, pinned }) => {
        const library = libraries.find((l) => l.id === id);
        return library ? [{ ...library, pinned }] : [];
      }),
    );
  };
  return { ...mutation, save, retry };
}

/** Add a library. The server queues its first scan. */
export function useCreateLibrary() {
  const onSuccess = useInvalidateLibraries();
  return $api.useMutation("post", "/api/v1/libraries", { onSuccess });
}

/** Edit a library's name, folders and options. An added folder is scanned. */
export function useUpdateLibrary() {
  const onSuccess = useInvalidateLibraries();
  return $api.useMutation("patch", "/api/v1/libraries/{id}", { onSuccess });
}

/**
 * Delete a library. It leaves the list at once, and the Library changes
 * task forgets its folders and files.
 */
export function useDeleteLibrary() {
  const onSuccess = useInvalidateLibraries();
  return $api.useMutation("delete", "/api/v1/libraries/{id}", { onSuccess });
}

/**
 * Scan every library now. This runs the scan-libraries task, which is the
 * scheduled scan of every folder.
 */
export function useScanAllLibraries() {
  const onSuccess = useInvalidateLibraries();
  return $api.useMutation("post", "/api/v1/tasks/{task}/run", { onSuccess });
}

/**
 * Scan files now. Progress arrives in each library's scan, which
 * refetches on tasksChanged events.
 */
export function useScanLibrary() {
  const onSuccess = useInvalidateLibraries();
  return $api.useMutation("post", "/api/v1/libraries/{id}/scan", { onSuccess });
}

/** Stop the library's scan. */
export function useStopLibraryScan() {
  const onSuccess = useInvalidateLibraries();
  return $api.useMutation("post", "/api/v1/libraries/{id}/stop-scan", { onSuccess });
}

/**
 * Refresh all metadata. This runs one library task that refreshes every
 * item, as each item's own Refresh metadata would.
 */
export function useRefreshLibraryMetadata() {
  const onSuccess = useInvalidateLibraries();
  return $api.useMutation("post", "/api/v1/libraries/{id}/refresh-metadata", { onSuccess });
}
