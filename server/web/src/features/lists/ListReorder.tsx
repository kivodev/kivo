import { useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { DropIndicator, GridList, GridListItem, Button as RacButton, useDragAndDrop } from "react-aria-components";
import { useMoveListWorks } from "@/api/lists";
import type { WorkList, WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Artwork } from "@/components/Artwork";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { settingsCardClass } from "@/components/SettingsLayout";
import { GridSkeleton } from "@/components/Skeleton";
import { labelledTitle } from "@/features/browse/WorkCard";

/**
 * A manual list's works in its custom order, as rows: a small poster, the
 * title and year, and a handle to drag the row by. Dragging works from the
 * keyboard too. Press Enter on the handle, use the arrows, then press
 * Enter to drop. A row moves at once and the move is saved behind it; a
 * move that fails goes back.
 */
export function ListReorder({ list }: { list: WorkList }) {
  const { t } = useLingui();
  const read = useQuery(workQueries.listOrder(list.id));
  const move = useMoveListWorks();
  // The order after the moves being saved, until the list is read again.
  const [moved, setMoved] = useState<WorkSummary[]>();
  const saving = useRef(0);
  const works = moved ?? read.data;
  const { dragAndDropHooks } = useDragAndDrop({
    getItems: (keys) => [...keys].map((key) => ({ "text/plain": works?.find((w) => w.id === key)?.title ?? "" })),
    onReorder: ({ keys, target }) => {
      if (!works) return;
      const moving = works.filter((w) => keys.has(w.id));
      const rest = works.filter((w) => !keys.has(w.id));
      const at = rest.findIndex((w) => w.id === target.key) + (target.dropPosition === "after" ? 1 : 0);
      setMoved([...rest.slice(0, at), ...moving, ...rest.slice(at)]);
      saving.current++;
      move.mutate(
        { params: { path: { id: list.id } }, body: { workIds: moving.map((w) => w.id), after: rest[at - 1]?.id } },
        {
          onSettled: (_, error) => {
            if (--saving.current === 0 || error) setMoved(undefined);
          },
        },
      );
    },
    renderDropIndicator: (target) => (
      <DropIndicator
        target={target}
        className={({ isDropTarget }) => (isDropTarget ? "h-0.5 rounded-full bg-accent" : "")}
      />
    ),
  });
  if (!works) {
    return read.isError ? (
      <LoadError error={read.error} onRetry={() => void read.refetch()} />
    ) : (
      <GridSkeleton rail={false} />
    );
  }
  return (
    <div className="flex max-w-(--layout-settings-width) flex-col gap-3">
      <FormError error={move.error} />
      <GridList
        data-part="list-reorder"
        aria-label={t`Reorder ${list.name}`}
        items={works}
        dependencies={[works]}
        dragAndDropHooks={dragAndDropHooks}
        className={settingsCardClass}
      >
        {(work) => {
          const title = labelledTitle(work);
          return (
            <GridListItem
              id={work.id}
              textValue={title}
              className="flex items-center gap-3 py-2 pr-2 pl-3 outline-none first:rounded-t-lg last:rounded-b-lg data-dragging:opacity-50 data-focus-visible:bg-surface-hover"
            >
              <span className="w-10 shrink-0">
                {work.images.poster ? (
                  <Artwork image={work.images.poster} title={title} shape="poster" width={40} />
                ) : (
                  // A tile this small has no room for the title a missing poster shows.
                  <span className="block aspect-poster rounded-md bg-placeholder" />
                )}
              </span>
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate font-medium text-text">{title}</span>
                {work.year !== undefined && <span className="text-sm text-text-muted">{work.year}</span>}
              </span>
              <RacButton
                slot="drag"
                className="flex size-10 shrink-0 cursor-grab items-center justify-center rounded-md text-text-muted hover:bg-surface-hover hover:text-text"
              >
                <Icon name="grip" className="size-4" />
              </RacButton>
            </GridListItem>
          );
        }}
      </GridList>
    </div>
  );
}
