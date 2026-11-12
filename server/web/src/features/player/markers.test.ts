import { describe, expect, it } from "vitest";
import type { Chapter } from "@/api/schemas";
import { chapterAt, markerAt, type SkipMarker } from "./markers";

const chapters: Chapter[] = [
  { startMs: 0, endMs: 60_000, title: "Cold open" },
  { startMs: 60_000, endMs: 150_000, title: "Opening credits" },
  { startMs: 150_000, endMs: 2_550_000, title: "Episode" },
  { startMs: 2_550_000, endMs: 2_640_000, title: "End credits" },
];

const intro: SkipMarker = { kind: "intro", startMs: 60_000, endMs: 150_000, final: false };
const credits: SkipMarker = { kind: "credits", startMs: 2_550_000, endMs: 2_640_000, final: false };
const markers = [intro, credits];

describe("markers", () => {
  it("finds the chapter and the marker at a position", () => {
    expect(chapterAt(chapters, 30_000)?.title).toBe("Cold open");
    expect(markerAt(markers, 30_000)).toBeUndefined();
    expect(markerAt(markers, 60_000)?.kind).toBe("intro");
    expect(markerAt(markers, 2_600_000)?.kind).toBe("credits");
    expect(chapterAt([], 5)).toBeUndefined();
  });
});
