import { expect, it, vi } from "vitest";
import type { components } from "@/api/schema";
import type { AudioEntry, Capabilities, PlayCreate, QualityRequest, SubtitleEntry, VideoEntry } from "@/api/schemas";
import { playing, plays } from "@/test/player";
import { renderApp } from "@/test/render";
import { mockIds, serveMockApi } from "@/test/server";
import { setPackagedToken } from "@/tv/packaged";

// The play request the packaged TV app sends, held to the spec's
// PlayCreate. The server can't read a request in another shape (a
// subtitle entry that's a string, capabilities grouped by delivery), and
// answers it as malformed, so a TV built from an older contract can't
// play. Each key list is typed as every key of its schema, so a change to
// the spec fails the typecheck here until the list follows it.

type Keys<T> = { [K in keyof Required<T>]: true };
type Schemas = components["schemas"];

const playCreate: Keys<PlayCreate> = {
  workId: true,
  versionId: true,
  audioTrackId: true,
  subtitleTrackId: true,
  positionMs: true,
  capabilities: true,
  settings: true,
  dialogueBoost: true,
};
const capabilities: Keys<Capabilities> = { file: true, video: true, audio: true, subtitles: true };
const file: Keys<Schemas["FileCapabilities"]> = { containers: true, trackSelection: true };
const video: Keys<VideoEntry> = {
  codec: true,
  delivery: true,
  containers: true,
  tags: true,
  profiles: true,
  maxLevel: true,
  maxBitDepth: true,
  maxWidth: true,
  maxHeight: true,
  maxFrameRate: true,
  maxBitrateBps: true,
  maxRefFrames: true,
  interlaced: true,
  anamorphic: true,
  hdr: true,
  dolbyVision: true,
  doviFallback: true,
};
const dolbyVision: Keys<Schemas["DolbyVisionEntry"]> = { profile: true, compat: true, el: true };
const audio: Keys<AudioEntry> = {
  codec: true,
  delivery: true,
  containers: true,
  maxChannels: true,
  maxSampleRate: true,
  maxBitDepth: true,
  profiles: true,
};
const subtitle: Keys<SubtitleEntry> = { codec: true, delivery: true, containers: true };
const settings: Keys<Schemas["PlaybackSettings"]> = {
  localQuality: true,
  remoteQuality: true,
  directPlay: true,
  burnSubtitles: true,
};
const quality: Keys<QualityRequest> = { mode: true, maxBitrateBps: true, bandwidthBps: true };
const deliveries = ["file", "stream", "external"];

/** The keys of value that its schema doesn't have, and the required ones it lacks, at path. */
function strays(value: unknown, keys: Record<string, true>, required: string[], path: string): string[] {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return [`${path} is not an object`];
  const own = Object.keys(value);
  return [
    ...own.filter((k) => !(k in keys)).map((k) => `${path}.${k} is not in the spec`),
    ...required.filter((k) => !own.includes(k)).map((k) => `${path}.${k} is missing`),
  ];
}

/** Everything in body that the spec's PlayCreate doesn't allow. */
function problems(body: PlayCreate): string[] {
  const out = strays(body, playCreate, ["workId", "capabilities"], "body");
  const caps = body.capabilities;
  out.push(...strays(caps, capabilities, ["video", "audio", "subtitles"], "capabilities"));
  if (caps.file) out.push(...strays(caps.file, file, ["containers"], "capabilities.file"));
  const entries = (list: unknown, keys: Record<string, true>, required: string[], path: string) => {
    if (!Array.isArray(list)) return [`${path} is not a list`];
    return list.flatMap((entry, i) => {
      const at = `${path}[${i}]`;
      const found = strays(entry, keys, required, at);
      const delivery = (entry as { delivery?: unknown } | null)?.delivery;
      if (!Array.isArray(delivery) || delivery.some((d) => !deliveries.includes(d)))
        found.push(`${at}.delivery is not a list of deliveries`);
      const dv = (entry as { dolbyVision?: unknown } | null)?.dolbyVision;
      if (Array.isArray(dv))
        found.push(...dv.flatMap((d, j) => strays(d, dolbyVision, ["profile"], `${at}.dolbyVision[${j}]`)));
      return found;
    });
  };
  out.push(...entries(caps.video, video, ["codec", "delivery"], "capabilities.video"));
  out.push(...entries(caps.audio, audio, ["codec", "delivery", "maxChannels"], "capabilities.audio"));
  out.push(...entries(caps.subtitles, subtitle, ["codec", "delivery"], "capabilities.subtitles"));
  if (body.settings) {
    out.push(...strays(body.settings, settings, [], "settings"));
    for (const k of ["localQuality", "remoteQuality"] as const) {
      const q = body.settings[k];
      if (q) out.push(...strays(q, quality, ["mode"], `settings.${k}`));
    }
  }
  return out;
}

it("starts a play with the request the spec's PlayCreate describes", async () => {
  // A 4K panel.
  vi.stubGlobal("webapis", {
    productinfo: { isUdPanelSupported: () => true, is8KPanelSupported: () => false },
  });
  // A media element that plays every type asked of it, as jsdom's plays none.
  vi.spyOn(HTMLMediaElement.prototype, "canPlayType").mockReturnValue("probably");
  serveMockApi();
  setPackagedToken("app-token");
  renderApp(`/watch/${mockIds.dunePartTwo}`);
  await playing();
  const [body] = plays();
  expect(body).toBeDefined();
  if (!body) return;
  expect(problems(body)).toEqual([]);
  expect(body.capabilities.video.length).toBeGreaterThan(0);
  // A TV draws no ASS or PGS, so the server burns those in.
  expect(body.capabilities.subtitles.map((s) => s.codec)).toEqual(["webvtt"]);
});

it("finds the shape the server refused: subtitles as strings, capabilities by delivery, quality on its own", () => {
  const old = {
    workId: "work",
    capabilities: {
      file: { containers: ["mp4"], video: [], audio: [] },
      stream: { video: [], audio: [] },
      subtitles: ["vtt"],
      maxWidth: 1920,
      maxHeight: 1080,
    },
    versionId: "version",
    quality: { mode: "auto" },
    remoteQuality: { mode: "auto" },
    positionMs: 0,
    dialogueBoost: false,
  } as unknown as PlayCreate;
  expect(problems(old)).toEqual([
    "body.quality is not in the spec",
    "body.remoteQuality is not in the spec",
    "capabilities.stream is not in the spec",
    "capabilities.maxWidth is not in the spec",
    "capabilities.maxHeight is not in the spec",
    "capabilities.video is missing",
    "capabilities.audio is missing",
    "capabilities.file.video is not in the spec",
    "capabilities.file.audio is not in the spec",
    "capabilities.video is not a list",
    "capabilities.audio is not a list",
    "capabilities.subtitles[0] is not an object",
    "capabilities.subtitles[0].delivery is not a list of deliveries",
  ]);
});
