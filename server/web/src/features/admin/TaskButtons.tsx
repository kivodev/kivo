import { Trans, useLingui } from "@lingui/react/macro";
import { useEffect, useState } from "react";
import type { Task } from "@/api/schemas";
import { useRunTask, useStopTask } from "@/api/tasks";
import { Button, type ButtonSize, type ButtonVariant } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { FormError } from "@/components/FormError";
import { unwatchRun, watchRun } from "./runNotices";
import { useAdminLabels } from "./useAdminLabels";

/** The least time Run now shows its spinner, so it never just flickers. */
const minPending = 400;

type Props = {
  task: Task;
  size?: ButtonSize;
  variant?: ButtonVariant;
  className?: string;
};

/**
 * Run now. It runs a scheduled task at once, even outside the maintenance
 * window, and shows a notice when the run ends.
 */
export function RunButton({ task, size = "sm", variant = "secondary", className = "" }: Props) {
  const { t } = useLingui();
  const { taskName } = useAdminLabels();
  const run = useRunTask();
  const [held, setHeld] = useState(false);
  useEffect(() => {
    if (!held) return;
    const timer = setTimeout(() => setHeld(false), minPending);
    return () => clearTimeout(timer);
  }, [held]);
  const name = taskName(task);
  return (
    <>
      <Button
        size={size}
        variant={variant}
        className={className}
        aria-label={t`Run ${name} now`}
        isPending={run.isPending || held}
        onPress={() => {
          watchRun(task);
          setHeld(true);
          run.mutate({ params: { path: { task: task.id } } });
        }}
      >
        <Trans>Run now</Trans>
      </Button>
      <FormError error={run.error} />
    </>
  );
}

/** Stop. After asking, it ends a task's run and the work in flight. */
export function StopButton({ task, size = "sm", className = "" }: Omit<Props, "variant">) {
  const { t } = useLingui();
  const { taskName } = useAdminLabels();
  const stop = useStopTask();
  const [asking, setAsking] = useState(false);
  const name = taskName(task);
  return (
    <>
      <Button
        size={size}
        className={className}
        aria-label={t`Stop ${name}`}
        isPending={stop.isPending}
        onPress={() => setAsking(true)}
      >
        <Trans>Stop</Trans>
      </Button>
      <FormError error={stop.error} />
      <ConfirmDialog
        isOpen={asking}
        title={t`Stop ${name}?`}
        confirmLabel={t`Stop`}
        onConfirm={() => {
          unwatchRun(task);
          stop.mutate({ params: { path: { task: task.id } } });
          setAsking(false);
        }}
        onCancel={() => setAsking(false)}
      />
    </>
  );
}
