import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { type ReactElement, useState } from "react";
import { errorCode } from "@/api/problem";
import type { WorkDetail } from "@/api/schemas";
import { useRefreshWorkMetadata, useUnmatchWork, workQueries } from "@/api/works";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Icon } from "@/components/Icon";
import { Menu, MenuItem, MenuSeparator } from "@/components/Menu";
import { describeProblem } from "@/i18n/problems";
import { artworkKinds, ChooseArtworkDialog } from "./ChooseArtworkDialog";
import { EditFieldsDialog } from "./EditFieldsDialog";
import { FixMatchDialog } from "./FixMatchDialog";
import { ShowFilesDialog } from "./ShowFilesDialog";

/**
 * The kinds that Fix match applies to, as the server takes them. These are
 * a movie, and a series with its seasons and episodes, which all match a
 * series.
 */
const matchedKinds = new Set(["movie", "series", "season", "episode"]);
/** The kinds that Unmatch applies to. A series' episodes are unmatched with it. */
const unmatchableKinds = new Set(["movie", "series"]);

type Open = "fixMatch" | "unmatch" | "edit" | "artwork" | "files" | undefined;

/**
 * The item admin menu, in the "…" on a detail page, after the page's own
 * items (`children`). It has Fix match (Match for a book), Unmatch, Edit,
 * Choose artwork, Refresh metadata and Show files.
 */
export function ItemAdminMenu({ work, children = [] }: { work: WorkDetail; children?: ReactElement[] }) {
  const { t } = useLingui();
  const [open, setOpen] = useState<Open>(undefined);
  const refresh = useRefreshWorkMetadata();
  const unmatch = useUnmatchWork();
  const failed = refresh.error ?? unmatch.error;
  // Refreshing lasts until the work changes. The refresh's result arrives
  // as a workChanged event, which refetches the work.
  const updatedAt = useQuery(workQueries.detail(work.id)).dataUpdatedAt;
  const [refreshedAt, setRefreshedAt] = useState<number | undefined>(undefined);
  const refreshing = refresh.isPending || (refreshedAt !== undefined && updatedAt <= refreshedAt);
  const close = () => setOpen(undefined);
  const title = work.title;

  return (
    <span data-part="item-menu" className="flex items-center gap-2">
      <Menu>
        <Button aria-label={t`More actions for ${title}`} variant="secondary" size="icon">
          <Icon name="more" />
        </Button>
        {children}
        {children.length > 0 && <MenuSeparator />}
        {matchedKinds.has(work.kind) && (
          <MenuItem onAction={() => setOpen("fixMatch")}>
            <Trans>Fix match</Trans>
          </MenuItem>
        )}
        {work.kind === "book" && (
          <MenuItem onAction={() => setOpen("fixMatch")}>
            <Trans>Match</Trans>
          </MenuItem>
        )}
        {unmatchableKinds.has(work.kind) && (
          <MenuItem onAction={() => setOpen("unmatch")}>
            <Trans>Unmatch</Trans>
          </MenuItem>
        )}
        <MenuItem onAction={() => setOpen("edit")}>
          <Trans>Edit</Trans>
        </MenuItem>
        {artworkKinds.has(work.kind) && (
          <MenuItem onAction={() => setOpen("artwork")}>
            <Trans>Choose artwork</Trans>
          </MenuItem>
        )}
        <MenuItem
          isDisabled={refreshing}
          onAction={() =>
            refresh.mutate({ params: { path: { id: work.id } } }, { onSuccess: () => setRefreshedAt(Date.now()) })
          }
        >
          {refreshing ? <Trans>Refreshing…</Trans> : <Trans>Refresh metadata</Trans>}
        </MenuItem>
        {work.versions.length > 0 && (
          <MenuItem onAction={() => setOpen("files")}>
            <Trans>Show files</Trans>
          </MenuItem>
        )}
      </Menu>
      {failed && (
        <span role="alert" className="text-sm text-danger">
          {describeProblem(errorCode(failed))}
        </span>
      )}
      <FixMatchDialog work={work} isOpen={open === "fixMatch"} onClose={close} />
      <ConfirmDialog
        isOpen={open === "unmatch"}
        title={t`Unmatch ${title}?`}
        confirmLabel={t`Unmatch`}
        onConfirm={() => {
          close();
          unmatch.mutate({ params: { path: { id: work.id } } });
        }}
        onCancel={close}
      >
        <Trans>Removes metadata and artwork until it's matched again. Watch history is kept.</Trans>
      </ConfirmDialog>
      <EditFieldsDialog work={work} isOpen={open === "edit"} onClose={close} />
      <ChooseArtworkDialog work={work} isOpen={open === "artwork"} onClose={close} />
      <ShowFilesDialog work={work} isOpen={open === "files"} onClose={close} />
    </span>
  );
}
