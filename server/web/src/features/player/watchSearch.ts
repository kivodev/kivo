import { getRouteApi, useRouter } from "@tanstack/react-router";
import { z } from "zod";

/**
 * The player's URL state. It holds the version and tracks chosen on the
 * detail page. It also holds whether to start over rather than resume, or
 * where to start, as after Stop casting.
 */
export const watchSearchSchema = z.object({
  version: z.string().optional().catch(undefined),
  audio: z.string().optional().catch(undefined),
  /** A subtitle track's ID, or `off`. */
  subtitle: z.string().optional().catch(undefined),
  from: z.enum(["start"]).optional().catch(undefined),
  /** Where to start, in milliseconds on the version's timeline. */
  at: z.coerce.number().int().nonnegative().optional().catch(undefined),
});

export type WatchSearch = z.infer<typeof watchSearchSchema>;

const watchRoute = getRouteApi("/authed/watch/$workId");

/**
 * Where the player goes. It goes back to the page that started playback. It
 * replaces this work with another work, or another version of this one. Or
 * it replaces it with the series' page, as after a series' last episode.
 */
export function useWatchNavigation() {
  const { workId } = watchRoute.useParams();
  const navigate = watchRoute.useNavigate();
  const router = useRouter();
  return {
    exit: () =>
      router.history.canGoBack()
        ? router.history.back()
        : void navigate({ to: "/works/$workId", params: { workId }, replace: true }),
    // Back from the next episode or another version still returns to where playback started.
    playWork: (id: string, next?: WatchSearch) =>
      void navigate({ to: "/watch/$workId", params: { workId: id }, search: next ?? {}, replace: true }),
    // The series opens on the season of the episode that ended.
    showSeries: (seriesId: string, seasonId?: string) =>
      void navigate({
        to: "/works/$workId",
        params: { workId: seriesId },
        search: { season: seasonId },
        replace: true,
      }),
  };
}
