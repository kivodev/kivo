import { describe, expect, it } from "vitest";
import { KeyDispatcher, type KeyInput } from "./keyDispatcher";

/** A dispatcher that acts on `taken` keys and leaves the rest to the control. It records each run as "key@repeatCount". */
function dispatcher(...taken: string[]) {
  const ran: string[] = [];
  const asked: string[] = [];
  const keys = new KeyDispatcher();
  keys.register(({ key }) => {
    asked.push(key);
    return taken.includes(key) ? (repeatCount) => ran.push(`${key}@${repeatCount}`) : undefined;
  });
  const event = (type: KeyInput["type"], key: string, repeat = false) =>
    keys.dispatch({ type, key, repeat, target: null });
  /** Sends a first keydown and `repeats` repeats, and returns what the dispatcher answered to each. */
  const hold = (key: string, repeats: number) => [
    event("keydown", key),
    ...Array.from({ length: repeats }, () => event("keydown", key, true)),
  ];
  const up = (key: string) => event("keyup", key);
  return { keys, ran, asked, event, hold, up };
}

describe("KeyDispatcher", () => {
  it("runs OK and Back once, as they're released", () => {
    const { ran, hold, up } = dispatcher("Enter", "GoBack");
    for (const key of ["Enter", "GoBack"]) {
      expect(hold(key, 20).every(Boolean)).toBe(true);
      expect(ran).toEqual([]);
      expect(up(key)).toBe(true);
      expect(ran).toEqual([`${key}@0`]);
      ran.length = 0;
    }
  });

  it("runs other keys on their first keydown, and only arrows, fast forward and rewind again on repeats", () => {
    const { ran, hold, up } = dispatcher("MediaPlayPause", " ", "MediaFastForward", "ArrowRight");
    hold("MediaPlayPause", 20);
    up("MediaPlayPause");
    hold(" ", 5);
    up(" ");
    expect(ran).toEqual(["MediaPlayPause@0", " @0"]);
    ran.length = 0;
    hold("MediaFastForward", 2);
    up("MediaFastForward");
    hold("ArrowRight", 2);
    up("ArrowRight");
    expect(ran).toEqual([
      "MediaFastForward@0",
      "MediaFastForward@1",
      "MediaFastForward@2",
      "ArrowRight@0",
      "ArrowRight@1",
      "ArrowRight@2",
    ]);
  });

  it("decides a press once, at its first keydown", () => {
    const { asked, hold, up } = dispatcher("ArrowLeft");
    hold("ArrowLeft", 5);
    up("ArrowLeft");
    expect(asked).toEqual(["ArrowLeft"]);
  });

  it("leaves a whole press to the focused control", () => {
    const { hold, up } = dispatcher();
    expect(hold("Enter", 2)).toEqual([false, false, false]);
    expect(up("Enter")).toBe(false);
  });

  it("stops a press reaching its control once the player moves focus from under it", () => {
    const { keys, event, up } = dispatcher();
    expect(event("keydown", "Enter")).toBe(false);
    keys.focusMoved();
    expect(event("keydown", "Enter", true)).toBe(true);
    expect(up("Enter")).toBe(true);
    expect(event("keydown", "Enter")).toBe(false);
    expect(up("Enter")).toBe(false);
  });

  it("drops the events of a press whose first keydown it didn't see", () => {
    const { ran, asked, event, up } = dispatcher("GoBack");
    expect(up("Enter")).toBe(true);
    expect(event("keydown", "Enter", true)).toBe(true);
    expect(up("GoBack")).toBe(true);
    expect(ran).toEqual([]);
    expect(asked).toEqual([]);
  });

  it("follows presses of different keys at once", () => {
    const { ran, event, up } = dispatcher("GoBack");
    event("keydown", "Shift");
    event("keydown", "GoBack");
    expect(up("Shift")).toBe(false);
    up("GoBack");
    expect(ran).toEqual(["GoBack@0"]);
  });

  it("leaves every press to the control with no decide registered, and its removal keeps a later one", () => {
    const keys = new KeyDispatcher();
    const event = (type: KeyInput["type"], key: string) => keys.dispatch({ type, key, repeat: false, target: null });
    expect(event("keydown", " ")).toBe(false);
    expect(event("keyup", " ")).toBe(false);
    const removeFirst = keys.register(() => undefined);
    keys.register(() => () => {});
    removeFirst();
    expect(event("keydown", " ")).toBe(true);
    expect(event("keyup", " ")).toBe(true);
  });
});
