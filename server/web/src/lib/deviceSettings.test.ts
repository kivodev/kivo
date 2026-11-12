import { describe, expect, it } from "vitest";
import { deviceSettings } from "./deviceSettings";

describe("device settings", () => {
  it("ignores a value it doesn't know, taking the default", () => {
    localStorage.setItem("kivo.quality.local", "bogus");
    localStorage.setItem("kivo.subtitleSize", "huge");
    localStorage.setItem("kivo.theme", "sepia");
    const s = deviceSettings();
    expect(s.localQuality).toBeUndefined();
    expect(s.subtitleSize).toBe("medium");
    expect(s.theme).toBe("dark");
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
