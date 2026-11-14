import { useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { type CSSProperties, type RefObject, useState } from "react";
import { Slider, SliderThumb, SliderTrack } from "react-aria-components";
import { mediaUrl, playbackQueries } from "@/api/playback";
import type { PlayStarted } from "@/api/schemas";
import { formatClock } from "@/i18n/format";
import { chapterAt } from "./markers";
import { spriteFrame } from "./previews";

/** A keyboard's arrows on the bar step this far. */
const stepMs = 10_000;

/** Where `ms` falls along `span`, as a CSS percentage. */
export const percent = (ms: number, span: number) => `${span > 0 ? (ms / span) * 100 : 0}%`;

/** What the bar needs from a play. A cast has no previews, because they stay on the TV. */
type ScrubSource = Pick<PlayStarted, "durationMs" | "chapters"> & Partial<Pick<PlayStarted, "id" | "previews">>;

type Props = {
  play: ScrubSource;
  positionMs: number;
  /** Where a remote's Left and Right have moved the bar, before the seek. */
  target?: number;
  /** The slider's thumb, which takes focus. */
  thumbRef?: RefObject<HTMLInputElement | null>;
  /** "Ends at 9:42 PM", under the time left. */
  endsAt?: string;
  onSeek: (positionMs: number) => void;
};

/**
 * ScrubBar shows chapter ticks. While dragging, or while a remote has a
 * target, it shows the target time, chapter name and preview frame above
 * the bar. A drag's seek happens on release.
 */
export function ScrubBar({ play, positionMs, target, thumbRef, endsAt, onSeek }: Props) {
  const { t } = useLingui();
  const [dragMs, setDragMs] = useState<number | undefined>();
  const durationMs = play.durationMs;
  const targetMs = dragMs ?? target;
  const shownMs = targetMs ?? positionMs;
  const ticks = play.chapters.filter((c) => c.startMs > 0 && c.startMs < durationMs);

  // The time played and the time left sit on either side, and when it ends under the time left.
  return (
    <div>
      <div className="flex items-center gap-3 sm:gap-4">
        <span className="shrink-0 text-sm text-on-player tabular-nums">{formatClock(shownMs)}</span>
        <Slider
          data-part="scrub-bar"
          aria-label={t`Seek`}
          minValue={0}
          maxValue={durationMs}
          step={stepMs}
          value={shownMs}
          onChange={setDragMs}
          onChangeEnd={(ms) => {
            setDragMs(undefined);
            onSeek(ms);
          }}
          className="min-w-0 flex-1"
        >
          <SliderTrack className="group relative flex h-6 w-full cursor-pointer items-center">
            <div className="relative h-1 w-full rounded-full bg-player-track group-hover:h-1.5">
              <div
                className="absolute inset-y-0 left-0 w-(--played) rounded-full bg-progress"
                style={{ "--played": percent(shownMs, durationMs) } as CSSProperties}
              />
              {ticks.map((c) => (
                <span
                  key={c.startMs}
                  data-part="chapter-tick"
                  className="absolute inset-y-0 left-(--at) w-0.5 bg-on-player-muted"
                  style={{ "--at": percent(c.startMs, durationMs) } as CSSProperties}
                />
              ))}
            </div>
            {targetMs !== undefined && <ScrubPreview play={play} positionMs={targetMs} />}
            <SliderThumb
              inputRef={thumbRef}
              // On TV a focused thumb grows in a halo, as a lit control does, rather than taking a ring.
              // It grows from its top left, which its transform centres, so it stays on the bar.
              className="top-1/2 size-4 rounded-full bg-on-player shadow-overlay data-dragging:scale-125 data-focus-visible:outline-2 data-focus-visible:outline-focus tv:origin-top-left tv:transition-transform tv:data-focused:scale-150 tv:data-focused:ring-4 tv:data-focused:ring-on-player-muted tv:data-focus-visible:outline-none"
            />
          </SliderTrack>
        </Slider>
        <span className="shrink-0 text-sm text-on-player tabular-nums">
          −{formatClock(Math.max(0, durationMs - shownMs))}
        </span>
      </div>
      {endsAt && (
        <p data-part="ends-at" className="text-right text-sm text-on-player-muted tabular-nums">
          {endsAt}
        </p>
      )}
    </div>
  );
}

/** The target time and chapter name, and a preview frame once the play's previews are ready. */
function ScrubPreview({ play, positionMs }: { play: ScrubSource; positionMs: number }) {
  const manifestUrl = play.id && play.previews?.ready ? play.previews.url : undefined;
  const manifest = useQuery({
    ...playbackQueries.previews(play.id ?? "", manifestUrl ?? ""),
    enabled: manifestUrl !== undefined,
  });
  const frame =
    manifest.data && manifestUrl ? spriteFrame(manifest.data, mediaUrl(manifestUrl), positionMs) : undefined;
  const chapter = chapterAt(play.chapters, positionMs);
  return (
    <div
      aria-hidden="true"
      data-part="scrub-preview"
      className="-translate-x-1/2 absolute bottom-full left-(--at) mb-3 flex flex-col items-center gap-1"
      style={{ "--at": percent(positionMs, play.durationMs) } as CSSProperties}
    >
      {frame && (
        <div
          data-part="scrub-frame"
          className="h-(--frame-h) w-(--frame-w) rounded-md bg-(image:--sheet) bg-position-(--frame-at) bg-no-repeat shadow-overlay"
          style={
            {
              "--sheet": `url("${frame.sheet}")`,
              "--frame-at": `-${frame.x}px -${frame.y}px`,
              "--frame-w": `${frame.width}px`,
              "--frame-h": `${frame.height}px`,
            } as CSSProperties
          }
        />
      )}
      <span className="rounded-sm bg-player-panel px-2 py-1 text-sm font-semibold text-on-player tabular-nums">
        {formatClock(positionMs)}
      </span>
      {chapter?.title && <span className="text-xs text-on-player-muted">{chapter.title}</span>}
    </div>
  );
}
