import { getRouteApi } from "@tanstack/react-router";
import { z } from "zod";
import type { Library, MatchState, Resolution, Rule, WorkKind, WorkQuery } from "@/api/schemas";
import type { GridRequest } from "@/api/works";
import { readStoredJson, writeStoredJson } from "@/lib/storage";

// The library view's URL state: sort, filters, view and tab. A sort is the
// work query field it sorts by, and a filter's values are its field's.
// Anything invalid in a URL is dropped rather than failing the page.

/** Checks at compile time that a list names every value of a generated union. */
function allOf<U extends string>() {
  return <const L extends readonly U[]>(list: L & ([Exclude<U, L[number]>] extends [never] ? unknown : never)) => list;
}

const sorts = [
  "title",
  "addedAt",
  "releaseDate",
  "rating",
  "myRating",
  "runtimeMs",
  "lastPlayedAt",
  "author",
  "lastEpisodeAddedAt",
] as const;
export type GridSort = (typeof sorts)[number];
/**
 * The sorts that an audiobooks library and a shows library offer. Movies
 * and videos have the shows sorts except Last episode added.
 */
export const bookSorts = ["title", "author", "addedAt", "releaseDate"] as const satisfies readonly GridSort[];
export const showSorts = [
  "title",
  "addedAt",
  "lastEpisodeAddedAt",
  "releaseDate",
  "rating",
  "myRating",
  "runtimeMs",
  "lastPlayedAt",
] as const satisfies readonly GridSort[];
export const videoSorts = showSorts.filter((s) => s !== "lastEpisodeAddedAt");
/** The `playState` field's values. */
export const playedValues = ["unplayed", "inProgress", "played"] as const;
export const resolutions = allOf<Resolution>()(["4k", "1080p", "720p", "sd"]);
/** The `dynamicRange` field's values. */
export const dynamicRangeValues = ["hdr", "dolbyVision"] as const;
/** The grid takes only `unmatched` of the match states. */
export const matchStateValues = ["unmatched"] as const satisfies readonly MatchState[];

const list = <T extends z.ZodType>(item: T) => z.array(item).nonempty().optional().catch(undefined);

export const browseSearchSchema = z.object({
  /** A movies library's Collections tab, or an audiobooks library's Authors or Series tab. */
  tab: z.enum(["library", "collections", "authors", "series"]).optional().catch(undefined),
  view: z.enum(["grid", "list"]).optional().catch(undefined),
  sort: z.enum(sorts).optional().catch(undefined),
  played: z.enum(playedValues).optional().catch(undefined),
  genre: list(z.string()),
  contentRating: list(z.string()),
  yearFrom: z.number().int().optional().catch(undefined),
  yearTo: z.number().int().optional().catch(undefined),
  resolution: list(z.enum(resolutions)),
  dynamicRange: list(z.enum(dynamicRangeValues)),
  matchState: z.enum(matchStateValues).optional().catch(undefined),
  /** An audiobooks library's narrator filter, which is a person's ID. */
  narratorId: z.string().min(1).optional().catch(undefined),
});

export type BrowseSearch = z.infer<typeof browseSearchSchema>;

const libraryRoute = getRouteApi("/authed/libraries/$libraryId");

/**
 * The library view's URL state, and a change to it, which replaces it in
 * history and keeps the visit's history state.
 */
export function useBrowseSearch() {
  const search = libraryRoute.useSearch();
  const navigate = libraryRoute.useNavigate();
  const setSearch = (next: BrowseSearch) => void navigate({ search: next, replace: true, state: true });
  return [search, setSearch] as const;
}

/** Keys of BrowseSearch that filter (each shows as a chip). */
const filterKeys = [
  "played",
  "genre",
  "contentRating",
  "yearFrom",
  "resolution",
  "dynamicRange",
  "matchState",
  "narratorId",
] as const;

const gridKinds: Record<Library["kind"], WorkKind> = {
  movies: "movie",
  shows: "series",
  videos: "video",
  audiobooks: "book",
};

function yearRules(from: number | undefined, to: number | undefined): Rule[] {
  if (from !== undefined && to !== undefined) {
    return [{ field: "year", op: "between", values: [String(from), String(to)] }];
  }
  if (from !== undefined) return [{ field: "year", op: "gte", values: [String(from)] }];
  return to !== undefined ? [{ field: "year", op: "lte", values: [String(to)] }] : [];
}

const anyOf = (field: string, values: readonly string[] | undefined): Rule[] =>
  values && values.length > 0 ? [{ field, op: "anyOf", values: [...values] }] : [];

/**
 * A library grid's filters and sort as the query its grid reads in the
 * library: the library's top-level kind, a rule per filter, and the sort
 * in its field's own direction. The Match filter isn't a rule.
 */
export function queryFromGrid(library: Library, search: BrowseSearch): WorkQuery {
  const rules = [
    ...anyOf("playState", search.played ? [search.played] : undefined),
    ...anyOf("genre", search.genre),
    ...yearRules(search.yearFrom, search.yearTo),
    ...anyOf("contentRating", search.contentRating),
    ...anyOf("resolution", search.resolution),
    ...anyOf("dynamicRange", search.dynamicRange),
    ...anyOf("narrator", search.narratorId ? [search.narratorId] : undefined),
  ];
  return {
    kinds: [gridKinds[library.kind]],
    ...(rules.length > 0 ? { where: { match: "all", rules } } : {}),
    ...(search.sort ? { sort: [{ field: search.sort }] } : {}),
  };
}

/** A library grid's query as a list keeps it: with the library as a rule. */
export function listQueryFromGrid(library: Library, search: BrowseSearch): WorkQuery {
  const query = queryFromGrid(library, search);
  return {
    ...query,
    where: { match: "all", rules: [...anyOf("library", [library.id]), ...(query.where?.rules ?? [])] },
  };
}

/** The library grid's read for the URL state. The Match filter is for admins only. */
export function gridRequest(library: Library, search: BrowseSearch, isAdmin: boolean): GridRequest {
  return {
    query: queryFromGrid(library, search),
    library: library.id,
    ...(isAdmin && search.matchState === "unmatched" ? { unmatched: true } : {}),
  };
}

/** Whether a library of this kind has a Collections tab once it has collections. */
export function offersCollections(library: Library): boolean {
  return library.kind === "movies" || library.kind === "shows";
}

export function hasFilters(search: BrowseSearch): boolean {
  return filterKeys.some((k) => search[k] !== undefined);
}

export function clearFilters(search: BrowseSearch): BrowseSearch {
  const next = { ...search };
  for (const k of filterKeys) delete next[k];
  delete next.yearTo;
  return next;
}

// A library opens as this browser last left it. Its tab, sort, filters and
// view are kept per library in the browser's storage. Without storage, as
// in a private window, a library opens with its defaults. A visit opened
// sorted one way for once keeps nothing.
const storageKey = (libraryId: string) => `kivo.library.${libraryId}`;

/** The URL state this browser last used in a library. */
export function storedBrowse(libraryId: string): BrowseSearch {
  const stored = browseSearchSchema.safeParse(readStoredJson(storageKey(libraryId)) ?? {});
  return stored.success ? stored.data : {};
}

export function storeBrowse(libraryId: string, search: BrowseSearch): void {
  writeStoredJson(storageKey(libraryId), search);
}
