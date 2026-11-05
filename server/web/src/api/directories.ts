import { $api } from "./client";

export const directoryQueries = {
  /** The server's filesystem roots, or one folder's subfolders, for the folder picker. */
  list: (path?: string) =>
    $api.queryOptions("get", "/api/v1/directories", { params: { query: path === undefined ? {} : { path } } }),
};
