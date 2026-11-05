import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { fileQueries, useDeleteFile, useDeleteMissingFiles } from "@/api/files";
import { libraryQueries } from "@/api/libraries";
import type { MediaFile } from "@/api/schemas";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { EmptyState } from "@/components/EmptyState";
import { FormError } from "@/components/FormError";
import { Section } from "@/components/Section";
import { formatDateTime } from "@/i18n/format";
import { AdminPage, AdminRow } from "./AdminPage";

/**
 * Missing files, grouped by library. These are files gone from disk, kept
 * for 30 days so a moved or returning file keeps its watch state. Each has
 * Forget, and the page has Forget all.
 */
export function MissingFilesScreen() {
  const { t } = useLingui();
  const files = useSuspenseInfiniteQuery(fileQueries.missing());
  const libraries = useSuspenseQuery(libraryQueries.list()).data.items;
  const forgetAll = useDeleteMissingFiles();
  const [confirming, setConfirming] = useState(false);
  const items = files.data.pages.flatMap((p) => p.items);
  // Each library's files, in the libraries' order. Files of a library that
  // has since been deleted go last.
  const groups = [...libraries.map((l) => ({ id: l.id, name: l.name })), { id: "", name: t`Other libraries` }]
    .map((g) => ({
      ...g,
      files: items.filter((f) => (g.id === "" ? !libraries.some((l) => l.id === f.libraryId) : f.libraryId === g.id)),
    }))
    .filter((g) => g.files.length > 0);
  return (
    <AdminPage
      page="missing-files"
      title={t`Missing files`}
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
        groups.map((g) => (
          <Section key={g.id} title={g.name}>
            <ul className="flex flex-col gap-2">
              {g.files.map((file) => (
                <MissingFileRow key={file.id} file={file} />
              ))}
            </ul>
          </Section>
        ))
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
        title={t`Forget every missing file?`}
        confirmLabel={t`Forget all`}
        tone="danger"
        onCancel={() => setConfirming(false)}
        onConfirm={() => {
          setConfirming(false);
          forgetAll.mutate({ params: { query: { status: "gone" } } });
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
