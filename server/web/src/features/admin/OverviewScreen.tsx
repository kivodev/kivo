import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { activityQueries } from "@/api/activity";
import { libraryQueries } from "@/api/libraries";
import { playQueries } from "@/api/playback";
import type { Play } from "@/api/schemas";
import { serverQueries } from "@/api/server";
import { taskQueries } from "@/api/tasks";
import { Banner } from "@/components/Banner";
import { Button } from "@/components/Button";
import { Section } from "@/components/Section";
import { formatBitrate, formatCount } from "@/i18n/format";
import { playMethod } from "@/i18n/playback";
import { readStored, writeStored } from "@/lib/storage";
import { ActivityList } from "./ActivityList";
import { AdminList, AdminListItem, AdminPage } from "./AdminPage";
import { PlayCards } from "./PlayCards";
import { ScanLine } from "./ScanLine";
import { useAdminLabels } from "./useAdminLabels";

const actionLink = "shrink-0 text-sm font-semibold text-accent hover:underline";

// The release whose notice this browser dismissed.
const dismissedReleaseKey = "kivo.dismissedRelease";

/**
 * A newer Kivo release, as the server's daily check found it, with its
 * release notes. Dismissing it hides it until the next one.
 */
function NewRelease() {
  const release = useQuery(serverQueries.info()).data?.newRelease;
  const [dismissed, setDismissed] = useState(() => readStored(dismissedReleaseKey));
  if (!release || release.version === dismissed) return null;
  const version = release.version;
  return (
    <Banner
      action={
        <>
          <a href={release.url} target="_blank" rel="noreferrer" className={actionLink}>
            <Trans>Release notes</Trans>
          </a>
          <Button
            size="sm"
            variant="ghost"
            onPress={() => {
              writeStored(dismissedReleaseKey, version);
              setDismissed(version);
            }}
          >
            <Trans>Dismiss</Trans>
          </Button>
        </>
      }
    >
      <Trans>Kivo {version} is available</Trans>
    </Banner>
  );
}

/**
 * The number of plays, how many are transcoded, and the bitrate they send,
 * such as "2 playing · 1 transcoding · 12 Mb/s".
 */
function usePlaysSummary(plays: Play[]): string {
  const { t } = useLingui();
  const playing = formatCount(plays.length);
  const transcoding = plays.filter((p) => playMethod(p.tracks) === "transcode").length;
  const sending = plays.reduce((sum, p) => sum + (p.bitrateBps ?? 0), 0);
  const count = formatCount(transcoding);
  return [
    t`${playing} playing`,
    transcoding > 0 ? t`${count} transcoding` : undefined,
    sending > 0 ? formatBitrate(sending) : undefined,
  ]
    .filter(Boolean)
    .join(" · ");
}

/** The plays as cards, which open their details, with how many and what they send beside the title. */
function NowPlaying() {
  const plays = useQuery(playQueries.active()).data?.items;
  const summary = usePlaysSummary(plays ?? []);
  return (
    <Section
      title={
        <>
          <Trans>Now playing</Trans>
          {plays && plays.length > 0 && (
            <span data-part="plays-summary" className="ml-2 text-sm font-normal text-text-muted">
              {summary}
            </span>
          )}
        </>
      }
    >
      {/* A line held while plays load, so the sections below stay put. */}
      {plays === undefined && <div aria-hidden="true" className="h-5 w-40 animate-pulse rounded-sm bg-placeholder" />}
      {plays && (
        <PlayCards
          plays={plays}
          empty={
            <p className="text-sm text-text-muted">
              <Trans>Nothing playing</Trans>
            </p>
          }
        />
      )}
    </Section>
  );
}

/**
 * Each library that is scanning, and each task running a counted run, gets
 * a line and a bar. A library's scan shows on its own line, not under Scan
 * all libraries. Nothing shows while nothing runs.
 */
function WorkingNow() {
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const tasks = useQuery(taskQueries.list()).data?.items ?? [];
  const { taskName, doing } = useAdminLabels();
  const scanning = libraries.flatMap(({ scan: status, ...library }) =>
    status?.state === "scanning" ? [{ library, status }] : [],
  );
  const running = tasks.filter((t) => t.running && t.id !== "scan-libraries" && (t.progress?.total ?? 0) > 0);
  if (scanning.length === 0 && running.length === 0) return null;
  return (
    <Section title={<Trans>Working now</Trans>}>
      <AdminList>
        {scanning.map(({ library, status }) => {
          const name = library.name;
          return (
            <AdminListItem key={library.id}>
              <ScanLine name={<Trans>Scanning {name}</Trans>} progress={status.progress} />
            </AdminListItem>
          );
        })}
        {running.map((task) => {
          const step = doing(task);
          return (
            <AdminListItem key={task.id}>
              <ScanLine
                name={
                  <>
                    {taskName(task)}
                    {step && <span className="text-text-muted">{` · ${step}`}</span>}
                  </>
                }
                progress={task.progress && { done: task.progress.done, total: task.progress.total }}
              />
            </AdminListItem>
          );
        })}
      </AdminList>
    </Section>
  );
}

/** The latest events, with See all for the Activity page. Nothing shows before there are any. */
function LatestActivity() {
  const events = useSuspenseQuery(activityQueries.latest()).data.items;
  if (events.length === 0) return null;
  return (
    <Section
      title={<Trans>Activity</Trans>}
      action={
        <Link to="/admin/activity" className="text-sm text-text-muted touch-target hover:text-text">
          <Trans>See all</Trans>
        </Link>
      }
    >
      <ActivityList events={events} dated />
    </Section>
  );
}

/**
 * The admin home, in one column. It shows a newer release when one is out,
 * then what's playing, what's working, and the latest activity.
 */
export function OverviewScreen() {
  const { t } = useLingui();
  return (
    <AdminPage page="overview" title={t`Overview`}>
      <NewRelease />
      <NowPlaying />
      <WorkingNow />
      <LatestActivity />
    </AdminPage>
  );
}
