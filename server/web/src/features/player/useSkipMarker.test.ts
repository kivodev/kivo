import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PlayStarted } from "@/api/schemas";
import { useSkipMarker } from "./useSkipMarker";

const play = {
  chapters: [],
  markers: [{ kind: "intro", startMs: 60_000, endMs: 150_000, source: "chapter" }],
} as unknown as PlayStarted;

/** The hook at a position, with the controls hidden unless `controls`. */
function renderSkip(positionMs: number, controls = false) {
  return renderHook(
    (props) => useSkipMarker(play, true, props.positionMs, { controls: props.controls, upNext: false }),
    {
      initialProps: { positionMs, controls },
    },
  );
}

describe("useSkipMarker", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("hides 10 seconds after entering the marker, even while paused", () => {
    const { result, rerender } = renderSkip(30_000);
    expect(result.current.marker).toBeUndefined();
    rerender({ positionMs: 100_000, controls: false });
    expect(result.current.marker?.kind).toBe("intro");
    // While paused, the position stays and the clock runs.
    act(() => vi.advanceTimersByTime(9_999));
    expect(result.current.marker?.kind).toBe("intro");
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.marker).toBeUndefined();
  });

  it("comes back with the controls inside the marker, and goes at its end", () => {
    const { result, rerender } = renderSkip(60_000);
    act(() => vi.advanceTimersByTime(10_000));
    expect(result.current.marker).toBeUndefined();
    rerender({ positionMs: 80_000, controls: true });
    expect(result.current.marker?.kind).toBe("intro");
    rerender({ positionMs: 81_000, controls: false });
    expect(result.current.marker).toBeUndefined();
    rerender({ positionMs: 150_000, controls: true });
    expect(result.current.marker).toBeUndefined();
  });

  it("stays away once pressed, and shows again on a seek back into the marker", () => {
    const { result, rerender } = renderSkip(60_000, true);
    act(() => result.current.skip());
    expect(result.current.marker).toBeUndefined();
    rerender({ positionMs: 150_000, controls: true });
    rerender({ positionMs: 140_000, controls: false });
    expect(result.current.marker?.kind).toBe("intro");
    act(() => vi.advanceTimersByTime(10_000));
    expect(result.current.marker).toBeUndefined();
  });
});
