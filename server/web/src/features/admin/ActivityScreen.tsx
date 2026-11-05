import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { useDeferredValue } from "react";
import { activityQueries } from "@/api/activity";
import type { ActivityEvent, ActivityGroup } from "@/api/schemas";
import { Button } from "@/components/Button";
import { EmptyState } from "@/components/EmptyState";
import { Select } from "@/components/Select";
import { ActivityList } from "./ActivityList";
import { AdminPage } from "./AdminPage";

const activityRoute = getRouteApi("/authed/admin/activity");

// The Show filter's choice for every event.
const everything = "everything";

/** Events by the day they happened on, in the viewer's zone, newest first. */
function byDay(events: ActivityEvent[]): { day: string; events: ActivityEvent[] }[] {
  const days: { day: string; events: ActivityEvent[] }[] = [];
  for (const e of events) {
    const day = new Date(e.occurredAt).toDateString();
    const last = days.at(-1);
    if (last?.day === day) last.events.push(e);
    else days.push({ day, events: [e] });
  }
  return days;
}

/**
 * The Activity page. It shows what happened on the server, newest first,
 * grouped by day ("Today", "Yesterday", then dates), a page at a time. The
 * Show filter picks plays, root works added, sign-ins, or the server's own
 * events.
 */
export function ActivityScreen() {
  const { t, i18n } = useLingui();
  // The events for another filter replace these once they arrive.
  const show = useDeferredValue(activityRoute.useSearch().show);
  const navigate = activityRoute.useNavigate();
  const activity = useSuspenseInfiniteQuery(activityQueries.list(show));
  const events = activity.data.pages.flatMap((p) => p.items);
  const shows: { id: ActivityGroup | typeof everything; label: string }[] = [
    { id: everything, label: t`Everything` },
    { id: "plays", label: t`Plays` },
    { id: "added", label: t`Added` },
    { id: "signIns", label: t`Sign-ins` },
    { id: "server", label: t`Server` },
  ];
  const now = new Date();
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1).toDateString();
  const dayName = (day: string) =>
    day === now.toDateString()
      ? t`Today`
      : day === yesterday
        ? t`Yesterday`
        : i18n.date(new Date(day), { weekday: "long", month: "long", day: "numeric" });

  return (
    <AdminPage
      page="activity"
      title={t`Activity`}
      actions={
        <Select
          variant="inline"
          label={t`Show`}
          options={shows}
          selected={show ?? everything}
          onChange={(id) =>
            void navigate({ search: { show: id === everything ? undefined : (id as ActivityGroup) }, replace: true })
          }
        />
      }
    >
      {events.length === 0 ? (
        <EmptyState title={t`Nothing yet`} />
      ) : (
        byDay(events).map(({ day, events: onDay }) => (
          <section key={day} data-part="activity-day" className="flex flex-col gap-2.5">
            <h2 className="text-sm font-semibold text-text-muted">{dayName(day)}</h2>
            <ActivityList events={onDay} />
          </section>
        ))
      )}
      {activity.hasNextPage && (
        <Button
          className="self-center"
          isPending={activity.isFetchingNextPage}
          onPress={() => void activity.fetchNextPage()}
        >
          <Trans>Show more</Trans>
        </Button>
      )}
    </AdminPage>
  );
}
