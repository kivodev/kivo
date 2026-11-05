import type en from "@shared/display/en.json";
import { describe, expectTypeOf, it } from "vitest";
import type { Problem } from "@/api/schemas";

describe("problem codes", () => {
  it("has a sentence for every code the spec lists, and no others", () => {
    // The client's own code for a request that got no answer.
    expectTypeOf<keyof typeof en.problems>().toEqualTypeOf<Problem["code"] | "network">();
  });
});
