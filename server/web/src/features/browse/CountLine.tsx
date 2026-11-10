import { Plural, useLingui } from "@lingui/react/macro";
import { Select } from "@/components/Select";
import type { BrowseSearch, GridSort } from "./browseSearch";

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

/** Each grid sort in a word or two, as a count line ends with it. */
export function useSortWords(): Record<GridSort, string> {
  const { t } = useLingui();
  return {
    title: t`A–Z`,
    author: t`Author A–Z`,
    addedAt: t`Recently added`,
    releaseDate: t`Newest`,
    rating: t`Top rated`,
    myRating: t`My rating`,
    runtimeMs: t`Shortest`,
    lastPlayedAt: t`Recently played`,
    lastEpisodeAddedAt: t`Last episode added`,
  };
}

/**
 * The sort in a word or two at the end of a count line ("A–Z", "Recently
 * added"), which opens the sort menu. `words` says each sort so, and
 * `labels` names each in the menu.
 */
export function SortMenu<T extends string>({
  options,
  selected,
  labels,
  words,
  onChange,
}: {
  options: readonly T[];
  selected: T;
  labels: Record<T, string>;
  words: Record<T, string>;
  onChange: (sort: T) => void;
}) {
  const { t } = useLingui();
  return (
    <Select
      variant="compact"
      part="sort"
      label={t`Sort`}
      options={options.map((s) => ({ id: s, label: labels[s] }))}
      selected={selected}
      value={words[selected]}
      onChange={onChange}
    />
  );
}
