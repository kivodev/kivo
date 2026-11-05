import { i18n } from "@lingui/core";
import { describe, expect, it, vi } from "vitest";
import { formatCommunityRating, formatCount, formatRuntime, formatTimes } from "./format";

describe("number formatter reuse", () => {
  it("reuses each locale and options combination without mixing their digits or precision", () => {
    const locale = i18n.locale;
    const construct = vi.spyOn(Intl, "NumberFormat");
    try {
      i18n.activate("de");
      for (let i = 0; i < 10; i++) {
        expect(formatCount(1234.5)).toBe("1.234,5");
        expect(formatCommunityRating(8)).toBe("8,0");
        expect(formatTimes(1.25)).toBe("1,3×");
        expect(formatRuntime(90 * 60_000)).toContain("1");
      }
      expect(construct).toHaveBeenCalledTimes(4);
      i18n.activate("en");
      expect(formatCount(1234.5)).toBe("1,234.5");
      i18n.activate("de");
      expect(formatCount(1234.5)).toBe("1.234,5");
      expect(construct).toHaveBeenCalledTimes(5);
    } finally {
      i18n.activate(locale);
    }
  });
});
