import { Plural, Trans } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { useState } from "react";
import { bookQueries, useRenameSeries } from "@/api/books";
import type { BookSeries } from "@/api/schemas";
import { useIsAdmin } from "@/api/users";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { LoadError } from "@/components/LoadError";
import { GridSkeleton } from "@/components/Skeleton";
import { isTv } from "@/tv/flag";
import { useTvEmptyScreen } from "@/tv/focus";
import { BookShelf } from "./BookShelf";
import { RenameDialog } from "./RenameDialog";

const bookSeriesRoute = getRouteApi("/authed/book-series/$seriesId");

/** A book series' page: its books in series order, each labelled with its position. */
export function BookSeriesScreen() {
  const { seriesId } = bookSeriesRoute.useParams();
  const read = useQuery(bookQueries.series(seriesId));
  if (read.data) return <BookSeriesPage series={read.data} />;
  return read.isError ? (
    <LoadError error={read.error} onRetry={() => void read.refetch()} />
  ) : (
    <div className="flex flex-col gap-6 px-4 py-6 sm:px-8">
      <div className="flex min-h-9 items-center">
        <div className="h-9 w-64 animate-pulse rounded-md bg-placeholder" />
      </div>
      <GridSkeleton rail={false} />
    </div>
  );
}

function BookSeriesPage({ series }: { series: BookSeries }) {
  const { seriesId } = bookSeriesRoute.useParams();
  const isAdmin = useIsAdmin();
  const navigate = bookSeriesRoute.useNavigate();
  const rename = useRenameSeries();
  const [renaming, setRenaming] = useState(false);
  const name = series.name;
  useDocumentTitle(name);
  useTvEmptyScreen(series.items.length === 0);

  return (
    <div data-part="series-page" className="flex flex-col gap-6 px-4 py-6 sm:px-8">
      <header className="flex flex-wrap items-center gap-4">
        <h1 className="text-3xl font-bold text-text">{name}</h1>
        <span className="text-sm text-text-muted">
          <Plural value={series.items.length} one="# book" other="# books" />
        </span>
        {isAdmin && !isTv && (
          <Button size="sm" variant="ghost" onPress={() => setRenaming(true)}>
            <Trans>Rename</Trans>
          </Button>
        )}
      </header>
      <BookShelf books={series.items} seriesId={series.id} autoFocus />
      <RenameDialog
        isOpen={renaming}
        name={name}
        isPending={rename.isPending}
        error={rename.error}
        onClose={() => setRenaming(false)}
        onRename={(next) =>
          rename.mutate(
            { params: { path: { id: seriesId } }, body: { name: next } },
            {
              onSuccess: (s) => {
                setRenaming(false);
                // Merged into another series, so show that series' page in
                // this one's place.
                if (s.id !== seriesId) {
                  void navigate({ to: "/book-series/$seriesId", params: { seriesId: s.id }, replace: true });
                }
              },
            },
          )
        }
      />
    </div>
  );
}
