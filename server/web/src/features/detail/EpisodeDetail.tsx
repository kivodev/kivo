import { useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import type { WorkDetail } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { buttonClass } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { episodeCode } from "@/features/browse/WorkCard";
import { formatDate, formatRuntime } from "@/i18n/format";
import { TvFocusable } from "@/tv/focus";
import { communityRating, DetailHeader } from "./DetailHeader";
import { EpisodeList } from "./EpisodeList";
import { PlayActions, PlaybackPickers, useResumeParts } from "./PlaybackControls";
import { WorkRows } from "./WorkRows";

type Props = { work: WorkDetail };

/**
 * An episode's page. It shows the still, the series link, "S2 · E5 ·
 * Title", previous and next, the movie page's controls, then the rest of
 * its season.
 */
export function EpisodeDetail({ work }: Props) {
  const { t } = useLingui();
  const episode = work.episode;
  const resume = useResumeParts(work);
  const still = work.images.still;
  // The series backdrop stands in for a missing still.
  const series = useQuery({ ...workQueries.detail(episode?.seriesId ?? ""), enabled: !!episode && !still });
  // While loading, the season's card says how many rows its episode list
  // holds.
  const seasons = useQuery({ ...workQueries.children(episode?.seriesId ?? ""), enabled: !!episode });
  const episodeCount = seasons.data?.items.find((s) => s.id === episode?.seasonId)?.season?.episodeCount;
  const code = episodeCode(work);
  const seasonNumber = episode?.seasonNumber ?? 0;
  const meta = [
    episode?.airDate ? formatDate(episode.airDate) : undefined,
    work.durationMs ? formatRuntime(work.durationMs) : undefined,
    communityRating(work),
    work.contentRating,
  ].filter(Boolean);

  return (
    <div data-part="detail" data-kind="episode">
      <DetailHeader
        work={work}
        backdrop={still ?? series.data?.images.backdrop ?? work.images.backdrop}
        eyebrow={
          episode && (
            <div className="flex flex-wrap items-center gap-2">
              <TvFocusable focusKey="series">
                <Link
                  to="/works/$workId"
                  params={{ workId: episode.seriesId }}
                  search={{ season: episode.seasonId }}
                  className="font-semibold text-text hover:underline"
                >
                  {episode.seriesTitle}
                </Link>
              </TvFocusable>
              <span className="flex">
                <AdjacentLink id={episode.previousId} label={t`Previous episode`} icon="chevronLeft" />
                <AdjacentLink id={episode.nextId} label={t`Next episode`} icon="chevronRight" />
              </span>
            </div>
          )
        }
        heading={
          code ? (
            <>
              <span className="text-text-muted">{code} ·</span> {work.title}
            </>
          ) : undefined
        }
        meta={<p className="text-sm text-text-muted">{meta.join(" · ")}</p>}
        primary={<PlayActions work={work} />}
        playing={resume.playing}
        onPlayFromStart={resume.onPlayFromStart}
        holdOverview
      >
        <PlaybackPickers work={work} />
      </DetailHeader>
      {episode?.seasonId && (
        <section className="flex flex-col gap-4 px-4 pb-10 sm:px-8 tv:relative">
          <h2 className="text-lg font-semibold">{seasonNumber === 0 ? t`Specials` : t`Season ${seasonNumber}`}</h2>
          <EpisodeList seasonId={episode.seasonId} currentId={work.id} count={episodeCount} />
        </section>
      )}
      <WorkRows work={work} />
    </div>
  );
}

function AdjacentLink({
  id,
  label,
  icon,
}: {
  id: string | undefined;
  label: string;
  icon: "chevronLeft" | "chevronRight";
}) {
  if (!id) {
    return (
      <span aria-hidden="true" className={`${buttonClass("ghost", "icon")} opacity-50`}>
        <Icon name={icon} />
      </span>
    );
  }
  return (
    <TvFocusable focusKey={icon === "chevronLeft" ? "previous" : "next"}>
      <Link
        to="/works/$workId"
        params={{ workId: id }}
        aria-label={label}
        title={label}
        className={buttonClass("ghost", "icon")}
      >
        <Icon name={icon} />
      </Link>
    </TvFocusable>
  );
}
