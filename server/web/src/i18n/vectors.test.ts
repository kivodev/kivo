import formats from "@shared/display/vectors/formats.json";
import problems from "@shared/display/vectors/problems.json";
import { describe, expect, it } from "vitest";
import {
  formatBitrate,
  formatClock,
  formatEpisodeCode,
  formatQuality,
  formatRung,
  formatRuntime,
  formatSize,
  formatTimeLeft,
} from "./format";
import { describeReason } from "./playback";
import { describePlayFailure, describePlayStopped, describeProblem, describeSubtitleFailure } from "./problems";

// The shared conformance vectors, which the core's tests also read. Each
// case is an input and the words it should come out as.

describe("formats", () => {
  const numbers = { formatBitrate, formatSize, formatRuntime, formatTimeLeft, formatClock };
  const cases = {
    formatBitrate: formats.bitrate,
    formatSize: formats.size,
    formatRuntime: formats.runtime,
    formatTimeLeft: formats.timeLeft,
    formatClock: formats.clock,
  } as Record<keyof typeof numbers, [number, string][]>;
  for (const [name, format] of Object.entries(numbers)) {
    it.each(cases[name as keyof typeof numbers])(`${name}(%j) is %j`, (input, out) => {
      expect(format(input)).toBe(out);
    });
  }

  it.each(formats.episodeCode as [{ seasonNumber: number; number: number; endNumber?: number }, string][])(
    "episode code of %j is %j",
    (episode, out) => expect(formatEpisodeCode(episode)).toBe(out),
  );

  it.each(formats.rung as [{ bitrateBps: number; height: number }, string][])("rung %j is %j", (rung, out) =>
    expect(formatRung(rung)).toBe(out),
  );

  type QualityCase = {
    quality: "auto" | "original" | number;
    rung?: { bitrateBps: number; height: number };
    sourceBps?: number;
  };
  it.each(formats.quality as [QualityCase, string][])("quality %j is %j", (q, out) =>
    expect(formatQuality(q.quality, q.rung, q.sourceBps)).toBe(out),
  );
});

describe("problems", () => {
  it.each(problems.problem)("%s is %j", (code, out) => expect(describeProblem(code)).toBe(out));

  it.each(problems.subtitleSearch as [{ code: string; time?: string }, string][])("subtitle %j is %j", (p, out) =>
    expect(describeSubtitleFailure(p.code, p.time)).toBe(out),
  );

  it.each(problems.reason)("reason %s is %j", (code, out) => expect(describeReason(code)).toBe(out));

  it.each(problems.playerFailure as [{ ended?: boolean; message?: string; code?: string; reason?: string }, string][])(
    "play failure %j is %j",
    (f, out) =>
      expect(f.ended ? describePlayStopped(f.message) : describePlayFailure({ code: f.code ?? "", ...f })).toBe(out),
  );
});
