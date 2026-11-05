import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { Link, useNavigate } from "@tanstack/react-router";
import { type MouseEvent, useRef, useState } from "react";
import { mergeProps, useLongPress } from "react-aria";
import type { UserStateUpdate, WorkSummary } from "@/api/schemas";
import { useRemoveFromContinue, useUpdateUserState } from "@/api/works";
import { Artwork, type ArtworkShape, cardDensity } from "@/components/Artwork";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Icon } from "@/components/Icon";
import { AnchoredMenu, MenuItem, MenuSeparator } from "@/components/Menu";
import { ProgressBar } from "@/components/ProgressBar";
import { formatEpisodeCode, formatTimeLeft } from "@/i18n/format";
import { isTv } from "@/tv/flag";

// TV cards have no touch menu. The build-time flag keeps their hook calls fixed.
const useCardLongPress = isTv ? () => ({ longPressProps: {} }) : useLongPress;

/** The kinds a Play button can start. The rest open their page. */
const playableKinds = new Set(["movie", "episode", "video", "extra"]);

type Status = "identifying" | "unmatched";

/** The card's single status badge, if any. Identifying takes precedence over unmatched. */
export function cardStatus(work: WorkSummary): Status | undefined {
  if (work.matchState === "identifying") return "identifying";
  if (work.matchState === "unmatched") return "unmatched";
  return undefined;
}

/**
 * The status badge. It sits bottom-left, or bottom-right on a landscape
 * card or on a card with an episode code, because the code sits
 * bottom-left.
 */
export function StatusBadge({ status, right }: { status: Status; right: boolean }) {
  return (
    <span
      data-part="badge"
      data-status={status}
      className={`absolute bottom-2 rounded-sm bg-badge px-1.5 py-0.5 text-xs font-semibold tracking-wide text-on-badge uppercase ${right ? "right-2" : "left-2"}`}
    >
      {status === "identifying" && <Trans>Identifying</Trans>}
      {status === "unmatched" && <Trans>Unmatched</Trans>}
    </span>
  );
}

/** The top-right corner, with a played check or a series' unplayed episode count. */
export function Corner({ work }: { work: WorkSummary }) {
  const unplayed = work.series?.unplayedCount ?? work.season?.unplayedCount;
  if (unplayed !== undefined && unplayed > 0) {
    return (
      <span className="absolute top-2 right-2 min-w-6 rounded-full bg-accent px-1.5 text-center text-xs leading-6 font-bold text-on-accent">
        {unplayed}
      </span>
    );
  }
  if (!work.userState?.played) return null;
  return (
    <span className="absolute top-2 right-2 flex size-6 items-center justify-center rounded-full bg-accent text-on-accent">
      <Icon name="check" className="size-4" />
    </span>
  );
}

/** A work's title with its edition, "Blade Runner · Final Cut", for a movie whose cut is named. */
export function labelledTitle(work: Pick<WorkSummary, "title" | "movie">): string {
  const edition = work.movie?.edition;
  return edition ? `${work.title} · ${edition}` : work.title;
}

/** An episode's code, "S3 · E4", or "S1 · E1 to E2" for a multi-episode file. */
export function episodeCode(work: WorkSummary): string | undefined {
  return work.episode && formatEpisodeCode(work.episode);
}

function Subtitle({ work }: { work: WorkSummary }) {
  const left =
    work.userState?.positionMs !== undefined && work.durationMs
      ? formatTimeLeft(work.durationMs - work.userState.positionMs)
      : undefined;
  // A Recently Added bundle, with the episodes the card stands for.
  const added = work.series?.recentlyAddedCount;
  switch (work.kind) {
    case "series": {
      if (added) return <Plural value={added} one="# episode" other="# episodes" />;
      const last = work.series?.lastAirYear;
      return <>{work.year && last && last !== work.year ? `${work.year}–${last}` : work.year}</>;
    }
    case "season":
      if (added) {
        return (
          <>
            {work.title}
            {" · "}
            <Plural value={added} one="# episode" other="# episodes" />
          </>
        );
      }
      return work.season?.episodeCount !== undefined ? (
        <Plural value={work.season.episodeCount} one="# episode" other="# episodes" />
      ) : null;
    case "book": {
      const author = work.book?.authors[0]?.name;
      const position = work.userState?.positionMs;
      if (position === undefined || !work.durationMs) return <>{author}</>;
      const left = formatTimeLeft(work.durationMs - position);
      return <>{author ? `${author} · ${left}` : left}</>;
    }
    case "episode":
      return (
        <>
          {work.title}
          {left !== undefined && (
            <>
              {" · "}
              {left}
            </>
          )}
        </>
      );
    case "movie":
      // For a work in progress, as in Continue Watching, the time left says
      // where you are.
      return <>{left ?? work.year}</>;
    default:
      return <>{work.year}</>;
  }
}

type Props = {
  work: WorkSummary;
  shape: ArtworkShape;
  /** The displayed width in CSS pixels, to pick the artwork size. */
  width?: number;
  /** Whether the card is in its library's own grid, which has no Go to library. */
  inLibrary?: boolean;
  /** The Home row the card is in, when its menu can remove it from that row. */
  continueRow?: "continueWatching" | "continueListening";
};

const aspects: Record<ArtworkShape, string> = {
  poster: "aspect-poster",
  landscape: "aspect-landscape",
  square: "aspect-square",
};

/**
 * The card for a work, the same everywhere it appears. It is a poster for
 * movies, series, and episodes in Favorites. It is landscape for episodes,
 * videos and the Continue Watching row, and square for books. An episode's
 * card uses its series, with the episode's code on the artwork.
 *
 * The card's menu plays it, marks it played or favorite for you, removes it
 * from Continue Watching or Continue Listening in those rows, and goes to
 * its series and to the first library it's in. Pointing at the card
 * dims the artwork under a Play button, with the ⋯ menu at the bottom
 * right. A right-click or a long press opens the same menu. A touch screen
 * shows the ⋯ alone. On TV the card is only a link, which its row or grid
 * focuses: nothing hovers, and a remote's OK opens it.
 */
export function WorkCard({ work, shape, width, inLibrary = false, continueRow }: Props) {
  const { t } = useLingui();
  const navigate = useNavigate();
  const update = useUpdateUserState();
  const remove = useRemoveFromContinue();
  const setState = (body: UserStateUpdate) => update.mutate({ params: { path: { id: work.id } }, body });
  const libraryId = inLibrary ? undefined : work.libraryId;
  const [confirming, setConfirming] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const anchor = useRef<HTMLButtonElement>(null);
  const status = cardStatus(work);
  const played = work.userState?.played ?? false;
  const favorite = work.userState?.favorite ?? false;
  const landscape = shape === "landscape";
  const image = landscape ? (work.images.still ?? work.images.backdrop) : work.images.poster;
  const cascade = work.kind === "series" || work.kind === "season";
  // An episode, or a season bundled in Recently Added, uses its series.
  const title =
    work.kind === "episode"
      ? (work.episode?.seriesTitle ?? work.title)
      : work.kind === "season" && work.series?.recentlyAddedCount
        ? (work.season?.seriesTitle ?? work.title)
        : labelledTitle(work);
  const code = episodeCode(work);
  const position = work.userState?.positionMs;
  const progress = position !== undefined && work.durationMs ? position / work.durationMs : undefined;
  const playable = playableKinds.has(work.kind) && work.available !== false;
  const unplayedEpisodes = work.series?.unplayedCount ?? work.season?.unplayedCount ?? 0;

  // A touch held on the card opens its menu, as a right-click does, and
  // doesn't open the card when the finger lifts.
  const { longPressProps } = useCardLongPress({
    pointerType: "touch",
    onLongPress: () => setMenuOpen(true),
  });

  const markPlayed = () => {
    if (!played && cascade && unplayedEpisodes > 1) setConfirming(true);
    else setState({ played: !played });
  };
  const play = (fromStart: boolean) =>
    void navigate({
      to: "/watch/$workId",
      params: { workId: work.id },
      search: { from: fromStart ? "start" : undefined },
    });

  return (
    <div
      data-part="card"
      data-kind={work.kind}
      data-menu-open={menuOpen || undefined}
      className="group relative flex min-w-0 touch-callout-none flex-col gap-2 pointer-coarse:select-none"
      {...mergeProps(longPressProps, {
        onContextMenu: (e: MouseEvent) => {
          e.preventDefault();
          setMenuOpen(true);
        },
      })}
    >
      {/* The page shows the card's title and backdrop while it loads. */}
      <Link
        to="/works/$workId"
        params={{ workId: work.id }}
        state={{ work }}
        className="flex flex-col gap-2 rounded-md"
      >
        {/* On TV the focus ring hugs the artwork, with the caption below it. */}
        <div data-tv-ring className="relative">
          <Artwork
            image={image}
            title={title}
            shape={shape}
            width={width ?? (isTv ? (landscape ? 520 : 260) : landscape ? 288 : 160)}
            density={cardDensity}
            part={{ poster: "poster", landscape: "still", square: "cover" }[shape]}
            className="relative shadow-card"
          >
            {progress !== undefined && <ProgressBar value={progress} className="absolute inset-x-0 bottom-0" />}
          </Artwork>
          <Corner work={work} />
          {code &&
            (landscape ? (
              <span className="absolute bottom-3 left-3 text-sm font-semibold text-on-badge">{code}</span>
            ) : (
              // A poster has no dark frame to sit on, so the code is a badge.
              <span className="absolute bottom-2 left-2 rounded-sm bg-badge px-1.5 py-0.5 text-xs font-semibold text-on-badge">
                {code}
              </span>
            ))}
          {status && <StatusBadge status={status} right={landscape || code !== undefined} />}
        </div>
        {/* On TV the caption lines up with the artwork's edge. */}
        <div className="min-w-0 px-0.5 tv:px-0">
          <div className="truncate text-sm font-semibold text-text">{title}</div>
          <div className="truncate text-sm text-text-muted">
            <Subtitle work={work} />
          </div>
        </div>
      </Link>
      {/*
        Over the artwork: a dimmed hover with Play in the middle and ⋯ at the
        bottom right. A touch screen, where nothing hovers, shows the ⋯ alone.
      */}
      {!isTv && (
        <div className={`pointer-events-none absolute inset-x-0 top-0 ${aspects[shape]}`}>
          <span
            aria-hidden="true"
            className="absolute inset-0 rounded-md bg-badge opacity-0 transition-opacity group-hover:opacity-50 group-data-menu-open:opacity-50 pointer-coarse:hidden"
          />
          {playable && (
            <Link
              to="/watch/$workId"
              params={{ workId: work.id }}
              aria-label={position ? t`Resume ${work.title}` : t`Play ${work.title}`}
              data-part="card-play"
              className="pointer-events-auto absolute top-1/2 left-1/2 flex size-12 -translate-1/2 items-center justify-center rounded-full bg-accent text-on-accent opacity-0 shadow-card transition-opacity group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:hidden"
            >
              <Icon name="play" />
            </Link>
          )}
          <button
            ref={anchor}
            type="button"
            aria-label={t`More actions for ${work.title}`}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen(true)}
            className="pointer-events-auto absolute right-2 bottom-2 flex size-8 cursor-pointer items-center justify-center rounded-full bg-badge text-on-badge opacity-0 transition-opacity group-hover:opacity-100 group-data-menu-open:opacity-100 focus-visible:opacity-100 touch-target pointer-coarse:size-7 pointer-coarse:opacity-100"
          >
            <Icon name="more" className="size-5" />
          </button>
        </div>
      )}
      {menuOpen && (
        <AnchoredMenu
          label={t`Actions for ${work.title}`}
          triggerRef={anchor}
          isOpen={menuOpen}
          onOpenChange={setMenuOpen}
        >
          {playable && <MenuItem onAction={() => play(false)}>{position ? t`Resume` : t`Play`}</MenuItem>}
          {playable && position ? <MenuItem onAction={() => play(true)}>{t`Play from beginning`}</MenuItem> : null}
          {playable && <MenuSeparator />}
          <MenuItem onAction={markPlayed}>{played ? t`Mark as unwatched` : t`Mark as watched`}</MenuItem>
          <MenuItem onAction={() => setState({ favorite: !favorite })}>
            {favorite ? t`Remove from favorites` : t`Add to favorites`}
          </MenuItem>
          {continueRow && (
            <MenuItem onAction={() => remove.mutate({ params: { path: { id: work.id } } })}>
              {continueRow === "continueListening"
                ? t`Remove from Continue Listening`
                : t`Remove from Continue Watching`}
            </MenuItem>
          )}
          {work.episode && (
            <MenuItem
              href={{
                to: "/works/$workId",
                params: { workId: work.episode.seriesId },
                search: { season: work.episode.seasonId },
              }}
            >
              {t`Go to series`}
            </MenuItem>
          )}
          {libraryId && (
            <MenuItem href={{ to: "/libraries/$libraryId", params: { libraryId } }}>{t`Go to library`}</MenuItem>
          )}
        </AnchoredMenu>
      )}
      {!isTv && (
        <ConfirmDialog
          isOpen={confirming}
          title={t`Mark ${unplayedEpisodes} episodes as watched?`}
          confirmLabel={t`Mark as watched`}
          onConfirm={() => {
            setConfirming(false);
            setState({ played: true });
          }}
          onCancel={() => setConfirming(false)}
        />
      )}
    </div>
  );
}
