import { getRouteApi } from "@tanstack/react-router";
import { z } from "zod";

/**
 * The detail page's URL state. It holds the chosen version and tracks,
 * which carry into playback, and the open season on a series page.
 */
export const detailSearchSchema = z.object({
  version: z.string().optional().catch(undefined),
  audio: z.string().optional().catch(undefined),
  /** A subtitle track's ID, or `off`. */
  subtitle: z.string().optional().catch(undefined),
  season: z.string().optional().catch(undefined),
});

export type DetailSearch = z.infer<typeof detailSearchSchema>;

const workRoute = getRouteApi("/authed/works/$workId");

/** The detail page's URL state, and a change to it, which replaces it in history. */
export function useDetailSearch() {
  const search = workRoute.useSearch();
  const navigate = workRoute.useNavigate();
  const setSearch = (next: DetailSearch) => void navigate({ search: next, replace: true });
  return [search, setSearch] as const;
}
