import { describe, expect, it } from "vitest";
import { deviceSettings } from "./deviceSettings";

describe("device settings", () => {
  // Migration (api-naming): remove after every client has updated past this release.
  it("moves a local quality kept under its earlier key, once", () => {
    localStorage.setItem("kivo.quality.home", "original");
    expect(deviceSettings().localQuality).toEqual({ mode: "original" });
    expect(localStorage.getItem("kivo.quality.local")).toBe("original");
    expect(localStorage.getItem("kivo.quality.home")).toBeNull();

    localStorage.setItem("kivo.quality.home", "2000000");
    expect(deviceSettings().localQuality).toEqual({ mode: "original" });
    expect(localStorage.getItem("kivo.quality.home")).toBeNull();
  });

  it("keeps stereo, 5.1 and 7.1 as the most audio channels, and Auto for anything else", () => {
    for (const n of [2, 6, 8]) {
      localStorage.setItem("kivo.audioChannels", String(n));
      expect(deviceSettings().maxAudioChannels).toBe(n);
    }
    localStorage.setItem("kivo.audioChannels", "4");
    expect(deviceSettings().maxAudioChannels).toBeUndefined();
  });
});
