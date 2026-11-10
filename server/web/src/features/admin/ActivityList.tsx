import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import type { ActivityEvent } from "@/api/schemas";
import { formatCount, formatEpisodeCode, formatTimeOfDay } from "@/i18n/format";
import { useProblemActions } from "./problemActions";
import { useAdminLabels } from "./useAdminLabels";

type Work = NonNullable<ActivityEvent["play"]>["work"];

/** A work's name in a row, in bold. An episode also shows its "S3 · E4". */
function Name({ work }: { work: Work }) {
  const code =
    work.seasonNumber !== undefined && work.episodeNumber !== undefined
      ? formatEpisodeCode({ seasonNumber: work.seasonNumber, number: work.episodeNumber })
      : undefined;
  return (
    <>
      <strong className="font-semibold">{work.title}</strong>
      {code && ` ${code}`}
    </>
  );
}

/** What follows a row's words, muted: " · Remote · 203.0.113.24". */
function Muted({ parts }: { parts: (string | undefined)[] }) {
  const shown = parts.filter(Boolean);
  return shown.length > 0 ? <span className="text-text-muted">{` · ${shown.join(" · ")}`}</span> : null;
}

const rowLink = "font-semibold hover:underline";

/** An event in words, with a link to what it's about where there's one to follow. */
function EventText({ event }: { event: ActivityEvent }): ReactNode {
  const { t } = useLingui();
  const { taskName } = useAdminLabels();
  const actionOf = useProblemActions();
  const { play, added, signIn, joined, password, task, problem } = event;
  if (play) {
    const user = play.username;
    const name = <Name work={play.work} />;
    const where = [play.clientName, play.local ? undefined : t`Remote`];
    return event.kind === "playFinished" ? (
      <>
        <Trans>
          {user} finished {name}
        </Trans>
        <Muted parts={where} />
      </>
    ) : (
      <>
        <Trans>
          {user} started {name}
        </Trans>
        <Muted parts={where} />
      </>
    );
  }
  if (added) {
    const library = added.libraryName;
    const name = <Name work={added.work} />;
    if (added.work.kind === "episode" && added.count > 1) {
      const episodes = plural(added.count, { one: "# episode", other: "# episodes" });
      const series = <strong className="font-semibold">{added.work.title}</strong>;
      return (
        <Trans>
          {episodes} of {series} added to {library}
        </Trans>
      );
    }
    if (added.work.kind === "video" && added.count > 1) {
      const videos = plural(added.count, { one: "# video", other: "# videos" });
      return (
        <Trans>
          {videos} added to {library}
        </Trans>
      );
    }
    return (
      <Trans>
        {name} added to {library}
      </Trans>
    );
  }
  if (signIn && event.kind === "signInFailed") {
    const count = signIn.count ?? 1;
    const user = signIn.username;
    return (
      <>
        <span className="text-warning">
          {user
            ? plural(count, { one: `# failed sign-in for ${user}`, other: `# failed sign-ins for ${user}` })
            : plural(count, {
                one: "# failed sign-in for an unknown user",
                other: "# failed sign-ins for an unknown user",
              })}
        </span>
        <Muted parts={[signIn.address]} />
      </>
    );
  }
  if (signIn) {
    const user = signIn.username;
    const device = <strong className="font-semibold">{signIn.clientName}</strong>;
    return (
      <>
        {signIn.clientName ? (
          <Trans>
            {user} signed in on {device}
          </Trans>
        ) : (
          <Trans>{user} signed in</Trans>
        )}
        <Muted parts={[signIn.local ? undefined : t`Remote`, signIn.address]} />
      </>
    );
  }
  if (joined) {
    const user = <strong className="font-semibold">{joined.username}</strong>;
    const admin = joined.addedBy;
    return admin ? (
      <Trans>
        {admin} added {user}
      </Trans>
    ) : (
      <Trans>{user} joined from an invite</Trans>
    );
  }
  if (password) {
    const user = <strong className="font-semibold">{password.username}</strong>;
    const admin = password.changedBy;
    if (password.commandLine) return <Trans>{user}'s password was reset from the command line</Trans>;
    return admin ? (
      <Trans>
        {admin} changed {user}'s password
      </Trans>
    ) : (
      <>
        <Trans>{user} changed their password</Trans>
        <Muted parts={[password.address]} />
      </>
    );
  }
  if (task) {
    const name = taskName({ id: task.task });
    const failed = formatCount(task.failedCount);
    return (
      <>
        {task.error ? (
          <span className="text-danger">
            <Trans>{name} failed</Trans>
          </span>
        ) : (
          name
        )}
        <Muted parts={[task.error ?? task.summary]} />
        {task.failedCount > 0 && (
          <>
            {" · "}
            <Link to="/admin/tasks/$task" params={{ task: task.task }} className={`${rowLink} text-warning`}>
              <Trans>{failed} failed</Trans>
            </Link>
          </>
        )}
      </>
    );
  }
  if (problem) {
    const action = actionOf(problem.code);
    return (
      <>
        <span className={problem.severity === "error" ? "text-danger" : "text-warning"}>{problem.message}</span>
        {action?.page && (
          <>
            {" · "}
            <Link to={action.page} search={action.search ?? {}} className={rowLink}>
              {action.link ?? t`Fix`}
            </Link>
          </>
        )}
      </>
    );
  }
  return null;
}

/**
 * Events in a bordered list, newest first, each showing when and then
 * what. `dated` shows the date of an event from another day, when nothing
 * above it gives the date.
 */
export function ActivityList({ events, dated = false }: { events: ActivityEvent[]; dated?: boolean }) {
  const { i18n } = useLingui();
  const today = new Date().toDateString();
  // An event from a day other than today shows its date, when nothing else
  // says which day it was.
  const whenOf = (iso: string) =>
    dated && new Date(iso).toDateString() !== today
      ? i18n.date(iso, { month: "short", day: "numeric" })
      : formatTimeOfDay(iso);
  return (
    <ol className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-surface">
      {events.map((e) => (
        <li key={e.id} data-part="activity-event" data-kind={e.kind} className="flex gap-4 px-4 py-2.5 text-sm">
          <time dateTime={e.occurredAt} className="w-18 shrink-0 text-text-subtle tabular-nums">
            {whenOf(e.occurredAt)}
          </time>
          <span className="min-w-0 flex-1">
            <EventText event={e} />
          </span>
        </li>
      ))}
    </ol>
  );
}
