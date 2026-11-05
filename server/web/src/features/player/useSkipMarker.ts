import { useEffect, useMemo, useState } from "react";
import type { PlayStarted } from "@/api/schemas";
import { markerAt, markersOf, type SkipEntry, type SkipMarker, skipEntered, skipShows, skipShowsMs } from "./markers";

/**
 * useSkipMarker returns the marker whose Skip button shows (an intro, a
 * recap or credits, or undefined) and `skip`, which hides it for that
 * marker. Skip shows for 10 seconds after playback enters a marker, whether
 * playing or paused, and also with the controls while playback stays in it.
 * A seek back into the marker shows it again. Skip Credits gives way to up
 * next when `upNext` shows.
 */
export function useSkipMarker(
  play: PlayStarted | undefined,
  live: boolean,
  positionMs: number,
  { controls, upNext }: { controls: boolean; upNext: boolean },
): { marker: SkipMarker | undefined; skip: () => void } {
  const markers = useMemo(() => (play ? markersOf(play) : []), [play]);
  const marker = live ? markerAt(markers, positionMs) : undefined;
  const [entry, setEntry] = useState<SkipEntry>();
  const entered = skipEntered(entry, marker);
  if (entered !== entry) setEntry(entered);

  const startMs = entered?.startMs;
  const counting = entered !== undefined && !entered.timedOut;
  useEffect(() => {
    if (startMs === undefined || !counting) return;
    const timer = setTimeout(
      () => setEntry((e) => (e?.startMs === startMs ? { ...e, timedOut: true } : e)),
      skipShowsMs,
    );
    return () => clearTimeout(timer);
  }, [startMs, counting]);

  return {
    marker: marker && entered && skipShows(marker, entered, { controls, upNext }) ? marker : undefined,
    skip: () => setEntry((e) => e && { ...e, skipped: true }),
  };
}
