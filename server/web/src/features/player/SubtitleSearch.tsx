import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { subtitleQueries, useAddFileSubtitle } from "@/api/files";
import { errorCode, isProblem } from "@/api/problem";
import type { SubtitleResult } from "@/api/schemas";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { ModalDialog } from "@/components/ModalDialog";
import { languageName } from "@/components/trackLabels";
import { formatTimeOfDay } from "@/i18n/format";
import { describeProblem, describeSubtitleFailure } from "@/i18n/problems";

export type SubtitleSearchProps = {
  /** The version's file, which is its first part. */
  fileId: string;
  isOpen: boolean;
  onClose: () => void;
  /** Called with the downloaded subtitle's track ID, to choose it. */
  onDownloaded: (trackId: string) => void;
};

/**
 * SubtitleSearch lists OpenSubtitles' subtitles for a file, best first as
 * the server ranks them. Picking one downloads it to the server, for
 * everyone who sees the file, and chooses it.
 */
export function SubtitleSearch({ fileId, isOpen, onClose, onDownloaded }: SubtitleSearchProps) {
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} title={<Trans>Search subtitles</Trans>}>
      <Results fileId={fileId} onDownloaded={onDownloaded} />
    </ModalDialog>
  );
}

function Results({ fileId, onDownloaded }: { fileId: string; onDownloaded: (trackId: string) => void }) {
  const { t } = useLingui();
  const results = useQuery(subtitleQueries.results(fileId));
  const add = useAddFileSubtitle();
  const choose = (c: SubtitleResult) =>
    add.mutate(
      { params: { path: { id: fileId } }, body: { resultId: c.id } },
      { onSuccess: (track) => onDownloaded(track.id) },
    );

  if (results.isPending) {
    return (
      <p role="status" className="text-sm text-text-muted">
        <Trans>Searching…</Trans>
      </p>
    );
  }
  if (results.isError) {
    return <FormError>{subtitleFailure(results.error)}</FormError>;
  }
  const items = results.data.items;
  return (
    <div className="flex flex-col gap-3">
      {items.length === 0 ? (
        <p className="text-sm text-text-muted">
          <Trans>No subtitles found</Trans>
        </p>
      ) : (
        <ul data-part="subtitle-results" className="flex flex-col gap-1">
          {items.map((c) => {
            const flags = [
              c.hashMatch ? t`Exact match` : undefined,
              c.forced ? t`Forced` : undefined,
              c.hearingImpaired ? t`SDH` : undefined,
            ].filter(Boolean);
            return (
              <li key={c.id} className="flex items-center gap-3 rounded-md px-2 py-2 hover:bg-surface-hover">
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="font-medium text-text">{[languageName(c.language), ...flags].join(" · ")}</span>
                  {c.release && <span className="truncate text-xs text-text-muted">{c.release}</span>}
                </div>
                <Button
                  size="sm"
                  isPending={add.isPending && add.variables?.body.resultId === c.id}
                  isDisabled={add.isPending}
                  onPress={() => choose(c)}
                >
                  <Trans>Download</Trans>
                </Button>
              </li>
            );
          })}
        </ul>
      )}
      {add.isError && <FormError>{subtitleFailure(add.error, true)}</FormError>}
    </div>
  );
}

/**
 * A failed search or download in words, from the server's facts. Not found
 * means the subtitle is gone only for a download.
 */
function subtitleFailure(error: unknown, downloading = false): string {
  const code = errorCode(error);
  if (code === "notFound" && !downloading) return describeProblem(code);
  const resetsAt = isProblem(error) ? error.resetsAt : undefined;
  return describeSubtitleFailure(code, resetsAt && formatTimeOfDay(resetsAt));
}
