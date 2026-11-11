import { Trans } from "@lingui/react/macro";
import type { Task } from "@/api/schemas";
import { ProgressBar } from "@/components/ProgressBar";
import { formatCount } from "@/i18n/format";
import { useAdminLabels } from "./useAdminLabels";

/**
 * A task's work. While the task runs, it shows what it's doing, with
 * "37 of 120" over a thin bar when there's a count. Otherwise it shows a muted
 * "18,489 left", or nothing when nothing is left.
 */
export function TaskProgress({ task }: { task: Task }) {
  const { doing } = useAdminLabels();
  const p = task.progress;
  if (!p) {
    if (task.pendingCount === 0) return null;
    const left = formatCount(task.pendingCount);
    return (
      <span data-part="task-left" className="text-text-muted">
        <Trans>{left} left</Trans>
      </span>
    );
  }
  const done = formatCount(p.done);
  const total = formatCount(p.total);
  return (
    <div data-part="task-progress" className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-2 whitespace-nowrap text-xs">
        <span>{doing(task)}</span>
        {p.total > 0 && (
          <span className="text-text-muted tabular-nums">
            <Trans>
              {done} of {total}
            </Trans>
          </span>
        )}
      </div>
      {p.total > 0 && <ProgressBar value={p.done / p.total} className="rounded-full" />}
    </div>
  );
}
