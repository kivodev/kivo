import type { BookSeriesRef, WorkSummary } from "@/api/schemas";

type BookInfo = NonNullable<WorkSummary["book"]>;

/** The series a book is mainly in: the one marked primary, else the first. */
export function primarySeries(book: BookInfo | undefined): BookSeriesRef | undefined {
  return book?.series.find((s) => s.primary) ?? book?.series[0];
}

/** A book's position in the series `seriesId` as written ("1.5"), or undefined. */
export function positionIn(work: WorkSummary, seriesId: string): string | undefined {
  return work.book?.series.find((s) => s.id === seriesId)?.position;
}

/** The primary author more than half the books share, whose name their cards leave out; undefined when none does. */
export function commonAuthorId(books: readonly WorkSummary[]): string | undefined {
  const counts = new Map<string, number>();
  for (const w of books) {
    const id = w.book?.authors[0]?.id;
    if (id) counts.set(id, (counts.get(id) ?? 0) + 1);
  }
  let best: [string, number] | undefined;
  for (const entry of counts) if (!best || entry[1] > best[1]) best = entry;
  return best && best[1] * 2 > books.length ? best[0] : undefined;
}
