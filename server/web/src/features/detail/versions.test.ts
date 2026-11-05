import { describe, expect, it } from "vitest";
import type { Version } from "@/api/schemas";
import { defaultVersion, selectedVersion } from "./versions";

const version = (id: string) => ({ id }) as Version;
const versions = [version("first"), version("second")];

describe("defaultVersion", () => {
  it("is the version last played", () => {
    expect(defaultVersion({ versions, userState: { versionId: "second" } as never })?.id).toBe("second");
  });

  it("is the first when none was played, or the one played is gone", () => {
    expect(defaultVersion({ versions })?.id).toBe("first");
    expect(defaultVersion({ versions, userState: { versionId: "gone" } as never })?.id).toBe("first");
  });

  it("is none without versions", () => {
    expect(defaultVersion({ versions: [] })).toBeUndefined();
  });
});

describe("selectedVersion", () => {
  it("is the version chosen, else the default", () => {
    const work = { versions, userState: { versionId: "second" } as never };
    expect(selectedVersion(work, "first")?.id).toBe("first");
    expect(selectedVersion(work, undefined)?.id).toBe("second");
    expect(selectedVersion(work, "gone")?.id).toBe("second");
  });
});
