import type { QueryClient } from "@tanstack/react-query";
import type { paths } from "./schema";

/**
 * Spec paths the client refers to outside a typed call. These are used for
 * invalidation by path (query keys start `["get", path]`), image URLs and
 * the event socket. They are checked against the generated `paths`, so a
 * renamed route fails to compile.
 */
export const apiPaths = {
  setup: "/api/v1/setup",
  currentUser: "/api/v1/users/me",
  sessions: "/api/v1/sessions",
  libraries: "/api/v1/libraries",
  library: "/api/v1/libraries/{id}",
  libraryFacets: "/api/v1/libraries/{id}/facets",
  libraryWorks: "/api/v1/libraries/{id}/works",
  libraryCollections: "/api/v1/libraries/{id}/collections",
  libraryAuthors: "/api/v1/libraries/{id}/authors",
  libraryBookSeries: "/api/v1/libraries/{id}/book-series",
  home: "/api/v1/home",
  favorites: "/api/v1/favorites",
  work: "/api/v1/works/{id}",
  workChildren: "/api/v1/works/{id}/children",
  workFiles: "/api/v1/works/{id}/files",
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

/** The prefix of every GET query key for a path, for invalidation. */
export function pathKey(path: ApiPath): readonly ["get", ApiPath] {
  return ["get", path];
}

/** A library's grids: its works, collections, authors and book series. */
export const libraryGridPaths: readonly ApiPath[] = [
  apiPaths.libraryWorks,
  apiPaths.libraryCollections,
  apiPaths.libraryAuthors,
  apiPaths.libraryBookSeries,
];

/**
 * Paths whose responses show works, each with its user state. These are a
 * work's page, the grids, children, collections, the Unmatched list, Home,
 * Favorites, search, and the book pages of people and series. Whatever
 * changes a work refetches these.
 */
export const workViewPaths: readonly ApiPath[] = [
  apiPaths.work,
  ...libraryGridPaths,
  apiPaths.workChildren,
  apiPaths.collection,
  apiPaths.unmatchedWorks,
  apiPaths.home,
  apiPaths.favorites,
  apiPaths.search,
  apiPaths.person,
  apiPaths.bookSeries,
];

/** Marks every query of the paths stale, refetching those on screen. */
export async function invalidatePaths(queryClient: QueryClient, paths: readonly ApiPath[]): Promise<void> {
  await Promise.all(paths.map((path) => queryClient.invalidateQueries({ queryKey: pathKey(path) })));
}
