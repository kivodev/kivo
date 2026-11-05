import { describe, expect, it } from "vitest";
import type { PlayTrack } from "@/api/schemas";
import { pictureInPictureNeedsNewPlay } from "./pictureInPicture";

const shown = (delivery: PlayTrack["delivery"]) => ({ delivery });

describe("pictureInPictureNeedsNewPlay", () => {
  it("entering starts a new play only for a subtitle delivered external", () => {
    expect(pictureInPictureNeedsNewPlay(true, false, shown("external"))).toBe(true);
    for (const delivery of ["stream", "file", "burn", "none"] as const) {
      expect(pictureInPictureNeedsNewPlay(true, false, shown(delivery))).toBe(false);
    }
    expect(pictureInPictureNeedsNewPlay(true, false, undefined)).toBe(false);
  });

  it("leaving starts a new play only when the play started in picture-in-picture burned the subtitle in", () => {
    expect(pictureInPictureNeedsNewPlay(false, true, shown("burn"))).toBe(true);
    for (const delivery of ["stream", "file", "none"] as const) {
      expect(pictureInPictureNeedsNewPlay(false, true, shown(delivery))).toBe(false);
    }
    // A play that burned the subtitle in outside picture-in-picture burns it anyway.
    expect(pictureInPictureNeedsNewPlay(false, false, shown("burn"))).toBe(false);
  });

  it("leaves a play started where it is alone", () => {
    expect(pictureInPictureNeedsNewPlay(true, true, shown("burn"))).toBe(false);
    expect(pictureInPictureNeedsNewPlay(false, false, shown("external"))).toBe(false);
  });
});
