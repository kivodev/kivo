import { plural } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";
import type { WorkField, WorkKind } from "@/api/schemas";

/** Words for a query's fields, operators and kinds. A field's label falls back to the catalog's name. */
export function useSmartLabels() {
  const { t } = useLingui();
  const fields: Record<string, string> = {
    library: t`Library`,
    contentRating: t`Content rating`,
    playState: t`Play state`,
    genre: t`Genre`,
    tag: t`Tag`,
    company: t`Company`,
    resolution: t`Resolution`,
    dynamicRange: t`Dynamic range`,
    collection: t`Collection`,
    bookSeries: t`Book series`,
    inList: t`In list`,
    rootWork: t`Show or movie`,
    actor: t`Actor`,
    director: t`Director`,
    author: t`Author`,
    narrator: t`Narrator`,
    rating: t`Rating`,
    myRating: t`My rating`,
    runtimeMs: t`Runtime`,
    year: t`Year`,
    addedAt: t`Date added`,
    lastEpisodeAddedAt: t`Latest addition`,
    releaseDate: t`Release date`,
    lastPlayedAt: t`Last played`,
    title: t`Title`,
  };
  const ops: Record<string, string> = {
    anyOf: t`is`,
    allOf: t`is all of`,
    noneOf: t`is not`,
    isSet: t`is set`,
    isNotSet: t`is not set`,
    eq: t`is`,
    between: t`between`,
    within: t`in the last`,
    notWithin: t`not in the last`,
    contains: t`contains`,
    notContains: t`doesn't contain`,
    startsWith: t`starts with`,
  };
  const numberOps: Record<string, string> = { gte: t`at least`, lte: t`at most` };
  const dateOps: Record<string, string> = { gte: t`on or after`, lte: t`on or before` };
  const kinds: Record<WorkKind, string> = {
    movie: t`Movies`,
    series: t`Shows`,
    season: t`Seasons`,
    episode: t`Episodes`,
    extra: t`Extras`,
    video: t`Other videos`,
    book: t`Books`,
  };
  return {
    field: (field: WorkField) => fields[field.field] ?? field.name,
    op: (op: string, type: WorkField["type"]) => (type === "date" ? dateOps[op] : numberOps[op]) ?? ops[op] ?? op,
    kind: (kind: WorkKind) => kinds[kind],
    duration: (value: string) => {
      const count = Number(value.slice(1, -1));
      switch (value.at(-1)) {
        case "D":
          return plural(count, { one: "# day", other: "# days" });
        case "W":
          return plural(count, { one: "# week", other: "# weeks" });
        case "M":
          return plural(count, { one: "# month", other: "# months" });
        default:
          return plural(count, { one: "# year", other: "# years" });
      }
    },
  };
}

export type SmartLabels = ReturnType<typeof useSmartLabels>;

/** A count of works of these kinds, as "42 movies", or "42 titles" for a mix. */
export function countOfKinds(count: number, kinds: readonly WorkKind[]): string {
  switch (kinds.length === 1 ? kinds[0] : undefined) {
    case "movie":
      return plural(count, { one: "# movie", other: "# movies" });
    case "series":
      return plural(count, { one: "# show", other: "# shows" });
    case "season":
      return plural(count, { one: "# season", other: "# seasons" });
    case "episode":
      return plural(count, { one: "# episode", other: "# episodes" });
    case "video":
      return plural(count, { one: "# video", other: "# videos" });
    case "book":
      return plural(count, { one: "# book", other: "# books" });
    default:
      return plural(count, { one: "# title", other: "# titles" });
  }
}
