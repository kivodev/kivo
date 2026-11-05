import { describe, expect, it } from "vitest";
import type { PlayStarted } from "@/api/schemas";
import { initialPlayerState, type PlayerAction, type PlayerState, playerReducer } from "./reducer";

const play: PlayStarted = {
  id: "0190a000-0000-7000-8000-000000000001",
  workId: "0190a000-0000-7000-8000-000000000002",
  versionId: "0190a000-0000-7000-8000-000000000003",
  startMs: 90_000,
  durationMs: 600_000,
  local: true,
  delivery: "file",
  url: "plays/1/files/0",
  tracks: [],
  parts: [{ durationMs: 600_000, url: "plays/1/files/0" }],
  chapters: [],
};

const run = (actions: PlayerAction[], from: PlayerState = initialPlayerState) => actions.reduce(playerReducer, from);

const playing = run([{ type: "start" }, { type: "started", play }, { type: "ready" }, { type: "play" }]);

describe("playerReducer", () => {
  it("loads a play at its start position, then plays", () => {
    const loading = run([{ type: "start" }, { type: "started", play }]);
    expect(loading).toMatchObject({ status: "loading", positionMs: 90_000, play });
    expect(playerReducer(loading, { type: "ready" }).status).toBe("playing");
    expect(playing).toMatchObject({ status: "playing", paused: false });
  });

  it("ignores media events before the play's media is ready", () => {
    const loading = run([{ type: "start" }]);
    for (const action of [
      { type: "time", positionMs: 5 },
      { type: "seek", positionMs: 5 },
      { type: "waiting" },
      { type: "ended" },
    ] as const) {
      expect(playerReducer(loading, action)).toBe(loading);
    }
  });

  it("buffers while waiting for data and plays on", () => {
    const buffering = playerReducer(playing, { type: "waiting" });
    expect(buffering.status).toBe("buffering");
    expect(playerReducer(buffering, { type: "playing" }).status).toBe("playing");
  });

  it("shows the seek target until the seek lands, ignoring stale times", () => {
    const seeking = playerReducer(playing, { type: "seek", positionMs: 300_000 });
    expect(seeking).toMatchObject({ status: "seeking", positionMs: 300_000 });
    expect(playerReducer(seeking, { type: "time", positionMs: 91_000 }).positionMs).toBe(300_000);
    expect(playerReducer(seeking, { type: "playing" }).status).toBe("seeking");
    const landed = playerReducer(seeking, { type: "seeked" });
    expect(landed.status).toBe("playing");
    expect(playerReducer(landed, { type: "time", positionMs: 301_000 }).positionMs).toBe(301_000);
  });

  it("keeps pause as a flag on a live player", () => {
    const paused = playerReducer(playing, { type: "pause" });
    expect(paused).toMatchObject({ status: "playing", paused: true });
    expect(playerReducer(paused, { type: "seek", positionMs: 10 }).status).toBe("seeking");
  });

  it("loads a fallback at the position the server answers with, keeping the failure's detail", () => {
    const fallback = playerReducer(playing, { type: "fallback", detail: "decode failed" });
    expect(fallback).toMatchObject({ status: "loading", detail: "decode failed" });
    const reloaded = run([{ type: "started", play: { ...play, startMs: 120_000 } }, { type: "ready" }], fallback);
    expect(reloaded).toMatchObject({ status: "playing", positionMs: 120_000 });
  });

  it("ends paused at the end, and plays again after a seek back", () => {
    const ended = playerReducer(playing, { type: "ended" });
    expect(ended).toMatchObject({ ended: true, paused: true, status: "playing" });
    expect(playerReducer(ended, { type: "seek", positionMs: 0 }).ended).toBe(false);
  });

  it("stops on a failure until the next start", () => {
    const failed = playerReducer(playing, {
      type: "fail",
      failure: { kind: "problem", code: "noFallback" },
      detail: "decode failed",
    });
    expect(failed).toMatchObject({ status: "error", paused: true, detail: "decode failed" });
    expect(playerReducer(failed, { type: "playing" })).toBe(failed);
    const retried = playerReducer(failed, { type: "start" });
    expect(retried.status).toBe("loading");
    expect(retried.failure).toBeUndefined();
  });

  it("starts a switch where the last play was, showing that play until the next arrives", () => {
    const at = playerReducer(playing, { type: "time", positionMs: 200_000 });
    expect(playerReducer(at, { type: "start" })).toMatchObject({ status: "loading", positionMs: 200_000, play });
    expect(playerReducer(at, { type: "reset" })).toEqual(initialPlayerState);
  });
});
