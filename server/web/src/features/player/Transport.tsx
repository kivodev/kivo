import { useLingui } from "@lingui/react/macro";
import type { RefObject } from "react";
import { Button as AriaButton } from "react-aria-components";
import { Icon } from "@/components/Icon";
import { playerButton } from "@/components/PlayerMenu";
import { backMs, forwardMs } from "./skips";

type Props = {
  /** Whether this is an episode, which has Previous and Next episode on either side. */
  episode: boolean;
  hasPrevious: boolean;
  hasNext: boolean;
  /** Whether the media is playing or paused, so the skips and play work. */
  live: boolean;
  paused: boolean;
  playRef: RefObject<HTMLButtonElement | null>;
  onPrevious: () => void;
  onSkip: (deltaMs: number) => void;
  onTogglePlay: () => void;
  onNext: () => void;
};

/**
 * Transport is the player's Back 10 seconds, play or pause, and Forward 30
 * seconds. An episode also has Previous and Next around them.
 */
export function Transport({
  episode,
  hasPrevious,
  hasNext,
  live,
  paused,
  playRef,
  onPrevious,
  onSkip,
  onTogglePlay,
  onNext,
}: Props) {
  const { t } = useLingui();
  return (
    <div className="flex shrink-0 items-center justify-center gap-1 max-sm:order-first max-sm:w-full sm:gap-2">
      {episode && (
        <AriaButton
          aria-label={t`Previous episode`}
          className={playerButton.icon}
          data-tv-face=""
          isDisabled={!hasPrevious}
          onPress={onPrevious}
        >
          <Icon name="previous" />
        </AriaButton>
      )}
      <AriaButton
        aria-label={t`Back 10 seconds`}
        className={playerButton.icon}
        data-tv-face=""
        isDisabled={!live}
        onPress={() => onSkip(-backMs)}
      >
        <Icon name="replay" />
      </AriaButton>
      <AriaButton
        ref={playRef}
        aria-label={paused ? t`Play` : t`Pause`}
        className={playerButton.primary}
        data-tv-face=""
        isDisabled={!live}
        onPress={onTogglePlay}
      >
        <Icon name={paused ? "play" : "pause"} className="size-7" />
      </AriaButton>
      <AriaButton
        aria-label={t`Forward 30 seconds`}
        className={playerButton.icon}
        data-tv-face=""
        isDisabled={!live}
        onPress={() => onSkip(forwardMs)}
      >
        <Icon name="forward" />
      </AriaButton>
      {episode && (
        <AriaButton
          aria-label={t`Next episode`}
          className={playerButton.icon}
          data-tv-face=""
          isDisabled={!hasNext}
          onPress={onNext}
        >
          <Icon name="next" />
        </AriaButton>
      )}
    </div>
  );
}
