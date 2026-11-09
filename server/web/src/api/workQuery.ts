import { $api } from "./client";
import type { WorkQuery } from "./schemas";

export const workQueryQueries = {
  /** What rules and sorts can name. */
  fields: () => $api.queryOptions("get", "/api/v1/work-fields"),

  /** A field's values, those matching `q` when given. */
  values: (field: string, q: string) =>
    $api.queryOptions("get", "/api/v1/work-fields/{field}/values", {
      params: { path: { field }, query: q ? { q } : {} },
    }),

  /** The names of a field's values. */
  names: (field: string, ids: string[]) =>
    $api.queryOptions("get", "/api/v1/work-fields/{field}/values", {
      params: { path: { field }, query: { ids } },
    }),

  /** How many works a query matches, as the page's `total`. */
  count: (query: WorkQuery) => $api.queryOptions("post", "/api/v1/works/query", { body: { query, limit: 1 } }),
};
