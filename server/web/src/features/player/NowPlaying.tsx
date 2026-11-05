import { Trans, useLingui } from "@lingui/react/macro";
import { type CSSProperties, useState } from "react";
import { Dialog, Heading, Modal, ModalOverlay, Slider, SliderThumb, SliderTrack } from "react-aria-components";
import type { Chapter, WorkDetail } from "@/api/schemas";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { Icon } from "@/components/Icon";
import { ProgressBar } from "@/components/ProgressBar";
import { Select } from "@/components/Select";
import { formatClock, formatRuntime } from "@/i18n/format";
import { percent } from "./ScrubBar";
import { SkipControls } from "./SkipControls";
import { useFailureText } from "./useFailureText";
import { useListening } from "./useListening";

/** The speeds offered, from 0.5x to 3x. */
const speeds = [0.5, 0.75, 0.8, 0.9, 1, 1.1, 1.2, 1.25, 1.3, 1.4, 1.5, 1.6, 1.75, 2, 2.25, 2.5, 2.75, 3];

/** Arrow keys on the chapter's seek bar step this far. */
const stepMs = 10_000;

type Props = { book: WorkDetail; isOpen: boolean; onClose: () => void };

/**
 * NowPlaying opens from the mini player. It shows the large cover, the
 * book's name, authors and narrators, and a seek bar over the chapter
 * playing with the whole book's progress under it. It also has previous and
 * next chapter, the skips, the speed and the chapter list. It is full screen
 * on phones.
 */
export function NowPlaying({ book, isOpen, onClose }: Props) {
  const { t } = useLingui();
  const listening = useListening(book);
  const { state, durationMs, positionMs, chapters, index, chapter, live, speed } = listening;
  const failure = useFailureText(state.failure);
  const names = (people: readonly { name: string }[] | undefined) => people?.map((p) => p.name).join(", ");
  const authors = names(book.book?.authors);
  const narrators = names(book.book?.narrators);
  const speedLabel = (s: number) => t`${s}×`;

  return (
    <ModalOverlay
      isOpen={isOpen}
      onOpenChange={(open) => !open && onClose()}
      isDismissable
      className="fixed inset-x-0 top-0 z-50 flex h-(--visual-viewport-height) items-end justify-center bg-scrim-soft sm:items-center sm:p-4"
    >
      <Modal
        data-part="now-playing"
        className="h-full w-full overflow-auto bg-surface-raised pb-(--layout-safe-area-bottom) sm:h-auto sm:pb-0 sm:max-h-full sm:max-w-(--layout-now-playing-width) sm:rounded-lg sm:border sm:border-border sm:shadow-overlay"
      >
        <Dialog className="flex flex-col gap-6 p-4 outline-none sm:p-6">
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" aria-label={t`Close Now Playing`} onPress={onClose}>
              <Icon name="chevronDown" />
            </Button>
            <Heading slot="title" className="flex-1 text-sm font-semibold text-text-muted">
              <Trans>Now Playing</Trans>
            </Heading>
            <Button
              variant="ghost"
              size="sm"
              onPress={() => {
                onClose();
                listening.close();
              }}
            >
              <Trans>Stop listening</Trans>
            </Button>
          </div>
          <div className="grid gap-8 md:grid-cols-2">
            <div className="flex flex-col items-center gap-4">
              <div className="w-full max-w-72">
                <Artwork image={book.images.poster} title={book.title} shape="square" width={288} part="cover" />
              </div>
              <div className="flex w-full flex-col items-center gap-1 text-center">
                <h2 className="text-xl font-bold text-text">{book.title}</h2>
                {authors && (
                  <p className="text-sm text-text-muted">
                    <Trans>By {authors}</Trans>
                  </p>
                )}
                {narrators && (
                  <p className="text-sm text-text-muted">
                    <Trans>Narrated by {narrators}</Trans>
                  </p>
                )}
              </div>
              <div className="flex w-full flex-col gap-1">
                <p data-part="now-playing-chapter" className="truncate text-center text-sm font-semibold text-text">
                  {chapter?.title ?? book.title}
                </p>
                {chapter && (
                  <ChapterSeekBar chapter={chapter} positionMs={positionMs} disabled={!live} onSeek={listening.seek} />
                )}
              </div>
              <FormError tone="warning">{failure}</FormError>
              <div data-part="now-playing-controls" className="flex items-center justify-center gap-2">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t`Previous chapter`}
                  isDisabled={!live}
                  onPress={listening.previous}
                >
                  <Icon name="previous" />
                </Button>
                <SkipControls live={live} onSkip={listening.skip}>
                  <Button
                    variant="primary"
                    aria-label={state.paused ? t`Play` : t`Pause`}
                    isDisabled={!live}
                    onPress={listening.togglePlay}
                    className="size-14 rounded-full"
                  >
                    <Icon name={state.paused ? "play" : "pause"} className="size-6" />
                  </Button>
                </SkipControls>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t`Next chapter`}
                  isDisabled={!live || index >= chapters.length - 1}
                  onPress={listening.next}
                >
                  <Icon name="next" />
                </Button>
              </div>
              <div className="flex w-full flex-col gap-1" data-part="book-progress">
                <ProgressBar value={durationMs > 0 ? positionMs / durationMs : 0} />
                <p className="flex justify-between text-xs text-text-muted tabular-nums">
                  <span>
                    {chapters.length > 1 ? (
                      <Trans>
                        Chapter {index + 1} of {chapters.length}
                      </Trans>
                    ) : null}
                  </span>
                  <span>
                    <Trans>{formatRuntime(Math.max(0, durationMs - positionMs))} left in the book</Trans>
                  </span>
                </p>
              </div>
              <Select
                variant="inline"
                part="speed"
                label={t`Speed`}
                options={speeds.map((s) => ({ id: String(s), label: speedLabel(s) }))}
                selected={String(speed)}
                onChange={(id) => listening.setSpeed(Number(id))}
              />
            </div>
            {chapters.length > 1 && (
              <section data-part="chapter-list" className="flex min-w-0 flex-col gap-2">
                <h3 className="text-lg font-semibold text-text">
                  <Trans>Chapters</Trans>
                </h3>
                <ol className="flex flex-col md:max-h-(--layout-chapter-list-height) md:overflow-auto">
                  {chapters.map((c, i) => (
                    <li key={c.startMs}>
                      <button
                        type="button"
                        aria-current={i === index ? "true" : undefined}
                        onClick={() => listening.seekAndPlay(c.startMs)}
                        className={`flex w-full cursor-pointer gap-3 rounded-md px-2 py-2 text-left text-sm hover:bg-surface-hover ${i === index ? "font-semibold text-accent" : "text-text"}`}
                      >
                        <span className="min-w-0 flex-1 truncate">{c.title ?? t`Chapter ${i + 1}`}</span>
                        <span className="shrink-0 text-text-muted tabular-nums">{formatClock(c.startMs)}</span>
                      </button>
                    </li>
                  ))}
                </ol>
              </section>
            )}
          </div>
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}

/**
 * The seek bar of the chapter playing. A whole book of many hours is too
 * long to scrub. The time played in the chapter and the time left in it sit
 * on either side. The seek happens on release.
 */
function ChapterSeekBar({
  chapter,
  positionMs,
  disabled,
  onSeek,
}: {
  chapter: Chapter;
  positionMs: number;
  disabled: boolean;
  onSeek: (ms: number) => void;
}) {
  const { t } = useLingui();
  const [dragMs, setDragMs] = useState<number | undefined>();
  const shownMs = Math.min(Math.max(dragMs ?? positionMs, chapter.startMs), chapter.endMs);
  const span = chapter.endMs - chapter.startMs;
  return (
    <div className="flex flex-col gap-1">
      <Slider
        data-part="chapter-seek-bar"
        aria-label={t`Seek in the chapter`}
        minValue={chapter.startMs}
        maxValue={chapter.endMs}
        step={stepMs}
        value={shownMs}
        isDisabled={disabled}
        onChange={setDragMs}
        onChangeEnd={(ms) => {
          setDragMs(undefined);
          onSeek(ms);
        }}
      >
        <SliderTrack className="group relative flex h-6 w-full cursor-pointer items-center">
          <div className="relative h-1 w-full rounded-full bg-progress-track">
            <div
              className="absolute inset-y-0 left-0 w-(--played) rounded-full bg-progress"
              style={{ "--played": percent(shownMs - chapter.startMs, span) } as CSSProperties}
            />
          </div>
          <SliderThumb className="top-1/2 size-4 rounded-full bg-accent shadow-card data-dragging:scale-125 data-focus-visible:outline-2 data-focus-visible:outline-focus" />
        </SliderTrack>
      </Slider>
      <p className="flex justify-between text-xs text-text-muted tabular-nums">
        <span>{formatClock(shownMs - chapter.startMs)}</span>
        <span>-{formatClock(chapter.endMs - shownMs)}</span>
      </p>
    </div>
  );
}
