import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { ModalDialog } from "@/components/ModalDialog";
import { isTv } from "@/tv/flag";
import { TvDialogButton } from "@/tv/forms/dialog";
import { TvControl } from "@/tv/forms/TvControl";
import { type Credit, loadLicenses } from "./licenses";

/**
 * Open source licenses, from Credits: each package in the build with its
 * version and license, whose text opens under it. On TV each is a row the
 * remote steps through, without the text, which a remote can't scroll.
 */
export function LicensesDialog({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const { t } = useLingui();
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} title={t`Open source licenses`} width="prose" part="licenses">
      {isOpen && <LicenseList />}
      <div className="mt-5 flex justify-end">
        {isTv ? (
          <TvDialogButton focusKey="tv:licenses:close" autoFocus onPress={onClose}>
            <Trans>Close</Trans>
          </TvDialogButton>
        ) : (
          <Button autoFocus onPress={onClose}>
            <Trans>Close</Trans>
          </Button>
        )}
      </div>
    </ModalDialog>
  );
}

function LicenseList() {
  const licenses = useQuery({ queryKey: ["licenses"], queryFn: loadLicenses, staleTime: Number.POSITIVE_INFINITY });
  if (licenses.isPending) {
    return (
      <p className="text-sm text-text-muted">
        <Trans>Loading…</Trans>
      </p>
    );
  }
  if (licenses.isError) return <FormError error={licenses.error} />;
  return (
    <ul className="flex flex-col gap-1 text-sm">
      {licenses.data.map((credit) => (
        <li key={`${credit.name}@${credit.version ?? ""}`}>
          {isTv ? <TvRow credit={credit} /> : <WebRow credit={credit} />}
        </li>
      ))}
    </ul>
  );
}

function Heading({ credit }: { credit: Credit }) {
  return (
    <span className="flex min-w-0 flex-col">
      <span className="font-medium text-text">
        {credit.name}
        {credit.version && <span className="font-normal text-text-muted"> {credit.version}</span>}
      </span>
      <span className="text-text-muted">{credit.license}</span>
    </span>
  );
}

function WebRow({ credit }: { credit: Credit }) {
  if (!credit.text) {
    return (
      <div className="px-3 py-2">
        {credit.url ? (
          <a href={credit.url} target="_blank" rel="noreferrer" className="hover:underline">
            <Heading credit={credit} />
          </a>
        ) : (
          <Heading credit={credit} />
        )}
      </div>
    );
  }
  return (
    <details className="rounded-md open:bg-surface">
      <summary className="cursor-pointer rounded-md px-3 py-2 hover:bg-surface-hover">
        <Heading credit={credit} />
      </summary>
      <pre className="max-h-80 overflow-auto px-3 pb-3 font-mono text-xs whitespace-pre-wrap text-text-muted">
        {credit.text}
      </pre>
    </details>
  );
}

function TvRow({ credit }: { credit: Credit }) {
  return (
    <TvControl kind="row" onPress={() => {}} className="rounded-md px-3 py-2">
      <Heading credit={credit} />
    </TvControl>
  );
}
