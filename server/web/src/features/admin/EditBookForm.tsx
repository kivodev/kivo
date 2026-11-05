import { Trans, useLingui } from "@lingui/react/macro";
import { useState } from "react";
import { Radio, RadioGroup } from "react-aria-components";
import type { BookSeriesUpdate, WorkDetail, WorkUpdate } from "@/api/schemas";
import { useUpdateWork } from "@/api/works";
import { Button } from "@/components/Button";
import { FieldLabel } from "@/components/FieldLabel";
import { Icon } from "@/components/Icon";
import { fieldText, linesOf, listOf, SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { changes } from "@/lib/draft";

type Series = { name: string; position: string };

/** The book's ASIN, kept as its Audible external ID. */
function asinOf(work: WorkDetail): string {
  return work.externalIds.find((x) => x.provider === "audible")?.id ?? "";
}

/** A series as a save sends it. */
const seriesUpdate = (name: string, position: string | undefined, primary: boolean): BookSeriesUpdate => ({
  name,
  primary,
  position: position || undefined,
});

/** The fields that differ from the book, which a save sends and locks. */
function bookChanges(work: WorkDetail, data: FormData, series: Series[], primary: number): WorkUpdate {
  const text = (name: string) => fieldText(data, name);
  const book = work.book ?? { authors: [], narrators: [], series: [] };
  // With no row marked primary, the server makes the first one primary.
  const rows = series
    .map((s, i) => seriesUpdate(s.name.trim(), s.position.trim(), i === primary))
    .filter((s) => s.name !== "");
  const first = rows[0];
  if (first && !rows.some((r) => r.primary)) first.primary = true;
  const body: WorkUpdate = changes(
    { title: work.title, overview: work.overview ?? "", genres: work.genres },
    { title: text("title"), overview: text("overview"), genres: listOf(text("genres")) },
  );
  const changed = changes(
    {
      subtitle: book.subtitle ?? "",
      publisher: book.publisher ?? "",
      releaseDate: book.releaseDate ?? "",
      asin: asinOf(work),
      authors: book.authors.map((p) => p.name),
      narrators: book.narrators.map((p) => p.name),
      series: book.series.map((s) => seriesUpdate(s.name, s.position, s.primary)),
    },
    {
      subtitle: text("subtitle"),
      publisher: text("publisher"),
      releaseDate: text("releaseDate"),
      asin: text("asin"),
      authors: linesOf(text("authors")),
      narrators: linesOf(text("narrators")),
      series: rows,
    },
  );
  if (Object.keys(changed).length > 0) body.book = changed;
  return body;
}

/**
 * Edit fields for a book. It edits the title, subtitle, authors and
 * narrators in order, series with positions and the primary one, ASIN,
 * publisher, description, genres and release date. Each edited field
 * locks.
 */
export function EditBookForm({ work, onDone }: { work: WorkDetail; onDone: () => void }) {
  const { t } = useLingui();
  const update = useUpdateWork();
  const book = work.book ?? { authors: [], narrators: [], series: [] };
  const locked = new Set(work.lockedFields ?? []);
  const lock = (field: string) => locked.has(field);
  const [series, setSeries] = useState<Series[]>(() =>
    book.series.map((s) => ({ name: s.name, position: s.position ?? "" })),
  );
  const [primary, setPrimary] = useState(() =>
    Math.max(
      0,
      book.series.findIndex((s) => s.primary),
    ),
  );
  const setRow = (i: number, patch: Partial<Series>) =>
    setSeries(series.map((s, j) => (j === i ? { ...s, ...patch } : s)));

  return (
    <SubmitForm
      onSubmit={(data) => {
        const body = bookChanges(work, data, series, primary);
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
        label={t`Subtitle`}
        name="subtitle"
        defaultValue={book.subtitle ?? ""}
        locked={lock("book.subtitle")}
      />
      <div className="grid grid-cols-2 gap-4">
        <TextField
          variant="form"
          label={t`Authors`}
          name="authors"
          multiline
          defaultValue={book.authors.map((p) => p.name).join("\n")}
          description={t`One per line, in order.`}
          locked={lock("book.authors")}
        />
        <TextField
          variant="form"
          label={t`Narrators`}
          name="narrators"
          multiline
          defaultValue={book.narrators.map((p) => p.name).join("\n")}
          description={t`One per line, in order.`}
          locked={lock("book.narrators")}
        />
      </div>
      <fieldset className="flex flex-col gap-2">
        <legend>
          <FieldLabel label={t`Series`} locked={lock("book.series")} />
        </legend>
        <RadioGroup
          aria-label={t`Primary series`}
          value={String(primary)}
          onChange={(v) => setPrimary(Number(v))}
          className="flex flex-col gap-2"
        >
          {series.map((s, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: rows are edited in place, by position.
            <div key={i} className="flex items-end gap-2">
              <div className="flex-1">
                <TextField
                  variant="form"
                  label={t`Series name`}
                  value={s.name}
                  onChange={(name) => setRow(i, { name })}
                />
              </div>
              <div className="w-24">
                <TextField
                  variant="form"
                  label={t`Book`}
                  value={s.position}
                  onChange={(position) => setRow(i, { position })}
                />
              </div>
              <Radio value={String(i)} className="group flex h-10 cursor-pointer items-center gap-1 text-sm text-text">
                <span className="size-4 rounded-full border border-border group-data-selected:border-accent group-data-selected:bg-accent" />
                <Trans>Primary</Trans>
              </Radio>
              <Button
                size="icon"
                aria-label={t`Remove ${s.name || t`series`}`}
                onPress={() => {
                  setSeries(series.filter((_, j) => j !== i));
                  if (primary >= i && primary > 0) setPrimary(primary - 1);
                }}
              >
                <Icon name="close" />
              </Button>
            </div>
          ))}
        </RadioGroup>
        <Button size="sm" className="self-start" onPress={() => setSeries([...series, { name: "", position: "" }])}>
          <Icon name="plus" className="size-4" />
          <Trans>Add series</Trans>
        </Button>
      </fieldset>
      <div className="grid grid-cols-2 gap-4">
        <TextField
          variant="form"
          label={t`ASIN`}
          name="asin"
          defaultValue={asinOf(work)}
          maxLength={20}
          locked={lock("book.asin")}
        />
        <TextField
          variant="form"
          label={t`Release date`}
          name="releaseDate"
          defaultValue={book.releaseDate ?? ""}
          description={t`A year, YYYY-MM or YYYY-MM-DD.`}
          pattern="\d{4}(-\d{2}(-\d{2})?)?"
          locked={lock("book.releaseDate")}
        />
      </div>
      <TextField
        variant="form"
        label={t`Publisher`}
        name="publisher"
        defaultValue={book.publisher ?? ""}
        locked={lock("book.publisher")}
      />
      <TextField
        variant="form"
        label={t`Description`}
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
    </SubmitForm>
  );
}
