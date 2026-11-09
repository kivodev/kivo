import { describe, expect, it } from "vitest";
import type { Library, WorkField } from "@/api/schemas";
import {
  blankGroup,
  draftOf,
  queryFromGrid,
  queryOf,
  refusal,
  ruleComplete,
  storedValue,
  typedValue,
} from "./smartQuery";

const library = (kind: Library["kind"]) => ({ id: "lib", name: "Lib", kind }) as Library;

describe("queryFromGrid", () => {
  it("maps each filter to its rule and the library to a library rule", () => {
    const query = queryFromGrid(library("movies"), {
      played: "inProgress",
      genre: ["Comedy", "Drama"],
      yearFrom: 1990,
      yearTo: 1999,
      contentRating: ["PG"],
      resolution: ["4k"],
      dynamicRange: ["hdr"],
      matchState: "unmatched",
      sort: "runtime",
    });
    expect(query).toEqual({
      kinds: ["movie"],
      where: {
        match: "all",
        rules: [
          { field: "library", op: "anyOf", values: ["lib"] },
          { field: "playState", op: "anyOf", values: ["inProgress"] },
          { field: "genre", op: "anyOf", values: ["Comedy", "Drama"] },
          { field: "year", op: "between", values: ["1990", "1999"] },
          { field: "contentRating", op: "anyOf", values: ["PG"] },
          { field: "resolution", op: "anyOf", values: ["4k"] },
          { field: "dynamicRange", op: "anyOf", values: ["hdr"] },
        ],
      },
      sort: [{ field: "runtimeMs" }],
    });
  });

  it("uses each library kind's works, a narrator's ID, and a one-sided year", () => {
    expect(queryFromGrid(library("shows"), {}).kinds).toEqual(["series"]);
    expect(queryFromGrid(library("videos"), {}).kinds).toEqual(["video"]);
    const books = queryFromGrid(library("audiobooks"), { narratorId: "person", yearFrom: 2000, sort: "author" });
    expect(books.kinds).toEqual(["book"]);
    expect(books.where?.rules).toContainEqual({ field: "narrator", op: "anyOf", values: ["person"] });
    expect(books.where?.rules).toContainEqual({ field: "year", op: "gte", values: ["2000"] });
    expect(books.sort).toBeUndefined();
  });
});

describe("draftOf and queryOf", () => {
  it("round-trips a query, a root matching all's library rule becoming the Libraries row", () => {
    const query = {
      kinds: ["movie" as const],
      where: {
        match: "all" as const,
        rules: [
          { field: "library", op: "anyOf", values: ["lib"] },
          { field: "genre", op: "noneOf", values: ["Comedy"] },
        ],
        groups: [{ match: "all" as const, rules: [{ field: "year", op: "isSet" }] }],
      },
      sort: [{ field: "rating", direction: "asc" as const }, { field: "title" }],
      maxWorks: 10,
      collapse: "series" as const,
    };
    const draft = draftOf("X", query);
    expect(draft.libraries).toEqual(["lib"]);
    expect(queryOf(draft)).toEqual(query);
  });

  it("keeps a root matching any's library rule among its rules", () => {
    const query = {
      kinds: ["movie" as const],
      where: {
        match: "any" as const,
        rules: [
          { field: "genre", op: "noneOf", values: ["Comedy"] },
          { field: "library", op: "anyOf", values: ["lib"] },
        ],
      },
    };
    const draft = draftOf("X", query);
    expect(draft.libraries).toEqual([]);
    expect(draft.rules.map((r) => r.field)).toEqual(["genre", "library"]);
    expect(queryOf(draft)).toEqual(query);
  });

  it("saves no library rule and no where for All and no rules", () => {
    expect(queryOf(draftOf("X", { kinds: ["movie"] }))).toEqual({ kinds: ["movie"] });
  });
});

const year: WorkField = {
  field: "year",
  name: "Year",
  type: "number",
  ops: [
    { op: "eq", arity: 1 },
    { op: "between", arity: 2 },
    { op: "isSet", arity: 0 },
  ],
  kinds: ["movie"],
  sortable: true,
};
const added: WorkField = {
  ...year,
  field: "addedAt",
  type: "date",
  ops: [
    { op: "gte", arity: 1 },
    { op: "within", arity: 1 },
  ],
};
const runtime: WorkField = { ...year, field: "runtimeMs", unit: "ms" };

describe("blankGroup", () => {
  it("matches any and holds one blank rule", () => {
    const group = blankGroup(year);
    expect(group.match).toBe("any");
    expect(group.rules).toMatchObject([{ field: "year", op: "eq", values: [] }]);
  });
});

describe("refusal", () => {
  it("refuses a newer version, an unknown field or operator, and an unknown sort", () => {
    expect(refusal({ version: 2, kinds: ["movie"] }, [year])).toBe("newerVersion");
    expect(refusal({ kinds: ["movie"], sort: [{ field: "mood" }] }, [year])).toBe("unknownField");
    const rule = (field: string, op: string) => ({
      kinds: ["movie" as const],
      where: { match: "all" as const, rules: [{ field, op }] },
    });
    expect(refusal(rule("mood", "eq"), [year])).toBe("unknownField");
    expect(refusal(rule("year", "soon"), [year])).toBe("unknownField");
    expect(refusal(rule("year", "isSet"), [year])).toBeUndefined();
    expect(refusal({ version: 1, kinds: ["movie"], sort: [{ field: "random" }] }, [year])).toBeUndefined();
  });
});

describe("ruleComplete", () => {
  const rule = (op: string, values: string[]) => ({ key: 1, field: "x", op, values });
  it("needs every value, well formed", () => {
    expect(ruleComplete(rule("isSet", []), year)).toBe(true);
    expect(ruleComplete(rule("eq", [""]), year)).toBe(false);
    expect(ruleComplete(rule("between", ["1990"]), year)).toBe(false);
    expect(ruleComplete(rule("between", ["1990", "1999"]), year)).toBe(true);
    expect(ruleComplete(rule("gte", ["2020-1-1"]), added)).toBe(false);
    expect(ruleComplete(rule("gte", ["2020-01-01"]), added)).toBe(true);
    expect(ruleComplete(rule("within", ["30 days"]), added)).toBe(false);
    expect(ruleComplete(rule("within", ["P30D"]), added)).toBe(true);
  });
});

describe("a millisecond field's typed value", () => {
  it("reads and writes minutes", () => {
    expect(typedValue(runtime, "7200000")).toBe("120");
    expect(storedValue(runtime, "120")).toBe("7200000");
    expect(storedValue(year, "1990")).toBe("1990");
  });
});
