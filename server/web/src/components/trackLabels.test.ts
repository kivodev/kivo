import { describe, expect, it } from "vitest";
import { languageName, output } from "./trackLabels";

describe("output", () => {
  it("names a transcode's video by resolution and its audio by channels", () => {
    expect(output({ codec: "h264", resolution: "1080p", width: 1920, height: 1080 })).toBe("H.264 1080p");
    expect(output({ codec: "aac", channels: 6 })).toBe("AAC 5.1");
    expect(output({ codec: "aac", channels: 2 })).toBe("AAC Stereo");
  });
});

describe("languageName", () => {
  it("reads BCP 47 tags and ISO 639-2 codes", () => {
    expect(languageName("zh-Hant-TW")).toBe("Chinese (Traditional, Taiwan)");
    expect(languageName("en-US")).toBe("American English");
    const codes = { chi: "Chinese", ger: "German", fre: "French", dut: "Dutch", gre: "Greek", ice: "Icelandic" };
    const more = { per: "Persian", rum: "Romanian", slo: "Slovak", baq: "Basque", eng: "English" };
    for (const [code, name] of Object.entries({ ...codes, ...more })) expect(languageName(code)).toBe(name);
    expect(languageName("und")).toBe("Unknown language");
  });
});
