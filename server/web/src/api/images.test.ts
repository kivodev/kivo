import { afterEach, describe, expect, it, vi } from "vitest";
import { imageUrl } from "./images";

describe("imageUrl", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("asks for the displayed width at the screen's density, or at most maxDensity", () => {
    vi.stubGlobal("devicePixelRatio", 3);
    const image = { id: "poster", width: 780, height: 1170 };
    expect(imageUrl(image, 96.4)).toMatch(/\?width=290$/);
    expect(imageUrl(image, 96.4, 2)).toMatch(/\?width=193$/);
    vi.stubGlobal("devicePixelRatio", 1);
    expect(imageUrl(image, 96.4, 2)).toMatch(/\?width=97$/);
  });
});
