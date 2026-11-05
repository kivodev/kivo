import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { type CSSProperties, type KeyboardEvent, type ReactNode, useRef } from "react";
import type { WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Artwork, cardDensity } from "@/components/Artwork";
import { Icon } from "@/components/Icon";
import { ProgressBar } from "@/components/ProgressBar";
import { Row } from "@/components/Row";
import { Corner, cardStatus, StatusBadge } from "@/features/browse/WorkCard";
import { keyName } from "@/features/player/keys";
import { formatRuntime, formatTimeLeft } from "@/i18n/format";

/** An episode's number, such as "3", or "1–2" for a multi-episode file. */
function episodeRange(w: WorkSummary): string {
  const e = w.episode;
  if (!e) return "";
  return e.endNumber && e.endNumber !== e.number ? `${e.number}–${e.endNumber}` : `${e.number}`;
}

/** The fraction of an episode played, while it's in progress. */
function progressOf(w: WorkSummary): number | undefined {
  return w.userState?.positionMs !== undefined && w.durationMs ? w.userState.positionMs / w.durationMs : undefined;
}

/** The time left in an episode in progress, else its runtime. */
function timeOf(w: WorkSummary): string | undefined {
  if (!w.durationMs) return undefined;
  return w.userState?.positionMs !== undefined
    ? formatTimeLeft(w.durationMs - w.userState.positionMs)
    : formatRuntime(w.durationMs);
}

type Props = {
  seasonId: string;
  currentId?: string;
  /** The season's episode count, which is how many rows to hold while the list loads. */
  count?: number;
};

/**
 * A season's episodes, with progress, played, Unmatched and Identifying
 * marks. The play button on each still starts that episode, resuming one
 * in progress. The rest of the row opens the episode's page. Until the
 * list arrives, a placeholder row of the same size holds each episode's
 * place. A TV shows them as an EpisodeShelf instead.
 */
export function EpisodeList({ seasonId, currentId, count = 1 }: Props) {
  const episodes = useQuery({ ...workQueries.children(seasonId), placeholderData: (prev) => prev });

  if (episodes.isPending) return <EpisodesSkeleton count={count} />;
  return (
    <ol data-part="episode-list" className="-mx-2 flex flex-col gap-1">
      {(episodes.data?.items ?? []).map((w) => (
        <EpisodeRow key={w.id} episode={w} current={w.id === currentId} />
      ))}
    </ol>
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
  const progress = progressOf(w);
  const status = cardStatus(w);
  const season = w.episode?.seasonNumber ?? 0;
  const number = episodeRange(w);
  const title = w.title;
  const playLabel =
    progress !== undefined ? t`Resume S${season} E${number}: ${title}` : t`Play S${season} E${number}: ${title}`;

  return (
    <li
      data-current={current || undefined}
      className="group relative flex items-center gap-3 rounded-md p-2 hover:bg-surface-hover data-current:bg-surface-raised sm:gap-4"
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
        <div className="text-sm text-text-muted">{timeOf(w) ?? ""}</div>
      </Link>
      {w.userState?.played ? (
        <Icon name="check" className="size-5 shrink-0 text-accent" />
      ) : status ? (
        <span data-status={status} className="shrink-0 text-xs font-semibold text-accent uppercase">
          {status === "unmatched" && <Trans>Unmatched</Trans>}
          {status === "identifying" && <Trans>Identifying</Trans>}
        </span>
      ) : null}
    </li>
  );
}

type ShelfProps = {
  seasonId: string;
  /** The episode whose page this is, its title in the accent. */
  currentId?: string;
  /** The episode the shelf opens showing and Up or Down enters it on: Next Up on a series' page, the open episode on an episode's. */
  entry?: string;
  /** Above the cards, such as the season strip and the season's heading. */
  header: ReactNode;
};

/**
 * A season's episodes on TV, as the Apple app shows them in a wide window:
 * a row of cards, each its still with the progress bar and a check once
 * watched, then "4. Reload", and the runtime, or the time left in the
 * accent, before two lines of its synopsis. A season with no synopses
 * keeps its cards a line shorter. The header heads the row, so the page
 * scrolls it into view with the cards.
 *
 * A card has two stops, as a touch screen's still and text are two targets:
 * focus lands on the still, where OK or the remote's Play plays the episode,
 * and Down moves to its text, where OK opens its page. Left and Right stay
 * on the same stop, so descriptions can be read along the row. It's one
 * focusable a card, and which stop is one attribute on the shelf that the
 * stylesheet reads, so changing stop renders nothing and moving along the
 * row measures nothing more.
 */
export function EpisodeShelf({ seasonId, currentId, entry, header }: ShelfProps) {
  const episodes = useQuery({ ...workQueries.children(seasonId), placeholderData: (prev) => prev });
  const navigate = useNavigate();
  const shelf = useRef<HTMLDivElement>(null);
  const onText = () => shelf.current?.hasAttribute("data-tv-card-text") === true;
  const moveToText = (on: boolean) => shelf.current?.toggleAttribute("data-tv-card-text", on);

  if (episodes.isPending) return <EpisodeShelfSkeleton header={header} />;
  const items = episodes.data?.items ?? [];
  const lines = items.some((w) => synopsisOf(w)) ? 2 : 1;
  const play = (w: WorkSummary) => void navigate({ to: "/watch/$workId", params: { workId: w.id } });
  // The page shows the card's title and still while it loads.
  const open = (w: WorkSummary) =>
    void navigate({ to: "/works/$workId", params: { workId: w.id }, state: { work: w } });
  const cardFocus = (w: WorkSummary) => ({
    onPress: () => (onText() || !w.available ? open(w) : play(w)),
    onArrow: (direction: string) => {
      if (direction === "down" && !onText()) {
        moveToText(true);
        return false;
      }
      if (direction === "up" && onText()) {
        moveToText(false);
        return false;
      }
      return true;
    },
  });
  const onKeyDown = (e: KeyboardEvent) => {
    const key = keyName(e, true);
    if (key !== "MediaPlay" && key !== "MediaPlayPause") return;
    const focused = (e.target as Element).closest<HTMLElement>("[data-tv-focusable]")?.dataset.tvFocusKey;
    const w = items.find((it) => `card:episodes:${it.id}` === focused);
    if (!w?.available) return;
    e.preventDefault();
    e.stopPropagation();
    play(w);
  };
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: it only hears the remote's Play on the card focused inside.
    <div
      ref={shelf}
      onKeyDown={onKeyDown}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) moveToText(false);
      }}
    >
      <Row
        id="episodes"
        header={header}
        shape="landscape"
        items={items}
        itemKey={(w) => w.id}
        enterOn={entry !== undefined && items.some((w) => w.id === entry) ? entry : undefined}
        tvAcross={shelfAcross * 2}
        cardFocus={cardFocus}
        renderItem={(w) => <EpisodeCard episode={w} lines={lines} current={w.id === currentId} />}
      />
    </div>
  );
}

/**
 * How many of a shelf's cards fit across: one more than other landscape
 * rows, so its row and its header fit under an episode's details without
 * the page scrolling, and each still has fewer pixels for a TV to draw.
 */
const shelfAcross = 4;

/** The shelf's place while its season's episodes load: its header, if any, over a screen's width of cards. */
export function EpisodeShelfSkeleton({ header }: { header?: ReactNode }) {
  return (
    <section data-part="row" data-row="episodes" aria-busy="true" className="flex min-w-0 flex-col gap-3">
      {header}
      <span className="sr-only">
        <Trans>Loading episodes…</Trans>
      </span>
      <ul
        aria-hidden="true"
        style={{ "--tv-across": shelfAcross } as CSSProperties}
        className="flex gap-(--layout-tv-row-gap)"
      >
        {[0, 1, 2, 3].map((i) => (
          <li key={i} className="flex shrink-0 flex-col gap-2 tv-card-width">
            <div className="aspect-landscape animate-pulse rounded-md bg-placeholder" />
            <div className="h-6 w-1/2 rounded-sm bg-placeholder" />
          </li>
        ))}
      </ul>
    </section>
  );
}

/** An episode's synopsis on one line, without markup, or undefined when it has none. */
function synopsisOf(w: WorkSummary): string | undefined {
  const text = w.episode?.overview
    ?.replace(/<[^>]+>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  return text || undefined;
}

function EpisodeCard({ episode: w, lines, current }: { episode: WorkSummary; lines: 1 | 2; current: boolean }) {
  const progress = progressOf(w);
  const status = cardStatus(w);
  const time = timeOf(w);
  const synopsis = synopsisOf(w);
  const left = w.userState?.positionMs !== undefined;

  return (
    <div data-part="episode-card" data-current={current || undefined} className="flex min-w-0 flex-col gap-2">
      <Link
        to="/works/$workId"
        params={{ workId: w.id }}
        state={{ work: w }}
        className="flex flex-col gap-2 rounded-md"
      >
        <div data-tv-ring className="relative">
          <Artwork
            image={w.images.still ?? w.images.backdrop}
            title={w.title}
            shape="landscape"
            width={400}
            density={cardDensity}
            part="still"
            className="relative shadow-card"
          >
            {progress !== undefined && <ProgressBar value={progress} className="absolute inset-x-0 bottom-0" />}
          </Artwork>
          <Corner work={w} />
          {status && <StatusBadge status={status} right />}
          {current && (
            <span className="absolute bottom-3 left-2 rounded-sm bg-bg px-2 text-xs font-semibold text-text">
              <Trans>Viewing</Trans>
            </span>
          )}
          {w.available && (
            <span
              aria-hidden="true"
              data-part="episode-play-hint"
              className="absolute inset-0 m-auto size-16 items-center justify-center rounded-full bg-scrim text-text"
            >
              <Icon name="play" />
            </span>
          )}
        </div>
        <div data-part="episode-text" className="-mx-2 flex min-w-0 flex-col gap-1 rounded-md px-2">
          <div className={`truncate text-sm font-semibold ${current ? "text-accent" : "text-text"}`}>
            {episodeRange(w)}. {w.title}
          </div>
          <p className={`text-sm text-text-muted ${lines === 2 ? "line-clamp-2 min-h-2lh" : "truncate"}`}>
            {time && <span className={`font-medium ${left ? "text-accent" : "text-text"}`}>{time}</span>}
            {time && synopsis ? " · " : ""}
            {synopsis}
          </p>
        </div>
      </Link>
    </div>
  );
}
