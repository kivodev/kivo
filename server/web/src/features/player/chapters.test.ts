import { describe, expect, it } from "vitest";
import { bookChapters, chapterIndexAt, nextChapterMs, previousChapterMs } from "./chapters";

const chapters = [
  { startMs: 0, endMs: 60_000, title: "Opening Credits" },
  { startMs: 60_000, endMs: 600_000, title: "Chapter 1" },
  { startMs: 600_000, endMs: 900_000, title: "Chapter 2" },
];

describe("book chapters", () => {
  it("is one chapter for a book without any", () => {
    expect(bookChapters([], 900_000)).toEqual([{ startMs: 0, endMs: 900_000 }]);
    expect(bookChapters(chapters, 900_000)).toBe(chapters);
  });

  it("finds the chapter at a position", () => {
    expect(chapterIndexAt(chapters, 0)).toBe(0);
    expect(chapterIndexAt(chapters, 59_999)).toBe(0);
    expect(chapterIndexAt(chapters, 60_000)).toBe(1);
    expect(chapterIndexAt(chapters, 900_000)).toBe(2);
  });

  it("goes back to the chapter's start, or to the chapter before right after a start", () => {
    expect(previousChapterMs(chapters, 300_000)).toBe(60_000);
    expect(previousChapterMs(chapters, 62_000)).toBe(0);
    expect(previousChapterMs(chapters, 2_000)).toBe(0);
  });

  it("goes on to the next chapter, and nowhere from the last", () => {
    expect(nextChapterMs(chapters, 0)).toBe(60_000);
    expect(nextChapterMs(chapters, 700_000)).toBeUndefined();
  });
});
