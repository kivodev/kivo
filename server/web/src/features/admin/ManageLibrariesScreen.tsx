import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import {
  libraryQueries,
  useDeleteLibrary,
  useRefreshLibraryMetadata,
  useScanAllLibraries,
  useScanLibrary,
  useStopLibraryScan,
} from "@/api/libraries";
import type { AdminLibrary as Library } from "@/api/schemas";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { EmptyState } from "@/components/EmptyState";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { LibraryKindIcon } from "@/components/LibraryKindIcon";
import { Menu, MenuItem, MenuSeparator } from "@/components/Menu";
import { ModalDialog } from "@/components/ModalDialog";
import { AddLibraryButton } from "@/features/libraries/AddLibraryButton";
import { EditLibraryForm } from "@/features/libraries/EditLibraryForm";
import { formatDateTime, formatTimeAgo } from "@/i18n/format";
import { AdminList, AdminListItem, AdminPage } from "./AdminPage";
import { ScanLine } from "./ScanLine";

/**
 * The admin Libraries page. It has Scan all libraries and Add library,
 * then a row per library with Scan, its other actions in a menu, and what
 * it's doing. A library added while the page is open scrolls into view.
 */
export function ManageLibrariesScreen() {
  const { t } = useLingui();
  const libraries = useSuspenseQuery(libraryQueries.admin()).data;
  const scanAll = useScanAllLibraries();
  const [listed] = useState(() => new Set(libraries.map((l) => l.id)));
  return (
    <AdminPage
      page="libraries"
      title={t`Libraries`}
      actions={
        <div className="flex flex-wrap gap-2">
          {libraries.length > 0 && (
            <Button
              isPending={scanAll.isPending}
              onPress={() => scanAll.mutate({ params: { path: { task: "scan-libraries" } } })}
            >
              <Trans>Scan all libraries</Trans>
            </Button>
          )}
          <AddLibraryButton />
        </div>
      }
    >
      {scanAll.isSuccess && <Started />}
      <FormError error={scanAll.error} />
      {libraries.length === 0 ? (
        <EmptyState title={t`No libraries`} action={<AddLibraryButton />}>
          <Trans>Add a folder of movies, shows, videos or audiobooks.</Trans>
        </EmptyState>
      ) : (
        <AdminList>
          {libraries.map((library) => (
            <LibraryRow key={library.id} library={library} added={!listed.has(library.id)} />
          ))}
        </AdminList>
      )}
    </AdminPage>
  );
}

/** "Started. View progress in Tasks." */
function Started() {
  return (
    <p role="status" className="text-text-muted">
      <Trans>
        Started. View progress in{" "}
        <Link to="/admin/tasks" className="font-semibold text-text hover:underline">
          Tasks
        </Link>
        .
      </Trans>
    </p>
  );
}

function lastScanOf(library: Library): string | undefined {
  const times = (library.folders ?? []).flatMap((f) => (f.lastScanAt ? [f.lastScanAt] : []));
  return times.sort().at(-1);
}

/** "237 movies", by the library's kind. */
function ItemCount({ library }: { library: Library }) {
  const n = library.workCount;
  switch (library.kind) {
    case "movies":
      return <Plural value={n} one="# movie" other="# movies" />;
    case "shows":
      return <Plural value={n} one="# show" other="# shows" />;
    case "videos":
      return <Plural value={n} one="# video" other="# videos" />;
    case "audiobooks":
      return <Plural value={n} one="# book" other="# books" />;
    default:
      return <Plural value={n} one="# item" other="# items" />;
  }
}

/** "Scanned 2 hours ago", with the full time on hover. */
function LastScan({ library }: { library: Library }) {
  const [now] = useState(Date.now);
  const at = lastScanOf(library);
  if (!at) return <Trans>Not scanned yet</Trans>;
  const ago = formatTimeAgo(now - Date.parse(at));
  return (
    <time dateTime={at} title={formatDateTime(at)}>
      <Trans>Scanned {ago}</Trans>
    </time>
  );
}

type Open = "refresh" | "edit" | "delete" | undefined;

function LibraryRow({
  library,
  added,
}: {
  library: Library;
  /** Whether the library was added while the page was open. If so, it scrolls into view. */
  added: boolean;
}) {
  const { t } = useLingui();
  const scan = useScanLibrary();
  const stop = useStopLibraryScan();
  const refresh = useRefreshLibraryMetadata();
  const remove = useDeleteLibrary();
  const row = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState<Open>(undefined);
  // Only the value of `added` when the row first shows matters here.
  useEffect(() => {
    if (added) row.current?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [added]);
  const close = () => setOpen(undefined);
  const status = library.scan;
  const scanning = status?.state === "scanning";
  const matching = status?.state === "matching";
  const unreachable = (library.folders ?? []).filter((f) => !f.reachable).map((f) => f.path);
  const path = { params: { path: { id: library.id } } };
  const name = library.name;
  const failed = [scan, stop, refresh, remove].find((m) => m.isError);

  return (
    <AdminListItem part="admin-library">
      <div ref={row} className="flex scroll-m-4 items-center gap-3">
        <LibraryKindIcon kind={library.kind} className="size-5 shrink-0 text-text-muted" />
        <div className="flex min-w-0 flex-1 flex-col">
          <h2 className="truncate text-base font-semibold">{name}</h2>
          {/* On a phone the line breaks between its parts rather than within them. */}
          <p className="text-text-muted">
            <span className="whitespace-nowrap">
              <ItemCount library={library} />
            </span>
            {" · "}
            <span className="whitespace-nowrap">
              <LastScan library={library} />
            </span>
          </p>
        </div>
        <Button size="sm" aria-label={t`Scan ${name}`} isPending={scan.isPending} onPress={() => scan.mutate(path)}>
          <Trans>Scan</Trans>
        </Button>
        <Menu>
          <Button aria-label={t`More actions for ${name}`} variant="ghost" size="sm">
            <Icon name="more" />
          </Button>
          <MenuItem isDisabled={refresh.isPending} onAction={() => setOpen("refresh")}>
            <Trans>Refresh all metadata</Trans>
          </MenuItem>
          <MenuSeparator />
          <MenuItem onAction={() => setOpen("edit")}>
            <Trans>Edit</Trans>
          </MenuItem>
          <MenuItem tone="danger" isDisabled={remove.isPending} onAction={() => setOpen("delete")}>
            <Trans>Delete library</Trans>
          </MenuItem>
        </Menu>
      </div>

      <div className="mt-1.5 flex flex-col gap-1 pl-8 empty:hidden">
        {(unreachable.length > 0 || status?.state === "unreachable") && (
          <p className="flex min-w-0 items-center gap-1.5 text-warning">
            <Icon name="alert" className="size-4 shrink-0" />
            <span className="truncate">
              {unreachable.length > 0 ? (
                <Trans>Can't reach {unreachable.join(", ")}</Trans>
              ) : (
                <Trans>A folder can't be reached</Trans>
              )}
            </span>
          </p>
        )}
        {scanning && status && (
          <div data-part="scan" role="status">
            <ScanLine
              name={<Trans>Scanning</Trans>}
              progress={status.progress}
              edge
              action={
                <Button
                  size="sm"
                  variant="ghost"
                  className="-my-1.5 -mr-3"
                  isPending={stop.isPending}
                  onPress={() => stop.mutate(path)}
                >
                  <Trans>Stop</Trans>
                </Button>
              }
            />
          </div>
        )}
        {matching && (
          <div data-part="scan" role="status">
            <ScanLine name={<Trans>Matching</Trans>} edge />
          </div>
        )}
        <FormError error={failed?.error} />
      </div>

      <ConfirmDialog
        isOpen={open === "refresh"}
        title={t`Refresh all metadata for ${name}?`}
        confirmLabel={t`Refresh all metadata`}
        onConfirm={() => {
          close();
          refresh.mutate(path);
        }}
        onCancel={close}
      >
        <Trans>Metadata and artwork are downloaded again for every item. Fields edited by hand are kept.</Trans>
      </ConfirmDialog>
      <ConfirmDialog
        isOpen={open === "delete"}
        title={t`Delete ${name}?`}
        confirmLabel={t`Delete library`}
        tone="danger"
        onConfirm={() => {
          close();
          remove.mutate(path);
        }}
        onCancel={close}
      >
        <Trans>
          The library's items are removed. Files on disk aren't touched. Watch history returns if a title is added
          again.
        </Trans>
      </ConfirmDialog>
      <ModalDialog isOpen={open === "edit"} onClose={close} title={<Trans>Edit {name}</Trans>}>
        <EditLibraryForm library={library} onSaved={close} onCancel={close} />
      </ModalDialog>
    </AdminListItem>
  );
}
