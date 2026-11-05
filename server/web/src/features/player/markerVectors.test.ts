import vectors from "@shared/display/vectors/player.json";
import { describe, expect, it } from "vitest";
import type { PlayStarted } from "@/api/schemas";
import { markerAt, markersOf, type SkipEntry, skipEntered, skipShows } from "./markers";
import { upNextAtMs, upNextCredits } from "./upNext";

// The shared conformance vectors for Skip and up next. The core's tests
// read them too.

type Step =
  | { positionMs: number; controls: boolean; upNext: boolean; shows: number | null }
  | { expire: true }
  | { skip: true };

describe("player vectors", () => {
  type Markers = { markers?: PlayStarted["markers"] };
  const sequences = vectors.skipMarker as unknown as ({ about: string; steps: Step[] } & Markers)[];

  it.each(sequences.map((s) => [s.about, s] as const))("%s", (_, sequence) => {
    const markers = markersOf(sequence);
    let entry: SkipEntry | undefined;
    for (const step of sequence.steps) {
      if ("expire" in step) {
        entry = entry && { ...entry, timedOut: true };
      } else if ("skip" in step) {
        entry = entry && { ...entry, skipped: true };
      } else {
        const marker = markerAt(markers, step.positionMs);
        entry = skipEntered(entry, marker);
        const shows = marker && entry && skipShows(marker, entry, step) ? marker.startMs : null;
        expect(shows, `at ${step.positionMs}`).toBe(step.shows);
      }
    }
  });

  it.each(vectors.upNextAt as [number, number | null][])("up next in %i ms is at %j", (durationMs, at) =>
    expect(upNextAtMs(durationMs) ?? null).toBe(at),
  );

  const credits = vectors.upNextAtCredits as unknown as ({
    about: string;
    durationMs: number;
    at: number | null;
  } & Markers)[];
  it.each(credits.map((c) => [c.about, c] as const))("%s", (_, c) =>
    expect(upNextAtMs(c.durationMs, upNextCredits(markersOf(c))?.startMs) ?? null).toBe(c.at),
  );
});
