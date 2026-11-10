import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { listQueries } from "@/api/lists";
import type { WorkList, WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Artwork, cardDensity } from "@/components/Artwork";
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

/**
 * Lists: My List as a row of covers, then the user's other lists, manual
 * then smart, as a grid of cards. My List is hidden when it's empty. A TV
 * shows the other lists as one card of rows, and with no lists focus goes
 * to the rail.
 */
export function ListsScreen() {
  const { t } = useLingui();
  useDocumentTitle(t`Lists`);
  const read = useQuery(listQueries.counted());
  const lists = read.data?.items;
  const myList = lists?.find((list) => list.builtIn && list.workCount > 0);
  const custom = lists?.filter((list) => !list.builtIn) ?? [];
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
              <MenuItem textValue={t`New List`} onAction={() => setCreating("list")}>
                <Icon name="list" className="size-4" />
                {t`New List`}
              </MenuItem>
              <MenuItem textValue={t`New Smart List`} onAction={() => setCreating("smart")}>
                <Icon name="sparkle" className="size-4 text-accent" />
                {t`New Smart List`}
              </MenuItem>
            </Menu>
          )}
        </div>
        {!ready && failed && <LoadError error={failed.error} onRetry={() => void failed.refetch()} />}
        {!ready && !failed && <RowsSkeleton />}
        {empty && (
          <EmptyState title={t`Nothing in your lists yet.`}>
            <Trans>Titles you add to Watchlist or to a list of your own appear here.</Trans>
          </EmptyState>
        )}
        {ready && myList && row.data && <MyListRow list={myList} items={row.data.items} />}
        {ready &&
          custom.length > 0 &&
          (isTv ? <TvLists lists={custom} autoFocus={myList === undefined} /> : <ListCards lists={custom} />)}
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

/** A TV's lists, smart ones among the rest: one card of rows, each with its count. */
function TvLists({ lists, autoFocus }: { lists: WorkList[]; autoFocus: boolean }) {
  return (
    <section aria-labelledby="your-lists" className="flex flex-col gap-3">
      <h2 id="your-lists" className="text-lg font-semibold text-text">
        <Trans>Your lists</Trans>
      </h2>
      <ul className={settingsCardClass}>
        {lists.map((list, index) => (
          <LibraryRow
            key={list.id}
            link={{ to: "/lists/$listId", params: { listId: list.id } }}
            icon={
              list.kind === "smart" ? (
                <Icon name="sparkle" className="size-5 text-accent" />
              ) : (
                <Icon name="list" className="size-5" />
              )
            }
            name={list.name}
            count={list.kind === "smart" ? undefined : list.workCount}
            detail={
              list.kind === "smart"
                ? plural(list.workCount, { one: "Smart list, # title", other: "Smart list, # titles" })
                : undefined
            }
            focusKey={`list:${list.id}`}
            autoFocus={autoFocus && index === 0}
          />
        ))}
      </ul>
    </section>
  );
}

/** The lists as cards, two across on a phone and as many as fit wider: manual lists, then smart ones, each by name. */
function ListCards({ lists }: { lists: WorkList[] }) {
  const ordered = [...lists.filter((list) => list.kind !== "smart"), ...lists.filter((list) => list.kind === "smart")];
  return (
    <ul
      data-part="list-cards"
      className="grid max-w-page grid-cols-2 gap-x-(--layout-grid-gap) gap-y-6 sm:grid-cols-(--layout-landscape-grid) sm:gap-x-5 sm:gap-y-7"
    >
      {ordered.map((list) => (
        <li key={list.id} className="min-w-0">
          <ListCard list={list} />
        </li>
      ))}
    </ul>
  );
}

const coverSlots = [0, 1, 2] as const;

/**
 * A list's card: up to three of its posters side by side, then its name and
 * count. An empty list shows its icon on a plain tile, with no count.
 */
function ListCard({ list }: { list: WorkList }) {
  const { i18n } = useLingui();
  const empty = list.workCount === 0;
  return (
    <Link
      to="/lists/$listId"
      params={{ listId: list.id }}
      data-part="list-card"
      className="group flex flex-col gap-2.5 rounded-md"
    >
      <div
        className={`relative grid grid-cols-3 gap-0.5 overflow-hidden rounded-md ${empty ? "bg-placeholder" : "shadow-card"}`}
      >
        {coverSlots.map((slot) => {
          const image = empty ? undefined : list.covers?.[slot];
          return image ? (
            // The link is named by the list; a cover adds nothing to it.
            <Artwork
              key={slot}
              image={image}
              title=""
              shape="poster"
              width={160}
              density={cardDensity}
              corners="none"
              className="h-full"
            />
          ) : (
            <span
              key={slot}
              data-part="cover-placeholder"
              className={`aspect-poster h-full ${empty ? "" : "bg-placeholder"}`}
            />
          );
        })}
        {empty && <Icon name="list" className="absolute inset-0 m-auto size-8 text-text-subtle" />}
        <span
          aria-hidden="true"
          className="absolute inset-0 bg-badge opacity-0 transition-opacity group-hover:opacity-50 pointer-coarse:hidden"
        />
      </div>
      <div className="flex min-w-0 items-center gap-2 px-0.5">
        {list.kind === "smart" && <Icon name="sparkle" className="size-4 shrink-0 text-accent" />}
        <span className="min-w-0 flex-1 truncate text-sm font-medium text-text">{list.name}</span>
        {!empty && (
          <span data-part="list-count" className="shrink-0 text-xs text-text-muted tabular-nums">
            {i18n.number(list.workCount)}
          </span>
        )}
      </div>
    </Link>
  );
}

function MyListRow({ list, items }: { list: WorkList; items: WorkSummary[] }) {
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
        isTv && (
          <Link {...link} className="text-sm font-semibold text-text">
            <Trans>See all</Trans>
          </Link>
        )
      }
    />
  );
}
