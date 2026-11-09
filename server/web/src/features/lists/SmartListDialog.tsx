import { plural } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Fragment, type ReactNode, useEffect, useState } from "react";
import {
  Button as AriaButton,
  Checkbox,
  CheckboxGroup,
  Dialog,
  DialogTrigger,
  Input,
  SearchField,
} from "react-aria-components";
import { useCreateList, useUpdateList } from "@/api/lists";
import { errorCode } from "@/api/problem";
import type { SortKey, WorkField, WorkKind, WorkQuery } from "@/api/schemas";
import { workQueryQueries } from "@/api/workQuery";
import { Button } from "@/components/Button";
import { CheckBox } from "@/components/CheckBox";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { ModalDialog, useDismissal } from "@/components/ModalDialog";
import { Popover } from "@/components/Popover";
import { Select } from "@/components/Select";
import { settingsCardClass } from "@/components/SettingsLayout";
import { SubmitForm } from "@/components/SubmitForm";
import { TextField } from "@/components/TextField";
import { countOfKinds, type SmartLabels, useSmartLabels } from "./smartLabels";
import {
  blankGroup,
  blankRule,
  type Draft,
  type DraftRule,
  draftOf,
  emptyRule,
  isDurationOp,
  queryOf,
  refusal,
  ruleComplete,
  storedValue,
  typedValue,
} from "./smartQuery";

const debounceMs = 350;

type Props = {
  /** The list being edited. Without one, Save makes a new list and opens it. */
  listId?: string;
  name?: string;
  /** The query to start from. Without one, a blank list of the catalog's first kind. */
  query?: WorkQuery;
  onClose: () => void;
};

/** The builder of a smart list's query, driven by the field catalog. */
export function SmartListDialog({ listId, name, query, onClose }: Props) {
  const { t } = useLingui();
  const fields = useQuery(workQueryQueries.fields());
  const catalog = fields.data?.items;
  const refused = catalog && query && refusal(query, catalog);
  return (
    <ModalDialog
      isOpen
      onClose={onClose}
      title={listId ? <Trans>Edit Smart List</Trans> : <Trans>New Smart List</Trans>}
      width="prose"
      part="smart-list"
    >
      {fields.isError && <LoadError error={fields.error} onRetry={() => void fields.refetch()} />}
      {refused && (
        <div className="flex flex-col gap-5">
          <p className="text-text-muted">
            {refused === "newerVersion"
              ? t`This list was made by a newer version of Kivo, so it can't be edited here.`
              : t`This list uses a rule this version of Kivo doesn't know, so it can't be edited here.`}
          </p>
          <div className="flex justify-end">
            <Button variant="primary" onPress={onClose}>
              <Trans>Close</Trans>
            </Button>
          </div>
        </div>
      )}
      {catalog && !refused && <Builder fields={catalog} listId={listId} name={name} query={query} onClose={onClose} />}
    </ModalDialog>
  );
}

/** The kinds the catalog offers, in its order. */
const kindsOf = (fields: readonly WorkField[]) => [...new Set(fields.flatMap((f) => f.kinds))];

function Builder({ fields, listId, name = "", query, onClose }: Props & { fields: readonly WorkField[] }) {
  const { t } = useLingui();
  const labels = useSmartLabels();
  const navigate = useNavigate();
  const create = useCreateList();
  const update = useUpdateList();
  const offered = kindsOf(fields);
  const [start] = useState(() => draftOf(name, query ?? { kinds: offered.slice(0, 1) }));
  const [draft, setDraft] = useState(start);
  useDismissal(draft !== start ? "ask" : "close");
  const [sentName, setSentName] = useState<string>();

  // The fields a rule can use with these kinds. The Libraries row is the
  // library rule, unless a saved query holds one it can't show.
  const available = (current?: string) =>
    fields.filter(
      (f) => f.field === current || (f.field !== "library" && f.kinds.some((k) => draft.kinds.includes(k))),
    );
  const newRule = () => (available().length > 0 ? emptyRule() : undefined);
  const fieldOf = (key: string) => fields.find((f) => f.field === key);
  const allRules = [...draft.rules, ...draft.groups.flatMap((g) => g.rules)];
  const complete = allRules.every((rule) => {
    const field = fieldOf(rule.field);
    return field !== undefined && ruleComplete(rule, field);
  });

  const body = JSON.stringify(queryOf(draft));
  const [counted, setCounted] = useState(body);
  useEffect(() => {
    if (body === counted) return;
    const timer = setTimeout(() => setCounted(body), debounceMs);
    return () => clearTimeout(timer);
  }, [body, counted]);
  const count = useQuery({
    ...workQueryQueries.count(JSON.parse(counted) as WorkQuery),
    enabled: complete && counted === body,
    placeholderData: keepPreviousData,
  });

  const error = create.error ?? update.error;
  const code = errorCode(error);
  const trimmed = draft.name.trim();
  const taken = Boolean(error) && code === "nameTaken" && trimmed === sentName;
  const save = () => {
    setSentName(trimmed);
    const smart = { query: queryOf(draft) };
    if (listId) {
      update.mutate({ params: { path: { id: listId } }, body: { name: trimmed, smart } }, { onSuccess: onClose });
    } else {
      create.mutate(
        { body: { name: trimmed, smart } },
        {
          onSuccess: (list) => {
            onClose();
            void navigate({ to: "/lists/$listId", params: { listId: list.id } });
          },
        },
      );
    }
  };
  const librariesField = fieldOf("library");
  const firstSort = draft.sort[0] ?? { field: "title" };
  const defaultDirection = (field?: WorkField) => field?.defaultDirection ?? "asc";
  const chosenSort =
    firstSort.field === "random"
      ? "random"
      : `${firstSort.field}:${firstSort.direction ?? defaultDirection(fieldOf(firstSort.field))}`;
  const sortOptions = [
    ...fields
      .filter((f) => f.sortable && (f.field === firstSort.field || f.kinds.some((k) => draft.kinds.includes(k))))
      .flatMap((f) => {
        const usual = defaultDirection(f);
        return [usual, usual === "asc" ? ("desc" as const) : ("asc" as const)].map((direction) => ({
          id: `${f.field}:${direction}`,
          label: labels.sort(f, direction),
          key: { field: f.field, direction },
        }));
      }),
    { id: "random", label: t`Random`, key: { field: "random" } },
  ];
  const total = count.data?.total ?? 0;
  const noun = countOfKinds(total, draft.kinds);

  return (
    <SubmitForm
      className="flex flex-col gap-5"
      onSubmit={save}
      submitLabel={<Trans>Save</Trans>}
      isPending={create.isPending || update.isPending}
      isSubmitDisabled={trimmed === "" || !complete}
      onCancel={onClose}
      error={code !== "nameTaken" ? error : null}
    >
      <TextField
        variant="form"
        label={t`Name`}
        value={draft.name}
        onChange={(value) => setDraft((d) => ({ ...d, name: value }))}
        autoComplete="off"
        isRequired
        autoFocus
        errorMessage={taken ? t`You already have a list with that name.` : undefined}
      />
      <div className={settingsCardClass}>
        <PickerRow label={t`Show`}>
          <Picker
            label={t`Show`}
            summary={draft.kinds.map(labels.kind).join(", ")}
            options={offered.map((k) => ({ id: k, label: labels.kind(k) }))}
            selected={draft.kinds}
            onChange={(next) => {
              if (next.length === 0) return;
              const kinds = next as WorkKind[];
              const fits = (key: SortKey) =>
                key.field === "random" || fieldOf(key.field)?.kinds.some((k) => kinds.includes(k));
              setDraft((d) => ({ ...d, kinds, sort: d.sort.filter(fits) }));
            }}
          />
        </PickerRow>
        {librariesField && draft.match === "all" && (
          <PickerRow label={t`Libraries`}>
            <ValuePicker
              label={t`Libraries`}
              field={librariesField}
              selected={draft.libraries}
              emptyLabel={t`All`}
              onChange={(next) => setDraft((d) => ({ ...d, libraries: next }))}
            />
          </PickerRow>
        )}
      </div>
      <RuleCard
        labels={labels}
        match={draft.match}
        rules={draft.rules}
        onRules={(rules) => setDraft((d) => ({ ...d, rules }))}
        available={available}
        newRule={newRule}
      >
        <button
          type="button"
          onClick={() => {
            if (available().length > 0) setDraft((d) => ({ ...d, groups: [...d.groups, blankGroup()] }));
          }}
          className="flex min-h-12 w-full cursor-pointer items-center px-4 text-left font-medium text-accent hover:bg-surface-hover"
        >
          <Trans>Add Group</Trans>
        </button>
      </RuleCard>
      {draft.groups.map((group) => (
        <RuleCard
          key={group.key}
          labels={labels}
          match={group.match}
          onMatch={(match) =>
            setDraft((d) => ({ ...d, groups: d.groups.map((g) => (g.key === group.key ? { ...g, match } : g)) }))
          }
          rules={group.rules}
          onRules={(rules) =>
            setDraft((d) => ({
              ...d,
              groups: d.groups.flatMap((g) => (g.key !== group.key ? [g] : rules.length > 0 ? [{ ...g, rules }] : [])),
            }))
          }
          available={available}
          newRule={newRule}
        />
      ))}
      <div className={settingsCardClass}>
        <PickerRow label={t`Sort`}>
          <Select
            variant="compact"
            label={t`Sort`}
            options={sortOptions}
            selected={chosenSort}
            onChange={(id) => {
              const chosen = sortOptions.find((o) => o.id === id);
              if (chosen) setDraft((d) => ({ ...d, sort: [chosen.key] }));
            }}
          />
        </PickerRow>
      </div>
      {complete && count.data && (
        <p data-part="smart-count" className="text-sm text-text-muted tabular-nums">
          {plural(total, { one: `${noun} matches`, other: `${noun} match` })}
        </p>
      )}
    </SubmitForm>
  );
}

function PickerRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-h-12 items-center justify-between gap-4 px-4 py-2">
      <span>{label}</span>
      {children}
    </div>
  );
}

/** A list of rules, its match, Add Rule, and whatever ends the card. The match is a picker when it has `onMatch`. */
function RuleCard({
  labels,
  match,
  onMatch,
  rules,
  onRules,
  available,
  newRule,
  children,
}: {
  labels: SmartLabels;
  match: Draft["match"];
  onMatch?: (match: Draft["match"]) => void;
  rules: DraftRule[];
  onRules: (rules: DraftRule[]) => void;
  available: (current?: string) => WorkField[];
  newRule: () => DraftRule | undefined;
  children?: ReactNode;
}) {
  const { t } = useLingui();
  return (
    <section className="flex flex-col gap-1.5">
      <h3 className="flex items-center gap-1 pl-4 text-sm font-semibold text-text-muted">
        {!onMatch ? (
          match === "any" ? (
            <Trans>Match Any of These</Trans>
          ) : (
            <Trans>Match All of These</Trans>
          )
        ) : (
          <Trans>
            Match{" "}
            <Select
              variant="compact"
              label={t`Match`}
              options={[
                { id: "all", label: t`All` },
                { id: "any", label: t`Any` },
              ]}
              selected={match}
              onChange={onMatch}
            />{" "}
            of These
          </Trans>
        )}
      </h3>
      <div className={settingsCardClass}>
        {rules.map((rule) => (
          <RuleRow
            key={rule.key}
            rule={rule}
            labels={labels}
            available={available}
            onChange={(next) => onRules(rules.map((r) => (r.key === rule.key ? next : r)))}
            onRemove={() => onRules(rules.filter((r) => r.key !== rule.key))}
          />
        ))}
        <button
          type="button"
          onClick={() => {
            const rule = newRule();
            if (rule) onRules([...rules, rule]);
          }}
          className="flex min-h-12 w-full cursor-pointer items-center px-4 text-left font-medium text-accent hover:bg-surface-hover"
        >
          <Trans>Add Rule</Trans>
        </button>
        {children}
      </div>
    </section>
  );
}

function RuleRow({
  rule,
  labels,
  available,
  onChange,
  onRemove,
}: {
  rule: DraftRule;
  labels: SmartLabels;
  available: (current?: string) => WorkField[];
  onChange: (rule: DraftRule) => void;
  onRemove: () => void;
}) {
  const { t } = useLingui();
  const fields = available(rule.field);
  const field = fields.find((f) => f.field === rule.field);
  if (!field && rule.field !== "") return null;
  const arity = field?.ops.find((o) => o.op === rule.op)?.arity ?? 0;
  const setOp = (op: string) => {
    const nextArity = field?.ops.find((o) => o.op === op)?.arity;
    const keep = nextArity === arity && isDurationOp(op) === isDurationOp(rule.op);
    onChange({ ...rule, op, values: keep ? rule.values : [] });
  };
  return (
    <div data-part="rule" className="flex min-h-12 flex-wrap items-center gap-x-2 gap-y-1 px-4 py-2">
      <Select
        variant="compact"
        label={t`Field`}
        options={fields.map((f) => ({ id: f.field, label: labels.field(f) }))}
        selected={rule.field}
        {...(field ? {} : { value: t`Choose` })}
        onChange={(key) => {
          const next = fields.find((f) => f.field === key);
          if (next) onChange({ ...blankRule(next), key: rule.key });
        }}
      />
      {field && (
        <Select
          variant="compact"
          label={t`Condition`}
          options={field.ops.map((o) => ({ id: o.op, label: labels.op(o.op, field.type) }))}
          selected={rule.op}
          onChange={setOp}
        />
      )}
      {field &&
        arity !== 0 &&
        (field.valueSource ? (
          <ValuePicker
            label={labels.field(field)}
            field={field}
            selected={rule.values}
            onChange={(values) => onChange({ ...rule, values })}
          />
        ) : (
          <TypedValues rule={rule} field={field} arity={arity} onChange={(values) => onChange({ ...rule, values })} />
        ))}
      <Button variant="ghost" size="icon" aria-label={t`Remove rule`} className="ml-auto" onPress={onRemove}>
        <Icon name="close" className="size-4" />
      </Button>
    </div>
  );
}

/** A typed value, or two for between. */
function TypedValues({
  rule,
  field,
  arity,
  onChange,
}: {
  rule: DraftRule;
  field: WorkField;
  arity: number;
  onChange: (values: string[]) => void;
}) {
  const { t } = useLingui();
  const labels = useSmartLabels();
  const placeholder = field.type === "date" ? "YYYY-MM-DD" : undefined;
  if (isDurationOp(rule.op)) return <DurationValue value={rule.values[0] ?? ""} onChange={onChange} />;
  return (
    <>
      {Array.from({ length: arity }, (_, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: the inputs are positional and never reorder.
        <Fragment key={i}>
          {i === 1 && (
            <span className="text-sm text-text-muted">
              <Trans>and</Trans>
            </span>
          )}
          <div className={field.type === "text" ? "w-48" : "w-28"}>
            <TextField
              variant="form"
              aria-label={labels.field(field)}
              placeholder={placeholder}
              inputMode={field.type === "number" ? "decimal" : undefined}
              value={typedValue(field, rule.values[i] ?? "")}
              onChange={(text) =>
                onChange(
                  Array.from({ length: arity }, (_, k) =>
                    k === i ? storedValue(field, text) : (rule.values[k] ?? ""),
                  ),
                )
              }
            />
          </div>
        </Fragment>
      ))}
      {field.unit === "ms" && <span className="text-sm text-text-muted">{t`min`}</span>}
    </>
  );
}

/** How far back a relative date looks: a number and a unit, saved as P30D. */
function DurationValue({ value, onChange }: { value: string; onChange: (values: string[]) => void }) {
  const { t } = useLingui();
  const [, amount = "", unit = "D"] = /^P(\d*)([DWMY])$/.exec(value) ?? [];
  return (
    <>
      <div className="w-20">
        <TextField
          variant="form"
          aria-label={t`How many`}
          inputMode="numeric"
          value={amount}
          onChange={(text) => onChange([`P${text}${unit}`])}
        />
      </div>
      <Select
        variant="compact"
        label={t`Unit`}
        options={[
          { id: "D", label: t`days` },
          { id: "W", label: t`weeks` },
          { id: "M", label: t`months` },
          { id: "Y", label: t`years` },
        ]}
        selected={unit}
        onChange={(next) => onChange([`P${amount}${next}`])}
      />
    </>
  );
}

/**
 * The values of a field with values to choose: the field's list, or a
 * search of it for a field with many. A value picked keeps its name
 * whatever the search shows.
 */
function ValuePicker({
  label,
  field,
  selected,
  emptyLabel,
  onChange,
}: {
  label: string;
  field: WorkField;
  selected: string[];
  /** What the picker says with nothing chosen, when that means something. */
  emptyLabel?: string;
  onChange: (values: string[]) => void;
}) {
  const { t } = useLingui();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [q, setQ] = useState("");
  useEffect(() => {
    if (text === q) return;
    const timer = setTimeout(() => setQ(text), debounceMs);
    return () => clearTimeout(timer);
  }, [text, q]);
  const searches = field.valueSource === "search";
  const results = useQuery({
    ...workQueryQueries.values(field.field, searches ? q : ""),
    enabled: open,
    placeholderData: keepPreviousData,
  });
  const named = useQuery({
    ...workQueryQueries.names(field.field, selected),
    enabled: selected.length > 0,
    placeholderData: keepPreviousData,
  });
  const shown = results.data?.items ?? [];
  const names = new Map([...(named.data?.items ?? []), ...shown].map((v) => [v.value, v.name]));
  const nameOf = (value: string) => names.get(value) ?? (named.isFetched ? value : "");
  const options = [
    ...selected.filter((v) => !shown.some((r) => r.value === v)).map((v) => ({ id: v, label: nameOf(v) })),
    ...shown.map((r) => ({ id: r.value, label: r.name })),
  ];
  return (
    <Picker
      label={label}
      summary={selected.length > 0 ? selected.map(nameOf).join(", ") : (emptyLabel ?? t`Choose`)}
      options={options}
      selected={selected}
      onChange={onChange}
      onOpenChange={setOpen}
      search={
        searches ? (
          <SearchField aria-label={t`Search`} value={text} onChange={setText} autoFocus>
            <Input
              placeholder={t`Search`}
              className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm text-text outline-none focus:border-accent"
            />
          </SearchField>
        ) : undefined
      }
    />
  );
}

/** A button naming what's chosen, opening a list to choose any number of. */
function Picker({
  label,
  summary,
  options,
  selected,
  onChange,
  onOpenChange,
  search,
}: {
  label: string;
  summary: string;
  options: readonly { id: string; label: string }[];
  selected: readonly string[];
  onChange: (values: string[]) => void;
  onOpenChange?: (open: boolean) => void;
  search?: ReactNode;
}) {
  return (
    <DialogTrigger onOpenChange={onOpenChange}>
      <AriaButton
        aria-label={`${label}: ${summary}`}
        className="-mx-1 inline-flex max-w-full min-w-0 cursor-pointer items-center gap-0.5 rounded-sm px-1 font-semibold touch-target text-text hover:text-accent"
      >
        <span className="truncate">{summary}</span>
        <Icon name="chevronDown" className="size-4 shrink-0" />
      </AriaButton>
      <Popover placement="bottom end" className="w-72">
        <Dialog aria-label={label} className="flex flex-col gap-1 p-1 outline-none">
          {search}
          <CheckboxGroup
            aria-label={label}
            value={[...selected]}
            onChange={onChange}
            className="flex max-h-72 flex-col overflow-auto"
          >
            {options.map((o) => (
              <Checkbox
                key={o.id}
                value={o.id}
                className="group flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-text hover:bg-surface-hover"
              >
                <CheckBox />
                <span className="truncate">{o.label}</span>
              </Checkbox>
            ))}
          </CheckboxGroup>
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}
