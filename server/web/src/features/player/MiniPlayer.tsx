import { useLingui } from "@lingui/react/macro";
import { useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import type { WorkDetail } from "@/api/schemas";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { ProgressBar } from "@/components/ProgressBar";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { TV_DOCK } from "@/tv/spatial";
import { KeyDispatcher } from "./keyDispatcher";
import { type KeyCommand, mediaKeyCommand } from "./keys";
import { useMediaSession } from "./mediaSession";
import { NowPlaying } from "./NowPlaying";
import { SkipControls } from "./SkipControls";
import { backMs, forwardMs } from "./skips";
import { useBookPlayer } from "./useBookPlayer";
import { useFailureText } from "./useFailureText";
import { useListening } from "./useListening";
import { usePlayerKeys } from "./usePlayerKeys";

/**
 * MiniPlayer is the docked mini player. It sits above the bottom bar on
 * phones and along the bottom of the window elsewhere. It shows the cover,
 * the book's name, the chapter playing, play or pause and the skips, with
 * the whole book's progress along its top edge. Pressing it opens Now
 * Playing. It also drives the lock screen and the OS's media controls,
 * where previous and next move by chapter.
 *
 * On TV, where books are on the web TV build, the dock sits in the
 * content's margins, and its play and stop buttons are focus targets that
 * Down from the last row reaches, and Up leaves for the item it came from.
 * It never takes focus itself. Now Playing
 * and the skips are for a pointer, so the TV's dock leaves them out, and
 * the remote's media keys do the same: a TV sends them to the page as
 * keys, which the Media Session never sees.
 */
export function MiniPlayer() {
  const { book } = useBookPlayer();
  return book ? <Dock book={book} /> : null;
}

/**
 * Gives the remote's media keys to the book while it's docked, through a
 * key dispatcher as the video player's keys go. A video stops the book, so
 * the dock is gone while the video player has the keys; in case both are
 * briefly on screen, the dock's keys come last and leave a key the video
 * player took. Previous and next move by chapter, as the OS's media
 * controls do, or skip back and forward in a book without chapters.
 */
function RemoteMediaKeys({ listening }: { listening: ReturnType<typeof useListening> }) {
  const [keys] = useState(() => new KeyDispatcher());
  const skipBack = () => listening.skip(-backMs);
  const skipForward = () => listening.skip(forwardMs);
  const chapters = listening.chapters.length > 1;
  const commands: Partial<Record<KeyCommand, () => void>> = {
    togglePlay: listening.togglePlay,
    play: () => listening.run((c) => c.resume()),
    pause: () => listening.run((c) => c.pause()),
    skipBack,
    skipForward,
    previous: chapters ? listening.previous : skipBack,
    next: chapters ? listening.next : skipForward,
  };
  usePlayerKeys(
    keys,
    true,
    ({ key }) => mediaKeyCommand(key),
    (command) => {
      if (listening.live) commands[command]?.();
    },
    { last: true },
  );
  return null;
}

function Dock({ book }: { book: WorkDetail }) {
  const { t } = useLingui();
  const [open, setOpen] = useState(false);
  const listening = useListening(book);
  const { state, durationMs, positionMs, chapter, live, speed } = listening;
  const failure = useFailureText(state.failure);
  const authors = book.book?.authors.map((a) => a.name).join(", ");
  const loading = state.status === "loading";

  useMediaSession(
    {
      title: book.title,
      artist: authors,
      album: chapter?.title,
      artwork: book.images.poster,
    },
    {
      play: () => listening.run((c) => c.resume()),
      pause: () => listening.run((c) => c.pause()),
      seekBy: listening.skip,
      seekTo: listening.seek,
      previous: listening.previous,
      next: listening.next,
    },
    state.play ? { positionMs, durationMs, paused: state.paused, rate: speed } : undefined,
  );

  const cover = (
    <>
      <span className="w-12 shrink-0">
        {book.images.poster ? (
          <Artwork image={book.images.poster} title={book.title} shape="square" width={48} />
        ) : (
          // Too small for a tile with a name, and the name is beside it anyway.
          <span className="flex aspect-square items-center justify-center rounded-md bg-placeholder text-on-placeholder">
            <Icon name="book" className="size-5" />
          </span>
        )}
      </span>
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-sm font-semibold text-text">{book.title}</span>
        {failure ? (
          <span role="alert" className="truncate text-xs text-warning">
            {failure}
          </span>
        ) : (
          <span data-part="mini-player-chapter" className="truncate text-xs text-text-muted">
            {chapter?.title ?? authors}
          </span>
        )}
      </span>
    </>
  );
  const playPause = (
    <Button
      variant="ghost"
      size="icon"
      aria-label={state.paused ? t`Play` : t`Pause`}
      isDisabled={!live}
      onPress={listening.togglePlay}
    >
      {loading ? <Icon name="spinner" className="animate-spin" /> : <Icon name={state.paused ? "play" : "pause"} />}
    </Button>
  );

  const progress = (
    <ProgressBar value={durationMs > 0 ? positionMs / durationMs : 0} thin className="absolute inset-x-0 top-0" />
  );
  const stop = (
    <Button
      variant="ghost"
      size="icon"
      aria-label={t`Stop listening`}
      onPress={listening.close}
      className="phone:hidden"
    >
      <Icon name="close" />
    </Button>
  );

  if (isTv) {
    return (
      <TvFocusGroup
        focusKey={TV_DOCK}
        part="mini-player"
        className="fixed inset-x-0 bottom-0 z-30 box-content h-(--layout-mini-player-height) border-t border-border bg-surface pr-(--layout-tv-overscan-x) pb-(--layout-tv-overscan-y) pl-(--layout-tv-content-left)"
      >
        <RemoteMediaKeys listening={listening} />
        {progress}
        <div className="flex h-full items-center gap-2 px-3 sm:px-4">
          <div className="flex min-w-0 flex-1 items-center gap-3">{cover}</div>
          <TvFocusable focusKey="dock:play" part="dock-play">
            {playPause}
          </TvFocusable>
          <TvFocusable focusKey="dock:stop">{stop}</TvFocusable>
        </div>
      </TvFocusGroup>
    );
  }
  return (
    <div
      data-part="mini-player"
      className="fixed inset-x-0 z-30 box-content h-(--layout-mini-player-height) border-t border-border bg-surface pr-(--layout-safe-area-right) pl-(--layout-safe-area-left) phone:bottom-(--layout-bottom-bar-inset) wide:bottom-0 wide:pb-(--layout-safe-area-bottom) wide:max-lg:left-(--layout-sidebar-collapsed-width) lg:left-(--layout-sidebar-width)"
    >
      {progress}
      <div className="flex h-full items-center gap-2 px-3 sm:px-4">
        <AriaButton
          aria-label={t`Now Playing: ${book.title}`}
          onPress={() => setOpen(true)}
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-3 rounded-md text-left"
        >
          {cover}
        </AriaButton>
        <SkipControls live={live} onSkip={listening.skip}>
          {playPause}
        </SkipControls>
        {stop}
      </div>
      <NowPlaying book={book} isOpen={open} onClose={() => setOpen(false)} />
    </div>
  );
}
