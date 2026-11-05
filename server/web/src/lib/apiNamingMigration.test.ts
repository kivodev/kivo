// Migration (api-naming): remove after every client has updated past this release.
import { describe, expect, it } from "vitest";
import { storedBrowse } from "@/features/browse/browseSearch";

describe("a library's kept view from an earlier version", () => {
  it("reads its dynamic range, match state and last-episode sort in the names the server uses", () => {
    localStorage.setItem(
      "kivo.library.lib1",
      JSON.stringify({ sort: "lastEpisodeAdded", hdr: ["dolbyVision"], match: "unmatched", played: "unplayed" }),
    );
    expect(storedBrowse("lib1")).toEqual({
      sort: "lastEpisodeAddedAt",
      dynamicRange: ["dolbyVision"],
      matchState: "unmatched",
      played: "unplayed",
    });
  });
});
