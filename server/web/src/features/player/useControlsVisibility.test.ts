import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useControlsVisibility } from "./useControlsVisibility";

function renderControls(autoHide: boolean) {
  return renderHook((props) => useControlsVisibility(props.autoHide), { initialProps: { autoHide } });
}

describe("useControlsVisibility", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("hides 3 seconds after the last input while playing, as every player does", () => {
    const { result } = renderControls(true);
    expect(result.current.visible).toBe(true);
    act(() => vi.advanceTimersByTime(2_999));
    expect(result.current.visible).toBe(true);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.visible).toBe(false);

    act(() => result.current.reveal());
    act(() => vi.advanceTimersByTime(2_000));
    act(() => result.current.reveal());
    act(() => vi.advanceTimersByTime(2_999));
    expect(result.current.visible).toBe(true);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.visible).toBe(false);
  });

  it("waits 5 seconds with a remote, which steps along the controls a press at a time", () => {
    const { result } = renderHook(() => useControlsVisibility(true, { remote: true }));
    act(() => vi.advanceTimersByTime(4_999));
    expect(result.current.visible).toBe(true);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.visible).toBe(false);
  });

  it("stays up while paused or with a menu open", () => {
    const { result, rerender } = renderControls(false);
    act(() => vi.advanceTimersByTime(60_000));
    expect(result.current.visible).toBe(true);
    rerender({ autoHide: true });
    act(() => result.current.reveal());
    act(() => vi.advanceTimersByTime(3_000));
    expect(result.current.visible).toBe(false);
  });
});
