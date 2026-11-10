import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQueries, useQuery } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { listQueries, useDeleteList, useUpdateList } from "@/api/lists";
import type { ListSort, WorkList, WorkSummary } from "@/api/schemas";
import { gridPageSize, gridPages, workQueries } from "@/api/works";
import { Button, buttonClass } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { Menu, MenuItem } from "@/components/Menu";
import { ModalDialog } from "@/components/ModalDialog";
import { CountSkeleton, GridSkeleton } from "@/components/Skeleton";
import { SortMenu, useSortWords } from "@/features/browse/CountLine";
import { useBrowseLabels } from "@/features/browse/useBrowseLabels";
import { type GridRange, useRefetchFailed, VirtualGrid } from "@/features/browse/VirtualGrid";
import { WorkCard, workShape } from "@/features/browse/WorkCard";
import { isTv } from "@/tv/flag";
import { TvFocusable, useTvEmptyScreen } from "@/tv/focus";
import { ListNameForm } from "./ListNameForm";
import { ListReorder } from "./ListReorder";
import { SmartListDialog } from "./SmartListDialog";
import { SmartSummary } from "./SmartSummary";

const listRoute = getRouteApi("/authed/lists/$listId");

/** The sorts of a manual list, in the menu's order. */
const listSorts = ["custom", "title", "releaseDate", "rating", "dateAdded"] as const satisfies readonly ListSort[];

/**
 * A list's works in a grid without filters or letters, each card in its
 * own shape: a manual list's in the sort its count line names and sets, a
 * smart list's by its rules. Except on TV, a manual list's ⋯ menu has
 * Reorder, which lists the works as rows to drag into a custom order, a
 * list the user made can be renamed or deleted there, and a smart list's
 * rules can be edited.
 */
export function ListScreen() {
  const { t } = useLingui();
  const { listId } = listRoute.useParams();
  const list = useQuery(listQueries.detail(listId));
  useDocumentTitle(list.data?.name ?? t`Lists`);
  const manual = list.data?.kind === "manual" ? list.data : undefined;
  const smart = list.data?.smart;
  const [reordering, setReordering] = useState(false);
  const renderCard = useCallback(
    (work: WorkSummary, width: number) => (
      <WorkCard work={work} shape={workShape(work.kind)} width={width} list={manual} />
    ),
    [manual],
  );
  const read = useQuery(workQueries.listPage(listId, 0));
  const first = read.data;
  const total = first ? (first.total ?? first.items.length) : 0;
  useTvEmptyScreen(first !== undefined && total === 0);
  const [range, setRange] = useState<GridRange>({ start: 0, end: 0, top: 0, topEnd: 0, settles: 0 });
  const indexes = gridPages(range.start, range.end, total);
  const rest = useQueries({ queries: indexes.map((i) => workQueries.listPage(listId, i)) });
  useRefetchFailed(rest, range.settles);
  const pages = new Map([[0, first], ...indexes.map((i, k) => [i, rest[k]?.data] as const)]);
  const items: (WorkSummary | undefined)[] = Array.from(
    { length: total },
    (_, i) => pages.get(Math.floor(i / gridPageSize))?.items[i % gridPageSize],
  );

  if (list.isError && !list.data) {
    return <LoadError error={list.error} onRetry={() => void list.refetch()} />;
  }

  return (
    <div data-part="list" className="flex flex-col gap-4 px-4 py-6 sm:px-8">
      <header className="flex flex-col gap-3">
        <div className="flex min-h-10 items-center gap-1">
          {!isTv && (
            <Link to="/lists" aria-label={t`Lists`} className={`-ml-3 ${buttonClass("ghost", "icon")}`}>
              <Icon name="chevronLeft" />
            </Link>
          )}
          <h1 className="min-w-0 flex-1 truncate text-2xl font-semibold">{list.data?.name}</h1>
          {reordering ? (
            <Button variant="primary" size="md" onPress={() => setReordering(false)}>
              <Trans>Done</Trans>
            </Button>
          ) : (
            !isTv && list.data && <ListActions list={list.data} onReorder={() => setReordering(true)} />
          )}
        </div>
        {first ? (
          smart && !isTv ? (
            <p data-part="library-count" className="truncate text-sm text-text-muted">
              <SmartSummary query={smart.query} count={total} />
            </p>
          ) : (
            <CountLine count={total} list={reordering ? undefined : manual} />
          )
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
        <EmptyState
          title={
            smart?.errors
              ? t`These rules no longer work`
              : smart
                ? t`Nothing matches yet`
                : t`Nothing in this list yet.`
          }
        >
          {smart?.errors ? (
            !isTv && <Trans>Edit the rules to fix them.</Trans>
          ) : smart ? (
            <Trans>Titles appear here when they match the rules.</Trans>
          ) : (
            <Trans>Titles you add to this list appear here.</Trans>
          )}
        </EmptyState>
      ) : reordering && manual ? (
        <ListReorder list={manual} />
      ) : (
        <VirtualGrid
          items={items}
          shape="poster"
          view="grid"
          onRangeChange={setRange}
          renderCard={renderCard}
          renderListRow={() => null}
        />
      )}
    </div>
  );
}

/**
 * How many titles the list has and, for a manual list, its sort, which
 * opens the sort menu. On TV the two are one focusable, as on a library.
 */
function CountLine({ count, list }: { count: number; list: WorkList | undefined }) {
  const { t } = useLingui();
  const labels = useBrowseLabels().sort;
  const words = useSortWords();
  const update = useUpdateList();
  const line = (
    <div data-part="library-count" className="flex items-center gap-1.5 text-sm text-text-muted">
      <span>
        <Plural value={count} one="# title" other="# titles" />
      </span>
      {list && (
        <>
          <span aria-hidden="true">·</span>
          <SortMenu
            options={listSorts}
            // The sort just chosen shows while it is being saved.
            selected={(update.isPending && update.variables.body.sort) || (list.sort ?? "custom")}
            labels={{ ...labels, custom: t`Custom order`, dateAdded: t`Date added to list` }}
            words={{ ...words, custom: t`Custom order`, dateAdded: words.addedAt }}
            onChange={(sort) => update.mutate({ params: { path: { id: list.id } }, body: { sort } })}
          />
        </>
      )}
    </div>
  );
  return isTv && list ? (
    <TvFocusable focusKey="sort" face="inside" className="w-fit">
      {line}
    </TvFocusable>
  ) : (
    line
  );
}

/**
 * A list's actions: Edit, if it's smart, and the ⋯ menu's Reorder for a
 * manual list and Rename and Delete for one the user made.
 */
function ListActions({ list, onReorder }: { list: WorkList; onReorder: () => void }) {
  const { t } = useLingui();
  const navigate = useNavigate();
  const rename = useUpdateList();
  const remove = useDeleteList();
  const [open, setOpen] = useState<"edit" | "rename" | "delete">();
  return (
    <>
      {list.smart && (
        <Button variant="secondary" size="md" onPress={() => setOpen("edit")}>
          <Trans>Edit</Trans>
        </Button>
      )}
      {(!list.smart || !list.builtIn) && (
        <Menu>
          <Button aria-label={t`More actions for ${list.name}`} variant="secondary" size="icon">
            <Icon name="more" />
          </Button>
          {!list.smart && <MenuItem onAction={onReorder}>{t`Reorder`}</MenuItem>}
          {!list.builtIn && <MenuItem onAction={() => setOpen("rename")}>{t`Rename`}</MenuItem>}
          {!list.builtIn && <MenuItem onAction={() => setOpen("delete")}>{t`Delete`}</MenuItem>}
        </Menu>
      )}
      {open === "edit" && list.smart && (
        <SmartListDialog
          listId={list.id}
          name={list.name}
          query={list.smart.query}
          onClose={() => setOpen(undefined)}
        />
      )}
      <ModalDialog
        isOpen={open === "rename"}
        onClose={() => setOpen(undefined)}
        title={<Trans>Rename List</Trans>}
        width="form"
      >
        <ListNameForm
          initialName={list.name}
          submitLabel={<Trans>Rename</Trans>}
          isPending={rename.isPending}
          error={rename.error}
          onCancel={() => setOpen(undefined)}
          onSubmit={(name) =>
            rename.mutate(
              { params: { path: { id: list.id } }, body: { name } },
              { onSuccess: () => setOpen(undefined) },
            )
          }
        />
      </ModalDialog>
      <ConfirmDialog
        isOpen={open === "delete"}
        title={t`Delete ${list.name}?`}
        confirmLabel={t`Delete`}
        tone="danger"
        onCancel={() => setOpen(undefined)}
        onConfirm={() =>
          remove.mutate(
            { params: { path: { id: list.id } } },
            { onSuccess: () => void navigate({ to: "/lists", replace: true }) },
          )
        }
      >
        <Trans>The titles stay in your libraries.</Trans>
      </ConfirmDialog>
      <FormError error={remove.error} />
    </>
  );
}
