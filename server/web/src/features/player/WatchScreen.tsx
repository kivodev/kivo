import { getRouteApi, lazyRouteComponent } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { isTv } from "@/tv/flag";
import { useCast } from "./CastButton";
import { useBookPlayer } from "./useBookPlayer";

// The player is its own chunk, with hls.js loaded only when a play needs it.
const PlayerScreen = lazyRouteComponent(() => import("./PlayerScreen"), "PlayerScreen");
// The Cast remote isn't in the TV build; the flag is read directly, as in
// CastButton, so the bundler drops the import.
const CastRemote =
  import.meta.env.VITE_KIVO_TV === "1" ? undefined : lazyRouteComponent(() => import("./CastRemote"), "CastRemote");

const watchRoute = getRouteApi("/authed/watch/$workId");

/**
 * WatchScreen is the player's page. When connected to a Cast device, the
 * work plays on the TV and the page is its remote.
 */
export function WatchScreen() {
  const { workId } = watchRoute.useParams();
  const { version } = watchRoute.useSearch();
  // A video stops the book that is playing. The book keeps its place.
  const { book, close } = useBookPlayer();
  const listening = book !== undefined;
  useEffect(() => {
    if (listening) close();
  }, [listening, close]);
  // The run counts episodes that up next played on by themselves in a row
  // without input. It lasts across each episode's player, and "Still
  // watching?" comes after three. Leaving the player ends the run.
  const [autoplayRun, setAutoplayRun] = useState(0);
  const casting = useCast().connection === "connected";
  if (CastRemote && casting) return <CastRemote />;
  return (
    <PlayerScreen
      // A new work or version gets a new player, with fresh state.
      key={`${workId}:${version ?? ""}`}
      // The TV build is driven by a remote.
      remote={isTv}
      autoplayRun={autoplayRun}
      onAutoplayRun={setAutoplayRun}
    />
  );
}
