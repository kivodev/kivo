import { $api } from "./client";

export const homeQueries = {
  rows: () => $api.queryOptions("get", "/api/v1/home"),
};
