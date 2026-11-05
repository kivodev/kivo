import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { Button as AriaButton } from "react-aria-components";
import type { WorkDetail, WorkSummary } from "@/api/schemas";
import { useUpdateUserState, workQueries } from "@/api/works";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Icon } from "@/components/Icon";
import { LoadError } from "@/components/LoadError";
import { Menu, MenuItem } from "@/components/Menu";
import { episodeCode } from "@/features/browse/WorkCard";
import { formatTimeLeft } from "@/i18n/format";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { communityRating, DetailHeader } from "./DetailHeader";
import { useDetailSearch } from "./detailSearch";
import { EpisodeList, EpisodeShelf, EpisodeShelfSkeleton, EpisodesSkeleton } from "./EpisodeList";
import { PlayButton } from "./PlaybackControls";
import { WorkRows } from "./WorkRows";

type Props = { work: WorkDetail };

function seasonLabel(season: WorkSummary, specials: string): string {
  const n = season.season?.number;
  return n === 0 ? specials : n !== undefined ? `S${n}` : season.title;
}

/**
 * A series' page, with Next Up, the season strip, the open season's
 * heading with its watched check and episode count, and its episodes. A
 * single-season series lists its episodes without the strip. On TV, as in
 * the Apple app's wide layout, the episodes are a row of cards that opens
 * on Next Up, under the strip and the heading.
 */
export function SeriesDetail({ work }: Props) {
  const { t } = useLingui();
  const update = useUpdateUserState();
  const setPlayed = (id: string, played: boolean) => update.mutate({ params: { path: { id } }, body: { played } });
  // The header shows at once, from the work. The seasons show as they
  // arrive.
  const read = useQuery(workQueries.children(work.id));
  const seasons = read.data?.items ?? [];
  const [search, setSearch] = useDetailSearch();
  const navigate = useNavigate();
  const nextUp = work.series?.nextUp;
  const nextPosition = nextUp?.userState?.positionMs;
  const open = openSeason(seasons, search.season, nextUp?.episode?.seasonId);
  const [confirmSeason, setConfirmSeason] = useState(false);
  const years = work.series?.lastAirYear && work.year !== work.series.lastAirYear ? work.series.lastAirYear : undefined;
  const openSeasonId = (id: string) => setSearch({ ...search, season: id });

  const strip = seasons.length > 1 && <SeasonStrip seasons={seasons} open={open} onOpen={openSeasonId} />;
  const heading = open && (
    <div className="flex items-center gap-2">
      <h2 className={`text-lg font-semibold ${seasons.length > 1 ? "max-sm:hidden" : ""}`}>
        {seasons.length > 1 ? open.title : <Trans>Episodes</Trans>}
      </h2>
      {seasons.length > 1 && (
        // On a phone the seasons are a menu, as in the iPhone app.
        <div className="-ml-4 sm:hidden">
          <Menu onAction={(id) => openSeasonId(String(id))}>
            <Button aria-label={t`Season`} variant="ghost">
              <span data-part="season-menu" className="flex items-center gap-1 text-lg">
                {open.title}
                <Icon name="chevronDown" className="size-5 text-text-muted" />
              </span>
            </Button>
            {seasons.map((s) => (
              <MenuItem key={s.id} id={s.id} textValue={s.title}>
                <span className="flex-1">{s.title}</span>
                {s.userState?.played ? (
                  <span className="text-text-muted">
                    <Trans>Watched</Trans>
                  </span>
                ) : (s.season?.unplayedCount ?? 0) > 0 ? (
                  <span className="text-text-muted">{s.season?.unplayedCount}</span>
                ) : null}
                {s.id === open.id && <Icon name="check" className="size-4 text-accent" />}
              </MenuItem>
            ))}
          </Menu>
        </div>
      )}
      {seasons.length > 1 && (
        <TvFocusable focusKey="season-played">
          <Button
            size="icon"
            variant="ghost"
            aria-label={open.userState?.played ? t`Mark season as unwatched` : t`Mark season as watched`}
            aria-pressed={open.userState?.played === true}
            onPress={() =>
              open.userState?.played
                ? setPlayed(open.id, false)
                : (open.season?.unplayedCount ?? 0) > 1
                  ? setConfirmSeason(true)
                  : setPlayed(open.id, true)
            }
            className="max-sm:-ml-4"
          >
            <Icon name={open.userState?.played ? "checkCircleFill" : "checkCircle"} />
          </Button>
        </TvFocusable>
      )}
      {open.season?.episodeCount !== undefined && (
        <span data-part="episode-count" className="ml-auto text-sm text-text-muted">
          <Plural value={open.season.episodeCount} one="# episode" other="# episodes" />
        </span>
      )}
    </div>
  );

  return (
    <div data-part="detail" data-kind="series">
      <DetailHeader
        work={work}
        cascadeCount={work.series?.unplayedCount ?? 0}
        meta={
          <p className="text-sm text-text-muted">
            {work.year && years ? (
              <Trans>
                {work.year} to {years}
              </Trans>
            ) : (
              work.year
            )}
            {work.year ? " · " : ""}
            <Plural value={work.series?.seasonCount ?? seasons.length} one="# season" other="# seasons" />
            {work.communityRating !== undefined ? ` · ${communityRating(work)}` : ""}
            {work.contentRating ? ` · ${work.contentRating}` : ""}
          </p>
        }
        primary={
          nextUp && (
            <PlayButton
              workId={nextUp.id}
              positionMs={nextPosition}
              label={
                <Trans>
                  Play S{nextUp.episode?.seasonNumber} E{nextUp.episode?.number}
                </Trans>
              }
            />
          )
        }
        playing={
          nextUp &&
          [
            episodeCode(nextUp),
            nextUp.title,
            nextPosition && nextUp.durationMs ? formatTimeLeft(nextUp.durationMs - nextPosition) : undefined,
          ]
            .filter(Boolean)
            .join(" · ")
        }
        onPlayFromStart={
          nextUp && nextPosition
            ? () => void navigate({ to: "/watch/$workId", params: { workId: nextUp.id }, search: { from: "start" } })
            : undefined
        }
      />
      <section className="flex flex-col gap-4 px-4 pb-10 sm:px-8 tv:relative">
        {!read.data &&
          (read.isError ? (
            <LoadError error={read.error} onRetry={() => void read.refetch()} />
          ) : isTv ? (
            <EpisodeShelfSkeleton />
          ) : (
            <EpisodesSkeleton count={3} />
          ))}
        {open &&
          (isTv ? (
            <EpisodeShelf
              seasonId={open.id}
              entry={nextUp?.id}
              header={
                <>
                  {strip}
                  {heading}
                </>
              }
            />
          ) : (
            <>
              {strip}
              {heading}
              <EpisodeList seasonId={open.id} count={open.season?.episodeCount} />
            </>
          ))}
        {open && (
          <ConfirmDialog
            isOpen={confirmSeason}
            title={t`Mark ${open.season?.unplayedCount ?? 0} episodes as watched?`}
            confirmLabel={t`Mark as watched`}
            onConfirm={() => {
              setConfirmSeason(false);
              setPlayed(open.id, true);
            }}
            onCancel={() => setConfirmSeason(false)}
          />
        )}
      </section>
      <WorkRows work={work} />
    </div>
  );
}

/** The seasons as pills, the open one outlined in the accent, each with its unwatched count or a check once watched. */
function SeasonStrip({
  seasons,
  open,
  onOpen,
}: {
  seasons: readonly WorkSummary[];
  open: WorkSummary | undefined;
  onOpen: (id: string) => void;
}) {
  const { t } = useLingui();
  return (
    <TvFocusGroup focusKey="seasons" enterOn={open && `season:${open.id}`}>
      <nav data-part="season-strip" aria-label={t`Seasons`} className="flex flex-wrap gap-2 max-sm:hidden">
        {seasons.map((s) => {
          const unplayed = s.season?.unplayedCount ?? 0;
          const active = s.id === open?.id;
          return (
            <TvFocusable key={s.id} focusKey={`season:${s.id}`}>
              <AriaButton
                aria-current={active}
                data-current={active || undefined}
                onPress={() => onOpen(s.id)}
                className="inline-flex h-9 cursor-pointer items-center gap-2 rounded-full border border-border px-4 touch-target text-sm font-semibold text-text hover:bg-surface-hover data-current:border-accent data-current:bg-surface-raised"
              >
                {seasonLabel(s, t`Specials`)}
                {s.userState?.played ? (
                  <Icon name="check" className="size-4 text-accent" />
                ) : unplayed > 0 ? (
                  <span className="text-xs text-text-muted">{unplayed}</span>
                ) : null}
              </AriaButton>
            </TvFocusable>
          );
        })}
      </nav>
    </TvFocusGroup>
  );
}

/**
 * The season a series' page opens on. A season asked for opens when it's
 * among them; with none asked, Next Up's season does. Otherwise the first
 * season that isn't Specials does.
 */
function openSeason<S extends { id: string; season?: { number?: number } }>(
  seasons: readonly S[],
  asked: string | undefined,
  nextUp: string | undefined,
): S | undefined {
  const wanted = asked ?? nextUp;
  return seasons.find((s) => s.id === wanted) ?? seasons.find((s) => s.season?.number !== 0) ?? seasons[0];
}
