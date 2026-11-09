import { useLingui } from "@lingui/react/macro";

/** Labels for sort and filter values, shared by the menus and the chips. */
export function useBrowseLabels() {
  const { t } = useLingui();
  return {
    sort: {
      title: t`Title`,
      addedAt: t`Date added`,
      releaseDate: t`Release date`,
      rating: t`Rating`,
      myRating: t`My rating`,
      runtime: t`Runtime`,
      lastPlayedAt: t`Last played`,
      author: t`Author`,
      lastEpisodeAddedAt: t`Last episode added`,
    },
    played: { unplayed: t`Unwatched`, inProgress: t`In progress`, played: t`Watched` },
    resolution: { "4k": "4K", "1080p": "1080p", "720p": "720p", sd: "SD" },
    dynamicRange: { hdr: "HDR", dolbyVision: "Dolby Vision" },
    matchState: { unmatched: t`Unmatched` },
    decade: (from: number) => t`${from}s`,
    years: (from: number | undefined, to: number | undefined) =>
      from !== undefined && to !== undefined && to - from === 9 && from % 10 === 0
        ? t`${from}s`
        : from !== undefined && to !== undefined
          ? `${from}–${to}`
          : from !== undefined
            ? t`From ${from}`
            : t`Until ${to}`,
    narrator: (name: string) => t`Narrated by ${name}`,
  };
}

export type BrowseLabels = ReturnType<typeof useBrowseLabels>;
