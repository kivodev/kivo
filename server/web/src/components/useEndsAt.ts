import { useLingui } from "@lingui/react/macro";
import { useEffect, useState } from "react";
import { formatEndTime } from "@/i18n/format";

/**
 * "Ends at 9:42 PM": when what's left finishes, played from now at `rate`.
 * It moves on with the clock every `everyMs`. It is undefined while what's
 * left isn't known.
 */
export function useEndsAt(remainingMs: number | undefined, rate = 1, everyMs = 60_000): string | undefined {
  const { t } = useLingui();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), everyMs);
    return () => clearInterval(timer);
  }, [everyMs]);
  if (remainingMs === undefined) return undefined;
  const time = formatEndTime(now + Math.max(0, remainingMs) / rate);
  return t`Ends at ${time}`;
}
