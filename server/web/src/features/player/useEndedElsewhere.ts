import { useQuery } from "@tanstack/react-query";
import { useEffect, useEffectEvent } from "react";
import { liveQueries } from "@/api/live";

/**
 * useEndedElsewhere tells the player when the live events say its play ended
 * elsewhere. The play may have been stopped by an admin, from another
 * client, or by expiry. An admin's message is passed on if there is one.
 */
export function useEndedElsewhere(
  playId: string | undefined,
  onEnded: (playId: string, message: string | undefined) => void,
) {
  const news = useQuery(liveQueries.play(playId ?? "")).data;
  const ended = news?.ended === true;
  const message = news?.message;
  const endedNow = useEffectEvent(onEnded);
  useEffect(() => {
    if (ended && playId) endedNow(playId, message);
  }, [ended, message, playId]);
}
