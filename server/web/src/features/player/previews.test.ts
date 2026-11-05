import { describe, expect, it } from "vitest";
import type { PreviewManifest } from "@/api/schemas";
import { spriteFrame } from "./previews";

const manifest: PreviewManifest = {
  intervalMs: 10_000,
  width: 240,
  height: 135,
  columns: 10,
  rows: 10,
  sheets: ["sheet-0.webp", "sheet-1.webp"],
};
const url = "https://kivo.test/api/v1/plays/s/previews/manifest.json";

describe("spriteFrame", () => {
  it("finds the sheet and the frame's offset for a position", () => {
    expect(spriteFrame(manifest, url, 0)).toEqual({
      sheet: "https://kivo.test/api/v1/plays/s/previews/sheet-0.webp",
      x: 0,
      y: 0,
      width: 240,
      height: 135,
    });
    // Frame 123 is on the second sheet, at row 2, column 3.
    expect(spriteFrame(manifest, url, 1_234_000)).toMatchObject({
      sheet: expect.stringMatching(/sheet-1/),
      x: 720,
      y: 270,
    });
  });

  it("holds the last frame past the end, and skips an empty manifest", () => {
    expect(spriteFrame(manifest, url, 99_999_000)).toMatchObject({
      sheet: expect.stringMatching(/sheet-1/),
      x: 2160,
      y: 1215,
    });
    expect(spriteFrame({ ...manifest, sheets: [] }, url, 0)).toBeUndefined();
  });
});
