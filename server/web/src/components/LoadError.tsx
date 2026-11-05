import { Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { errorCode, isProblem } from "@/api/problem";
import { describeProblem } from "@/i18n/problems";
import { TvControl } from "@/tv/forms/TvControl";
import { Button, buttonClass } from "./Button";
import { EmptyState } from "./EmptyState";

/**
 * Shown in place of a page, or the part of it that was loading, when it
 * failed. It says what failed and offers Try again, so it's never a dead
 * end. A 404 shows as a page that doesn't exist. An alternative to trying
 * again, when there is one, shows beside Try again.
 */
export function LoadError({
  error,
  onRetry,
  alternative,
}: {
  error: unknown;
  onRetry: () => void;
  alternative?: ReactNode;
}) {
  const { t } = useLingui();
  if (isProblem(error) && error.status === 404) return <NotFoundState />;
  const retry = (
    <TvControl focusKey="load-error:retry" autoFocus>
      <Button variant="primary" onPress={onRetry}>
        <Trans>Try again</Trans>
      </Button>
    </TvControl>
  );
  return (
    <EmptyState
      tone="error"
      title={t`Couldn't load this page.`}
      action={
        alternative ? (
          <div className="flex justify-center gap-4">
            {retry}
            {alternative}
          </div>
        ) : (
          retry
        )
      }
    >
      {describeProblem(errorCode(error))}
    </EmptyState>
  );
}

export function NotFoundState() {
  const { t } = useLingui();
  return (
    <EmptyState
      title={t`This page doesn't exist.`}
      action={
        <TvControl focusKey="not-found:home" autoFocus>
          <Link to="/" className={buttonClass("primary")}>
            <Trans>Go to Home</Trans>
          </Link>
        </TvControl>
      }
    >
      {describeProblem("notFound")}
    </EmptyState>
  );
}
