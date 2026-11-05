import { $api } from "./client";

export const searchQueries = {
  results: (q: string) => $api.queryOptions("get", "/api/v1/search", { params: { query: { q, limit: 100 } } }),
};
