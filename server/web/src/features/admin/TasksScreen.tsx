import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { Dialog, DialogTrigger, Separator } from "react-aria-components";
import type { SettingsUpdate, Task } from "@/api/schemas";
import { settingsQueries, useUpdateSettings } from "@/api/settings";
import { taskQueries } from "@/api/tasks";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { Popover } from "@/components/Popover";
import { Section } from "@/components/Section";
import { Select } from "@/components/Select";
import { formatCount, formatDateTime, formatUpcoming } from "@/i18n/format";
import { AdminCell, AdminColumn, AdminPage, AdminTable, AdminTableRow } from "./AdminPage";
import { useRunNotices } from "./runNotices";
import { RunButton, StopButton } from "./TaskButtons";
import { TaskProgress } from "./TaskProgress";
import { useAdminLabels, useHours } from "./useAdminLabels";
import { When } from "./When";

/** The tables' columns: task, last run, next run, progress, action. */
const columns = ["", "w-40", "w-24", "w-48", "w-24"];

/** The action buttons' width, the same in every row. */
const actionWidth = "w-18";

/**
 * The Tasks page. It shows when the nightly tasks run, with Schedule to
 * change that and how many files are worked on at once. Then comes
 * On demand, the work done as it's asked for, shown only while it's busy.
 * Then come the Library and Maintenance tasks. Each row has its last and
 * next run, its progress or how much is left, and Stop or Run now.
 */
export function TasksScreen() {
  const { t } = useLingui();
  const { byOrder } = useAdminLabels();
  const list = useSuspenseQuery(taskQueries.list());
  const tasks = byOrder(list.data.items);
  useRunNotices(tasks);
  // Future times are relative to when the list was fetched.
  const now = list.dataUpdatedAt;
  const busy = tasks.filter((task) => task.section === "onDemand" && (task.running || task.pendingCount > 0));
  return (
    <AdminPage page="tasks" title={t`Tasks`} narrow description={<NightlyWindow />} actions={<ScheduleButton />}>
      {busy.length > 0 && (
        <Section title={<Trans>On demand</Trans>}>
          <AdminTable columns={columns}>
            {busy.map((task) => (
              <TaskRow key={task.id} task={task} now={now} />
            ))}
          </AdminTable>
        </Section>
      )}
      <Section title={<Trans>Library</Trans>}>
        <TaskTable tasks={tasks.filter((t) => t.section === "library")} now={now} />
      </Section>
      <Section title={<Trans>Maintenance</Trans>}>
        <TaskTable tasks={tasks.filter((t) => t.section === "maintenance")} now={now} />
      </Section>
    </AdminPage>
  );
}

/** When the nightly tasks run, in the server's time, such as "Nightly tasks run 2:00 AM to 6:00 AM". */
function NightlyWindow() {
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  const hours = useHours();
  const start = hours[settings.maintenanceStartHour]?.label;
  const end = hours[settings.maintenanceEndHour]?.label;
  return (
    <span data-part="nightly-window">
      <Trans>
        Nightly tasks run {start} to {end}
      </Trans>
    </span>
  );
}

/** Choices from 1 to `most`. */
const upTo = (most: number) =>
  Array.from({ length: most }, (_, i) => ({ id: String(i + 1), label: formatCount(i + 1) }));

/**
 * Schedule, which opens the settings for the nightly window's hours, in
 * the server's time. It also sets how many files at once get preview
 * thumbnails made and intros and credits found. A change saves at once.
 */
function ScheduleButton() {
  const { t } = useLingui();
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  const update = useUpdateSettings();
  const hours = useHours();
  const save = (body: SettingsUpdate) => update.mutate({ body });
  const pending = update.isPending;
  return (
    <DialogTrigger>
      <Button size="sm">
        <Icon name="settings" className="size-4" />
        <Trans>Schedule</Trans>
      </Button>
      <Popover placement="bottom end" className="w-80">
        <Dialog aria-label={t`Schedule`} data-part="schedule" className="flex flex-col gap-4 p-3 outline-none">
          <ScheduleGroup title={<Trans>Nightly window</Trans>}>
            <div className="flex items-center gap-2 text-sm">
              <Select
                label={t`Nightly start`}
                options={hours}
                selected={String(settings.maintenanceStartHour)}
                onChange={(h) => save({ maintenanceStartHour: Number(h) })}
                isDisabled={pending}
              />
              <span>
                <Trans>to</Trans>
              </span>
              <Select
                label={t`Nightly end`}
                options={hours}
                selected={String(settings.maintenanceEndHour)}
                onChange={(h) => save({ maintenanceEndHour: Number(h) })}
                isDisabled={pending}
              />
            </div>
          </ScheduleGroup>
          <Separator className="border-t border-border" />
          <ScheduleGroup title={<Trans>Files at once</Trans>}>
            <AtOnceRow label={<Trans>Preview thumbnails</Trans>}>
              <Select
                label={t`Preview thumbnails at once`}
                options={upTo(8)}
                selected={String(settings.previewsAtOnce)}
                onChange={(n) => save({ previewsAtOnce: Number(n) })}
                isDisabled={pending}
              />
            </AtOnceRow>
            <AtOnceRow label={<Trans>Intro and credits</Trans>}>
              <Select
                label={t`Intro and credits at once`}
                options={upTo(4)}
                selected={String(settings.markersAtOnce)}
                onChange={(n) => save({ markersAtOnce: Number(n) })}
                isDisabled={pending}
              />
            </AtOnceRow>
          </ScheduleGroup>
          <FormError error={update.error} />
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}

/** A part of Schedule under its muted title. */
function ScheduleGroup({ title, children }: { title: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-xs font-semibold text-text-muted">{title}</p>
      {children}
    </div>
  );
}

/** How many files a task works on at once, shown as its name and then its select. */
function AtOnceRow({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 text-sm">
      <span aria-hidden>{label}</span>
      {children}
    </div>
  );
}

/** A table of scheduled tasks, with its column headers. `now` is when the tasks were read. */
function TaskTable({ tasks, now }: { tasks: Task[]; now: number }) {
  return (
    <AdminTable
      columns={columns}
      head={
        <>
          <AdminColumn>
            <Trans>Task</Trans>
          </AdminColumn>
          <AdminColumn>
            <Trans>Last run</Trans>
          </AdminColumn>
          <AdminColumn>
            <Trans>Next run</Trans>
          </AdminColumn>
          <AdminColumn>
            <Trans>Progress</Trans>
          </AdminColumn>
          <AdminColumn />
        </>
      }
    >
      {tasks.map((task) => (
        <TaskRow key={task.id} task={task} now={now} />
      ))}
    </AdminTable>
  );
}

/**
 * A task's row. It has the task's name, linking to its page when it keeps
 * failures, then its last run, its next run and its progress. Last comes
 * Stop while the task can be stopped, or else Run now.
 */
function TaskRow({ task, now }: { task: Task; now: number }) {
  const { taskName } = useAdminLabels();
  const name = taskName(task);
  return (
    <AdminTableRow part="task">
      <AdminCell>
        {task.failedCount !== undefined ? (
          <Link to="/admin/tasks/$task" params={{ task: task.id }} className="truncate hover:underline">
            {name}
          </Link>
        ) : (
          <span className="truncate">{name}</span>
        )}
      </AdminCell>
      <AdminCell>
        <LastRun task={task} />
      </AdminCell>
      <AdminCell>
        <NextRun task={task} now={now} />
      </AdminCell>
      <AdminCell>
        <TaskProgress task={task} />
      </AdminCell>
      <AdminCell action>
        {task.stoppable ? (
          <StopButton task={task} className={actionWidth} />
        ) : (
          task.section !== "onDemand" && <RunButton task={task} className={actionWidth} />
        )}
      </AdminCell>
    </AdminTableRow>
  );
}

/**
 * When a task last finished.
 *  - "8 hours ago", with the exact time and the summary on hover.
 *  - "Failed 8 hours ago", with the reason on hover.
 *  - A muted "Never".
 *
 * Nothing shows in On demand.
 */
function LastRun({ task }: { task: Task }) {
  const last = task.lastRun;
  if (task.section === "onDemand") return null;
  if (!last) {
    return (
      <span className="text-text-muted">
        <Trans>Never</Trans>
      </span>
    );
  }
  const at = last.finishedAt;
  if (last.error) {
    return (
      <span className="text-danger">
        <Trans>
          Failed <When at={at} detail={last.error} />
        </Trans>
      </span>
    );
  }
  return (
    <span className="text-text-muted">
      <When at={at} detail={last.summary} />
    </span>
  );
}

/**
 * When a task runs next, as seen at `now`.
 *  - "Nightly" for a task that runs in the window, with its next time on
 *    hover.
 *  - For a task that runs every so often, its time of day ("7:14 PM"),
 *    with its weekday or date when it's past the next day.
 *  - A muted "—" when every library is set never to do the task's work.
 *
 * Nothing shows in On demand.
 */
function NextRun({ task, now }: { task: Task; now: number }) {
  if (task.section === "onDemand") return null;
  if (!task.nextRunAt) return <span className="text-text-muted">—</span>;
  return (
    <time dateTime={task.nextRunAt} title={formatDateTime(task.nextRunAt)} className="text-text-muted">
      {task.nightly ? <Trans>Nightly</Trans> : formatUpcoming(task.nextRunAt, now)}
    </time>
  );
}
