import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import type { Task, TaskFailure } from "@/api/schemas";
import { taskQueries, useRetryTask } from "@/api/tasks";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { Section } from "@/components/Section";
import { AdminList, AdminListItem, AdminPage } from "./AdminPage";
import { useRunNotices } from "./runNotices";
import { RunButton, StopButton } from "./TaskButtons";
import { TaskProgress } from "./TaskProgress";
import { useAdminLabels } from "./useAdminLabels";
import { When } from "./When";

const taskRoute = getRouteApi("/authed/admin/tasks/$task");

/**
 * A task's page, for a task that keeps failures. It shows the progress
 * while the task runs, or else how much is left. It offers Stop while the
 * task can be stopped, or else Run now. Then it lists the failed items.
 */
export function TaskScreen() {
  const { task: id } = taskRoute.useParams();
  const { taskName } = useAdminLabels();
  const tasks = useSuspenseQuery(taskQueries.list()).data.items;
  useRunNotices(tasks);
  const task = tasks.find((t) => t.id === id);
  if (!task) return null;
  return (
    <AdminPage
      page="task"
      title={taskName(task)}
      parent={
        <Link to="/admin/tasks" className="hover:text-text">
          <Trans>Tasks</Trans>
        </Link>
      }
      actions={
        task.stoppable ? (
          <StopButton task={task} size="md" />
        ) : (
          task.section !== "onDemand" && <RunButton task={task} size="md" variant="primary" />
        )
      }
    >
      {(task.progress || task.pendingCount > 0) && (
        <div data-part="task-status" className="-mt-2 max-w-80 text-sm">
          <TaskProgress task={task} />
        </div>
      )}
      <Failed task={task} />
    </AdminPage>
  );
}

/** The task's failed items, each with Retry, and Retry all when there's more than one. */
function Failed({ task }: { task: Task }) {
  const failures = useSuspenseQuery(taskQueries.failures(task.id)).data.items;
  const retryAll = useRetryTask();
  return (
    <Section
      title={<Trans>Failed</Trans>}
      action={
        failures.length > 1 && (
          <Button
            size="sm"
            isPending={retryAll.isPending}
            onPress={() => retryAll.mutate({ params: { path: { task: task.id } } })}
          >
            <Trans>Retry all</Trans>
          </Button>
        )
      }
    >
      <FormError error={retryAll.error} />
      {failures.length === 0 ? (
        <p className="text-sm text-text-muted">
          <Trans>Nothing failed</Trans>
        </p>
      ) : (
        <AdminList>
          {failures.map((failure) => (
            <FailedRow key={failure.itemId} task={task} failure={failure} />
          ))}
        </AdminList>
      )}
    </Section>
  );
}

/** A failed item, showing what it is, then why and when it failed, with Retry. */
function FailedRow({ task, failure }: { task: Task; failure: TaskFailure }) {
  const { t } = useLingui();
  const retry = useRetryTask();
  const title = failure.title ?? failure.itemId;
  return (
    <AdminListItem part="task-failure">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex min-w-0 flex-1 basis-72 flex-col gap-0.5">
          <span className="font-medium">{title}</span>
          <span className="text-text-muted">
            {failure.reason} · <When at={failure.failedAt} />
          </span>
        </div>
        <Button
          size="sm"
          aria-label={t`Retry ${title}`}
          isPending={retry.isPending}
          onPress={() => retry.mutate({ params: { path: { task: task.id } }, body: { itemIds: [failure.itemId] } })}
        >
          <Trans>Retry</Trans>
        </Button>
      </div>
      <FormError error={retry.error} />
    </AdminListItem>
  );
}
