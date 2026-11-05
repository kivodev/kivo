import { useLingui } from "@lingui/react/macro";
import type { Play } from "@/api/schemas";

/** The play's ffmpeg job, to reproduce it by hand. */
export function TranscodeDiagnostics({ transcode }: { transcode: NonNullable<Play["transcode"]> }) {
  const { t } = useLingui();
  const exitCode = transcode.exitCode;
  const states: Record<string, string> = { running: t`Running`, paused: t`Paused`, done: t`Done`, failed: t`Failed` };
  return (
    <dl data-part="transcode-diagnostics" className="flex flex-col gap-1 text-xs">
      {transcode.state && (
        <div>
          <dt className="inline font-semibold">{t`Job`}: </dt>
          <dd className="inline">
            {states[transcode.state] ?? transcode.state}
            {exitCode !== undefined ? ` (${t`exit code ${exitCode}`})` : ""}
          </dd>
        </div>
      )}
      <div>
        <dt className="font-semibold">{t`Command`}</dt>
        <dd className="font-mono break-all">{transcode.command}</dd>
      </div>
      {transcode.stderr && (
        <div>
          <dt className="font-semibold">{t`Error output`}</dt>
          <dd className="font-mono break-all whitespace-pre-wrap">{transcode.stderr}</dd>
        </div>
      )}
    </dl>
  );
}
