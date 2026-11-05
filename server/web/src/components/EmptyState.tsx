import type { ReactNode } from "react";

type Props = {
  title: ReactNode;
  /** One sentence of explanation. */
  children?: ReactNode;
  /** One primary action, when there is one. */
  action?: ReactNode;
  tone?: "neutral" | "error";
};

/**
 * Every empty or failed state uses the same pattern: a one-line title, one
 * sentence, and one primary action.
 */
export function EmptyState({ title, children, action, tone = "neutral" }: Props) {
  return (
    <div
      data-part="empty-state"
      data-tone={tone}
      role={tone === "error" ? "alert" : undefined}
      className="mx-auto flex max-w-prose flex-col items-center gap-3 px-4 py-16 text-center"
    >
      <h2 className={`text-xl font-semibold ${tone === "error" ? "text-danger" : "text-text"}`}>{title}</h2>
      {children && <p className="text-text-muted">{children}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
