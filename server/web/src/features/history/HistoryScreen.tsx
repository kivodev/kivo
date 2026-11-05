import { Trans, useLingui } from "@lingui/react/macro";
import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
  useQuery,
  useSuspenseInfiniteQuery,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useDeferredValue } from "react";
import { historyQueries } from "@/api/history";
import type { PlayEvent, PlayEventFeed } from "@/api/schemas";
import { userQueries } from "@/api/users";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { LoadError } from "@/components/LoadError";
import { Select } from "@/components/Select";
import { formatClock, formatDateTime, formatEpisodeCode } from "@/i18n/format";
import { TvFocusable, TvFocusGroup, useTvEmptyScreen } from "@/tv/focus";

const serverHistoryRoute = getRouteApi("/authed/admin/history");

// The user picker's ID for every user.
const allUsers = "all";

/** Your watch history, from the user menu. */
export function HistoryScreen() {
  const history = useInfiniteQuery(historyQueries.mine());
  return (
    <div className="px-4 py-6 sm:px-8">
      <History history={history} />
    </div>
  );
}

/** The server-wide watch history, in the admin area, which can show one user's plays. */
export function ServerHistoryScreen() {
  // Another user's plays replace these once they arrive.
  const userId = useDeferredValue(serverHistoryRoute.useSearch().userId);
  // The page stays blank until the plays and users arrive, like other
  // admin pages, rather than showing the list's placeholders.
  useSuspenseInfiniteQuery(historyQueries.everyone(userId));
  useSuspenseQuery(userQueries.list());
  const history = useInfiniteQuery(historyQueries.everyone(userId));
  return <History history={history} everyone={{ userId }} />;
}

type Props = {
  /** The plays, a page at a time. */
  history: UseInfiniteQueryResult<InfiniteData<PlayEventFeed>>;
  /**
   * Whether this is the server-wide history that admins see, with every
   * user's plays, or the plays of the one user it's filtered to.
   */
  everyone?: { userId: string | undefined };
};

/**
 * Watch history, with plays newest first, each with when it was and how
 * far it got. The server-wide history also says who played each one, and
 * can show one user's plays.
 */
function History({ history, everyone }: Props) {
  const { t } = useLingui();
  useDocumentTitle(t`Watch history`);
  const plays = history.data?.pages.flatMap((p) => p.items) ?? [];
  // With no plays there is nothing to focus, so focus goes to the rail on TV.
  useTvEmptyScreen(history.data !== undefined && plays.length === 0);
  return (
    <section data-part="history" aria-labelledby="history-title" className="flex max-w-page flex-col gap-6">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <h1 id="history-title" className="text-2xl font-semibold">
          <Trans>Watch history</Trans>
        </h1>
        {everyone && <UserPicker userId={everyone.userId} />}
      </header>
      {!history.data ? (
        history.isError ? (
          <LoadError error={history.error} onRetry={() => void history.refetch()} />
        ) : (
          <ul aria-busy="true" className="flex flex-col gap-2">
            <li className="sr-only">
              <Trans>Loading…</Trans>
            </li>
            {[0, 1, 2, 3, 4].map((i) => (
              <li key={i} aria-hidden="true" className="flex rounded-md border border-border bg-surface p-2">
                <div className="aspect-poster w-16 animate-pulse rounded-md bg-placeholder" />
              </li>
            ))}
          </ul>
        )
      ) : plays.length === 0 ? (
        <EmptyState title={t`No watch history`} />
      ) : (
        <TvFocusGroup focusKey="plays">
          <ul className="flex flex-col gap-2">
            {plays.map((p, i) => (
              <PlayRow key={p.id} play={p} everyone={everyone !== undefined} autoFocus={i === 0} />
            ))}
          </ul>
        </TvFocusGroup>
      )}
      {history.hasNextPage && (
        <TvFocusable focusKey="show-more" className="self-start">
          <Button
            size="sm"
            variant="ghost"
            className="self-start"
            isPending={history.isFetchingNextPage}
            onPress={() => void history.fetchNextPage()}
          >
            <Trans>Show more</Trans>
          </Button>
        </TvFocusable>
      )}
    </section>
  );
}

/** The server-wide history's choice of one user's plays. */
function UserPicker({ userId }: { userId: string | undefined }) {
  const { t } = useLingui();
  const navigate = serverHistoryRoute.useNavigate();
  const users = useQuery(userQueries.list()).data?.items ?? [];
  return (
    <Select
      variant="inline"
      label={t`User`}
      options={[{ id: allUsers, label: t`All users` }, ...users.map((u) => ({ id: u.id, label: u.username }))]}
      selected={userId ?? allUsers}
      onChange={(id) => void navigate({ search: { userId: id === allUsers ? undefined : id }, replace: true })}
    />
  );
}

function PlayRow({ play, everyone, autoFocus }: { play: PlayEvent; everyone: boolean; autoFocus: boolean }) {
  const { t } = useLingui();
  const work = play.work;
  const episode = work.episode;
  const title = episode ? `${episode.seriesTitle} · ${formatEpisodeCode(episode)} · ${work.title}` : work.title;
  const when = formatDateTime(play.startedAt);
  const at = formatClock(play.positionMs);
  const progress = play.completed ? t`Watched` : t`Stopped at ${at}`;

  return (
    <li
      data-part="history-play"
      className="flex items-center gap-4 rounded-md border border-border bg-surface p-2 tv:block tv:p-0"
    >
      <TvFocusable
        focusKey={`play:${play.id}`}
        autoFocus={autoFocus}
        kind="row"
        className="flex items-center gap-4 rounded-md p-2"
      >
        <div className="w-16 shrink-0">
          <Artwork image={work.images.poster} title={work.title} shape="poster" width={64} />
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <Link to="/works/$workId" params={{ workId: work.id }} className="truncate font-semibold hover:underline">
            {title}
          </Link>
          <span className="text-sm text-text-muted">
            {when} · {progress}
            {everyone && (
              <>
                {" · "}
                <Link to="/admin/history" search={{ userId: play.userId }} className="hover:text-text hover:underline">
                  {play.username}
                </Link>
              </>
            )}
          </span>
        </div>
      </TvFocusable>
    </li>
  );
}
