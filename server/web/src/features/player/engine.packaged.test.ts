import { afterEach, expect, it, vi } from "vitest";

// AVPlay is looked up as it's needed, so each test sets it before asking.
afterEach(() => {
  vi.unstubAllGlobals();
});

it("plays a file at normal speed only where AVPlay plays it, and a stream at any", async () => {
  const { playsAtSpeed } = await import("./engine");
  vi.stubGlobal("webapis", { avplay: {} });
  expect(playsAtSpeed("file")).toBe(false);
  expect(playsAtSpeed("stream")).toBe(true);
  vi.stubGlobal("webapis", undefined);
  expect(playsAtSpeed("file")).toBe(true);
});
