import { describe, expect, it } from "vitest";
import type { WorkSummary } from "@/api/schemas";
import { commonAuthorId, primarySeries } from "./bookSeries";

const by = (author: string | undefined) =>
  ({
    book: { authors: author ? [{ id: author, name: author }] : [], narrators: [], series: [] },
  }) as unknown as WorkSummary;

describe("book series", () => {
  it("takes the series marked primary, else the first", () => {
    const a = { id: "a", name: "A", primary: false };
    const b = { id: "b", name: "B", primary: true };
    expect(primarySeries({ authors: [], narrators: [], series: [a, b] })?.id).toBe("b");
    expect(primarySeries({ authors: [], narrators: [], series: [a] })?.id).toBe("a");
    expect(primarySeries(undefined)).toBeUndefined();
  });

  it("finds the author more than half the books share", () => {
    expect(commonAuthorId([by("x"), by("x"), by("y")])).toBe("x");
    expect(commonAuthorId([by("x"), by("y")])).toBeUndefined();
    expect(commonAuthorId([by("x"), by("y"), by(undefined)])).toBeUndefined();
    expect(commonAuthorId([])).toBeUndefined();
  });
});
