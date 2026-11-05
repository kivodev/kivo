import type { ReactNode } from "react";
import { Icon } from "./Icon";

type Props = {
  children: ReactNode;
  action?: ReactNode;
  tone?: "info" | "warning";
};

/** A one-line notice above content, such as the unmatched count. */
export function Banner({ children, action, tone = "info" }: Props) {
  return (
    <div
      data-part="banner"
      data-tone={tone}
      role="status"
      className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-md border border-border bg-surface px-4 py-2 text-sm"
    >
      <Icon name="alert" className={`size-4 shrink-0 ${tone === "warning" ? "text-warning" : "text-text-muted"}`} />
      <span className="min-w-0 flex-1 text-text">{children}</span>
      {action}
    </div>
  );
}
