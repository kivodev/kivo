import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Artwork } from "@/components/Artwork";
import { Icon } from "@/components/Icon";
import { ProgressBar } from "@/components/ProgressBar";
import { cardStatus } from "@/features/browse/WorkCard";
import { formatRuntime, formatTimeLeft } from "@/i18n/format";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";

/** An episode's number, such as "3", or "1–2" for a multi-episode file. */
function episodeRange(w: WorkSummary): string {
  const e = w.episode;
  if (!e) return "";
  return e.endNumber && e.endNumber !== e.number ? `${e.number}–${e.endNumber}` : `${e.number}`;
}

type Props = {
  seasonId: string;
  currentId?: string;
  /** The season's episode count, which is how many rows to hold while the list loads. */
  count?: number;
  /** On TV, the episode Up or Down enters the list on, such as Next Up, when it's in the list. */
  entry?: string;
};

/**
 * A season's episodes, with progress, played, Unmatched and Identifying
 * marks. The play button on each still starts that episode, resuming one
 * in progress. The rest of the row opens the episode's page. Until the
 * list arrives, a placeholder row of the same size holds each episode's
 * place. On TV a row takes focus and OK plays the episode.
 */
export function EpisodeList({ seasonId, currentId, count = 1, entry }: Props) {
  const episodes = useQuery({ ...workQueries.children(seasonId), placeholderData: (prev) => prev });

  if (episodes.isPending) return <EpisodesSkeleton count={count} />;
  return (
    <TvFocusGroup focusKey="episodes" enterOn={entry && `episode:${entry}`}>
      <ol data-part="episode-list" className="-mx-2 flex flex-col gap-1">
        {(episodes.data?.items ?? []).map((w) => (
          <EpisodeRow key={w.id} episode={w} current={w.id === currentId} />
        ))}
      </ol>
    </TvFocusGroup>
  );
}

/** A season's episodes while they load, with a placeholder row for each. */
export function EpisodesSkeleton({ count }: { count: number }) {
  return (
    <ol data-part="episode-list" aria-busy="true" className="-mx-2 flex flex-col gap-1">
      <li className="sr-only">
        <Trans>Loading episodes…</Trans>
      </li>
      {Array.from({ length: count }, (_, i) => i).map((i) => (
        <EpisodeRowSkeleton key={i} />
      ))}
    </ol>
  );
}

function EpisodeRowSkeleton() {
  return (
    <li aria-hidden="true" className="flex items-center gap-4 p-2">
      <div className="aspect-landscape w-28 shrink-0 animate-pulse sm:w-(--layout-still-width) rounded-md bg-placeholder" />
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="h-4 w-1/2 rounded-sm bg-placeholder" />
        <div className="h-3 w-16 rounded-sm bg-placeholder" />
      </div>
    </li>
  );
}

function EpisodeRow({ episode: w, current }: { episode: WorkSummary; current: boolean }) {
  const { t } = useLingui();
  const progress =
    w.userState?.positionMs !== undefined && w.durationMs ? w.userState.positionMs / w.durationMs : undefined;
  const status = cardStatus(w);
  const season = w.episode?.seasonNumber ?? 0;
  const number = episodeRange(w);
  const title = w.title;
  const playLabel =
    progress !== undefined ? t`Resume S${season} E${number}: ${title}` : t`Play S${season} E${number}: ${title}`;

  return (
    <li
      data-current={current || undefined}
      className="group relative flex items-center gap-3 rounded-md p-2 hover:bg-surface-hover data-current:bg-surface-raised sm:gap-4 tv:block tv:p-0"
    >
      <TvFocusable
        focusKey={`episode:${w.id}`}
        kind="row"
        className="group relative flex items-center gap-3 rounded-md p-2 sm:gap-4"
      >
        <div className="relative w-28 shrink-0 sm:w-(--layout-still-width)">
          <Artwork image={w.images.still} title={title} shape="landscape" width={160} part="still">
            {progress !== undefined && <ProgressBar value={progress} className="absolute inset-x-0 bottom-0" />}
          </Artwork>
          {w.available && (
            <Link
              to="/watch/$workId"
              params={{ workId: w.id }}
              aria-label={playLabel}
              data-part="episode-play"
              className="absolute inset-0 z-10 m-auto flex size-10 items-center justify-center rounded-full bg-accent text-on-accent opacity-0 shadow-card transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100"
            >
              <Icon name="play" />
            </Link>
          )}
        </div>
        {/* The link's box stretches over the whole row, under the play button. */}
        <Link
          to="/works/$workId"
          params={{ workId: w.id }}
          aria-current={current ? "page" : undefined}
          className="min-w-0 flex-1 rounded-sm after:absolute after:inset-0 after:rounded-md"
        >
          <div className="truncate font-semibold">
            <span className="text-text-muted">E{number}</span> {title}
          </div>
          <div className="text-sm text-text-muted">
            {w.durationMs && w.userState?.positionMs !== undefined
              ? formatTimeLeft(w.durationMs - w.userState.positionMs)
              : w.durationMs
                ? formatRuntime(w.durationMs)
                : ""}
          </div>
        </Link>
        {w.userState?.played ? (
          <Icon name="check" className="size-5 shrink-0 text-accent" />
        ) : status ? (
          <span data-status={status} className="shrink-0 text-xs font-semibold text-accent uppercase">
            {status === "unmatched" && <Trans>Unmatched</Trans>}
            {status === "identifying" && <Trans>Identifying</Trans>}
          </span>
        ) : null}
      </TvFocusable>
    </li>
  );
}
