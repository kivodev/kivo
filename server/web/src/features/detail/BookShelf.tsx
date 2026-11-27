import { Trans } from "@lingui/react/macro";
import type { WorkSummary } from "@/api/schemas";
import { CardGrid } from "@/components/CardGrid";
import { WorkCard } from "@/features/browse/WorkCard";
import { formatTimeLeft } from "@/i18n/format";
import { commonAuthorId, positionIn } from "./bookSeries";

/** A book's place in a series: "Book 1.5". */
export function BookNumber({ position }: { position: string }) {
  return <Trans>Book {position}</Trans>;
}

/** The time left of a book in progress, or undefined. */
function timeLeft(work: WorkSummary): string | undefined {
  const position = work.userState?.positionMs;
  return position !== undefined && work.durationMs ? formatTimeLeft(work.durationMs - position) : undefined;
}

/**
 * The line under a series book's title: its author when it isn't the one
 * most of the series' books share, then its time left while in progress.
 */
function seriesLine(work: WorkSummary, common: string | undefined): string {
  const author = work.book?.authors[0];
  const parts = [author && author.id !== common ? author.name : undefined, timeLeft(work)];
  return parts.filter(Boolean).join(" · ");
}

/**
 * Books as square cards. In a series, each says "Book 1.5" in a small muted
 * line above its title, and names its author only when it isn't the one
 * most of the series' books share. Lines some cards leave empty stay as
 * blank lines, so the cards line up.
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
  const common = seriesId ? commonAuthorId(books) : undefined;
  const positioned = seriesId !== undefined && books.some((w) => positionIn(w, seriesId) !== undefined);
  const lined = seriesId !== undefined && books.some((w) => seriesLine(w, common) !== "");
  return (
    <CardGrid
      shape="square"
      focusKey={focusKey}
      autoFocus={autoFocus}
      items={books}
      itemKey={(w) => w.id}
      renderItem={(w) => {
        if (seriesId === undefined) return <WorkCard work={w} shape="square" />;
        const position = positionIn(w, seriesId);
        const line = seriesLine(w, common);
        return (
          <WorkCard
            work={w}
            shape="square"
            overline={positioned ? position ? <BookNumber position={position} /> : <>&nbsp;</> : undefined}
            subtitle={lined ? line || <>&nbsp;</> : null}
          />
        );
      }}
    />
  );
}
