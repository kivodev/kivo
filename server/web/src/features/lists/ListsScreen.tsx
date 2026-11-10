import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { listQueries } from "@/api/lists";
import type { WorkList, WorkSummary } from "@/api/schemas";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { Menu, MenuItem } from "@/components/Menu";
import { ModalDialog } from "@/components/ModalDialog";
import { Row } from "@/components/Row";
import { RowsSkeleton } from "@/components/Skeleton";
import { LibrarySwitcher, listsKey } from "@/features/browse/LibrarySwitcher";
import { WorkCard, workShape } from "@/features/browse/WorkCard";
import { isTv } from "@/tv/flag";
import { TvFocusGroup, useTvEmptyScreen } from "@/tv/focus";
import { NewListForm } from "./AddToListDialog";
import { SmartListDialog } from "./SmartListDialog";

/**
 * Lists: a shelf for each of the user's lists, Watchlist first and the
 * others by name, each a row of the list's first titles under its name.
 * Watchlist is hidden when it's empty. With no shelves a TV's focus goes
 * to the rail.
 */
export function ListsScreen() {
  const { t } = useLingui();
  useDocumentTitle(t`Lists`);
  const read = useQuery(listQueries.counted());
  const shelves = read.data?.items.filter((list) => !list.builtIn || list.workCount > 0);
  const empty = shelves?.length === 0;
  useTvEmptyScreen(empty);
  const [creating, setCreating] = useState<"list" | "smart">();

  return (
    <TvFocusGroup focusKey="lists-screen">
      <div
        data-part="lists"
        className="flex flex-col gap-10 px-4 py-6 sm:px-8 tv:*:last:min-h-(--layout-tv-home-last-row-height)"
      >
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
        {!shelves && read.isError && <LoadError error={read.error} onRetry={() => void read.refetch()} />}
        {!shelves && !read.isError && <RowsSkeleton />}
        {empty && (
          <EmptyState title={t`Nothing in your lists yet.`}>
            <Trans>Titles you add to Watchlist or to a list of your own appear here.</Trans>
          </EmptyState>
        )}
        {shelves?.map((list, index) => (
          <Shelf key={list.id} list={list} autoFocus={index === 0} />
        ))}
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

/** What stands in an empty list's shelf for its titles. */
const nothing = "empty";

/**
 * A list's shelf: its name, which opens the list, a smart list's mark and
 * the count, over its first titles as cards. An empty list's has one
 * placeholder in a card's place, which opens the list. On TV the name is
 * no link, and the shelf ends in See all.
 */
function Shelf({ list, autoFocus }: { list: WorkList; autoFocus: boolean }) {
  const link = { to: "/lists/$listId", params: { listId: list.id } } as const;
  const works = list.items ?? [];
  const items: readonly (WorkSummary | typeof nothing)[] = works.length > 0 ? works : [nothing];
  // Only a manual list's works can be taken off it.
  const manual = list.kind === "manual" ? list : undefined;
  return (
    <Row
      id="list"
      focusKey={`list:${list.id}`}
      autoFocus={autoFocus}
      title={
        <span className="flex min-w-0 items-center gap-3">
          {isTv ? (
            <span className="truncate">{list.name}</span>
          ) : (
            <Link {...link} className="inline-flex min-w-0 items-center gap-1 hover:underline">
              <span className="truncate">{list.name}</span>
              <Icon name="chevronRight" className="size-5 shrink-0 text-text-muted" />
            </Link>
          )}
          {list.kind === "smart" && (
            <span className="inline-flex shrink-0 items-center gap-1 text-sm font-medium text-accent">
              <Icon name="sparkle" className="size-4" />
              <Trans>Smart list</Trans>
            </span>
          )}
          <span data-part="list-count" className="shrink-0 text-sm font-normal text-text-muted">
            {list.workCount === 0 ? (
              <Trans>Empty</Trans>
            ) : (
              <Plural value={list.workCount} one="# title" other="# titles" />
            )}
          </span>
        </span>
      }
      shape="poster"
      items={items}
      itemKey={(w) => (w === nothing ? nothing : w.id)}
      renderItem={(w) =>
        w === nothing ? (
          <Link
            {...link}
            data-part="empty-shelf"
            data-tv-ring
            className="flex aspect-poster items-center justify-center rounded-md border border-dashed border-border p-3 text-center text-sm text-text-muted"
          >
            <Trans>Nothing in this list yet</Trans>
          </Link>
        ) : (
          <WorkCard work={w} shape={workShape(w.kind)} list={manual} />
        )
      }
      action={
        isTv &&
        works.length > 0 && (
          <Link {...link} className="text-sm font-semibold text-text">
            <Trans>See all</Trans>
          </Link>
        )
      }
      endsInAction
    />
  );
}
