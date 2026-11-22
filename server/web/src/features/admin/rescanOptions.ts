import { plural } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";
import type { Option } from "@/components/Select";

// How often libraries can be rescanned, in minutes.
const choices = [15, 30, 60, 6 * 60, 12 * 60, 24 * 60];

/** The time between rescans, worded to follow "every": "30 minutes", "12 hours", or "hour" for one. */
export function rescanInterval(minutes: number): string {
  return minutes % 60 === 0
    ? plural(minutes / 60, { one: "hour", other: "# hours" })
    : plural(minutes, { one: "minute", other: "# minutes" });
}

/**
 * The choices for how often libraries are rescanned, such as "Every 12
 * hours". Each one's ID is its minutes. `current` is listed too when it's
 * none of the usual choices.
 */
export function useRescanOptions(current: number): Option[] {
  const { t } = useLingui();
  const all = choices.includes(current) ? choices : [...choices, current].toSorted((a, b) => a - b);
  return all.map((minutes) => {
    const interval = rescanInterval(minutes);
    return { id: String(minutes), label: t`Every ${interval}` };
  });
}
