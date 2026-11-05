import { describe, expect, it } from "vitest";
import type { Chapter } from "@/api/schemas";
import { chapterAt, markerAt, markersOf, type SkipMarker, skipEntered, skipShows } from "./markers";

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

  it("are the server's, of the kinds the player knows and 3 seconds or longer", () => {
    const server = [
      { kind: "recap", startMs: 0, endMs: 40_000, source: "chapter" },
      { kind: "intro", startMs: 40_000, endMs: 42_000, source: "chapter" },
      { kind: "sponsor", startMs: 50_000, endMs: 80_000, source: "chapter" },
      { kind: "credits", startMs: 2_550_000, endMs: 2_640_000, source: "chapter", final: true },
    ] as const;
    expect(markersOf({ markers: server as never })).toEqual([
      { kind: "recap", startMs: 0, endMs: 40_000, final: false },
      { ...credits, final: true },
    ]);
    expect(markersOf({})).toEqual([]);
    expect(markersOf({ markers: [] })).toEqual([]);
  });

  it("enters a marker once, and leaves it", () => {
    const entry = skipEntered(undefined, intro);
    expect(entry).toEqual({ startMs: 60_000, timedOut: false, skipped: false });
    expect(skipEntered(entry, intro)).toBe(entry);
    expect(skipEntered(entry, undefined)).toBeUndefined();
    expect(skipEntered(entry, credits)?.startMs).toBe(2_550_000);
  });

  it("shows Skip until its time passes, then only with the controls, and never once pressed", () => {
    const fresh = { startMs: 60_000, timedOut: false, skipped: false };
    const hidden = { controls: false, upNext: false };
    const shown = { controls: true, upNext: false };
    expect(skipShows(intro, fresh, hidden)).toBe(true);
    expect(skipShows(intro, { ...fresh, timedOut: true }, hidden)).toBe(false);
    expect(skipShows(intro, { ...fresh, timedOut: true }, shown)).toBe(true);
    expect(skipShows(intro, { ...fresh, skipped: true }, shown)).toBe(false);
    // Skip Credits gives way to up next.
    expect(skipShows(credits, { ...fresh, startMs: credits.startMs }, { controls: true, upNext: true })).toBe(false);
  });
});
