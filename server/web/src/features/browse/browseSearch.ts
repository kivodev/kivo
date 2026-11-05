import { getRouteApi } from "@tanstack/react-router";
import { z } from "zod";
import type { BrowseSort, DynamicRangeFilter, MatchStateFilter, PlayedFilter, ResolutionFilter } from "@/api/schemas";
import type { BrowseFilters } from "@/api/works";
// Migration (api-naming): remove after every client has updated past this release.
import { migrateStoredBrowse } from "@/lib/apiNamingMigration";
import { readStoredJson, writeStoredJson } from "@/lib/storage";

// The library view's URL state: sort, filters, view and tab. Values are
// the API's own, from the generated types. Anything invalid in a URL is
// dropped rather than failing the page.

/** Checks at compile time that a list names every value of a generated union. */
function allOf<U extends string>() {
  return <const L extends readonly U[]>(list: L & ([Exclude<U, L[number]>] extends [never] ? unknown : never)) => list;
}

export const sorts = allOf<BrowseSort>()([
  "title",
  "addedAt",
  "releaseDate",
  "rating",
  "myRating",
  "runtime",
  "lastPlayedAt",
  "author",
  "lastEpisodeAddedAt",
]);
/**
 * The sorts that an audiobooks library and a shows library offer. Movies
 * and videos have the shows sorts except Last episode added.
 */
export const bookSorts = ["title", "author", "addedAt", "releaseDate"] as const satisfies readonly BrowseSort[];
export const showSorts = [
  "title",
  "addedAt",
  "lastEpisodeAddedAt",
  "releaseDate",
  "rating",
  "myRating",
  "runtime",
  "lastPlayedAt",
] as const satisfies readonly BrowseSort[];
export const videoSorts = showSorts.filter((s) => s !== "lastEpisodeAddedAt");
export const playedValues = allOf<PlayedFilter>()(["unplayed", "inProgress", "played"]);
const resolutions = allOf<ResolutionFilter>()(["4k", "1080p", "720p", "sd"]);
const dynamicRangeValues = allOf<DynamicRangeFilter>()(["hdr", "dolbyVision"]);
/** The grid takes only `unmatched` of the match states. */
export const matchStateValues = ["unmatched"] as const satisfies readonly MatchStateFilter[];

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
  favorite: z.boolean().optional().catch(undefined),
  matchState: z.enum(matchStateValues).optional().catch(undefined),
  /** An audiobooks library's narrator filter, which is a person's ID. */
  narratorId: z.string().min(1).optional().catch(undefined),
});

export type BrowseSearch = z.infer<typeof browseSearchSchema>;

const libraryRoute = getRouteApi("/authed/libraries/$libraryId");

/** The library view's URL state, and a change to it, which replaces it in history. */
export function useBrowseSearch() {
  const search = libraryRoute.useSearch();
  const navigate = libraryRoute.useNavigate();
  const setSearch = (next: BrowseSearch) => void navigate({ search: next, replace: true });
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
  "favorite",
  "matchState",
  "narratorId",
] as const;

/** The library grid's query for the URL state. `matchState` is for admins only. */
export function toFilters(search: BrowseSearch, isAdmin: boolean): BrowseFilters {
  const { tab: _tab, view: _view, matchState, ...rest } = search;
  return { ...rest, matchState: isAdmin ? matchState : undefined };
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
// in a private window, a library opens with its defaults.
const storageKey = (libraryId: string) => `kivo.library.${libraryId}`;

/** The URL state this browser last used in a library. */
export function storedBrowse(libraryId: string): BrowseSearch {
  // Migration (api-naming): remove after every client has updated past this release.
  const stored = browseSearchSchema.safeParse(migrateStoredBrowse(readStoredJson(storageKey(libraryId)) ?? {}));
  return stored.success ? stored.data : {};
}

export function storeBrowse(libraryId: string, search: BrowseSearch): void {
  writeStoredJson(storageKey(libraryId), search);
}
