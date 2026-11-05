import type { ReactNode } from "react";
import { errorCode } from "@/api/problem";
import { describeProblem } from "@/i18n/problems";

type Props = {
  /** A failed request's error, shown as its problem's message. Nothing shows while it's null or undefined. */
  error?: unknown;
  /** The words to show instead of the error's own. */
  children?: ReactNode;
  /** Warning for a failure the page works around, such as a book's next file. */
  tone?: "danger" | "warning";
  id?: string;
  className?: string;
};

/** Why something failed, announced to screen readers as it appears. */
export function FormError({ error, children, tone = "danger", id, className = "" }: Props) {
  const message = children ?? (error === null || error === undefined ? undefined : describeProblem(errorCode(error)));
  if (message === undefined || message === null || message === false || message === "") return null;
  return (
    <p
      id={id}
      role="alert"
      className={`text-sm ${tone === "warning" ? "text-warning" : "text-danger"} ${className}`.trimEnd()}
    >
      {message}
    </p>
  );
}
