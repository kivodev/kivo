import { Trans } from "@lingui/react/macro";
import { type ReactNode, useState } from "react";
import { Button, type ButtonVariant } from "@/components/Button";
import { ModalDialog } from "@/components/ModalDialog";
import { AddLibraryForm } from "./AddLibraryForm";

type Props = {
  /** Secondary where adding a library isn't the page's main action. */
  variant?: ButtonVariant;
  /** The button's label, "Add library" unless given. */
  children?: ReactNode;
};

/**
 * "Add library", which opens the form in a dialog. The no-libraries state,
 * the admin Libraries page and setup use it.
 */
export function AddLibraryButton({ variant = "primary", children }: Props) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant={variant} onPress={() => setOpen(true)}>
        {children ?? <Trans>Add library</Trans>}
      </Button>
      <ModalDialog isOpen={open} onClose={() => setOpen(false)} title={<Trans>Add a library</Trans>}>
        <AddLibraryForm onAdded={() => setOpen(false)} onCancel={() => setOpen(false)} />
      </ModalDialog>
    </>
  );
}
