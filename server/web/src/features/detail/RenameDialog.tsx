import { Trans, useLingui } from "@lingui/react/macro";
import { ModalDialog } from "@/components/ModalDialog";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";

type Props = {
  isOpen: boolean;
  name: string;
  isPending: boolean;
  error: unknown;
  onRename: (name: string) => void;
  onClose: () => void;
};

/**
 * Renames an author, narrator or series. If another one already has the
 * name, the two are merged, and the dialog says so.
 */
export function RenameDialog({ isOpen, name, isPending, error, onRename, onClose }: Props) {
  const { t } = useLingui();
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} title={<Trans>Rename {name}</Trans>} width="form">
      <SubmitForm
        onSubmit={(data) => {
          const next = String(data.get("name") ?? "").trim();
          if (next === "" || next === name) onClose();
          else onRename(next);
        }}
        submitLabel={<Trans>Rename</Trans>}
        isPending={isPending}
        onCancel={onClose}
        error={error}
      >
        <TextField
          variant="form"
          label={t`Name`}
          name="name"
          defaultValue={name}
          isRequired
          maxLength={500}
          description={t`Using an existing name merges the two. The new name is kept through rescans.`}
        />
      </SubmitForm>
    </ModalDialog>
  );
}
