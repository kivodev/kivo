import { Trans } from "@lingui/react/macro";
import type { WorkSummary } from "@/api/schemas";
import { CardGrid } from "@/components/CardGrid";
import { WorkCard } from "@/features/browse/WorkCard";

/** A book's position in a series, as written ("1.5"), or undefined. */
function positionIn(work: WorkSummary, seriesId: string): string | undefined {
  return work.book?.series.find((s) => s.id === seriesId)?.position;
}

/**
 * Books as square cards. In a series, each is labelled with its position,
 * such as "Book 1.5", above its cover.
 */
export function BookShelf({
  books,
  seriesId,
  focusKey = seriesId ?? "books",
  autoFocus = false,
}: {
  books: readonly WorkSummary[];
  seriesId?: string;
  /** The shelf's focus key on TV, unique on the screen. */
  focusKey?: string;
  /** Whether its first book is the screen's initial focus on TV. */
  autoFocus?: boolean;
}) {
  return (
    <CardGrid
      shape="square"
      focusKey={focusKey}
      autoFocus={autoFocus}
      items={books}
      itemKey={(w) => w.id}
      renderItem={(w) => {
        const position = seriesId ? positionIn(w, seriesId) : undefined;
        return (
          <div className="flex flex-col gap-1">
            {seriesId && (
              <p data-part="series-position" className="text-xs font-semibold text-text-muted uppercase">
                {position ? <Trans>Book {position}</Trans> : <>&nbsp;</>}
              </p>
            )}
            <WorkCard work={w} shape="square" />
          </div>
        );
      }}
    />
  );
}
