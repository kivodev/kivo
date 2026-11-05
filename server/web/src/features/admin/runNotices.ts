import { useLingui } from "@lingui/react/macro";
import { useEffect } from "react";
import type { Task } from "@/api/schemas";
import { toast } from "@/components/Toast";
import { useAdminLabels } from "./useAdminLabels";

/**
 * The runs started from Run now, by task. Each holds when the task's last
 * run had finished at the time this run was started.
 */
const started = new Map<string, string | undefined>();

/** Watches a run started from Run now, to report it when it ends. */
export function watchRun(task: Task) {
  started.set(task.id, task.lastRun?.finishedAt);
}

/** Stops watching a task's run. The run was stopped, so its end needs no notice. */
export function unwatchRun(task: Task) {
  started.delete(task.id);
}

/**
 * Shows a notice for each watched run that has ended, saying what it did
 * or why it failed. A run that nobody started from Run now ends quietly.
 */
export function useRunNotices(tasks: Task[]) {
  const { t } = useLingui();
  const { taskName } = useAdminLabels();
  useEffect(() => {
    for (const task of tasks) {
      const last = task.lastRun;
      if (!started.has(task.id) || task.running || !last || last.finishedAt === started.get(task.id)) {
        continue;
      }
      started.delete(task.id);
      const name = taskName(task);
      toast(
        last.error
          ? { title: t`${name} failed`, detail: last.error, tone: "danger" }
          : { title: t`${name} finished`, detail: last.summary },
      );
    }
  }, [tasks, taskName, t]);
}
