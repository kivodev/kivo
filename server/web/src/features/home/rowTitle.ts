import { useLingui } from "@lingui/react/macro";
import type { HomeRow, HomeRowInfo } from "@/api/schemas";

/** The title of a Home row, in the editor as on Home. */
export function useHomeRowTitle() {
  const { t } = useLingui();
  return (row: HomeRow | HomeRowInfo): string => {
    switch (row.kind) {
      case "continueWatching":
        return t`Continue Watching`;
      case "continueListening":
        return t`Continue Listening`;
      case "favorites":
        return t`Favorites`;
      default: {
        const name = row.library?.name;
        return name ? t`Recently Added in ${name}` : t`Recently Added`;
      }
    }
  };
}
