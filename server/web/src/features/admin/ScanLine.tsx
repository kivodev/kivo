import { Trans } from "@lingui/react/macro";
import { Fragment, type ReactNode } from "react";
import type { ScanProgress } from "@/api/schemas";
import { ProgressBar } from "@/components/ProgressBar";
import { formatCount, formatRuntime } from "@/i18n/format";
import { useAdminLabels } from "./useAdminLabels";

/**
 * A library's scan on one line, with a thin bar under it, such as
 * "Movies · Matching    1,240 of 3,100 · about 4m left". On the left is
 * what runs (the library's name, or Scanning) and the step it's on. On the
 * right are the count, the time left and the action. Before the first
 * step reports, only the name shows. A task's run has a count and no step.
 */
export function ScanLine({
  name,
  progress,
  action,
  edge = false,
}: {
  name: ReactNode;
  progress?: ScanProgress | { done: number; total: number };
  /** After the count, such as Stop. */
  action?: ReactNode;
  /** The bar runs along the bottom edge of the enclosing AdminListItem rather than under the text. */
  edge?: boolean;
}) {
  const { stepName } = useAdminLabels();
  const parts: { key: string; node: ReactNode }[] = [];
  if (progress) {
    const done = formatCount(progress.done);
    const total = formatCount(progress.total);
    parts.push({
      key: "count",
      node: (
        <Trans>
          {done} of {total}
        </Trans>
      ),
    });
    if ("remainingMs" in progress && progress.remainingMs !== undefined) {
      const left = formatRuntime(progress.remainingMs);
      parts.push({ key: "left", node: <Trans>about {left} left</Trans> });
    }
  }

  return (
    <div
      data-part="step-progress"
      data-step={progress && "step" in progress ? progress.step : undefined}
      className="flex flex-col gap-1.5"
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-0.5">
        <span className="min-w-0 truncate">
          {name}
          {progress && "step" in progress && <span className="text-text-muted">{` · ${stepName(progress.step)}`}</span>}
        </span>
        <span className="flex flex-wrap items-center gap-x-2 text-text-muted tabular-nums">
          {parts.length > 0 && (
            <span>
              {parts.map((p, i) => (
                <Fragment key={p.key}>
                  {i > 0 && " · "}
                  <span className="whitespace-nowrap">{p.node}</span>
                </Fragment>
              ))}
            </span>
          )}
          {action}
        </span>
      </div>
      {progress && (
        <ProgressBar
          value={progress.total > 0 ? progress.done / progress.total : 0}
          thin={edge}
          className={edge ? "absolute inset-x-0 bottom-0" : "rounded-full"}
        />
      )}
    </div>
  );
}
