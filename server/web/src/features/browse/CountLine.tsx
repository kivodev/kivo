import { Plural, useLingui } from "@lingui/react/macro";
import type { BrowseSort } from "@/api/schemas";
import { Select } from "@/components/Select";
import type { BrowseSearch } from "./browseSearch";
import type { BrowseLabels } from "./useBrowseLabels";

type Tab = BrowseSearch["tab"];

/** How many items the section lists, such as "237 movies", "2 authors" or "1 series". */
export function CountNoun({ count, kind, tab }: { count: number; kind: string; tab: Tab }) {
  switch (tab) {
    case "collections":
      return <Plural value={count} one="# collection" other="# collections" />;
    case "authors":
      return <Plural value={count} one="# author" other="# authors" />;
    case "series":
      return <Plural value={count} one="# series" other="# series" />;
  }
  switch (kind) {
    case "movies":
      return <Plural value={count} one="# movie" other="# movies" />;
    case "shows":
      return <Plural value={count} one="# show" other="# shows" />;
    case "audiobooks":
      return <Plural value={count} one="# book" other="# books" />;
    default:
      return <Plural value={count} one="# video" other="# videos" />;
  }
}

/**
 * What an empty filtered result says, matching the iPhone app. It is "No
 * unplayed movies" for the played filter alone, or else "No movies match
 * these filters".
 */
export function useEmptyTitle(kind: string, tab: Tab, search: BrowseSearch, onlyPlayed: boolean): string {
  const { t } = useLingui();
  const noun =
    tab === "collections"
      ? t`collections`
      : (({ movies: t`movies`, shows: t`shows`, audiobooks: t`books` } as Record<string, string>)[kind] ?? t`videos`);
  if (onlyPlayed && search.played) {
    const state = { unplayed: t`unwatched`, inProgress: t`in-progress`, played: t`watched` }[search.played];
    return t`No ${state} ${noun}`;
  }
  return t`No ${noun} match these filters`;
}

/**
 * The sort in a word or two at the end of the count line ("A–Z", "Recently
 * added"), which opens the sort menu.
 */
export function SortMenu({
  options,
  selected,
  labels,
  onChange,
}: {
  options: readonly BrowseSort[];
  selected: BrowseSort;
  labels: BrowseLabels;
  onChange: (sort: string) => void;
}) {
  const { t } = useLingui();
  const short: Record<BrowseSort, string> = {
    title: t`A–Z`,
    author: t`Author A–Z`,
    addedAt: t`Recently added`,
    releaseDate: t`Newest`,
    rating: t`Top rated`,
    myRating: t`My rating`,
    runtime: t`Shortest`,
    lastPlayedAt: t`Recently played`,
    lastEpisodeAddedAt: t`Last episode added`,
  };
  return (
    <Select
      variant="compact"
      part="sort"
      label={t`Sort`}
      options={options.map((s) => ({ id: s, label: labels.sort[s] }))}
      selected={selected}
      value={short[selected]}
      onChange={onChange}
    />
  );
}
