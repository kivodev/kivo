import { Trans, useLingui } from "@lingui/react/macro";
import { useQueries } from "@tanstack/react-query";
import { type ReactNode, useEffect, useState } from "react";
import {
  Button as AriaButton,
  Checkbox,
  CheckboxGroup,
  Dialog,
  DialogTrigger,
  Label,
  Popover,
  Radio,
  RadioGroup,
} from "react-aria-components";
import type { WorkFieldValue } from "@/api/schemas";
import { useIsAdmin } from "@/api/users";
import { workQueryQueries } from "@/api/workQuery";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { useTvDialog } from "@/tv/forms/dialog";
import { focusTv } from "@/tv/spatial";
import {
  type BrowseSearch,
  dynamicRangeValues,
  hasFilters,
  matchStateValues,
  playedValues,
  resolutions,
  useBrowseSearch,
} from "./browseSearch";
import type { BrowseLabels } from "./useBrowseLabels";

const videoFields = ["genre", "year", "contentRating", "resolution", "dynamicRange"] as const;
const bookFields = ["genre", "year", "narrator"] as const;

/** The values a library's works have, by the field each filter is a rule on. */
export type FilterValues = Record<(typeof videoFields | typeof bookFields)[number], WorkFieldValue[]>;

/**
 * What a library's filters offer. A field has no values until its read
 * arrives, if its read failed, and when the library's kind has no such
 * filter.
 */
export function useFilterValues(libraryId: string, books: boolean): FilterValues {
  const fields = books ? bookFields : videoFields;
  const reads = useQueries({ queries: fields.map((field) => workQueryQueries.libraryValues(field, libraryId)) });
  const read = Object.fromEntries(fields.map((field, i) => [field, reads[i]?.data?.items ?? []]));
  return { genre: [], year: [], contentRating: [], resolution: [], dynamicRange: [], narrator: [], ...read };
}

type Props = {
  /** The values this library's works have. */
  values: FilterValues;
  labels: BrowseLabels;
  /** Books have narrators to filter by, and no video to filter by. */
  books: boolean;
};

const optionClass =
  "cursor-pointer rounded-full border border-border px-3 py-1 text-sm text-text data-selected:border-accent data-selected:bg-surface-hover data-selected:font-semibold";

type Choice = { id: string; label: ReactNode };

function Group({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <Label className="text-xs font-semibold tracking-wide text-text-muted uppercase">{label}</Label>
      <div className="flex flex-wrap gap-2">{children}</div>
    </div>
  );
}

/**
 * One choice among options, as radio buttons. On TV they are buttons
 * instead: a radio group takes the arrow keys for its own choice, which
 * would change the choice rather than move focus.
 */
function SingleChoice({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: readonly Choice[];
  onChange: (value: string) => void;
}) {
  if (isTv) {
    return (
      <TvGroup label={label}>
        {options.map((o) => (
          <TvOption
            key={o.id}
            focusKey={`filter:${label}:${o.id}`}
            selected={o.id === value}
            onPress={() => onChange(o.id)}
          >
            {o.label}
          </TvOption>
        ))}
      </TvGroup>
    );
  }
  return (
    <RadioGroup aria-label={label} value={value} onChange={onChange}>
      <Group label={label}>
        {options.map((o) => (
          <Radio key={o.id} value={o.id} className={optionClass}>
            {o.label}
          </Radio>
        ))}
      </Group>
    </RadioGroup>
  );
}

/** Any number of options, as checkboxes, or as buttons on TV, as SingleChoice is. */
function MultiChoice({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: readonly string[];
  options: readonly Choice[];
  onChange: (value: string[]) => void;
}) {
  if (isTv) {
    return (
      <TvGroup label={label}>
        {options.map((o) => (
          <TvOption
            key={o.id}
            focusKey={`filter:${label}:${o.id}`}
            selected={value.includes(o.id)}
            onPress={() => onChange(value.includes(o.id) ? value.filter((v) => v !== o.id) : [...value, o.id])}
          >
            {o.label}
          </TvOption>
        ))}
      </TvGroup>
    );
  }
  return (
    <CheckboxGroup aria-label={label} value={[...value]} onChange={onChange}>
      <Group label={label}>
        {options.map((o) => (
          <Checkbox key={o.id} value={o.id} className={optionClass}>
            {o.label}
          </Checkbox>
        ))}
      </Group>
    </CheckboxGroup>
  );
}

function TvGroup({ label, children }: { label: string; children: ReactNode }) {
  return <Group label={label}>{children}</Group>;
}

function TvOption({
  focusKey,
  selected,
  onPress,
  className,
  children,
}: {
  focusKey: string;
  selected: boolean;
  onPress: () => void;
  className?: string;
  children: ReactNode;
}) {
  return (
    <TvFocusable focusKey={focusKey} className={className}>
      <AriaButton
        aria-pressed={selected}
        data-selected={selected || undefined}
        onPress={onPress}
        className={optionClass}
      >
        {children}
      </AriaButton>
    </TvFocusable>
  );
}

// Focus starts on the dialog's first option as it opens, once the options
// have registered, which is after this does.
function FocusFirst() {
  useEffect(() => {
    const timer = setTimeout(() => focusTv("filters"), 0);
    return () => clearTimeout(timer);
  }, []);
  return null;
}

function set<K extends keyof BrowseSearch>(search: BrowseSearch, key: K, value: BrowseSearch[K]): BrowseSearch {
  return { ...search, [key]: Array.isArray(value) && value.length === 0 ? undefined : value };
}

/**
 * The filter menu. Filters combine with AND and apply at once.
 * Genres, content ratings, decades, resolutions and dynamic ranges offer
 * only what the library has.
 */
export function FilterPanel({ values, labels, books }: Props) {
  const { t } = useLingui();
  const isAdmin = useIsAdmin();
  const [search, setSearch] = useBrowseSearch();
  const any = "any";
  const active = hasFilters(search);
  const decade = search.yearFrom !== undefined && search.yearTo === search.yearFrom + 9 ? String(search.yearFrom) : any;
  const [open, setOpen] = useState(false);
  useTvDialog(open, () => setOpen(false));
  const anyOption = { id: any, label: <Trans>Any</Trans> };
  const has = (field: keyof FilterValues, value: string) => values[field].some((v) => v.value === value);
  const genres = values.genre.map((v) => v.value);
  const contentRatings = values.contentRating.map((v) => v.value);
  const offeredResolutions = resolutions.filter((r) => has("resolution", r));
  const offeredRanges = dynamicRangeValues.filter((h) => has("dynamicRange", h));

  return (
    <DialogTrigger isOpen={open} onOpenChange={setOpen}>
      <Button size="sm" aria-label={active ? t`Filter, some on` : t`Filter`}>
        <Trans>Filter</Trans>
        {active ? <span className="size-2 rounded-full bg-accent" /> : <Icon name="chevronDown" className="size-4" />}
      </Button>
      <Popover
        placement="bottom end"
        className="w-80 rounded-lg border border-border bg-surface-raised p-4 shadow-overlay tv:w-160 tv:overflow-y-auto"
      >
        <Dialog aria-label={t`Filters`} className="flex flex-col gap-5 outline-none">
          <TvFocusGroup focusKey="filters" trap className="flex flex-col gap-5">
            {isTv && <FocusFirst />}
            <SingleChoice
              label={t`Watch status`}
              value={search.played ?? any}
              options={[anyOption, ...playedValues.map((p) => ({ id: p, label: labels.played[p] }))]}
              onChange={(v) =>
                setSearch(
                  set(
                    search,
                    "played",
                    playedValues.find((p) => p === v),
                  ),
                )
              }
            />
            <SingleChoice
              label={t`Decade`}
              value={decade}
              options={[
                anyOption,
                ...decadesOf(values.year.map((v) => Number(v.value))).map((d) => ({
                  id: String(d),
                  label: labels.decade(d),
                })),
              ]}
              onChange={(v) => {
                const from = v === any ? undefined : Number(v);
                setSearch(set(set(search, "yearFrom", from), "yearTo", from === undefined ? undefined : from + 9));
              }}
            />
            {genres.length > 0 && (
              <MultiChoice
                label={t`Genre`}
                value={search.genre ?? []}
                options={genres.map((g) => ({ id: g, label: g }))}
                onChange={(v) =>
                  setSearch(
                    set(
                      search,
                      "genre",
                      genres.filter((g) => v.includes(g)),
                    ),
                  )
                }
              />
            )}
            {books && values.narrator.length > 0 && (
              <SingleChoice
                label={t`Narrator`}
                value={search.narratorId ?? any}
                options={[anyOption, ...values.narrator.map((n) => ({ id: n.value, label: n.name }))]}
                onChange={(v) => setSearch(set(search, "narratorId", v === any ? undefined : v))}
              />
            )}
            {!books && contentRatings.length > 0 && (
              <MultiChoice
                label={t`Content rating`}
                value={search.contentRating ?? []}
                options={contentRatings.map((r) => ({ id: r, label: r }))}
                onChange={(v) =>
                  setSearch(
                    set(
                      search,
                      "contentRating",
                      contentRatings.filter((r) => v.includes(r)),
                    ),
                  )
                }
              />
            )}
            {!books && offeredResolutions.length > 0 && (
              <MultiChoice
                label={t`Resolution`}
                value={search.resolution ?? []}
                options={offeredResolutions.map((r) => ({ id: r, label: labels.resolution[r] }))}
                onChange={(v) =>
                  setSearch(
                    set(
                      search,
                      "resolution",
                      offeredResolutions.filter((r) => v.includes(r)),
                    ),
                  )
                }
              />
            )}
            {!books && offeredRanges.length > 0 && (
              <MultiChoice
                label={t`Dynamic range`}
                value={search.dynamicRange ?? []}
                options={offeredRanges.map((h) => ({ id: h, label: labels.dynamicRange[h] }))}
                onChange={(v) =>
                  setSearch(
                    set(
                      search,
                      "dynamicRange",
                      offeredRanges.filter((h) => v.includes(h)),
                    ),
                  )
                }
              />
            )}
            {isAdmin && !isTv && (
              <SingleChoice
                label={t`Match`}
                value={search.matchState ?? any}
                options={[anyOption, ...matchStateValues.map((m) => ({ id: m, label: labels.matchState[m] }))]}
                onChange={(v) =>
                  setSearch(
                    set(
                      search,
                      "matchState",
                      matchStateValues.find((m) => m === v),
                    ),
                  )
                }
              />
            )}
          </TvFocusGroup>
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}

/** The decades a library's years fall in, newest first. */
function decadesOf(years: readonly number[]): number[] {
  return [...new Set(years.map((y) => y - (y % 10)))].sort((a, b) => b - a);
}
