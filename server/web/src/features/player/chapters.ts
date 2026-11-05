import type { Chapter } from "@/api/schemas";

// A book's chapters in the audio player. The seek bar spans the current
// chapter, and previous and next move by chapter. A book without chapters is
// one chapter spanning the whole book.

/** Previous within this long of a chapter's start goes to the chapter before. Later, it goes back to the chapter's start. */
const restartWithinMs = 3_000;

/** The book's chapters, or one spanning it when it has none. */
export function bookChapters(chapters: readonly Chapter[], durationMs: number): readonly Chapter[] {
  return chapters.length > 0 ? chapters : [{ startMs: 0, endMs: durationMs }];
}

/** The index of the chapter at a position, which is the last chapter to start at or before it. */
export function chapterIndexAt(chapters: readonly Chapter[], positionMs: number): number {
  return Math.max(
    0,
    chapters.findLastIndex((c) => c.startMs <= positionMs),
  );
}

/** Where previous goes: the chapter's start, or the previous chapter's start within 3 seconds of it. */
export function previousChapterMs(chapters: readonly Chapter[], positionMs: number): number {
  const i = chapterIndexAt(chapters, positionMs);
  const start = chapters[i]?.startMs ?? 0;
  if (positionMs - start > restartWithinMs || i === 0) return start;
  return chapters[i - 1]?.startMs ?? 0;
}

/** Where next goes: the next chapter's start, or undefined in the last chapter. */
export function nextChapterMs(chapters: readonly Chapter[], positionMs: number): number | undefined {
  return chapters[chapterIndexAt(chapters, positionMs) + 1]?.startMs;
}
