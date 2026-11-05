import { useSuspenseQuery } from "@tanstack/react-query";
import { lazy, type ReactNode, Suspense, useEffect, useEffectEvent, useReducer, useRef, useState } from "react";
import type { WorkDetail } from "@/api/schemas";
import { currentUserQueries, useUpdateCurrentUser } from "@/api/users";
import type { PlayerController } from "./controller";
import { initialPlayerState, playerReducer } from "./reducer";
import { type BookPlayer, BookPlayerContext, useBookPlayer } from "./useBookPlayer";
import { useEndedElsewhere } from "./useEndedElsewhere";

// The audio player for books is one audio element owned by the app shell,
// so a book keeps playing while you browse. It runs the same play machinery
// as the video player (PlayerController): plays, heartbeats, and a book's
// parts on one timeline. The controller and the player's screens load only
// once a book plays.

const MiniPlayer = lazy(() => import("./MiniPlayer").then((m) => ({ default: m.MiniPlayer })));

export function BookPlayerProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(playerReducer, initialPlayerState);
  const [book, setBook] = useState<WorkDetail>();
  const audioRef = useRef<HTMLAudioElement>(null);
  const controllerRef = useRef<PlayerController | null>(null);
  const speed = useSuspenseQuery(currentUserQueries.current()).data.playback.bookSpeed ?? 1;
  const update = useUpdateCurrentUser();

  // Set the default rate too, because loading each part resets the rate to the default rate.
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    audio.defaultPlaybackRate = speed;
    audio.playbackRate = speed;
  }, [speed]);

  const controller = async (): Promise<PlayerController | undefined> => {
    const audio = audioRef.current;
    if (!audio) return undefined;
    if (!controllerRef.current) {
      const { PlayerController } = await import("./controller");
      controllerRef.current ??= new PlayerController(audio, dispatch, () => {}, { fallBack: false });
    }
    return controllerRef.current;
  };

  const play = (next: WorkDetail, positionMs?: number) => {
    const current = controllerRef.current;
    if (book?.id === next.id && current?.started) {
      if (positionMs !== undefined) current.seek(positionMs);
      current.resume();
      return;
    }
    setBook(next);
    void controller().then((c) => c?.play({ workId: next.id, positionMs }));
  };

  const close = () => {
    controllerRef.current?.dispose();
    controllerRef.current = null;
    setBook(undefined);
    dispatch({ type: "reset" });
  };

  useEndedElsewhere(state.play?.id, (id, message) => controllerRef.current?.playEnded(id, message));

  // Leaving the page stops the play with a request that outlives the page.
  // A page restored from the back/forward cache starts again where it was.
  // Signing out unmounts the provider, which also stops the play.
  const dispose = useEffectEvent(() => controllerRef.current?.dispose());
  useEffect(() => {
    const onPageHide = () => controllerRef.current?.stopOnExit();
    const onPageShow = (e: PageTransitionEvent) => e.persisted && controllerRef.current?.restore();
    window.addEventListener("pagehide", onPageHide);
    window.addEventListener("pageshow", onPageShow);
    return () => {
      window.removeEventListener("pagehide", onPageHide);
      window.removeEventListener("pageshow", onPageShow);
      dispose();
    };
  }, []);

  const player: BookPlayer = {
    book,
    state,
    speed,
    play,
    run: (command) => {
      const c = controllerRef.current;
      if (c) command(c);
    },
    setSpeed: (next) => {
      const audio = audioRef.current;
      if (audio) {
        audio.defaultPlaybackRate = next;
        audio.playbackRate = next;
      }
      update.mutate({ body: { playback: { bookSpeed: next } } });
    },
    close,
  };

  return (
    <BookPlayerContext value={player}>
      {children}
      {/* biome-ignore lint/a11y/useMediaCaption: a book has no captions. */}
      <audio ref={audioRef} preload="auto" hidden />
    </BookPlayerContext>
  );
}

/** The docked mini player, shown while a book is loaded. The app shell places it. */
export function BookDock() {
  const { book } = useBookPlayer();
  if (!book) return null;
  return (
    <Suspense fallback={null}>
      <MiniPlayer />
    </Suspense>
  );
}
