import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { Button as AriaButton, Dialog, DialogTrigger, Separator } from "react-aria-components";
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
import { AdminCell, AdminColumn, AdminPage, AdminTable, AdminTableRow, mutedLink } from "./AdminPage";
import { useEvery, useRescanOptions } from "./intervals";
import { useRunNotices } from "./runNotices";
import { RunButton, StopButton } from "./TaskButtons";
import { TaskProgress } from "./TaskProgress";
import { useAdminLabels, useHours } from "./useAdminLabels";
import { When } from "./When";

/** The tables' columns: task, schedule, last run, next run, action. */
const columns = ["", "w-34", "w-68", "w-24", "w-25"];

/** The action buttons' width, the same in every row. */
const actionWidth = "w-18";

/**
 * The Tasks page. It shows when the nightly tasks run, with Schedule to
 * change that, how often libraries are rescanned and how many files are
 * worked on at once. Then comes On demand, the work done as it's asked
 * for, shown only while it's busy. Then come the Library and Maintenance
 * tasks. Each row has its schedule, which opens Schedule, its last run
 * or its progress while it runs, its next run, and Stop or Run now.
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
 * the server's time. It also sets how often libraries are rescanned, and
 * how many files at once get preview thumbnails made and intros and
 * credits found. A change saves at once.
 */
function Schedule({
  children,
}: {
  /** The button that opens it. */
  children: ReactNode;
}) {
  const { t } = useLingui();
  const settings = useSuspenseQuery(settingsQueries.current()).data;
  const update = useUpdateSettings();
  const hours = useHours();
  const rescans = useRescanOptions(settings.rescanEveryMinutes);
  const save = (body: SettingsUpdate) => update.mutate({ body });
  const pending = update.isPending;
  return (
    <DialogTrigger>
      {children}
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
          <ScheduleGroup title={<Trans>New files</Trans>}>
            <ScheduleRow label={<Trans>Rescan libraries</Trans>}>
              <Select
                label={t`Rescan libraries`}
                options={rescans}
                selected={String(settings.rescanEveryMinutes)}
                onChange={(minutes) => save({ rescanEveryMinutes: Number(minutes) })}
                isDisabled={pending}
              />
            </ScheduleRow>
            <p className="text-xs text-text-muted">
              <Trans>Finds files the folder watcher misses.</Trans>
            </p>
          </ScheduleGroup>
          <Separator className="border-t border-border" />
          <ScheduleGroup title={<Trans>Files at once</Trans>}>
            <ScheduleRow label={<Trans>Preview thumbnails</Trans>}>
              <Select
                label={t`Preview thumbnails at once`}
                options={upTo(8)}
                selected={String(settings.previewsAtOnce)}
                onChange={(n) => save({ previewsAtOnce: Number(n) })}
                isDisabled={pending}
              />
            </ScheduleRow>
            <ScheduleRow label={<Trans>Intro and credits</Trans>}>
              <Select
                label={t`Intro and credits at once`}
                options={upTo(4)}
                selected={String(settings.markersAtOnce)}
                onChange={(n) => save({ markersAtOnce: Number(n) })}
                isDisabled={pending}
              />
            </ScheduleRow>
          </ScheduleGroup>
          <FormError error={update.error} />
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}

/** The page's button for Schedule. */
function ScheduleButton() {
  return (
    <Schedule>
      <Button size="sm">
        <Icon name="settings" className="size-4" />
        <Trans>Schedule</Trans>
      </Button>
    </Schedule>
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

/** A setting of Schedule, shown as its name and then its select. */
function ScheduleRow({ label, children }: { label: ReactNode; children: ReactNode }) {
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
            <Trans>Schedule</Trans>
          </AdminColumn>
          <AdminColumn>
            <Trans>Last run</Trans>
          </AdminColumn>
          <AdminColumn>
            <Trans>Next run</Trans>
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
 * failures, then its schedule, its last run or its progress, and its
 * next run. Last comes Stop while the task can be stopped, or else Run now.
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
        <TaskSchedule task={task} />
      </AdminCell>
      <AdminCell>
        <LastRun task={task} />
      </AdminCell>
      <AdminCell>
        <NextRun task={task} now={now} />
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
 * A task's progress while it runs, and otherwise when it last finished.
 * What's left for its next run follows on the same line, as in "8 hours
 * ago · 18,489 left". On demand has no runs, so there it is the progress
 * or what's left alone.
 */
function LastRun({ task }: { task: Task }) {
  if (task.progress || task.section === "onDemand") return <TaskProgress task={task} />;
  return (
    <>
      <Finished task={task} />
      {task.pendingCount > 0 && (
        <span className="text-text-muted">
          {" · "}
          <TaskProgress task={task} />
        </span>
      )}
    </>
  );
}

/**
 * When a task last finished.
 *  - "8 hours ago", with the exact time and the summary on hover.
 *  - "Failed 8 hours ago", with the reason on hover.
 *  - A muted "Never".
 */
function Finished({ task }: { task: Task }) {
  const last = task.lastRun;
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
 * How a task is scheduled, in Schedule's own words, which it opens.
 *  - "Nightly" for a task that runs in the window.
 *  - "Every 12 hours" for a task that runs every so often.
 *  - A muted "—" for a task that says neither.
 *
 * Nothing shows in On demand.
 */
function TaskSchedule({ task }: { task: Task }) {
  const { t } = useLingui();
  const every = useEvery();
  if (task.section === "onDemand") return null;
  const schedule = task.nightly ? t`Nightly` : task.everyMinutes ? every(task.everyMinutes) : undefined;
  if (!schedule) return <span className="text-text-muted">—</span>;
  return (
    <Schedule>
      <AriaButton className={`cursor-pointer text-text-muted ${mutedLink}`}>{schedule}</AriaButton>
    </Schedule>
  );
}

/**
 * When a task runs next, as seen at `now`: its time of day ("7:14 PM"),
 * with its weekday or date when it's past the next day. The exact time
 * shows on hover. A muted "—" shows when every library is set never to do
 * the task's work.
 *
 * Nothing shows in On demand.
 */
function NextRun({ task, now }: { task: Task; now: number }) {
  if (task.section === "onDemand") return null;
  if (!task.nextRunAt) return <span className="text-text-muted">—</span>;
  return (
    <time dateTime={task.nextRunAt} title={formatDateTime(task.nextRunAt)} className="text-text-muted">
      {formatUpcoming(task.nextRunAt, now)}
    </time>
  );
}
