import { useLingui } from "@lingui/react/macro";
import { useQueries, useQuery } from "@tanstack/react-query";
import type { Rule, WorkField, WorkQuery } from "@/api/schemas";
import { workQueryQueries } from "@/api/workQuery";
import { countOfKinds, useSmartLabels } from "./smartLabels";
import { isDurationOp, refusal, rulesOf, typedValue } from "./smartQuery";

/**
 * A smart list's count and a line of its rules: "42 movies. Comedy or Drama,
 * no Animation, Year 1990 to 1999, (Rating at least 7 or Runtime at most 90
 * min)". Rules that this client can't edit aren't spelled out.
 */
export function SmartSummary({ query, count }: { query: WorkQuery; count: number }) {
  const { t } = useLingui();
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
  const or = t` or `;
  const and = t` and `;

  const describe = (rule: Rule, field: WorkField): string => {
    const values = rule.values ?? [];
    const label = labels.field(field);
    if (rule.op === "isSet") return t`${label} is set`;
    if (rule.op === "isNotSet") return t`${label} isn't set`;
    const op = labels.op(rule.op, field.type);
    if (field.valueSource) {
      const shown = values.map((v) => names.get(v) ?? v);
      if (rule.op === "noneOf") return t`no ${shown.join(or)}`;
      return shown.join(rule.op === "allOf" ? and : or);
    }
    if (isDurationOp(rule.op) && values[0]) return [label, op, labels.duration(values[0])].join(" ");
    const typed = values.map((v) => {
      const amount = typedValue(field, v);
      return field.unit === "ms" ? t`${amount} min` : amount;
    });
    const [first, second] = typed;
    if (rule.op === "between" && first !== undefined && second !== undefined) return t`${label} ${first} to ${second}`;
    return [label, op, ...typed].join(" ");
  };
  const describeAll = (of: Rule[] = []) =>
    of
      .filter((rule) => rules.includes(rule))
      .flatMap((rule) => {
        const field = fieldOf(rule);
        return field ? [describe(rule, field)] : [];
      });
  const groups = (query.where?.groups ?? []).flatMap((group) => {
    const inner = describeAll(group.rules);
    return inner.length > 0 ? [`(${inner.join(group.match === "all" ? and : or)})`] : [];
  });
  const clauses = [...describeAll(query.where?.rules), ...groups];
  const noun = countOfKinds(count, query.kinds);
  return <>{clauses.length > 0 ? t`${noun}. ${clauses.join(", ")}` : noun}</>;
}
