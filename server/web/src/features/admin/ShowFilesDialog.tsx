import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { fileQueries } from "@/api/files";
import type { MediaFile, WorkDetail } from "@/api/schemas";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { ModalDialog } from "@/components/ModalDialog";
import { containerName, trackDetail, versionName } from "@/components/trackLabels";
import { formatBitrate, formatRuntime, formatSize } from "@/i18n/format";

type Props = { work: WorkDetail; isOpen: boolean; onClose: () => void };

/** Show files, which lists paths, sizes, bitrates and tracks per version and part. */
export function ShowFilesDialog({ work, isOpen, onClose }: Props) {
  const title = work.title;
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} width="page" title={<Trans>Files of {title}</Trans>}>
      <FileList work={work} />
      <div className="mt-5 flex justify-end">
        <Button onPress={onClose}>
          <Trans>Close</Trans>
        </Button>
      </div>
    </ModalDialog>
  );
}

function FileList({ work }: { work: WorkDetail }) {
  const files = useQuery(fileQueries.forWork(work.id));
  if (files.isPending) {
    return (
      <p className="text-sm text-text-muted">
        <Trans>Loading files…</Trans>
      </p>
    );
  }
  if (files.isError) {
    return <FormError error={files.error} />;
  }
  // Grouped by version, in the work's order (best first), then by part.
  const items = files.data.items;
  const known = new Set(work.versions.map((v) => v.id));
  const groups = [
    ...work.versions.map((v) => ({
      id: v.id,
      label: versionName(v),
      files: items.filter((f) => f.versionId === v.id),
    })),
    { id: "", label: undefined, files: items.filter((f) => f.versionId === undefined || !known.has(f.versionId)) },
  ].filter((g) => g.files.length > 0);
  return (
    <div className="flex flex-col gap-5">
      {groups.map((group) => (
        <section key={group.id} className="flex flex-col gap-2">
          {group.label && <h3 className="font-semibold">{group.label}</h3>}
          <ul className="flex flex-col gap-2">
            {group.files
              .sort((a, b) => (a.part ?? 0) - (b.part ?? 0))
              .map((f) => (
                <FileRow key={f.id} file={f} />
              ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

function FileRow({ file }: { file: MediaFile }) {
  const { t } = useLingui();
  const facts = [
    containerName(file.container),
    formatSize(file.sizeBytes),
    file.bitrateBps ? formatBitrate(file.bitrateBps) : undefined,
    file.durationMs !== undefined ? formatRuntime(file.durationMs) : undefined,
    file.status === "gone" ? t`Missing` : undefined,
  ].filter((x) => x !== undefined);
  return (
    <li className="flex flex-col gap-1 rounded-md bg-surface px-3 py-2">
      <span className="font-mono text-xs break-all text-text">{file.path}</span>
      <span className="text-sm text-text-muted">{facts.join(" · ")}</span>
      {file.tracks.length > 0 && (
        <ul className="flex flex-col text-xs text-text-muted">
          {file.tracks.map((track) => (
            <li key={track.id}>{trackDetail(track)}</li>
          ))}
        </ul>
      )}
    </li>
  );
}
