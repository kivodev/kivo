import { describe, expect, it } from "vitest";
import { castFailure, castLoad, castMessages, castStatus } from "./castProtocol";

const token = { token: "secret", expiresAt: "2026-10-25T00:00:00Z", serverId: "srv", urls: ["http://10.0.0.1:8484"] };

describe("castLoad", () => {
  it("names the work, carries the token and the choices, and leaves out what the server picks", () => {
    const load = castLoad(token, { workId: "w", subtitleTrackId: "off", positionMs: 90_500, dialogueBoost: true });
    expect(load).toEqual({
      contentId: "w",
      entity: "kivo://work/w",
      currentTimeSec: 90.5,
      customData: {
        protocol: 3,
        serverId: "srv",
        urls: ["http://10.0.0.1:8484"],
        token: "secret",
        subtitleTrackId: "off",
        dialogueBoost: true,
        localQuality: { mode: "auto" },
        remoteQuality: { mode: "auto" },
        autoplayCount: 0,
      },
    });
  });

  it("starts at the server's resume position without one", () => {
    expect(castLoad(token, { workId: "w" }).currentTimeSec).toBeUndefined();
  });
});

describe("castStatus", () => {
  it("reads a media status' custom data, a kivo.status message and the status itself alike", () => {
    const status = {
      workId: "w",
      versionId: "v",
      audioTrackId: "a",
      upNext: { phase: "countingDown", secondsLeft: 7 },
    };
    const fromMedia = castStatus({ status });
    expect(fromMedia).toMatchObject({ workId: "w", versionId: "v", audioTrackId: "a" });
    expect(fromMedia?.upNext).toEqual({ phase: "countingDown", secondsLeft: 7, atCredits: false });
    expect(castStatus(JSON.stringify({ type: "kivo.status", status }))).toEqual(fromMedia);
    expect(castStatus(status)).toEqual(fromMedia);
  });

  it("reads the status under `status` and nothing beside it", () => {
    const status = { workId: "w", versionId: "v", upNext: { phase: "countingDown", secondsLeft: 7 } };
    const kivo = { workId: "old", versionId: "old", upNext: { phase: "showing", secondsLeft: 3 } };
    const read = castStatus({ type: "kivo.status", status, kivo });
    expect(read).toMatchObject({ workId: "w", versionId: "v" });
    expect(read?.upNext).toEqual({ phase: "countingDown", secondsLeft: 7, atCredits: false });
  });

  it("carries the receiver's play's tracks", () => {
    const tracks = [{ trackId: "s", type: "subtitle", delivery: "stream", codec: "webvtt" }];
    expect(castStatus({ versionId: "v", tracks })?.tracks).toEqual(tracks);
    expect(castStatus({ versionId: "v" })?.tracks).toEqual([]);
  });

  it("has subtitles off with a play and none showing, and nothing without a play", () => {
    expect(castStatus({ versionId: "v" })?.subtitleTrackId).toBe("off");
    expect(castStatus({ error: "signedOut" })?.subtitleTrackId).toBeUndefined();
  });

  it("holds nothing that isn't a status", () => {
    expect(castStatus("not json")).toBeUndefined();
    expect(castStatus(42)).toBeUndefined();
  });
});

describe("castFailure", () => {
  it("says why in words, an admin's own when they stopped the play", () => {
    expect(castFailure("signedOut")).toBe("Signed out on the TV.");
    expect(castFailure("playEnded", "Server maintenance")).toBe("Server maintenance");
    expect(castFailure("noFallback")).toBe("This can't be played on this TV.");
  });
});

describe("castMessages", () => {
  it("sends Kivo's messages by type", () => {
    expect(castMessages.setTracks({ subtitleTrackId: "s" })).toEqual({ type: "kivo.setTracks", subtitleTrackId: "s" });
    expect(castMessages.setQuality({ mode: "original" }, { mode: "auto" })).toEqual({
      type: "kivo.setQuality",
      localQuality: { mode: "original" },
      remoteQuality: { mode: "auto" },
    });
    expect(castMessages.setDialogueBoost(true)).toEqual({ type: "kivo.setDialogueBoost", enabled: true });
    expect(castMessages.playNext()).toEqual({ type: "kivo.playNext" });
    expect(castMessages.hideUpNext()).toEqual({ type: "kivo.hideUpNext" });
  });
});
