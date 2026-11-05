import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { apiPaths, invalidatePaths } from "./paths";
import type { TaskName } from "./schemas";

export const taskQueries = {
  /**
   * The Tasks page's rows, each with its last and next run, progress and
   * failures. The data goes stale at once, so opening or returning to a
   * Tasks page asks again. Run now, Stop and tasksChanged events refetch it too.
   * Nothing polls.
   */
  list: () => $api.queryOptions("get", "/api/v1/tasks", undefined, { staleTime: 0 }),
  /** A task's items that failed, newest first. */
  failures: (task: TaskName) =>
    $api.queryOptions("get", "/api/v1/tasks/{task}/failures", { params: { path: { task } } }, { staleTime: 0 }),
};

function useInvalidateTasks() {
  const queryClient = useQueryClient();
  return () => invalidatePaths(queryClient, [apiPaths.tasks, apiPaths.taskFailures]);
}

/** Run a scheduled task now, outside the maintenance window too. */
export function useRunTask() {
  const onSuccess = useInvalidateTasks();
  return $api.useMutation("post", "/api/v1/tasks/{task}/run", { onSuccess });
}

/** Stop a task. Its items in flight stop, and what's left waits for its next run. */
export function useStopTask() {
  const onSuccess = useInvalidateTasks();
  return $api.useMutation("post", "/api/v1/tasks/{task}/stop", { onSuccess });
}

/** Retry a task's failed items, either those named or every one. */
export function useRetryTask() {
  const onSuccess = useInvalidateTasks();
  return $api.useMutation("post", "/api/v1/tasks/{task}/retry", { onSuccess });
}
