import { Trans } from "@lingui/react/macro";
import { useState } from "react";
import { Button } from "@/components/Button";
import { ModalDialog } from "@/components/ModalDialog";
import { AddLibraryForm } from "./AddLibraryForm";

/**
 * "Add library", which opens the form in a dialog. The no-libraries state
 * and the admin Libraries page use it.
 */
export function AddLibraryButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onPress={() => setOpen(true)}>
        <Trans>Add library</Trans>
      </Button>
      <ModalDialog isOpen={open} onClose={() => setOpen(false)} title={<Trans>Add a library</Trans>}>
        <AddLibraryForm onAdded={() => setOpen(false)} onCancel={() => setOpen(false)} />
      </ModalDialog>
    </>
  );
}
