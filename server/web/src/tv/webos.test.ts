import { afterEach, expect, it, vi } from "vitest";
import { webosHdrPanel } from "./webos";

afterEach(() => {
  vi.unstubAllGlobals();
});

function display(hdr: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: hdr && query === "(dynamic-range: high)" }));
}

it("says whether an LG TV's panel shows HDR", () => {
  vi.stubGlobal("PalmSystem", {});
  display(true);
  expect(webosHdrPanel()).toBe(true);
  display(false);
  expect(webosHdrPanel()).toBe(false);
});

it("can't say off an LG TV", () => {
  display(true);
  expect(webosHdrPanel()).toBeUndefined();
});

it("can't say where the display can't be asked", () => {
  vi.stubGlobal("PalmSystem", {});
  vi.stubGlobal("matchMedia", () => {
    throw new Error("unsupported");
  });
  expect(webosHdrPanel()).toBeUndefined();
});
