import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQueries, useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import type { FavoriteKind, WorkSummary } from "@/api/schemas";
import { favoriteKinds, gridPageSize, workQueries } from "@/api/works";
import type { ArtworkShape } from "@/components/Artwork";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { LoadError } from "@/components/LoadError";
import { Row } from "@/components/Row";
import { CountSkeleton, GridSkeleton, RowsSkeleton } from "@/components/Skeleton";
import { isTv } from "@/tv/flag";
import { useTvEmptyScreen } from "@/tv/focus";
import { favoritesKey, LibrarySwitcher } from "./LibrarySwitcher";
import { type GridRange, useRefetchFailed, VirtualGrid } from "./VirtualGrid";
import { WorkCard } from "./WorkCard";

const favoritesGridRoute = getRouteApi("/authed/favorites/$kind");

/**
 * A kind's card shape. An episode uses its series' poster. Other videos
 * are landscape, and books are square.
 */
function favoriteShape(kind: FavoriteKind): ArtworkShape {
  switch (kind) {
    case "video":
    case "extra":
      return "landscape";
    case "book":
      return "square";
    default:
      return "poster";
  }
}

/** A kind's row title: "Movies", "Shows". */
function useKindTitle(): (kind: FavoriteKind) => string {
  const { t } = useLingui();
  const titles: Record<FavoriteKind, string> = {
    movie: t`Movies`,
    series: t`Shows`,
    episode: t`Episodes`,
    book: t`Books`,
    season: t`Seasons`,
    video: t`Videos`,
    extra: t`Extras`,
  };
  return (kind) => titles[kind];
}

/** How many favorites of a kind there are, such as "12 movies" or "1 episode". */
function FavoritesCount({ kind, count }: { kind: FavoriteKind; count: number }) {
  switch (kind) {
    case "movie":
      return <Plural value={count} one="# movie" other="# movies" />;
    case "series":
      return <Plural value={count} one="# show" other="# shows" />;
    case "episode":
      return <Plural value={count} one="# episode" other="# episodes" />;
    case "book":
      return <Plural value={count} one="# book" other="# books" />;
    case "season":
      return <Plural value={count} one="# season" other="# seasons" />;
    case "extra":
      return <Plural value={count} one="# extra" other="# extras" />;
    default:
      return <Plural value={count} one="# video" other="# videos" />;
  }
}

function NoFavorites() {
  const { t } = useLingui();
  return (
    <EmptyState title={t`No favorites yet.`}>
      <Trans>Items you mark with ♥ appear here.</Trans>
    </EmptyState>
  );
}

/**
 * Favorites. It has a row for each kind that has any, most recently
 * marked first. A row's title and See all open the kind's grid. On TV the
 * title is plain, See all is the row's last card, and with no favorites
 * focus goes to the rail.
 */
export function FavoritesScreen() {
  const { t } = useLingui();
  useDocumentTitle(t`Favorites`);
  const kindTitle = useKindTitle();
  const pages = useQueries({ queries: favoriteKinds.map((kind) => workQueries.favorites(kind)) });
  const rows = favoriteKinds.flatMap((kind, i) => {
    const page = pages[i]?.data;
    return page && page.items.length > 0 ? [{ kind, page }] : [];
  });
  // The rows arrive together, once every kind's first cards have arrived.
  const loaded = pages.every((p) => p.data !== undefined);
  const failed = loaded ? undefined : pages.find((p) => p.isError);
  useTvEmptyScreen(loaded && rows.length === 0);

  return (
    <div data-part="favorites" className="flex flex-col gap-10 px-4 py-6 sm:px-8">
      <div className="flex min-h-10 items-center">
        <LibrarySwitcher title={t`Favorites`} current={favoritesKey} />
      </div>
      {failed ? (
        <LoadError
          error={failed.error}
          onRetry={() => {
            for (const p of pages) if (p.isError) void p.refetch();
          }}
        />
      ) : (
        !loaded && <RowsSkeleton />
      )}
      {loaded && rows.length === 0 && <NoFavorites />}
      {(loaded ? rows : []).map(({ kind, page }, index) => {
        const shape = favoriteShape(kind);
        const link = { to: "/favorites/$kind", params: { kind } } as const;
        return (
          <Row
            key={kind}
            id={kind}
            autoFocus={index === 0}
            title={
              isTv ? (
                kindTitle(kind)
              ) : (
                <Link {...link} className="hover:underline">
                  {kindTitle(kind)}
                </Link>
              )
            }
            shape={shape}
            items={page.items}
            itemKey={(w) => w.id}
            renderItem={(w) => <WorkCard work={w} shape={shape} />}
            action={
              <Link
                {...link}
                className="text-sm text-text-muted touch-target hover:text-text tv:font-semibold tv:text-text"
              >
                <Trans>See all</Trans>
              </Link>
            }
          />
        );
      })}
    </div>
  );
}

/**
 * Every favorite of one kind, most recently marked first, in a library
 * grid without sort, filters or letters.
 */
export function FavoritesGridScreen() {
  const { t } = useLingui();
  const kind = favoritesGridRoute.useLoaderData();
  const kindTitle = useKindTitle();
  const title = t`Favorites · ${kindTitle(kind)}`;
  useDocumentTitle(title);
  const shape = favoriteShape(kind);
  const renderCard = useCallback(
    (work: WorkSummary, width: number) => <WorkCard work={work} shape={shape} width={width} />,
    [shape],
  );
  const read = useQuery(workQueries.favoritesPage(kind, 0));
  const first = read.data;
  const total = first ? (first.total ?? first.items.length) : 0;
  const [range, setRange] = useState<GridRange>({ start: 0, end: 0, top: 0, topEnd: 0, settles: 0 });
  // The pages of the items around where the grid settles.
  const indexes: number[] = [];
  for (let i = Math.floor(range.start / gridPageSize); i * gridPageSize < Math.min(range.end, total); i++) {
    if (i > 0) indexes.push(i);
  }
  const rest = useQueries({ queries: indexes.map((i) => workQueries.favoritesPage(kind, i)) });
  useRefetchFailed(rest, range.settles);
  const pages = new Map([[0, first], ...indexes.map((i, k) => [i, rest[k]?.data] as const)]);
  const items: (WorkSummary | undefined)[] = Array.from(
    { length: total },
    (_, i) => pages.get(Math.floor(i / gridPageSize))?.items[i % gridPageSize],
  );

  return (
    <div data-part="favorites-grid" className="flex flex-col gap-4 px-4 py-6 sm:px-8">
      <header className="flex flex-col gap-3">
        <div className="flex min-h-10 items-center">
          <LibrarySwitcher title={title} current={favoritesKey} />
        </div>
        {first ? (
          <p data-part="library-count" className="text-sm text-text-muted">
            <FavoritesCount kind={kind} count={total} />
          </p>
        ) : (
          !read.isError && <CountSkeleton />
        )}
      </header>
      {!first ? (
        read.isError ? (
          <LoadError error={read.error} onRetry={() => void read.refetch()} />
        ) : (
          <GridSkeleton rail={false} />
        )
      ) : total === 0 ? (
        <NoFavorites />
      ) : (
        <VirtualGrid
          items={items}
          shape={shape}
          view="grid"
          onRangeChange={setRange}
          renderCard={renderCard}
          renderListRow={() => null}
        />
      )}
    </div>
  );
}
