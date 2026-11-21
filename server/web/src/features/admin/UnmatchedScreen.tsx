import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useState } from "react";
import { libraryQueries } from "@/api/libraries";
import type { WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Button } from "@/components/Button";
import { EmptyState } from "@/components/EmptyState";
import { AdminPage, AdminRow } from "./AdminPage";
import { FixMatchDialog } from "./FixMatchDialog";

const route = getRouteApi("/authed/admin/libraries/$libraryId/unmatched");

/**
 * A library's unmatched titles. These are the root works no provider
 * matched, each with what it was parsed from and Fix match. A matched one leaves
 * the list.
 */
export function UnmatchedScreen() {
  const { t } = useLingui();
  const { libraryId } = route.useParams();
  const works = useSuspenseInfiniteQuery(workQueries.unmatched(libraryId));
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const name = libraries.find((l) => l.id === libraryId)?.name ?? "";
  const items = works.data.pages.flatMap((p) => p.items);
  return (
    <AdminPage
      page="unmatched"
      parent={
        <Link to="/admin/libraries" className="hover:text-text hover:underline">
          <Trans>Libraries ›</Trans>
        </Link>
      }
      title={t`Unmatched in ${name}`}
    >
      {items.length === 0 ? (
        <EmptyState title={t`Nothing unmatched`} />
      ) : (
        <ul className="flex flex-col gap-2">
          {items.map((work) => (
            <UnmatchedRow key={work.id} work={work} />
          ))}
        </ul>
      )}
      {works.hasNextPage && (
        <Button
          size="sm"
          variant="ghost"
          className="self-start"
          isPending={works.isFetchingNextPage}
          onPress={() => void works.fetchNextPage()}
        >
          <Trans>Show more</Trans>
        </Button>
      )}
    </AdminPage>
  );
}

function UnmatchedRow({ work }: { work: WorkSummary }) {
  const { t } = useLingui();
  const [fixing, setFixing] = useState(false);
  const title = work.title;
  return (
    <AdminRow
      part="unmatched-title"
      actions={
        <Button
          size="sm"
          aria-label={work.kind === "book" ? t`Match ${title}` : t`Fix match for ${title}`}
          onPress={() => setFixing(true)}
        >
          {work.kind === "book" ? <Trans>Match</Trans> : <Trans>Fix match</Trans>}
        </Button>
      }
    >
      <span className="flex flex-wrap items-baseline gap-x-2">
        <Link to="/works/$workId" params={{ workId: work.id }} className="font-semibold hover:underline">
          {title}
        </Link>
        {work.year !== undefined && <span className="text-sm text-text-muted">{work.year}</span>}
      </span>
      {work.path && <span className="font-mono text-xs break-all text-text-muted">{work.path}</span>}
      <FixMatchDialog work={work} stay isOpen={fixing} onClose={() => setFixing(false)} />
    </AdminRow>
  );
}
