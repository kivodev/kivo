import { describe, expect, it } from "vitest";
import { storedBrowse } from "./browseSearch";

describe("a library's kept view", () => {
  it("drops what it doesn't know and keeps the rest", () => {
    localStorage.setItem(
      "kivo.library.lib1",
      JSON.stringify({ sort: "bogus", hdr: ["dolbyVision"], match: "unmatched", played: "unplayed" }),
    );
    expect(storedBrowse("lib1")).toStrictEqual({ played: "unplayed" });
  });

  it("reads as the defaults when all it holds is a sort it doesn't know", () => {
    localStorage.setItem("kivo.library.lib2", JSON.stringify({ sort: "lastEpisodeAdded" }));
    expect(storedBrowse("lib2")).toStrictEqual({});
  });
});
