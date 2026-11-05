import { Trans, useLingui } from "@lingui/react/macro";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { errorCode } from "@/api/problem";
import type { UserLimits } from "@/api/schemas";
import { useCreateUser, usernameMaxLength } from "@/api/users";
import { Button } from "@/components/Button";
import { ModalDialog, useDismissal } from "@/components/ModalDialog";
import { SettingsCard } from "@/components/SettingsLayout";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { describeProblem } from "@/i18n/problems";
import { accessChosen, LibraryAccessRows, LimitsRows, noAccess } from "./userFields";

/**
 * Add user. The admin sets the username, password and libraries, then
 * lands on the new user's page.
 */
export function AddUserButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onPress={() => setOpen(true)}>
        <Trans>Add user</Trans>
      </Button>
      <ModalDialog isOpen={open} onClose={() => setOpen(false)} title={<Trans>Add a user</Trans>} width="form">
        <AddUserForm onCancel={() => setOpen(false)} />
      </ModalDialog>
    </>
  );
}

function AddUserForm({ onCancel }: { onCancel: () => void }) {
  const { t } = useLingui();
  const create = useCreateUser();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [access, setAccess] = useState(noAccess);
  const [limits, setLimits] = useState<UserLimits>({});
  const code = errorCode(create.error);
  const held = username !== "" || password !== "" || accessChosen(access) || Object.keys(limits).length > 0;
  useDismissal(held ? "ask" : "close");

  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => {
        create.mutate(
          { body: { username, password, libraries: access, limits } },
          { onSuccess: (user) => void navigate({ to: "/admin/users/$userId", params: { userId: user.id } }) },
        );
      }}
      submitLabel={<Trans>Add user</Trans>}
      isPending={create.isPending}
      isSubmitDisabled={!accessChosen(access)}
      onCancel={onCancel}
      error={code !== "nameTaken" ? create.error : null}
    >
      <TextField
        variant="form"
        label={t`Username`}
        value={username}
        onChange={setUsername}
        autoComplete="off"
        isRequired
        maxLength={usernameMaxLength}
        autoFocus
        errorMessage={create.isError && code === "nameTaken" ? describeProblem(code) : undefined}
      />
      <TextField
        variant="form"
        label={t`Password`}
        value={password}
        onChange={setPassword}
        type="password"
        autoComplete="new-password"
        isRequired
        minLength={8}
        description={t`At least 8 characters.`}
      />
      <SettingsCard title={<Trans>Libraries</Trans>}>
        <LibraryAccessRows value={access} onChange={setAccess} />
      </SettingsCard>
      <SettingsCard title={<Trans>Limits</Trans>}>
        <LimitsRows value={limits} onChange={setLimits} />
      </SettingsCard>
    </SubmitForm>
  );
}
