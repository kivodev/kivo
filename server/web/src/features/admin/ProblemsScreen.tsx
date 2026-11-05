import { Trans, useLingui } from "@lingui/react/macro";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { problemQueries, useDismissProblem } from "@/api/problems";
import type { ServerProblem } from "@/api/schemas";
import { Button } from "@/components/Button";
import { EmptyState } from "@/components/EmptyState";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { formatDateTime } from "@/i18n/format";
import { AdminPage, AdminRow } from "./AdminPage";
import { useProblemActions } from "./problemActions";

/** The problems page, listing open problems newest first, each with its fix. Resolved problems close themselves. */
export function ProblemsScreen() {
  const { t } = useLingui();
  const problems = useSuspenseQuery(problemQueries.list()).data.items;
  return (
    <AdminPage page="problems" title={t`Problems`}>
      {problems.length === 0 ? (
        <EmptyState title={t`No problems`} />
      ) : (
        <ul className="flex flex-col gap-2">
          {problems.map((p) => (
            <ProblemRow key={p.id} problem={p} />
          ))}
        </ul>
      )}
    </AdminPage>
  );
}

const severityTone: Record<string, string> = { error: "text-danger", warning: "text-warning" };

function ProblemRow({ problem }: { problem: ServerProblem }) {
  const { t } = useLingui();
  const dismiss = useDismissProblem();
  const action = useProblemActions()(problem.code);
  const message = problem.message;

  return (
    <AdminRow
      part="problem"
      actions={
        <Button
          size="sm"
          variant="ghost"
          aria-label={t`Dismiss ${message}`}
          isPending={dismiss.isPending}
          onPress={() => dismiss.mutate({ params: { path: { id: problem.id } } })}
        >
          <Trans>Dismiss</Trans>
        </Button>
      }
    >
      <div className="flex items-start gap-2" data-severity={problem.severity}>
        <Icon
          name="alert"
          className={`mt-0.5 size-4 shrink-0 ${severityTone[problem.severity] ?? "text-text-muted"}`}
        />
        <span className="text-text">{message}</span>
      </div>
      {action && (
        <p className="text-sm font-semibold text-text">
          {action.page ? (
            <Link to={action.page} search={action.search ?? {}} className="hover:underline">
              {action.text}
            </Link>
          ) : (
            action.text
          )}
        </p>
      )}
      <span className="text-xs text-text-muted">{formatDateTime(problem.raisedAt)}</span>
      <FormError error={dismiss.error} />
    </AdminRow>
  );
}
