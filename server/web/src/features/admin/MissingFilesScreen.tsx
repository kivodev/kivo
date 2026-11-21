import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useState } from "react";
import { fileQueries, useDeleteFile, useDeleteMissingFiles } from "@/api/files";
import { libraryQueries } from "@/api/libraries";
import type { MediaFile } from "@/api/schemas";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { EmptyState } from "@/components/EmptyState";
import { FormError } from "@/components/FormError";
import { formatDateTime } from "@/i18n/format";
import { AdminPage, AdminRow } from "./AdminPage";

const route = getRouteApi("/authed/admin/libraries/$libraryId/missing-files");

/**
 * A library's missing files. These are files gone from disk, kept for 30
 * days so a moved or returning file keeps its watch state. Each has Forget,
 * and the page has Forget all.
 */
export function MissingFilesScreen() {
  const { t } = useLingui();
  const { libraryId } = route.useParams();
  const files = useSuspenseInfiniteQuery(fileQueries.missing(libraryId));
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const name = libraries.find((l) => l.id === libraryId)?.name ?? "";
  const forgetAll = useDeleteMissingFiles();
  const [confirming, setConfirming] = useState(false);
  const items = files.data.pages.flatMap((p) => p.items);
  return (
    <AdminPage
      page="missing-files"
      parent={
        <Link to="/admin/libraries" className="hover:text-text hover:underline">
          <Trans>Libraries ›</Trans>
        </Link>
      }
      title={t`Missing files in ${name}`}
      actions={
        items.length > 0 && (
          <Button isPending={forgetAll.isPending} onPress={() => setConfirming(true)}>
            <Trans>Forget all</Trans>
          </Button>
        )
      }
    >
      <p className="max-w-prose text-sm text-text-muted">
        <Trans>Missing files are kept for 30 days. A moved or restored file keeps its watch state.</Trans>
      </p>
      <FormError error={forgetAll.error} />
      {items.length === 0 ? (
        <EmptyState title={t`No missing files`} />
      ) : (
        <ul className="flex flex-col gap-2">
          {items.map((file) => (
            <MissingFileRow key={file.id} file={file} />
          ))}
        </ul>
      )}
      {files.hasNextPage && (
        <Button
          size="sm"
          variant="ghost"
          className="self-start"
          isPending={files.isFetchingNextPage}
          onPress={() => void files.fetchNextPage()}
        >
          <Trans>Show more</Trans>
        </Button>
      )}
      <ConfirmDialog
        isOpen={confirming}
        title={t`Forget every missing file in ${name}?`}
        confirmLabel={t`Forget all`}
        tone="danger"
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          setConfirming(false);
          forgetAll.mutate({ params: { query: { status: "gone", libraryId } } });
        }}
      >
        <Trans>Files that reappear are treated as new. Watch state is kept. Nothing on disk is deleted.</Trans>
      </ConfirmDialog>
    </AdminPage>
  );
}

function MissingFileRow({ file }: { file: MediaFile }) {
  const { t } = useLingui();
  const deleteFile = useDeleteFile();
  const [confirming, setConfirming] = useState(false);
  const path = file.path;
  const missingSince = file.goneAt ? formatDateTime(file.goneAt) : undefined;

  return (
    <AdminRow
      part="missing-file"
      actions={
        <Button
          size="sm"
          aria-label={t`Forget ${path}`}
          isPending={deleteFile.isPending}
          onPress={() => setConfirming(true)}
        >
          <Trans>Forget</Trans>
        </Button>
      }
    >
      {file.title &&
        (file.workId ? (
          <Link to="/works/$workId" params={{ workId: file.workId }} className="font-semibold hover:underline">
            {file.title}
          </Link>
        ) : (
          <span className="font-semibold">{file.title}</span>
        ))}
      <span className="font-mono text-xs break-all text-text-muted">{path}</span>
      {missingSince && (
        <span className="text-sm text-text-muted">
          <Trans>Missing since {missingSince}</Trans>
        </span>
      )}
      <FormError error={deleteFile.error} />
      <ConfirmDialog
        isOpen={confirming}
        title={t`Forget ${path}?`}
        confirmLabel={t`Forget`}
        tone="danger"
        onConfirm={() => {
          setConfirming(false);
          deleteFile.mutate({ params: { path: { id: file.id } } });
        }}
        onCancel={() => setConfirming(false)}
      >
        <Trans>Watch state is kept. Nothing on disk is deleted.</Trans>
      </ConfirmDialog>
    </AdminRow>
  );
}
