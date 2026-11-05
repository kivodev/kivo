import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useState } from "react";
import { Label, Radio, RadioGroup } from "react-aria-components";
import { useUpdateBooks } from "@/api/books";
import type { WorksUpdate } from "@/api/schemas";
import { ModalDialog } from "@/components/ModalDialog";
import { fieldText, linesOf, listOf, SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";

type Props = { workIds: string[]; isOpen: boolean; onClose: () => void; onDone: () => void };

const optionClass =
  "cursor-pointer rounded-md border border-border px-3 py-1.5 text-sm text-text data-selected:border-accent data-selected:bg-surface-raised data-selected:font-semibold";

/**
 * The batch edit for an audiobooks library's multi-select. Each field
 * filled in is set on every selected book and locked there. An empty field
 * is left as each book has it.
 */
export function BatchEditDialog({ workIds, isOpen, onClose, onDone }: Props) {
  const { t } = useLingui();
  const update = useUpdateBooks();
  const [position, setPosition] = useState("keep");
  const count = workIds.length;

  return (
    <ModalDialog
      isOpen={isOpen}
      onClose={onClose}
      title={<Plural value={count} one="Edit # book" other="Edit # books" />}
    >
      <SubmitForm
        onSubmit={(data) => {
          const text = (name: string) => fieldText(data, name);
          const body: WorksUpdate = { workIds };
          if (linesOf(text("authors")).length > 0) body.authors = linesOf(text("authors"));
          if (linesOf(text("narrators")).length > 0) body.narrators = linesOf(text("narrators"));
          const genres = listOf(text("genres"));
          if (genres.length > 0) body.genres = genres;
          if (text("series") !== "") body.series = { name: text("series"), keepPosition: position === "keep" };
          if (Object.keys(body).length === 1) onClose();
          else update.mutate({ body }, { onSuccess: onDone });
        }}
        submitLabel={<Trans>Save</Trans>}
        isPending={update.isPending}
        onCancel={onClose}
        error={update.error}
      >
        <p className="text-sm text-text-muted">
          <Trans>Empty fields aren't changed.</Trans>
        </p>
        <TextField
          variant="form"
          label={t`Authors`}
          name="authors"
          multiline
          description={t`One per line, in order.`}
        />
        <TextField
          variant="form"
          label={t`Narrators`}
          name="narrators"
          multiline
          description={t`One per line, in order.`}
        />
        <TextField
          variant="form"
          label={t`Series`}
          name="series"
          description={t`Replaces each book's series with this one.`}
        />
        <RadioGroup value={position} onChange={setPosition} className="flex flex-col gap-2">
          <Label className="text-sm font-medium text-text">
            <Trans>Position in the series</Trans>
          </Label>
          <div className="flex flex-wrap gap-2">
            <Radio value="keep" className={optionClass}>
              <Trans>Keep current</Trans>
            </Radio>
            <Radio value="clear" className={optionClass}>
              <Trans>Clear</Trans>
            </Radio>
          </div>
        </RadioGroup>
        <TextField variant="form" label={t`Genres`} name="genres" description={t`Separated by commas.`} />
      </SubmitForm>
    </ModalDialog>
  );
}
