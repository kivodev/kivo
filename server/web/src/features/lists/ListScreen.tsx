import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQueries, useQuery } from "@tanstack/react-query";
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { listQueries, useDeleteList, useRenameList } from "@/api/lists";
import type { WorkList, WorkSummary } from "@/api/schemas";
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
import { type GridRange, useRefetchFailed, VirtualGrid } from "@/features/browse/VirtualGrid";
import { WorkCard, workShape } from "@/features/browse/WorkCard";
import { isTv } from "@/tv/flag";
import { useTvEmptyScreen } from "@/tv/focus";
import { ListNameForm } from "./ListNameForm";

const listRoute = getRouteApi("/authed/lists/$listId");

/**
 * A list's works, most recently added first, in a grid without sort,
 * filters or letters, each card in its own shape. A list the user made can
 * be renamed or deleted, except on TV.
 */
export function ListScreen() {
  const { t } = useLingui();
  const { listId } = listRoute.useParams();
  const list = useQuery(listQueries.detail(listId));
  useDocumentTitle(list.data?.name ?? t`Lists`);
  const renderCard = useCallback(
    (work: WorkSummary, width: number) => (
      <WorkCard work={work} shape={workShape(work.kind)} width={width} listId={listId} />
    ),
    [listId],
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
          {!isTv && list.data && !list.data.builtIn && <ListActions list={list.data} />}
        </div>
        {first ? (
          <p data-part="library-count" className="text-sm text-text-muted">
            <Plural value={total} one="# title" other="# titles" />
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
        <EmptyState title={t`Nothing on this list yet`}>
          <Trans>Titles you add to this list appear here.</Trans>
        </EmptyState>
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

/** The ⋯ menu of a list the user made: Rename and Delete. */
function ListActions({ list }: { list: WorkList }) {
  const { t } = useLingui();
  const navigate = useNavigate();
  const rename = useRenameList();
  const remove = useDeleteList();
  const [open, setOpen] = useState<"rename" | "delete">();
  return (
    <>
      <Menu>
        <Button aria-label={t`More actions for ${list.name}`} variant="secondary" size="icon">
          <Icon name="more" />
        </Button>
        <MenuItem onAction={() => setOpen("rename")}>{t`Rename`}</MenuItem>
        <MenuItem onAction={() => setOpen("delete")}>{t`Delete`}</MenuItem>
      </Menu>
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
