import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { homeQueries, homeRowKinds } from "@/api/home";
import { libraryQueries } from "@/api/libraries";
import type { HomeRow } from "@/api/schemas";
import { useIsAdmin } from "@/api/users";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { Row } from "@/components/Row";
import { HomeSkeleton } from "@/components/Skeleton";
import { WorkCard, workShape } from "@/features/browse/WorkCard";
import { AddLibraryButton } from "@/features/libraries/AddLibraryButton";
import { isTv } from "@/tv/flag";
import { useTvEmptyScreen } from "@/tv/focus";
import { useHomeRowTitle } from "./rowTitle";

/** Rows of books, whose covers are square. */
const bookRows = new Set<HomeRow["kind"]>(["continueListening"]);

/**
 * Home. It shows the rows the server sends that have cards, skipping kinds this client doesn't know.
 * On TV the first card of the first row has focus, every row's cards take
 * it, and a row's link, such as See all, is its last card.
 */
export function HomeScreen() {
  const { t } = useLingui();
  const isAdmin = useIsAdmin();
  useDocumentTitle(undefined);
  const home = useQuery(homeQueries.rows());
  const rowTitle = useHomeRowTitle();
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const rows = (home.data?.rows ?? []).filter((r) => homeRowKinds.has(r.kind));
  // A screen with nothing to focus hands focus to the rail.
  useTvEmptyScreen(libraries.length === 0 || (home.data !== undefined && rows.length === 0));

  if (libraries.length === 0) {
    // Adding a library is an admin's job on the web.
    return isAdmin && !isTv ? (
      <EmptyState title={t`No libraries`} action={<AddLibraryButton />}>
        <Trans>Add a folder of movies, shows, videos or audiobooks.</Trans>
      </EmptyState>
    ) : (
      <EmptyState title={t`No libraries`}>
        <Trans>Ask an admin for access.</Trans>
      </EmptyState>
    );
  }
  if (!home.data) {
    return home.isError ? <LoadError error={home.error} onRetry={() => void home.refetch()} /> : <HomeSkeleton />;
  }

  return (
    // On TV the last row reaches on down the screen, so the page can scroll
    // it to the top as it does every other row (revealTvRow), rather than
    // stopping short with the row above it showing at the screen's top.
    <div
      data-part="home"
      className="flex flex-col gap-10 px-4 py-6 sm:px-8 tv:*:last:min-h-(--layout-tv-home-last-row-height)"
    >
      {rows.length === 0 &&
        (home.data.allHidden ? (
          <EmptyState
            title={t`Your Home is empty`}
            action={
              isTv ? undefined : (
                <Link to="/settings" hash="home" className="font-semibold text-accent hover:underline">
                  <Trans>Home</Trans>
                </Link>
              )
            }
          >
            <Trans>Choose its rows in Settings.</Trans>
          </EmptyState>
        ) : (
          <EmptyState title={t`Nothing here yet`}>
            <Trans>Items appear here as libraries are scanned.</Trans>
          </EmptyState>
        ))}
      {rows.map((row, index) => {
        // Continue Watching shows the frames where you stopped.
        const landscape = row.kind === "continueWatching" || row.library?.kind === "videos";
        const shape = landscape
          ? "landscape"
          : row.library?.kind === "audiobooks" || bookRows.has(row.kind)
            ? "square"
            : "poster";
        const continueRow = row.kind === "continueWatching" || row.kind === "continueListening" ? row.kind : undefined;
        const list = row.kind === "list" ? row.list : undefined;
        // Only a manual list's works can be taken off it.
        const manual = list?.kind === "manual" ? list : undefined;
        return (
          <Row
            key={row.id}
            id={row.kind}
            focusKey={row.id}
            autoFocus={index === rows.findIndex((r) => r.items.length > 0)}
            title={
              list && !isTv ? (
                <Link
                  to="/lists/$listId"
                  params={{ listId: list.id }}
                  className="inline-flex items-center gap-1 hover:underline"
                >
                  {rowTitle(row)}
                  <Icon name="chevronRight" className="size-5 text-text-muted" />
                </Link>
              ) : (
                rowTitle(row)
              )
            }
            shape={shape}
            items={row.items}
            itemKey={(w) => w.id}
            renderItem={(w) => (
              <WorkCard work={w} shape={list ? workShape(w.kind) : shape} continueRow={continueRow} list={manual} />
            )}
            action={
              list && isTv ? (
                <Link to="/lists/$listId" params={{ listId: list.id }} className="text-sm font-semibold text-text">
                  <Trans>See all</Trans>
                </Link>
              ) : row.library && row.kind === "recentlyAdded" ? (
                <Link
                  to="/libraries/$libraryId"
                  params={{ libraryId: row.library.id }}
                  search={{ sort: "addedAt" }}
                  className="text-sm text-text-muted touch-target hover:text-text tv:font-semibold tv:text-text"
                >
                  <Trans>See all</Trans>
                </Link>
              ) : undefined
            }
          />
        );
      })}
    </div>
  );
}
