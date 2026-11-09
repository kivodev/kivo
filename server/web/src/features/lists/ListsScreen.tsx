import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { listQueries } from "@/api/lists";
import type { WorkList, WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { Row } from "@/components/Row";
import { settingsCardClass } from "@/components/SettingsLayout";
import { RowsSkeleton } from "@/components/Skeleton";
import { LibraryRow } from "@/features/browse/LibraryRow";
import { LibrarySwitcher, listsKey } from "@/features/browse/LibrarySwitcher";
import { WorkCard, workShape } from "@/features/browse/WorkCard";
import { isTv } from "@/tv/flag";
import { TvFocusGroup, useTvEmptyScreen } from "@/tv/focus";

/**
 * Lists: My List as a row of covers with its count, then the user's other
 * lists, each with its count. A section with nothing in it is hidden. On TV
 * with no lists focus goes to the rail.
 */
export function ListsScreen() {
  const { t } = useLingui();
  useDocumentTitle(t`Lists`);
  const read = useQuery(listQueries.all());
  const lists = read.data?.items;
  const myList = lists?.find((list) => list.builtIn && list.workCount > 0);
  const others = lists?.filter((list) => !list.builtIn) ?? [];
  const empty = lists !== undefined && myList === undefined && others.length === 0;
  useTvEmptyScreen(empty);
  // The row and the lists arrive together, so a TV's first focus is the row's.
  const row = useQuery({ ...workQueries.listRow(myList?.id ?? ""), enabled: myList !== undefined });
  const ready = lists !== undefined && (myList === undefined || row.data !== undefined);
  const failed = read.isError ? read : row.isError ? row : undefined;

  return (
    <TvFocusGroup focusKey="lists-screen">
      <div data-part="lists" className="flex flex-col gap-10 px-4 py-6 sm:px-8">
        <div className="flex min-h-10 items-center">
          <LibrarySwitcher title={t`Lists`} current={listsKey} />
        </div>
        {!ready && failed && <LoadError error={failed.error} onRetry={() => void failed.refetch()} />}
        {!ready && !failed && <RowsSkeleton />}
        {empty && (
          <EmptyState title={t`No lists yet`}>
            <Trans>Titles you add to My List or to a list of your own appear here.</Trans>
          </EmptyState>
        )}
        {ready && myList && row.data && <MyListRow list={myList} items={row.data.items} />}
        {ready && others.length > 0 && (
          <section aria-labelledby="your-lists" className="flex flex-col gap-3">
            <h2 id="your-lists" className="text-lg font-semibold text-text">
              <Trans>Your lists</Trans>
            </h2>
            <ul className={settingsCardClass}>
              {others.map((list, index) => (
                <LibraryRow
                  key={list.id}
                  link={{ to: "/lists/$listId", params: { listId: list.id } }}
                  icon={<Icon name="list" className="size-5" />}
                  name={list.name}
                  count={list.workCount}
                  focusKey={`list:${list.id}`}
                  autoFocus={myList === undefined && index === 0}
                />
              ))}
            </ul>
          </section>
        )}
      </div>
    </TvFocusGroup>
  );
}

function MyListRow({ list, items }: { list: WorkList; items: WorkSummary[] }) {
  const { i18n } = useLingui();
  const link = { to: "/lists/$listId", params: { listId: list.id } } as const;
  return (
    <Row
      id="my-list"
      autoFocus
      title={
        isTv ? (
          list.name
        ) : (
          <Link {...link} className="inline-flex items-center gap-1 hover:underline">
            {list.name}
            <Icon name="chevronRight" className="size-5 text-text-muted" />
          </Link>
        )
      }
      shape="poster"
      items={items}
      itemKey={(w) => w.id}
      renderItem={(w) => <WorkCard work={w} shape={workShape(w.kind)} />}
      action={
        isTv ? (
          <Link {...link} className="text-sm font-semibold text-text">
            <Trans>See all</Trans>
          </Link>
        ) : (
          <span className="text-sm text-text-muted">{i18n.number(list.workCount)}</span>
        )
      }
    />
  );
}
