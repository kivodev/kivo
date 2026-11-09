import { useLingui } from "@lingui/react/macro";
import type { ReactNode } from "react";
import { useState } from "react";
import { errorCode } from "@/api/problem";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";

type Props = {
  initialName?: string;
  submitLabel: ReactNode;
  isPending: boolean;
  error: unknown;
  onSubmit: (name: string) => void;
  onCancel: () => void;
};

/**
 * A list's name, for making or renaming a list. A name in use says so under
 * the field until it's changed, since a field marked invalid can't submit.
 */
export function ListNameForm({ initialName = "", submitLabel, isPending, error, onSubmit, onCancel }: Props) {
  const { t } = useLingui();
  const [name, setName] = useState(initialName);
  const [sent, setSent] = useState<string>();
  const code = errorCode(error);
  const trimmed = name.trim();
  const taken = Boolean(error) && code === "nameTaken";
  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => {
        setSent(trimmed);
        onSubmit(trimmed);
      }}
      submitLabel={submitLabel}
      isPending={isPending}
      isSubmitDisabled={trimmed === ""}
      onCancel={onCancel}
      error={code !== "nameTaken" ? error : null}
    >
      <TextField
        variant="form"
        label={t`Name`}
        value={name}
        onChange={setName}
        autoComplete="off"
        isRequired
        autoFocus
        errorMessage={taken && trimmed === sent ? t`You already have a list with that name.` : undefined}
      />
    </SubmitForm>
  );
}
