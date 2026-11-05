import type { Chapter, PlayStarted } from "@/api/schemas";

// Skip markers are the parts of a video that the server marks, such as an
// intro, a recap or credits, and that the player offers to skip. They follow
// the shared core's rules.

/** How long Skip shows on its own after playback enters a marker, in wall-clock time. */
export const skipShowsMs = 10_000;
/** A marker shorter than this has no Skip. */
const markerMinMs = 3_000;

/** What a marker skips. The kind names its button. */
type MarkerKind = "intro" | "recap" | "credits";

/**
 * A part of the video the player offers to skip. Final credits have
 * nothing after them but, perhaps, a next episode's preview, so up next
 * appears at their start.
 */
export type SkipMarker = { kind: MarkerKind; startMs: number; endMs: number; final: boolean };

const markerKinds: readonly string[] = ["intro", "recap", "credits"] satisfies MarkerKind[];
const isKind = (kind: string | undefined): kind is MarkerKind => kind !== undefined && markerKinds.includes(kind);

/** The chapter at a position. */
export function chapterAt(chapters: readonly Chapter[], positionMs: number): Chapter | undefined {
  return chapters.find((c) => positionMs >= c.startMs && positionMs < c.endMs);
}

/**
 * A play's markers. These are the server's markers, leaving out kinds the
 * player doesn't know and markers shorter than 3 seconds.
 */
export function markersOf(play: { markers?: PlayStarted["markers"] }): SkipMarker[] {
  return (play.markers ?? []).flatMap((m) => {
    const kind: string = m.kind;
    return isKind(kind) && m.endMs - m.startMs >= markerMinMs
      ? [{ kind, startMs: m.startMs, endMs: m.endMs, final: m.final === true }]
      : [];
  });
}

/** The marker at a position. */
export function markerAt(markers: readonly SkipMarker[], positionMs: number): SkipMarker | undefined {
  return markers.find((m) => positionMs >= m.startMs && positionMs < m.endMs);
}

/**
 * The Skip state for the marker that playback is in. It records which
 * marker, whether its 10 seconds have passed, and whether Skip was pressed.
 */
export type SkipEntry = { startMs: number; timedOut: boolean; skipped: boolean };

/**
 * The entry at `marker`. It is a new entry as playback enters a marker, and
 * undefined outside one. It stays the same object while playback is in the marker.
 */
export function skipEntered(entry: SkipEntry | undefined, marker: SkipMarker | undefined): SkipEntry | undefined {
  if (marker?.startMs === entry?.startMs) return entry;
  return marker && { startMs: marker.startMs, timedOut: false, skipped: false };
}

/**
 * Whether Skip shows in `marker`. It shows for 10 seconds after playback
 * enters the marker, whether playing or paused, and also with the controls
 * while playback stays in it. It never shows once pressed, and Skip Credits
 * gives way to up next.
 */
export function skipShows(
  marker: SkipMarker,
  entry: SkipEntry,
  { controls, upNext }: { controls: boolean; upNext: boolean },
): boolean {
  if (entry.skipped || (marker.kind === "credits" && upNext)) return false;
  return controls || !entry.timedOut;
}
