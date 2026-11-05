import type { PersonRef } from "@/api/schemas";
import type { BrowseSearch } from "./browseSearch";
import type { BrowseLabels } from "./useBrowseLabels";

/** One chip per active filter value, each with the search that removes it. */
export function filterChips(
  search: BrowseSearch,
  labels: BrowseLabels,
  isAdmin: boolean,
  narrators: readonly PersonRef[] = [],
) {
  const chips: { key: string; label: string; without: BrowseSearch }[] = [];
  const without = (patch: Partial<BrowseSearch>): BrowseSearch => ({ ...search, ...patch });
  const listOf = <K extends "genre" | "contentRating" | "resolution" | "dynamicRange">(
    key: K,
    label: (v: NonNullable<BrowseSearch[K]>[number]) => string,
  ) => {
    const values = (search[key] ?? []) as NonNullable<BrowseSearch[K]>[number][];
    for (const v of values) {
      const rest = values.filter((x) => x !== v);
      chips.push({ key: `${key}:${v}`, label: label(v), without: without({ [key]: rest.length ? rest : undefined }) });
    }
  };
  if (search.played) {
    chips.push({ key: "played", label: labels.played[search.played], without: without({ played: undefined }) });
  }
  listOf("genre", (v) => v);
  listOf("contentRating", (v) => v);
  if (search.yearFrom !== undefined || search.yearTo !== undefined) {
    chips.push({
      key: "years",
      label: labels.years(search.yearFrom, search.yearTo),
      without: without({ yearFrom: undefined, yearTo: undefined }),
    });
  }
  listOf("resolution", (v) => labels.resolution[v]);
  listOf("dynamicRange", (v) => labels.dynamicRange[v]);
  if (search.favorite) {
    chips.push({ key: "favorite", label: labels.favorite, without: without({ favorite: undefined }) });
  }
  if (search.narratorId) {
    const name = narrators.find((n) => n.id === search.narratorId)?.name ?? "";
    chips.push({ key: "narrator", label: labels.narrator(name), without: without({ narratorId: undefined }) });
  }
  if (search.matchState && isAdmin) {
    chips.push({
      key: "matchState",
      label: labels.matchState[search.matchState],
      without: without({ matchState: undefined }),
    });
  }
  return chips;
}
