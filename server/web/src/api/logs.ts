import { $api, apiBaseUrl } from "./client";
import { apiPaths } from "./paths";

/** The lowest level the Logs page shows. */
export type LogLevel = "error" | "warning" | "info" | "debug";

export const logQueries = {
  /**
   * The newest 1,000 lines at or above a level that contain the search,
   * with the newest last. logsChanged events keep it current.
   */
  list: (level: LogLevel, search: string) =>
    $api.queryOptions("get", "/api/v1/logs", {
      params: { query: { level, limit: 1000, q: search || undefined } },
    }),
};

/** The URL of the recent log as a text file, for a download link. */
export function logFileUrl(): string {
  return `${apiBaseUrl()}${apiPaths.logFile}`;
}
