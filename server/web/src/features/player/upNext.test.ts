import { describe, expect, it } from "vitest";
import type { SkipMarker } from "./markers";
import {
  runAfterPlay,
  type UpNextEvent,
  type UpNextState,
  upNextAppears,
  upNextAtMs,
  upNextCountdownSeconds,
  upNextCredits,
  upNextHidden,
  upNextPlaysNext,
  upNextReducer,
  upNextShows,
} from "./upNext";

const minute = 60_000;
type Appears = Extract<UpNextEvent, { type: "position" }>["appears"];

/** The card after playback moves to `positionMs`. */
const at = (
  state: UpNextState,
  positionMs: number,
  durationMs: number,
  appears: Appears = "countingDown",
  credits?: SkipMarker,
) => upNextReducer(state, { type: "position", positionMs, durationMs, credits, appears });
const tick = (state: UpNextState, running = true) => upNextReducer(state, { type: "tick", running });
const hide = (state: UpNextState) => upNextReducer(state, { type: "hide" });

const duration = 24 * minute;
const countingDown = at(upNextHidden, duration - 30_000, duration);
const waiting = at(upNextHidden, duration - 30_000, duration, "waiting");
const stillWatching = at(upNextHidden, duration - 30_000, duration, "stillWatching");
const dismissed = hide(countingDown);
const finalCredits: SkipMarker = { kind: "credits", startMs: duration - 90_000, endMs: duration, final: true };
const atCredits = at(upNextHidden, duration - 90_000, duration, "countingDown", finalCredits);

describe("up next's time", () => {
  it("is a fixed time before the end, by runtime", () => {
    expect(upNextAtMs(9 * minute + 59_000)).toBeUndefined();
    expect(upNextAtMs(10 * minute)).toBe(10 * minute - 30_000);
    expect(upNextAtMs(24 * minute)).toBe(24 * minute - 30_000);
    expect(upNextAtMs(40 * minute)).toBe(40 * minute - 35_000);
    expect(upNextAtMs(50 * minute)).toBe(50 * minute - 40_000);
    expect(upNextAtMs(62 * minute)).toBe(62 * minute - 40_000);
  });

  it("is at credits only when they're final credits, if that's sooner", () => {
    const intro: SkipMarker = { kind: "intro", startMs: 60_000, endMs: 150_000, final: false };
    const credits: SkipMarker = { kind: "credits", startMs: 2_550_000, endMs: 2_640_000, final: false };
    expect(upNextCredits([intro, credits])).toBeUndefined();
    const finalCredits = { ...credits, final: true };
    expect(upNextCredits([intro, finalCredits])).toBe(finalCredits);
    expect(upNextAtMs(44 * minute, finalCredits.startMs)).toBe(finalCredits.startMs);
    expect(upNextAtMs(44 * minute, 44 * minute - 10_000)).toBe(44 * minute - 35_000);
  });

  it("waits for the last 30 seconds when the server marks no credits", () => {
    // Serial Experiments Lain has ordered chapters that each start at 0:00,
    // with an ending among them. The server marks none, and the card's time
    // doesn't read chapters at all.
    const lainDuration = 1_380_000;
    let state = upNextHidden;
    for (const position of [0, 1_000, 98_000, lainDuration - 31_000]) {
      state = at(state, position, lainDuration);
      expect(state).toBe(upNextHidden);
    }
    expect(at(state, lainDuration - 30_000, lainDuration).phase).toBe("countingDown");
  });
});

describe("upNextReducer", () => {
  describe("hidden", () => {
    it("stays hidden before the card's time", () => {
      expect(at(upNextHidden, duration - 31_000, duration)).toBe(upNextHidden);
    });

    it("appears at its time as it appears: counting down, waiting, or asking", () => {
      expect(countingDown).toEqual({ phase: "countingDown", secondsLeft: 30, atCredits: false });
      expect(waiting).toEqual({ phase: "waiting", atCredits: false });
      expect(stillWatching).toEqual({ phase: "stillWatching", atCredits: false });
    });

    it("appears with Watch credits for an episode with final credits", () => {
      expect(atCredits).toEqual({ phase: "countingDown", secondsLeft: upNextCountdownSeconds, atCredits: true });
      expect(at(upNextHidden, duration - 90_000, duration, "waiting", finalCredits)).toEqual({
        phase: "waiting",
        atCredits: true,
      });
    });

    it("doesn't appear on a seek to the last moments, or in an episode under 10 minutes", () => {
      expect(at(upNextHidden, duration - 19_000, duration)).toBe(upNextHidden);
      expect(at(upNextHidden, 8 * minute - 25_000, 8 * minute)).toBe(upNextHidden);
    });

    it("ignores a tick, Hide and its time passing on", () => {
      expect(tick(upNextHidden)).toBe(upNextHidden);
      expect(hide(upNextHidden)).toBe(upNextHidden);
      expect(upNextShows(upNextHidden)).toBe(false);
    });
  });

  describe("counting down", () => {
    it("counts the seconds left in the episode and never plays next before its end", () => {
      expect(tick(countingDown)).toBe(countingDown);
      expect(at(countingDown, duration - 29_500, duration)).toBe(countingDown);
      let state = at(countingDown, duration - 29_000, duration);
      expect(state).toMatchObject({ phase: "countingDown", secondsLeft: 29 });
      state = at(state, duration - 400, duration);
      expect(state).toMatchObject({ secondsLeft: 1 });
      expect(upNextPlaysNext(state)).toBe(false);
      state = at(state, duration, duration);
      expect(state).toMatchObject({ secondsLeft: 0 });
      expect(upNextPlaysNext(tick(state))).toBe(false);
    });

    it("acts as away from the credits when final credits shorter than its lead start after it", () => {
      const short: SkipMarker = { ...finalCredits, startMs: duration - 15_000 };
      let state = at(upNextHidden, duration - 30_000, duration, "countingDown", short);
      expect(state).toEqual({ phase: "countingDown", secondsLeft: 30, atCredits: false });
      expect(tick(state)).toBe(state);
      state = at(state, duration - 15_000, duration, "countingDown", short);
      expect(state).toEqual({ phase: "countingDown", secondsLeft: 15, atCredits: false });
      expect(upNextPlaysNext(at(state, duration, duration, "countingDown", short))).toBe(false);
      expect(at(upNextHidden, duration - 30_000, duration, "waiting", short)).toEqual({
        phase: "waiting",
        atCredits: false,
      });
    });

    it("counts down 10 seconds at the credits only while running, then plays next", () => {
      expect(tick(atCredits, false)).toBe(atCredits);
      expect(at(atCredits, duration - 80_000, duration, "countingDown", finalCredits)).toBe(atCredits);
      let state = atCredits;
      for (let i = 0; i < 9; i++) state = tick(state);
      expect(state).toMatchObject({ phase: "countingDown", secondsLeft: 1 });
      expect(upNextPlaysNext(state)).toBe(false);
      state = tick(state);
      expect(upNextPlaysNext(state)).toBe(true);
      expect(tick(state)).toBe(state);
    });

    it("stays on to the end, and hides and counts again on a seek back before its time", () => {
      const longer = 45 * minute;
      let state = at(at(upNextHidden, 10 * minute, longer), longer - 25_000, longer);
      expect(upNextShows(state)).toBe(true);
      expect(state).toMatchObject({ secondsLeft: 25 });
      expect(at(state, longer - 1_000, longer)).toMatchObject({ phase: "countingDown", secondsLeft: 1 });
      state = at(state, longer - 36_000, longer);
      expect(state).toBe(upNextHidden);
      expect(at(state, longer - 35_000, longer)).toEqual({ phase: "countingDown", secondsLeft: 35, atCredits: false });
      const credits = { ...finalCredits, startMs: longer - 90_000 };
      state = tick(tick(at(upNextHidden, longer - 90_000, longer, "countingDown", credits)));
      expect(state).toMatchObject({ secondsLeft: 8 });
      state = at(state, longer - 91_000, longer, "countingDown", credits);
      expect(state).toBe(upNextHidden);
      expect(at(state, longer - 90_000, longer, "countingDown", credits)).toMatchObject({ secondsLeft: 10 });
    });

    it("is dismissed by Hide", () => {
      expect(dismissed).toEqual({ phase: "dismissed" });
    });
  });

  describe("waiting and still watching", () => {
    it("never count down or play next, and stay to the end", () => {
      for (const state of [waiting, stillWatching]) {
        expect(upNextShows(state)).toBe(true);
        expect(tick(state)).toBe(state);
        expect(upNextPlaysNext(state)).toBe(false);
        expect(at(state, duration, duration, "countingDown")).toBe(state);
        expect(at(state, duration - 60_000, duration)).toBe(upNextHidden);
        expect(hide(state)).toEqual({ phase: "dismissed" });
      }
    });

    it("stay after the end of a short episode until a seek back before the last 20 seconds", () => {
      const short = 5 * minute;
      const ended = upNextReducer(upNextHidden, { type: "ended", appears: "stillWatching" });
      expect(at(ended, short - 200, short)).toBe(ended);
      expect(at(ended, short - 21_000, short)).toBe(upNextHidden);
    });
  });

  describe("dismissed", () => {
    it("stays away for the episode, and comes back after a seek back before its time", () => {
      expect(upNextShows(dismissed)).toBe(false);
      expect(at(dismissed, duration - 25_000, duration)).toBe(dismissed);
      expect(tick(dismissed)).toBe(dismissed);
      expect(hide(dismissed)).toBe(dismissed);
      expect(upNextPlaysNext(dismissed)).toBe(false);
      const again = at(at(dismissed, duration - 60_000, duration), duration - 30_000, duration);
      expect(again.phase).toBe("countingDown");
    });
  });

  describe("ended", () => {
    it("waits for an answer from any state, as it appears, with Hide rather than Watch credits", () => {
      for (const state of [upNextHidden, countingDown, waiting, stillWatching, dismissed]) {
        expect(upNextReducer(state, { type: "ended", appears: "waiting" })).toEqual({
          phase: "waiting",
          atCredits: false,
        });
        expect(upNextReducer(state, { type: "ended", appears: "stillWatching" })).toEqual({
          phase: "stillWatching",
          atCredits: false,
        });
      }
    });
  });
});

describe("the run", () => {
  it("counts down for the first three episodes played on without input, then asks", () => {
    expect([0, 1, 2, 3, 4].map((run) => upNextAppears(run, true))).toEqual([
      "countingDown",
      "countingDown",
      "countingDown",
      "stillWatching",
      "stillWatching",
    ]);
  });

  it("waits whatever the run with the user's autoplay off", () => {
    expect([0, 3].map((run) => upNextAppears(run, false))).toEqual(["waiting", "waiting"]);
  });

  it("is extended by an episode playing on by itself, and ended by one someone chose", () => {
    expect(runAfterPlay(2, true)).toBe(3);
    expect(runAfterPlay(2, false)).toBe(0);
  });

  it('plays on three times in a row, then asks "Still watching?" and waits at the end', () => {
    let run = 0;
    for (let episode = 0; episode < 3; episode++) {
      const state = at(upNextHidden, duration - 30_000, duration, upNextAppears(run, true));
      expect(state.phase).toBe("countingDown");
      run = runAfterPlay(run, true);
    }
    expect(run).toBe(3);
    const fourth = at(upNextHidden, duration - 30_000, duration, upNextAppears(run, true));
    expect(fourth.phase).toBe("stillWatching");
    expect(tick(fourth)).toBe(fourth);
    expect(upNextReducer(fourth, { type: "ended", appears: "stillWatching" }).phase).toBe("stillWatching");
    // Input ends the run, and the next card counts down again.
    run = 0;
    expect(at(upNextHidden, duration - 30_000, duration, upNextAppears(run, true)).phase).toBe("countingDown");
  });
});
