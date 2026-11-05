import { useLingui } from "@lingui/react/macro";
import type { Task } from "@/api/schemas";

/** Names for the states the admin pages show. Unknown values show as sent. */
export function useAdminLabels() {
  const { t } = useLingui();
  // A library scan's step, or a download's step, while it's in progress.
  const stepNames: Record<string, string> = {
    walk: t`Finding files`,
    probe: t`Reading files`,
    identify: t`Matching`,
    extractSubtitles: t`Extracting subtitles`,
    convert: t`Converting`,
  };
  // What a task is doing while it runs, when its progress names no step.
  const doingNames: Record<string, string> = {
    "refresh-metadata": t`Refreshing`,
    "make-previews": t`Generating`,
    "find-markers": t`Analyzing`,
    "prune-gone-files": t`Removing`,
    "prune-previews": t`Cleaning up`,
    "prune-markers": t`Cleaning up`,
    "prune-subtitles": t`Cleaning up`,
    "prune-downloads": t`Cleaning up`,
    "prune-provider-cache": t`Cleaning up`,
    "prune-activity": t`Cleaning up`,
    "make-backup": t`Backing up`,
    "optimize-database": t`Optimizing`,
    "prepare-downloads": t`Preparing`,
    "extract-subtitles": t`Extracting`,
    "library-changes": t`Removing`,
  };
  // A task's name on Tasks, in the order Tasks lists them: Library, Maintenance, Activity.
  const taskNames: Record<string, string> = {
    "scan-libraries": t`Scan all libraries`,
    "refresh-metadata": t`Refresh metadata`,
    "make-previews": t`Preview thumbnails`,
    "find-markers": t`Intro and credits`,
    "extract-subtitles": t`Extract subtitles`,
    "prune-gone-files": t`Remove missing files`,
    "prune-previews": t`Clean up preview thumbnails`,
    "prune-markers": t`Clean up intro and credits`,
    "prune-subtitles": t`Clean up subtitles`,
    "prune-downloads": t`Clean up downloads`,
    "prune-provider-cache": t`Clean up metadata cache`,
    "prune-activity": t`Clean up activity`,
    "make-backup": t`Back up database`,
    "optimize-database": t`Optimize database`,
    "prepare-downloads": t`Downloads`,
    "library-changes": t`Library changes`,
  };
  const order = Object.keys(taskNames);
  return {
    /** A library scan's step in plain words, saying what it's doing. */
    stepName: (step: string) => stepNames[step] ?? step,
    /** What a running task is doing: its step's word, or else the task's own word. */
    doing: (task: Task) => (task.progress?.step ? stepNames[task.progress.step] : undefined) ?? doingNames[task.id],
    taskName: (task: Pick<Task, "id">) => taskNames[task.id] ?? task.id,
    /** Tasks in the order the Tasks page lists them, with unknown tasks last. */
    byOrder: (tasks: Task[]) => tasks.toSorted((a, b) => rank(order, a.id) - rank(order, b.id)),
  };
}

/** Where an ID falls in order, or after all of them when it's not there. */
function rank(order: string[], id: string): number {
  const i = order.indexOf(id);
  return i < 0 ? order.length : i;
}

/** The hours of the server's day as choices, such as "2:00 AM", for the maintenance window. */
export function useHours() {
  const { i18n } = useLingui();
  return Array.from({ length: 24 }, (_, h) => ({
    id: String(h),
    label: i18n.date(new Date(Date.UTC(2023, 0, 1, h)), { hour: "numeric", minute: "2-digit", timeZone: "UTC" }),
  }));
}
