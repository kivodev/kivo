import { $api, client } from "./client";
import { keysetPages } from "./pages";
import { apiPaths } from "./paths";
import type { ActivityGroup } from "./schemas";

const pageSize = 50;

// What happened on the server, newest first. The Overview shows the latest
// events. The Activity page shows them all, one keyset page at a time.

export const activityQueries = {
  /** The newest six events, for the Overview. */
  latest: () => $api.queryOptions("get", "/api/v1/activity", { params: { query: { limit: 6 } } }),
  /** Every event, or every event in one group, a page at a time. */
  list: (group?: ActivityGroup) => {
    const query = { limit: pageSize, group };
    return keysetPages(["get", apiPaths.activity, { params: { query } }, "pages"] as const, async (after, signal) => {
      const { data, error } = await client.GET("/api/v1/activity", {
        signal,
        params: { query: { ...query, after } },
      });
      if (error) throw error;
      return data;
    });
  },
};
