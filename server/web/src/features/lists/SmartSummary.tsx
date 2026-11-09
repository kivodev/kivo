import { useLingui } from "@lingui/react/macro";
import { useQueries, useQuery } from "@tanstack/react-query";
import type { Rule, WorkField, WorkQuery } from "@/api/schemas";
import { workQueryQueries } from "@/api/workQuery";
import { countOfKinds, useSmartLabels } from "./smartLabels";
import { isDurationOp, refusal, rulesOf, typedValue } from "./smartQuery";

/**
 * A smart list's count and a line of its rules: "42 movies. Comedy, no
 * Animation, Year 1990 to 1999". Rules that this client can't edit aren't
 * spelled out.
 */
export function SmartSummary({ query, count }: { query: WorkQuery; count: number }) {
  const { t, i18n } = useLingui();
  const labels = useSmartLabels();
  const fields = useQuery(workQueryQueries.fields()).data?.items;
  const rules = fields && !refusal(query, fields) ? rulesOf(query).filter((r) => r.field !== "library") : [];
  const fieldOf = (rule: Rule) => fields?.find((f) => f.field === rule.field);
  const named = [...new Set(rules.filter((r) => fieldOf(r)?.valueSource && r.values?.length).map((r) => r.field))];
  const reads = useQueries({
    queries: named.map((field) =>
      workQueryQueries.names(
        field,
        rules.filter((r) => r.field === field).flatMap((r) => r.values ?? []),
      ),
    ),
  });
  const names = new Map(reads.flatMap((read) => read.data?.items ?? []).map((v) => [v.value, v.name]));
  const list = (values: string[], type: "conjunction" | "disjunction") =>
    new Intl.ListFormat(i18n.locale, { type }).format(values);

  const describe = (rule: Rule, field: WorkField): string => {
    const values = rule.values ?? [];
    const label = labels.field(field);
    const op = labels.op(rule.op, field.type);
    if (field.valueSource) {
      if (values.length === 0) return `${label} ${op}`;
      const shown = values.map((v) => names.get(v) ?? v);
      if (rule.op === "noneOf") return t`no ${list(shown, "conjunction")}`;
      return list(shown, rule.op === "allOf" ? "conjunction" : "disjunction");
    }
    if (isDurationOp(rule.op) && values[0]) return [label, op, labels.duration(values[0])].join(" ");
    const typed = values.map((v) => typedValue(field, v));
    const [first, second] = typed;
    if (rule.op === "between" && first !== undefined && second !== undefined) return t`${label} ${first} to ${second}`;
    return [label, op, ...typed].join(" ");
  };
  const clauses = rules.flatMap((rule) => {
    const field = fieldOf(rule);
    return field ? [describe(rule, field)] : [];
  });
  const noun = countOfKinds(count, query.kinds);
  return <>{clauses.length > 0 ? t`${noun}. ${clauses.join(", ")}` : noun}</>;
}
