import type { BrowseSort, Library, Rule, RuleGroup, SortKey, WorkField, WorkKind, WorkQuery } from "@/api/schemas";
import type { BrowseSearch } from "@/features/browse/browseSearch";

/** The newest query format this client reads. */
const supportedVersion = 1;

type Match = RuleGroup["match"];

export type DraftRule = { key: number; field: string; op: string; values: string[] };
export type DraftGroup = { key: number; match: Match; rules: DraftRule[] };

/**
 * A query as the builder edits it. A root matching all has its `anyOf`
 * library rule as the Libraries row; a root matching any keeps it among its
 * rules, since a row would AND it. Each rule and group has a key for its row.
 * maxWorks, collapse and further sort keys aren't edited and are saved as
 * they were.
 */
export type Draft = {
  name: string;
  kinds: WorkKind[];
  libraries: string[];
  match: Match;
  rules: DraftRule[];
  groups: DraftGroup[];
  sort: SortKey[];
  maxWorks?: number;
  collapse?: WorkQuery["collapse"];
};

let lastKey = 0;
const newKey = () => ++lastKey;

const libraryField = "library";

const draftRule = (rule: Rule): DraftRule => ({ ...rule, key: newKey(), values: rule.values ?? [] });

/** The rule a new row starts with: the field's first operator and no values. */
export function blankRule(field: WorkField): DraftRule {
  return { key: newKey(), field: field.field, op: field.ops[0]?.op ?? "", values: [] };
}

/** A group holding one blank rule. */
export function blankGroup(field: WorkField): DraftGroup {
  return { key: newKey(), match: "any", rules: [blankRule(field)] };
}

/** Why a saved query can't be edited here, if it can't. */
export function refusal(query: WorkQuery, fields: readonly WorkField[]): "newerVersion" | "unknownField" | undefined {
  if ((query.version ?? 0) > supportedVersion) return "newerVersion";
  const known = (rule: Rule) => fields.some((f) => f.field === rule.field && f.ops.some((o) => o.op === rule.op));
  const sortable = (key: SortKey) => key.field === "random" || fields.some((f) => f.field === key.field && f.sortable);
  return rulesOf(query).every(known) && (query.sort ?? []).every(sortable) ? undefined : "unknownField";
}

/** Every rule of a query, the root's and its groups'. */
export function rulesOf(query: WorkQuery): Rule[] {
  return [query.where, ...(query.where?.groups ?? [])].flatMap((g) => g?.rules ?? []);
}

export function draftOf(name: string, query: WorkQuery): Draft {
  const rules = query.where?.rules ?? [];
  const match = query.where?.match ?? "all";
  const libraries = match === "all" ? rules.find((r) => r.field === libraryField && r.op === "anyOf") : undefined;
  return {
    name,
    kinds: query.kinds,
    libraries: libraries?.values ?? [],
    match,
    rules: rules.filter((r) => r !== libraries).map(draftRule),
    groups: (query.where?.groups ?? []).map((g) => ({
      key: newKey(),
      match: g.match,
      rules: (g.rules ?? []).map(draftRule),
    })),
    sort: query.sort ?? [],
    maxWorks: query.maxWorks,
    collapse: query.collapse,
  };
}

const wireRule = ({ field, op, values }: DraftRule): Rule => ({ field, op, ...(values.length > 0 ? { values } : {}) });

/** The query a draft saves. The server stamps its version. */
export function queryOf(draft: Draft): WorkQuery {
  const rules = [
    ...(draft.libraries.length > 0 ? [{ field: libraryField, op: "anyOf", values: draft.libraries }] : []),
    ...draft.rules.map(wireRule),
  ];
  const groups = draft.groups.map((g) => ({ match: g.match, rules: g.rules.map(wireRule) }));
  const where = {
    match: draft.match,
    ...(rules.length > 0 ? { rules } : {}),
    ...(groups.length > 0 ? { groups } : {}),
  };
  return {
    kinds: draft.kinds,
    ...(rules.length > 0 || groups.length > 0 ? { where } : {}),
    ...(draft.sort.length > 0 ? { sort: draft.sort } : {}),
    ...(draft.maxWorks !== undefined ? { maxWorks: draft.maxWorks } : {}),
    ...(draft.collapse !== undefined ? { collapse: draft.collapse } : {}),
  };
}

/** Whether an operator's value is a relative duration. */
export const isDurationOp = (op: string) => op === "within" || op === "notWithin";

const msPerMinute = 60_000;

/** A number value as typed: a field in milliseconds takes minutes. */
export function typedValue(field: WorkField, value: string): string {
  return field.unit === "ms" && value !== "" && Number.isFinite(Number(value))
    ? String(Number(value) / msPerMinute)
    : value;
}

/** The value saved for what was typed. */
export function storedValue(field: WorkField, typed: string): string {
  return field.unit === "ms" && typed.trim() !== "" && Number.isFinite(Number(typed))
    ? String(Math.round(Number(typed) * msPerMinute))
    : typed;
}

const durationForm = /^P[1-9][0-9]{0,3}[DWMY]$/;
const dateForm = /^\d{4}-\d{2}-\d{2}$/;

function validValue(field: WorkField, op: string, value: string): boolean {
  if (isDurationOp(op)) return durationForm.test(value);
  switch (field.type) {
    case "number":
      return value.trim() !== "" && Number.isFinite(Number(value));
    case "date":
      return dateForm.test(value);
    default:
      return value.trim() !== "";
  }
}

/** Whether a rule has every value its operator takes, each well formed. */
export function ruleComplete(rule: DraftRule, field: WorkField): boolean {
  const arity = field.ops.find((o) => o.op === rule.op)?.arity ?? 0;
  if (arity === 0) return true;
  if (arity === -1 ? rule.values.length === 0 : rule.values.length !== arity) return false;
  return rule.values.every((v) => validValue(field, rule.op, v));
}

const gridKinds: Record<Library["kind"], WorkKind> = {
  movies: "movie",
  shows: "series",
  videos: "video",
  audiobooks: "book",
};

// The grid's author sort has no query field, so it isn't carried over.
const gridSortFields: Partial<Record<BrowseSort, string>> = {
  title: "title",
  addedAt: "addedAt",
  releaseDate: "releaseDate",
  rating: "rating",
  myRating: "myRating",
  runtime: "runtimeMs",
  lastPlayedAt: "lastPlayedAt",
  lastEpisodeAddedAt: "lastEpisodeAddedAt",
};

function yearRules(from: number | undefined, to: number | undefined): Rule[] {
  if (from !== undefined && to !== undefined) {
    return [{ field: "year", op: "between", values: [String(from), String(to)] }];
  }
  if (from !== undefined) return [{ field: "year", op: "gte", values: [String(from)] }];
  return to !== undefined ? [{ field: "year", op: "lte", values: [String(to)] }] : [];
}

/** A library grid's filters and sort as a query, leaving out its match state filter. */
export function queryFromGrid(library: Library, search: BrowseSearch): WorkQuery {
  const anyOf = (field: string, values: readonly string[] | undefined) =>
    values && values.length > 0 ? [{ field, op: "anyOf", values: [...values] }] : [];
  const sort = search.sort && gridSortFields[search.sort];
  return {
    kinds: [gridKinds[library.kind]],
    where: {
      match: "all",
      rules: [
        ...anyOf(libraryField, [library.id]),
        ...anyOf("playState", search.played ? [search.played] : undefined),
        ...anyOf("genre", search.genre),
        ...yearRules(search.yearFrom, search.yearTo),
        ...anyOf("contentRating", search.contentRating),
        ...anyOf("resolution", search.resolution),
        ...anyOf("dynamicRange", search.dynamicRange),
        ...anyOf("narrator", search.narratorId ? [search.narratorId] : undefined),
      ],
    },
    ...(sort ? { sort: [{ field: sort }] } : {}),
  };
}
