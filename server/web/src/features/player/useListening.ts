import type { WorkDetail } from "@/api/schemas";
import { bookChapters, chapterIndexAt, nextChapterMs, previousChapterMs } from "./chapters";
import { isLive } from "./reducer";
import { useBookPlayer } from "./useBookPlayer";

/**
 * useListening returns the loaded book as the mini player and Now Playing
 * show it: its position, its chapters, the chapter playing, and the
 * commands both offer.
 */
export function useListening(book: WorkDetail) {
  const player = useBookPlayer();
  const { state, run } = player;
  const play = state.play;
  const durationMs = play?.durationMs ?? book.durationMs ?? 0;
  const positionMs = Math.min(state.positionMs, durationMs);
  const chapters = bookChapters(play?.chapters ?? book.versions[0]?.chapters ?? [], durationMs);
  const index = chapterIndexAt(chapters, positionMs);
  const seekAndPlay = (ms: number) =>
    run((c) => {
      c.seek(ms);
      c.resume();
    });
  return {
    ...player,
    durationMs,
    positionMs,
    chapters,
    index,
    chapter: chapters[index],
    live: isLive(state.status),
    togglePlay: () => run((c) => c.togglePlay()),
    skip: (deltaMs: number) => run((c) => c.skip(deltaMs)),
    seek: (ms: number) => run((c) => c.seek(ms)),
    seekAndPlay,
    previous: () => run((c) => c.seek(previousChapterMs(chapters, c.positionMs()))),
    next: () =>
      run((c) => {
        const ms = nextChapterMs(chapters, c.positionMs());
        if (ms !== undefined) c.seek(ms);
      }),
  };
}
