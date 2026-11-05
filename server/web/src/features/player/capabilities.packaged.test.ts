import { afterEach, beforeEach, expect, it, vi } from "vitest";

// The report is asked once a page, so each test loads the module afresh.
// jsdom's video element has audioTracks, so it's taken away, leaving
// AVPlay the only way the report switches a file's tracks.
const audioTracks = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, "audioTracks");

beforeEach(() => {
  delete (HTMLMediaElement.prototype as { audioTracks?: unknown }).audioTracks;
});

afterEach(() => {
  if (audioTracks) Object.defineProperty(HTMLMediaElement.prototype, "audioTracks", audioTracks);
  vi.unstubAllGlobals();
  vi.resetModules();
});

const freshReport = async () => (await import("./capabilities")).capabilityReport();

it("switches a file's audio tracks where the packaged app has AVPlay", async () => {
  vi.stubGlobal("webapis", { avplay: {} });
  expect((await freshReport()).file?.trackSelection).toBe(true);
});

it("doesn't without AVPlay", async () => {
  vi.stubGlobal("webapis", undefined);
  expect((await freshReport()).file?.trackSelection).toBe(false);
});
