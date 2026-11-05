import { useState } from "react";
import { formatDateTime, formatWhen } from "@/i18n/format";

/**
 * When something happened, as "3 hours ago" or as a date after a week.
 * The exact time shows on hover. `detail`, such as a run's summary, goes
 * under it.
 */
export function When({ at, detail }: { at: string; detail?: string }) {
  const [now] = useState(Date.now);
  const exact = formatDateTime(at);
  return (
    <time dateTime={at} title={detail ? `${exact}\n${detail}` : exact}>
      {formatWhen(at, now)}
    </time>
  );
}
