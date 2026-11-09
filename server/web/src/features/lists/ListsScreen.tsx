import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { listQueries } from "@/api/lists";
import type { WorkList, WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { Menu, MenuItem } from "@/components/Menu";
import { ModalDialog } from "@/components/ModalDialog";
import { Row } from "@/components/Row";
import { settingsCardClass } from "@/components/SettingsLayout";
import { RowsSkeleton } from "@/components/Skeleton";
import { LibraryRow } from "@/features/browse/LibraryRow";
import { LibrarySwitcher, listsKey } from "@/features/browse/LibrarySwitcher";
import { WorkCard, workShape } from "@/features/browse/WorkCard";
import { isTv } from "@/tv/flag";
import { TvFocusGroup, useTvEmptyScreen } from "@/tv/focus";
import { NewListForm } from "./AddToListDialog";
import { SmartListDialog } from "./SmartListDialog";
import { useSmartLabels } from "./smartLabels";

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
  const custom = lists?.filter((list) => !list.builtIn) ?? [];
  // A TV doesn't read a smart list's query, so it lists them with the rest.
  const others = isTv ? custom : custom.filter((list) => list.kind !== "smart");
  const smart = isTv ? [] : custom.filter((list) => list.kind === "smart");
  const empty = lists !== undefined && myList === undefined && custom.length === 0;
  useTvEmptyScreen(empty);
  // The row and the lists arrive together, so a TV's first focus is the row's.
  const row = useQuery({ ...workQueries.listRow(myList?.id ?? ""), enabled: myList !== undefined });
  const ready = lists !== undefined && (myList === undefined || row.data !== undefined);
  const failed = read.isError ? read : row.isError ? row : undefined;
  const [creating, setCreating] = useState<"list" | "smart">();

  return (
    <TvFocusGroup focusKey="lists-screen">
      <div data-part="lists" className="flex flex-col gap-10 px-4 py-6 sm:px-8">
        <div className="flex min-h-10 items-center justify-between">
          <LibrarySwitcher title={t`Lists`} current={listsKey} />
          {!isTv && (
            <Menu>
              <Button size="sm" variant="ghost" aria-label={t`Create`}>
                <Icon name="plus" className="size-5" />
              </Button>
              <MenuItem onAction={() => setCreating("list")}>{t`New List`}</MenuItem>
              <MenuItem onAction={() => setCreating("smart")}>{t`New Smart List`}</MenuItem>
            </Menu>
          )}
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
                  count={list.kind === "smart" ? undefined : list.workCount}
                  detail={
                    list.kind === "smart"
                      ? plural(list.workCount, { one: "Smart list, # title", other: "Smart list, # titles" })
                      : undefined
                  }
                  focusKey={`list:${list.id}`}
                  autoFocus={myList === undefined && index === 0}
                />
              ))}
            </ul>
          </section>
        )}
        {ready && smart.length > 0 && (
          <section aria-labelledby="smart-lists" className="flex flex-col gap-3">
            <h2 id="smart-lists" className="text-lg font-semibold text-text">
              <Trans>Smart Lists</Trans>
            </h2>
            <ul className={settingsCardClass}>
              {smart.map((list) => (
                <SmartListRow key={list.id} list={list} />
              ))}
            </ul>
          </section>
        )}
      </div>
      {creating === "list" && (
        <ModalDialog
          isOpen
          onClose={() => setCreating(undefined)}
          title={<Trans>New List</Trans>}
          width="form"
          part="new-list"
        >
          <NewListForm onDone={() => setCreating(undefined)} />
        </ModalDialog>
      )}
      {creating === "smart" && <SmartListDialog onClose={() => setCreating(undefined)} />}
    </TvFocusGroup>
  );
}

/** A smart list's row: its name over what it shows and how many. */
function SmartListRow({ list }: { list: WorkList }) {
  const { i18n, t } = useLingui();
  const labels = useSmartLabels();
  const kinds = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(
    (list.smart?.query.kinds ?? []).map(labels.kind),
  );
  const count = i18n.number(list.workCount);
  return (
    <LibraryRow
      link={{ to: "/lists/$listId", params: { listId: list.id } }}
      icon={<Icon name="list" className="size-5" />}
      name={list.name}
      detail={t`${kinds}, ${count}`}
      focusKey={`list:${list.id}`}
    />
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
