import { afterEach, expect, it, vi } from "vitest";
import { tvMotion } from "./motion";

const press = (key: string, repeat = false) =>
  window.dispatchEvent(new KeyboardEvent("keydown", { key, repeat, bubbles: true }));

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("glides for an arrow just pressed, and jumps for its repeats", () => {
  press("ArrowRight");
  expect(tvMotion()).toBe("smooth");
  press("ArrowRight", true);
  expect(tvMotion()).toBe("instant");
  press("ArrowDown");
  expect(tvMotion()).toBe("smooth");
});

it("jumps for a move no arrow made, as Back restoring focus", () => {
  vi.useFakeTimers({ toFake: ["performance"] });
  press("ArrowLeft");
  vi.advanceTimersByTime(1000);
  expect(tvMotion()).toBe("instant");
});

it("jumps with reduced motion", () => {
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: query.includes("reduce") }));
  press("ArrowUp");
  expect(tvMotion()).toBe("instant");
});
