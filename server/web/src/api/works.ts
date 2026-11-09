import { type QueryClient, type QueryKey, queryOptions, useQueryClient } from "@tanstack/react-query";
import { $api, client } from "./client";
import { keysetPages } from "./pages";
import { type ApiPath, apiPaths, invalidatePaths, pathKey, workViewPaths } from "./paths";
import { retryPageDelay, shouldRetryPage } from "./problem";
import type { paths } from "./schema";
import type { ArtworkType, BrowseQuery, UserState, WorkDetail } from "./schemas";

/** A library grid's sort and filters: the listLibraryWorks query without the paging. */
export type BrowseFilters = Omit<BrowseQuery, "offset" | "limit">;

/** A library grid's page size. Page i holds the items from i * gridPageSize. */
export const gridPageSize = 100;

/**
 * The pages after the first that hold a grid's items from start up to
 * end. No page goes past the last of total.
 */
export function gridPages(start: number, end: number, total: number): number[] {
  const pages: number[] = [];
  for (let i = Math.max(Math.floor(start / gridPageSize), 1); i * gridPageSize < Math.min(end, total); i++)
    pages.push(i);
  return pages;
}

/**
 * Page `index` of a grid, read by offset so the grid can load any
 * position. Every page carries the total. `page` fetches the page from
 * `offset`, which is undefined for the first page. The key extends
 * `pathKey`, so invalidating the path reaches it too.
 */
function gridPage<Page>(
  pathKey: QueryKey,
  index: number,
  page: (offset: number | undefined, signal: AbortSignal) => Promise<Page>,
) {
  const offset = index * gridPageSize;
  const queryKey: QueryKey = [...pathKey, "page", index];
  return queryOptions({
    queryKey,
    queryFn: ({ signal }) => page(offset > 0 ? offset : undefined, signal),
    // A page past the first keeps retrying, because its cells have no error
    // state of their own.
    ...(index > 0 ? { retry: shouldRetryPage, retryDelay: retryPageDelay } : {}),
  });
}

/** The query of one page of a grid. */
export type GridPageQuery<Page> = ReturnType<typeof gridPage<Page>>;

/** A library's collections, authors and book series. These grids are sorted by name and have no sort or filters. */
type GroupPath =
  | "/api/v1/libraries/{id}/collections"
  | "/api/v1/libraries/{id}/authors"
  | "/api/v1/libraries/{id}/book-series";

type GroupPage<P extends GroupPath> = paths[P]["get"]["responses"][200]["content"]["application/json"];

/** Page `index` of one of a library's grids by name. */
function groupPage<P extends GroupPath>(path: P, libraryId: string, index: number) {
  const init = { params: { path: { id: libraryId } } };
  return gridPage(["get", path, init], index, async (offset, signal) => {
    const { data, error } = await client.GET(path as GroupPath, {
      signal,
      params: { path: { id: libraryId }, query: { limit: gridPageSize, offset } },
    });
    if (error) throw error;
    return data as GroupPage<P>;
  });
}

/** The most cards a list's row shows. */
const listRowSize = 20;

export const workQueries = {
  detail: (id: string) => $api.queryOptions("get", "/api/v1/works/{id}", { params: { path: { id } } }),

  /** All of a series' seasons or a season's episodes, in order, in one response. */
  children: (id: string) => $api.queryOptions("get", "/api/v1/works/{id}/children", { params: { path: { id } } }),

  /** A collection and all of its root works, in release order. */
  collection: (id: string) => $api.queryOptions("get", "/api/v1/collections/{id}", { params: { path: { id } } }),

  /** One page of a library's grid. Page 0 of the title sort carries the letter rail's letters. */
  browsePage: (libraryId: string, filters: BrowseFilters, index: number) => {
    const init = { params: { path: { id: libraryId }, query: { ...filters, limit: gridPageSize } } };
    return gridPage(["get", apiPaths.libraryWorks, init], index, async (offset, signal) => {
      const { data, error } = await client.GET("/api/v1/libraries/{id}/works", {
        signal,
        params: { ...init.params, query: { ...init.params.query, offset } },
      });
      if (error) throw error;
      return data;
    });
  },

  /** One page of a library's collections, by name. The first page's total says whether there are any. */
  collectionsPage: (libraryId: string, index: number) =>
    groupPage("/api/v1/libraries/{id}/collections", libraryId, index),

  /** One page of an audiobooks library's authors, by name. Page 0 carries the letters. */
  authorsPage: (libraryId: string, index: number) => groupPage("/api/v1/libraries/{id}/authors", libraryId, index),

  /** One page of an audiobooks library's book series, by name. Page 0 carries the letters. */
  bookSeriesPage: (libraryId: string, index: number) =>
    groupPage("/api/v1/libraries/{id}/book-series", libraryId, index),

  /** The first cards of a list for its row, most recently added first. */
  listRow: (listId: string) =>
    $api.queryOptions("get", "/api/v1/lists/{id}/works", {
      params: { path: { id: listId }, query: { limit: listRowSize } },
    }),

  /** One page of a list's grid, most recently added first. */
  listPage: (listId: string, index: number) =>
    gridPage(["get", apiPaths.listWorks, { params: { path: { id: listId } } }], index, async (offset, signal) => {
      const { data, error } = await client.GET("/api/v1/lists/{id}/works", {
        signal,
        params: { path: { id: listId }, query: { limit: gridPageSize, offset } },
      });
      if (error) throw error;
      return data;
    }),

  /** The Unmatched list for every library, library by library, one keyset page at a time. */
  unmatched: () => {
    const query = { limit: gridPageSize };
    return keysetPages(
      ["get", apiPaths.unmatchedWorks, { params: { query } }, "pages"] as const,
      async (after, signal) => {
        const { data, error } = await client.GET("/api/v1/unmatched-works", {
          signal,
          params: { query: { ...query, after } },
        });
        if (error) throw error;
        return data;
      },
    );
  },

  /**
   * The content ratings of a country. Parental controls use the server's
   * country. A work's edit dialog uses its library's country.
   */
  contentRatings: (workId?: string) =>
    $api.queryOptions("get", "/api/v1/content-ratings", { params: { query: { workId } } }),

  /**
   * The provider search for Fix match, best first. For a book, this is
   * Match's search of Audible. Without a query, the server searches the
   * work's parsed title and year, or a book's title and author.
   */
  matches: (id: string, search: { query?: string; year?: number; author?: string } = {}) =>
    $api.queryOptions("get", "/api/v1/works/{id}/matches", { params: { path: { id }, query: search } }),

  /** The images of one type for Choose artwork: the current one first, then local files, then TMDb's. */
  artwork: (id: string, type: ArtworkType) =>
    $api.queryOptions("get", "/api/v1/works/{id}/artwork", { params: { path: { id }, query: { type } } }),
};

/**
 * Applies an admin's change to a work. Its detail page takes the server's
 * answer, and every view that shows the work refetches, along with the
 * libraries' counts.
 */
function useWorkChanged() {
  const queryClient = useQueryClient();
  return (work: WorkDetail) => {
    queryClient.setQueryData(workQueries.detail(work.id).queryKey, work);
    void invalidatePaths(queryClient, [...workViewPaths, apiPaths.libraries]);
  };
}

/**
 * Fix match. It reassigns the work's versions to a provider's match, and
 * the answer may be another work. For a book this is Match. The match's
 * ASIN becomes the book's, and its details follow in a refresh.
 */
export function useMatchWork() {
  const onSuccess = useWorkChanged();
  return $api.useMutation("post", "/api/v1/works/{id}/match", { onSuccess });
}

/** Unmatch a movie or series. It keeps its watch state, and stays unmatched until Fix match. */
export function useUnmatchWork() {
  const onSuccess = useWorkChanged();
  return $api.useMutation("post", "/api/v1/works/{id}/unmatch", { onSuccess });
}

/** Edit fields. Each field sent is locked so refreshes keep it. */
export function useUpdateWork() {
  const onSuccess = useWorkChanged();
  return $api.useMutation("patch", "/api/v1/works/{id}", { onSuccess });
}

/** Refetches the work, its cards and the picker after the artwork changes. */
function useArtworkChanged() {
  const queryClient = useQueryClient();
  return () => void invalidatePaths(queryClient, [...workViewPaths, apiPaths.workArtwork]);
}

/** Make a listed image the work's artwork of its type. */
export function useSelectArtwork() {
  const onSuccess = useArtworkChanged();
  return $api.useMutation("post", "/api/v1/works/{id}/artwork/{artworkId}/select", { onSuccess });
}

/** Upload an image as the work's artwork of a type. */
export function useUploadArtwork() {
  const onSuccess = useArtworkChanged();
  const mutation = $api.useMutation("post", "/api/v1/works/{id}/artwork", { onSuccess });
  return {
    ...mutation,
    upload: (id: string, type: ArtworkType, file: File) =>
      mutation.mutate({
        params: { path: { id } },
        // The generated body types the file as a string; the form carries the File.
        body: { type, file: file.name },
        bodySerializer: () => {
          const form = new FormData();
          form.append("type", type);
          form.append("file", file);
          return form;
        },
      }),
  };
}

/** Refresh metadata. The work changes when the refresh finishes. */
export function useRefreshWorkMetadata() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/works/{id}/refresh-metadata", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.tasks) }),
  });
}

/** Mark played or unplayed, for you. */
export function useUpdateUserState() {
  const queryClient = useQueryClient();
  return $api.useMutation("patch", "/api/v1/works/{id}/user-state", {
    // Cards everywhere show the state, so it changes in place. A series or
    // season cascades to its episodes, whose views refetch.
    onSuccess: (state, { params }) => applyUserState(queryClient, params.path.id, state),
  });
}

/** Take a work out of Continue Watching or Continue Listening, for you. */
export function useRemoveFromContinue() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/works/{id}/user-state/remove-from-continue", {
    onSuccess: () => invalidatePaths(queryClient, [apiPaths.home]),
  });
}

/**
 * Replaces a work's user state wherever the cache holds it, on its detail
 * page and its card in every list. Returns the kinds of the copies it
 * found.
 */
function replaceUserState(data: unknown, workId: string, userState: UserState, kinds: Set<string>): unknown {
  if (Array.isArray(data)) return data.map((x) => replaceUserState(x, workId, userState, kinds));
  if (typeof data !== "object" || data === null) return data;
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(data)) out[k] = replaceUserState(v, workId, userState, kinds);
  if (out.id === workId && typeof out.kind === "string") {
    kinds.add(out.kind);
    out.userState = userState;
  }
  return out;
}

/**
 * Applies a work's new user state to the cache in place. Marking a series or
 * season also marks its episodes, whose views then refetch. Otherwise Home
 * refetches, because its rows follow what was watched.
 */
export function applyUserState(queryClient: QueryClient, workId: string, userState: UserState) {
  const kinds = new Set<string>();
  for (const query of queryClient.getQueryCache().getAll()) {
    const [method, path] = query.queryKey;
    if (method !== "get" || !workViewPaths.includes(path as ApiPath) || query.state.data === undefined) continue;
    queryClient.setQueryData(query.queryKey, replaceUserState(query.state.data, workId, userState, kinds));
  }
  const cascades = kinds.has("series") || kinds.has("season");
  void invalidatePaths(queryClient, cascades ? workViewPaths : [apiPaths.home]);
}
