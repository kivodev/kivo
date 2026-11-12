import type { QueryClient } from "@tanstack/react-query";
import type { paths } from "./schema";

/**
 * Spec paths the client refers to outside a typed call. These are used for
 * invalidation by path (query keys start with `pathKey`), image URLs and
 * the event socket. They are checked against the generated `paths`, so a
 * renamed route fails to compile.
 */
export const apiPaths = {
  sessions: "/api/v1/sessions",
  libraries: "/api/v1/libraries",
  library: "/api/v1/libraries/{id}",
  libraryCollections: "/api/v1/libraries/{id}/collections",
  libraryAuthors: "/api/v1/libraries/{id}/authors",
  libraryBookSeries: "/api/v1/libraries/{id}/book-series",
  home: "/api/v1/home",
  homeRows: "/api/v1/home/rows",
  lists: "/api/v1/lists",
  list: "/api/v1/lists/{id}",
  listWorks: "/api/v1/lists/{id}/works",
  work: "/api/v1/works/{id}",
  workChildren: "/api/v1/works/{id}/children",
  workFiles: "/api/v1/works/{id}/files",
  workQuery: "/api/v1/works/query",
  workFieldValues: "/api/v1/work-fields/{field}/values",
  unmatchedWorks: "/api/v1/unmatched-works",
  collection: "/api/v1/collections/{id}",
  workArtwork: "/api/v1/works/{id}/artwork",
  search: "/api/v1/search",
  person: "/api/v1/people/{id}",
  bookSeries: "/api/v1/book-series/{id}",
  files: "/api/v1/files",
  tasks: "/api/v1/tasks",
  taskFailures: "/api/v1/tasks/{task}/failures",
  problems: "/api/v1/problems",
  activity: "/api/v1/activity",
  logs: "/api/v1/logs",
  logFile: "/api/v1/logs/file",
  plays: "/api/v1/plays",
  currentUserPlayEvents: "/api/v1/users/me/play-events",
  playEvents: "/api/v1/play-events",
  image: "/api/v1/images/{id}",
  events: "/api/v1/events",
  users: "/api/v1/users",
  user: "/api/v1/users/{id}",
  invites: "/api/v1/invites",
} as const satisfies Record<string, keyof paths>;

export type ApiPath = (typeof apiPaths)[keyof typeof apiPaths];

/**
 * The prefix of every query key that reads a path, for invalidation. A path
 * is read with GET, and a work query with POST.
 */
export function pathKey(path: ApiPath): readonly ["get" | "post", ApiPath] {
  return [path === apiPaths.workQuery ? "post" : "get", path];
}

/** Whether a query reads a path. */
export function readsPath(queryKey: readonly unknown[], path: ApiPath): boolean {
  const [method, keyPath] = pathKey(path);
  return queryKey[0] === method && queryKey[1] === keyPath;
}

/**
 * A library's grids: its works, which a work query in the library reads,
 * and its collections, authors and book series.
 */
export const libraryGridPaths: readonly ApiPath[] = [
  apiPaths.workQuery,
  apiPaths.libraryCollections,
  apiPaths.libraryAuthors,
  apiPaths.libraryBookSeries,
];

/**
 * Paths whose responses show works, each with its user state. These are a
 * work's page, work queries and the grids, children, collections, the
 * Unmatched list, Home, lists, search, and the book pages of people and
 * series. Whatever
 * changes a work refetches these.
 */
export const workViewPaths: readonly ApiPath[] = [
  apiPaths.work,
  ...libraryGridPaths,
  apiPaths.workChildren,
  apiPaths.collection,
  apiPaths.unmatchedWorks,
  apiPaths.home,
  apiPaths.lists,
  apiPaths.listWorks,
  apiPaths.search,
  apiPaths.person,
  apiPaths.bookSeries,
];

/**
 * Paths whose responses show the user's lists or what is on them. These are
 * the lists, their works, Home and its layout, and a work's page, which says
 * which lists hold it. Whatever changes a list refetches these.
 */
export const listViewPaths: readonly ApiPath[] = [
  apiPaths.lists,
  apiPaths.list,
  apiPaths.listWorks,
  apiPaths.home,
  apiPaths.homeRows,
  apiPaths.work,
];

/** Marks every query of the paths stale, refetching those on screen. */
export async function invalidatePaths(queryClient: QueryClient, paths: readonly ApiPath[]): Promise<void> {
  await Promise.all(paths.map((path) => queryClient.invalidateQueries({ queryKey: pathKey(path) })));
}
