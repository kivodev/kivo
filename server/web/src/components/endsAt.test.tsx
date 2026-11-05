import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { formatEndTime } from "@/i18n/format";
import { useEndsAt } from "./endsAt";

const minute = 60_000;
const start = new Date(2026, 10, 1, 20, 8, 0).getTime();

function renderEndsAt(remainingMs: number | undefined, rate?: number, everyMs?: number) {
  return renderHook((props) => useEndsAt(props.remainingMs, rate, everyMs), {
    initialProps: { remainingMs },
    wrapper: ({ children }: { children: ReactNode }) => <I18nProvider i18n={i18n}>{children}</I18nProvider>,
  });
}

describe("useEndsAt", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(start);
  });
  afterEach(() => vi.useRealTimers());

  it("says when what's left ends, in the browser's own clock", () => {
    const { result } = renderEndsAt(94 * minute);
    expect(result.current).toBe(`Ends at ${formatEndTime(start + 94 * minute)}`);
    expect(formatEndTime(start + 94 * minute)).toBe(
      new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(start + 94 * minute),
    );
  });

  it("divides what's left by the playback speed", () => {
    const { result } = renderEndsAt(60 * minute, 2);
    expect(result.current).toBe(`Ends at ${formatEndTime(start + 30 * minute)}`);
  });

  it("moves on with the clock while what's left stays the same, as when paused", () => {
    const { result } = renderEndsAt(10 * minute);
    act(() => vi.advanceTimersByTime(minute));
    expect(result.current).toBe(`Ends at ${formatEndTime(start + 11 * minute)}`);
  });

  it("is undefined until what's left is known", () => {
    expect(renderEndsAt(undefined).result.current).toBeUndefined();
  });
});
