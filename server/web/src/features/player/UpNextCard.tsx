import { useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Button as AriaButton } from "react-aria-components";
import { workQueries } from "@/api/works";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { playerButton } from "@/components/PlayerMenu";
import type { UpNextShown } from "./upNext";

type Props = {
  nextId: string;
  /** The card's phase and countdown. */
  upNext: UpNextShown;
  onPlay: () => void;
  /** Hides the card for the rest of the episode. This is Hide, or Watch credits at the credits. */
  onHide: () => void;
};

/**
 * UpNextCard shows the next episode at the bottom right. It counts down to
 * play, waits for Play now, or asks "Still watching?".
 */
export function UpNextCard({ nextId, upNext, onPlay, onHide }: Props) {
  const { t } = useLingui();
  const next = useQuery(workQueries.detail(nextId)).data;
  const seconds = upNext.phase === "countingDown" ? upNext.secondsLeft : undefined;

  const episode = next?.episode;
  const title = next ? (episode ? `S${episode.seasonNumber} · E${episode.number} “${next.title}”` : next.title) : "";
  return (
    <section
      data-part="up-next"
      aria-label={t`Up next`}
      className="flex w-(--layout-up-next-width) max-w-full flex-col gap-3 rounded-md bg-player-panel p-3 shadow-overlay"
    >
      {next && <Artwork image={next.images.still} title={next.title} shape="landscape" width={320} />}
      <div className="flex flex-col">
        <span className="text-xs font-semibold text-on-player-muted uppercase">
          {upNext.phase === "stillWatching" ? t`Still watching?` : t`Up next`}
        </span>
        <span className="truncate text-sm font-semibold text-on-player">{title}</span>
        {seconds !== undefined && <span className="text-xs text-on-player-muted">{t`Playing in ${seconds} s`}</span>}
      </div>
      <div className="flex gap-2">
        <Button variant="primary" size="sm" data-tv-face="" onPress={onPlay}>
          {t`Play now`}
        </Button>
        <AriaButton className={playerButton.text} data-tv-face="" onPress={onHide}>
          {upNext.atCredits ? t`Watch credits` : t`Hide`}
        </AriaButton>
      </div>
    </section>
  );
}
