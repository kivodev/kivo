import { useLingui } from "@lingui/react/macro";
import type { Option } from "@/components/Select";
import { formatCount } from "@/i18n/format";

// The select's ID for no limit.
const noLimit = "none";

/** The select's ID for a count limit. It is the number, or `none` for no limit (0 or absent). */
export const limitKey = (limit: number | undefined): string => (limit ? String(limit) : noLimit);

/** The limit a select's ID stands for. It is a number, or undefined for no limit. */
export const limitOf = (id: string): number | undefined => (id === noLimit ? undefined : Number(id));

/**
 * The choices for a count limit, such as simultaneous streams. They are No
 * limit, then 1 to 10, and also `current` when it's more than 10.
 */
export function useLimitOptions(current: number | undefined): Option[] {
  const { t } = useLingui();
  const counts = Array.from({ length: 10 }, (_, i) => i + 1);
  if (current && current > 10) counts.push(current);
  return [{ id: noLimit, label: t`No limit` }, ...counts.map((n) => ({ id: String(n), label: formatCount(n) }))];
}
