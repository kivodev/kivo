import { Trans, useLingui } from "@lingui/react/macro";
import { useState } from "react";
import { Label, Radio, RadioGroup } from "react-aria-components";
import { useCreateLibrary } from "@/api/libraries";
import type { Library, LibraryKind } from "@/api/schemas";
import { useDismissal } from "@/components/ModalDialog";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { FoldersField } from "./FoldersField";

type Props = {
  onAdded: (library: Library) => void;
  onCancel?: () => void;
};

/**
 * The kind, name and folders for a new library. Adding it starts its first
 * scan. The name follows the kind until the admin types one.
 */
export function AddLibraryForm({ onAdded, onCancel }: Props) {
  const { t } = useLingui();
  const create = useCreateLibrary();
  const kinds: { id: LibraryKind; label: string }[] = [
    { id: "movies", label: t`Movies` },
    { id: "shows", label: t`Shows` },
    { id: "videos", label: t`Other videos` },
    { id: "audiobooks", label: t`Audiobooks` },
  ];
  const [kind, setKind] = useState<LibraryKind>("movies");
  const [typedName, setTypedName] = useState<string | undefined>(undefined);
  const [folders, setFolders] = useState<string[]>([]);
  const name = typedName ?? kinds.find((k) => k.id === kind)?.label ?? "";
  useDismissal(typedName === undefined && folders.length === 0 ? "close" : "ask");

  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={() => {
        create.mutate(
          { body: { name: name.trim(), kind, folders } },
          {
            onSuccess: (library) => {
              setFolders([]);
              setTypedName(undefined);
              onAdded(library);
            },
          },
        );
      }}
      submitLabel={<Trans>Add library</Trans>}
      isPending={create.isPending}
      isSubmitDisabled={folders.length === 0}
      onCancel={onCancel}
      error={create.error}
    >
      <RadioGroup value={kind} onChange={(id) => setKind(id as LibraryKind)} className="flex flex-col gap-2">
        <Label className="text-sm font-medium text-text">
          <Trans>Kind</Trans>
        </Label>
        <div className="flex flex-wrap gap-2">
          {kinds.map((k) => (
            <Radio
              key={k.id}
              value={k.id}
              className="cursor-pointer rounded-md border border-border px-3 py-1.5 text-sm text-text data-selected:border-accent data-selected:bg-surface-raised data-selected:font-semibold"
            >
              {k.label}
            </Radio>
          ))}
        </div>
      </RadioGroup>
      <TextField variant="form" label={t`Name`} value={name} onChange={setTypedName} isRequired maxLength={100} />
      <FoldersField folders={folders} onChange={setFolders} />
    </SubmitForm>
  );
}
