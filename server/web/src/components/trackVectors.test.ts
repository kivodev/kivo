import vectors from "@shared/display/vectors/tracks.json";
import { describe, expect, it } from "vitest";
import type { Track, Version } from "@/api/schemas";
import { languageName, output, type TrackOutput, trackDetail, trackNames, versionName } from "./trackLabels";

// The shared conformance vectors for track names, which the core's tests
// also read. They hold a real library's pickers and language tags, and
// edge cases.

describe("track vectors", () => {
  const pickers = vectors.pickers as { tracks: Track[]; names: [string, string | null][] }[];

  it.each(pickers.map((p, i) => [i, p] as const))("picker %i is named alike", (_, { tracks, names }) => {
    const named = trackNames(tracks);
    expect(tracks.map((t) => named.get(t.id))).toEqual(
      names.map(([name, note]) => (note === null ? { name } : { name, note })),
    );
  });

  it.each(vectors.languages as [string, string][])("language %j is %j", (tag, out) =>
    expect(languageName(tag)).toBe(out),
  );

  it.each(vectors.details as [Track, string][])("detail of %j is %j", (track, out) =>
    expect(trackDetail(track)).toBe(out),
  );

  it.each(vectors.versions as [Version, string][])("version %j is %j", (version, out) =>
    expect(versionName(version)).toBe(out),
  );

  it.each(vectors.outputs as [TrackOutput, string][])("output %j is %j", (o, out) => expect(output(o)).toBe(out));
});
