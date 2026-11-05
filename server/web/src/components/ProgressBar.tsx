import type { CSSProperties } from "react";

/**
 * A thin bar showing a fraction done, such as how much was played, along
 * the bottom of artwork. `thin` halves its height, for use along a list
 * row's edge.
 */
export function ProgressBar({
  value,
  thin = false,
  className = "",
}: {
  value: number;
  thin?: boolean;
  className?: string;
}) {
  const pct = Math.min(100, Math.max(0, value * 100));
  const style = { "--progress": `${pct}%` } as CSSProperties;
  return (
    <div
      data-part="progress"
      role="progressbar"
      aria-valuenow={Math.round(pct)}
      aria-valuemin={0}
      aria-valuemax={100}
      className={`${thin ? "h-0.5" : "h-1"} bg-progress-track ${className}`}
    >
      <div className="h-full w-(--progress) bg-progress" style={style} />
    </div>
  );
}
