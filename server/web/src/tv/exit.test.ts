import { afterEach, expect, it, vi } from "vitest";
import { exitApp } from "./exit";

afterEach(() => {
  vi.unstubAllGlobals();
});

it("uses Tizen's own exit where the page has it", () => {
  const close = vi.spyOn(window, "close").mockImplementation(() => {});
  const exit = vi.fn();
  vi.stubGlobal("tizen", { application: { getCurrentApplication: () => ({ exit }) } });
  exitApp();
  expect(exit).toHaveBeenCalledOnce();
  expect(close).not.toHaveBeenCalled();
});

it("closes the window elsewhere", () => {
  const close = vi.spyOn(window, "close").mockImplementation(() => {});
  exitApp();
  expect(close).toHaveBeenCalledOnce();
});
