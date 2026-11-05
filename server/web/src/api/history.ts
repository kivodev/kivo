import { client } from "./client";
import { keysetPages } from "./pages";
import { apiPaths } from "./paths";

// Watch history. It lists your plays, and for admins every user's plays,
// newest first, one keyset page at a time.

export const historyQueries = {
  /** Your own plays. */
  mine: () =>
    keysetPages(["get", apiPaths.currentUserPlayEvents, "pages"] as const, async (after, signal) => {
      const { data, error } = await client.GET("/api/v1/users/me/play-events", {
        signal,
        params: { query: { after } },
      });
      if (error) throw error;
      return data;
    }),
  /** Every user's plays, or one user's plays with `userId`, with who played them. */
  everyone: (userId?: string) => {
    const query = { userId };
    return keysetPages(["get", apiPaths.playEvents, { params: { query } }, "pages"] as const, async (after, signal) => {
      const { data, error } = await client.GET("/api/v1/play-events", {
        signal,
        params: { query: { ...query, after } },
      });
      if (error) throw error;
      return data;
    });
  },
};
