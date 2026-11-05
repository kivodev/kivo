import { Trans, useLingui } from "@lingui/react/macro";
import { useId, useState } from "react";
import { errorCode } from "@/api/problem";
import { useChangePassword } from "@/api/users";
import { ModalDialog, useDismissal } from "@/components/ModalDialog";
import { SubmitForm } from "@/components/SubmitForm";
import { Switch } from "@/components/Switch";
import { TextField } from "@/components/TextField";
import { describeProblem } from "@/i18n/problems";

/**
 * Change password. It asks for your current password, a new one, and
 * whether to sign out your other browsers and apps.
 */
export function ChangePasswordDialog({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const { t } = useLingui();
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} title={t`Change password`} width="form" part="change-password">
      <ChangePasswordForm onDone={onClose} />
    </ModalDialog>
  );
}

function ChangePasswordForm({ onDone }: { onDone: () => void }) {
  const { t } = useLingui();
  const change = useChangePassword();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [signOutOthers, setSignOutOthers] = useState(true);
  const switchLabel = useId();
  useDismissal(current || next ? "ask" : "close");
  const code = change.isError ? errorCode(change.error) : undefined;

  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => {
        change.mutate(
          { body: { currentPassword: current, password: next, signOutOthers } },
          { onSuccess: () => onDone() },
        );
      }}
      submitLabel={<Trans>Change password</Trans>}
      isPending={change.isPending}
      onCancel={onDone}
      error={code !== "wrongPassword" ? change.error : null}
    >
      <TextField
        variant="form"
        label={t`Current password`}
        type="password"
        autoComplete="current-password"
        isRequired
        autoFocus
        value={current}
        onChange={setCurrent}
        errorMessage={code === "wrongPassword" ? describeProblem(code) : undefined}
      />
      <TextField
        variant="form"
        label={t`New password`}
        type="password"
        autoComplete="new-password"
        isRequired
        minLength={8}
        description={t`At least 8 characters.`}
        value={next}
        onChange={setNext}
      />
      <div className="flex items-center justify-between gap-4">
        <span id={switchLabel} className="text-sm font-medium text-text">
          <Trans>Sign out other devices</Trans>
        </span>
        <Switch isSelected={signOutOthers} onChange={setSignOutOthers} aria-labelledby={switchLabel} />
      </div>
    </SubmitForm>
  );
}
