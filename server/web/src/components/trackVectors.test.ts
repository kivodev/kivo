import vectors from "@shared/display/vectors/tracks.json";
import { describe, expect, it } from "vitest";
import { languageName, output, type TrackOutput } from "./trackLabels";

// The shared conformance vectors for language names and transcode
// outputs, which the core's tests also read.

describe("track vectors", () => {
  it.each(vectors.languages as [string, string][])("language %j is %j", (tag, out) =>
    expect(languageName(tag)).toBe(out),
  );

  it.each(vectors.outputs as [TrackOutput, string][])("output %j is %j", (o, out) => expect(output(o)).toBe(out));
});
