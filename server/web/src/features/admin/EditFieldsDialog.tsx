import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import type { WorkDetail, WorkUpdate } from "@/api/schemas";
import { useUpdateWork, workQueries } from "@/api/works";
import { ModalDialog } from "@/components/ModalDialog";
import { NumberField } from "@/components/NumberField";
import { Select } from "@/components/Select";
import { fieldText, listOf, SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { changes } from "@/lib/draft";
import { EditBookForm } from "./EditBookForm";

type Props = { work: WorkDetail; isOpen: boolean; onClose: () => void };

/**
 * Edit fields. An edited field is locked, so refreshes keep it, and it
 * shows a lock. Only the fields that changed are sent.
 */
export function EditFieldsDialog({ work, isOpen, onClose }: Props) {
  const title = work.title;
  return (
    <ModalDialog isOpen={isOpen} onClose={onClose} title={<Trans>Edit {title}</Trans>}>
      {work.kind === "book" ? (
        <EditBookForm work={work} onDone={onClose} />
      ) : (
        <EditFieldsForm work={work} onDone={onClose} />
      )}
    </ModalDialog>
  );
}

/** The content rating option for none. */
const unrated = "unrated";

/** The fields that differ from the work, which a save sends and locks. */
function workChanges(work: WorkDetail, data: FormData, contentRating: string): WorkUpdate {
  const text = (name: string) => fieldText(data, name);
  const year = text("year") === "" ? undefined : Number(text("year"));
  return changes(
    {
      title: work.title,
      sortTitle: work.sortTitle ?? work.title,
      originalTitle: work.originalTitle ?? "",
      overview: work.overview ?? "",
      year: work.year,
      contentRating: work.contentRating ?? "",
      genres: work.genres,
      studios: work.studios,
    },
    {
      title: text("title"),
      sortTitle: text("sortTitle"),
      originalTitle: text("originalTitle"),
      overview: text("overview"),
      // A cleared year keeps the work's current year.
      year: year ?? work.year,
      contentRating: contentRating === unrated ? "" : contentRating,
      genres: listOf(text("genres")),
      studios: listOf(text("studios")),
    },
  );
}

function EditFieldsForm({ work, onDone }: { work: WorkDetail; onDone: () => void }) {
  const { t } = useLingui();
  const update = useUpdateWork();
  const locked = new Set(work.lockedFields ?? []);
  const lock = (field: keyof WorkUpdate) => locked.has(field);
  const [contentRating, setContentRating] = useState(work.contentRating || unrated);
  const ratings = useQuery(workQueries.contentRatings(work.id)).data?.items ?? [];
  // The work's own rating stays a choice even when the list lacks it, as
  // with a rating from an NFO.
  const names = ratings.map((r) => r.name);
  if (work.contentRating && !names.includes(work.contentRating)) names.push(work.contentRating);
  const ratingOptions = [{ id: unrated, label: t`Unrated` }, ...names.map((n) => ({ id: n, label: n }))];

  return (
    <SubmitForm
      onSubmit={(data) => {
        const body = workChanges(work, data, contentRating);
        if (Object.keys(body).length === 0) onDone();
        else update.mutate({ params: { path: { id: work.id } }, body }, { onSuccess: onDone });
      }}
      submitLabel={<Trans>Save</Trans>}
      isPending={update.isPending}
      onCancel={onDone}
      error={update.error}
    >
      <TextField
        variant="form"
        label={t`Title`}
        name="title"
        defaultValue={work.title}
        isRequired
        locked={lock("title")}
      />
      <TextField
        variant="form"
        label={t`Sort title`}
        name="sortTitle"
        defaultValue={work.sortTitle ?? work.title}
        isRequired
        locked={lock("sortTitle")}
      />
      <TextField
        variant="form"
        label={t`Original title`}
        name="originalTitle"
        defaultValue={work.originalTitle ?? ""}
        locked={lock("originalTitle")}
      />
      <div className="grid grid-cols-2 gap-4">
        <NumberField
          variant="form"
          label={t`Year`}
          name="year"
          defaultValue={work.year}
          formatOptions={{ useGrouping: false }}
          locked={lock("year")}
        />
        <Select
          variant="form"
          label={t`Content rating`}
          options={ratingOptions}
          selected={contentRating}
          onChange={setContentRating}
          locked={lock("contentRating")}
        />
      </div>
      <TextField
        variant="form"
        label={t`Overview`}
        name="overview"
        defaultValue={work.overview ?? ""}
        multiline
        locked={lock("overview")}
      />
      <TextField
        variant="form"
        label={t`Genres`}
        name="genres"
        defaultValue={work.genres.join(", ")}
        description={t`Separated by commas.`}
        locked={lock("genres")}
      />
      <TextField
        variant="form"
        label={t`Studios`}
        name="studios"
        defaultValue={work.studios.join(", ")}
        description={t`Separated by commas.`}
        locked={lock("studios")}
      />
    </SubmitForm>
  );
}
