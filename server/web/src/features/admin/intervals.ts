import { plural } from "@lingui/core/macro";
import { useLingui } from "@lingui/react/macro";
import type { Option } from "@/components/Select";

// How often libraries can be rescanned, in minutes.
const rescanChoices = [15, 30, 60, 6 * 60, 12 * 60, 24 * 60];

/** The time between runs, worded to follow "every": "30 minutes", "12 hours", or "hour" for one. */
export function intervalOf(minutes: number): string {
  return minutes % 60 === 0
    ? plural(minutes / 60, { one: "hour", other: "# hours" })
    : plural(minutes, { one: "minute", other: "# minutes" });
}

/** How often something runs, from its minutes, such as "Every 12 hours". */
export function useEvery(): (minutes: number) => string {
  const { t } = useLingui();
  return (minutes) => {
    const interval = intervalOf(minutes);
    return t`Every ${interval}`;
  };
}

/**
 * The choices for how often libraries are rescanned, such as "Every 12
 * hours". Each one's ID is its minutes. `current` is listed too when it's
 * none of the usual choices.
 */
export function useRescanOptions(current: number): Option[] {
  const every = useEvery();
  const all = rescanChoices.includes(current) ? rescanChoices : [...rescanChoices, current].toSorted((a, b) => a - b);
  return all.map((minutes) => ({ id: String(minutes), label: every(minutes) }));
}
