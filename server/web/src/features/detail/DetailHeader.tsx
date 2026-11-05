import type { ReactNode } from "react";
import type { ImageRef, WorkDetail, WorkSummary } from "@/api/schemas";
import { Artwork } from "@/components/Artwork";
import { labelledTitle } from "@/features/browse/WorkCard";
import { formatCommunityRating, formatRuntime } from "@/i18n/format";
import { isTv } from "@/tv/flag";
import { TvFocusGroup } from "@/tv/focus";
import { tvMotion } from "@/tv/motion";
import { DetailMenu } from "./DetailMenu";
import { Overview } from "./Overview";
import { PlayFromStartButton } from "./PlaybackControls";
import { WatchToggles } from "./WatchToggles";

type Props = {
  work: WorkDetail;
  /** Overrides the work's own backdrop, such as an episode's still. */
  backdrop?: ImageRef;
  /** Lines above the title, such as the series link on an episode. */
  eyebrow?: ReactNode;
  /** Replaces the title in the heading, such as "S2 · E5 · Title" on an episode. */
  heading?: ReactNode;
  /** Replaces the default meta line. */
  meta?: ReactNode;
  /** The primary button, with a short label such as Play, Resume 5:56 or Play S1 E1. */
  primary?: ReactNode;
  /** What the primary button plays, shown under the buttons, such as "Reload · 30 min left · Ends at 9:42 PM". */
  playing?: ReactNode;
  /** Play from beginning, a button beside the primary one once there's progress. */
  onPlayFromStart?: () => void;
  /** Pickers under the actions. */
  children?: ReactNode;
  /** How many episodes marking played covers, so a series or season can ask first. */
  cascadeCount?: number;
  /**
   * Keeps room for the collapsed overview's three lines even when it is
   * shorter or missing, so stepping between episodes keeps the hero's
   * height.
   */
  holdOverview?: boolean;
};

/** The community rating as the details line shows it, such as "★ 7.8". It is undefined without a rating. */
export function communityRating(work: WorkSummary): string | undefined {
  return work.communityRating === undefined ? undefined : `★ ${formatCommunityRating(work.communityRating)}`;
}

function MetaLine({ work }: { work: WorkDetail }) {
  const parts: ReactNode[] = [];
  if (work.year) parts.push(work.year);
  if (work.durationMs) parts.push(formatRuntime(work.durationMs));
  const rating = communityRating(work);
  if (rating) parts.push(rating);
  if (work.contentRating) parts.push(work.contentRating);
  if (work.genres.length > 0) parts.push(work.genres.slice(0, 2).join(", "));
  return <p className="text-sm text-text-muted">{parts.join(" · ")}</p>;
}

/**
 * A hero's backdrop, behind it, darkened at the left and fading into the
 * page. On TV it fills the screen's width and its first screen, under the
 * rail and past the overscan, fading into the page by the screen's foot,
 * whatever the hero's height. Its hero mustn't clip it there, and what
 * follows the hero is positioned (tv:relative) so that it's drawn over it.
 * Without an image it's a neutral wash, so the hero keeps its look.
 */
export function HeroBackdrop({ image }: { image?: ImageRef }) {
  return (
    <div
      data-part="hero-backdrop"
      className="absolute inset-0 -z-10 tv:-top-(--layout-tv-overscan-y) tv:-right-(--layout-tv-overscan-x) tv:bottom-auto tv:-left-(--layout-tv-content-left) tv:h-dvh"
    >
      {image ? (
        <Artwork image={image} title="" shape="landscape" width={1920} corners="none" className="size-full" />
      ) : (
        <div className="size-full bg-linear-to-bl from-surface-raised to-bg" />
      )}
      <div className="absolute inset-0 bg-linear-to-r from-scrim via-scrim to-scrim-soft" />
      <div className="absolute inset-0 bg-linear-to-t from-bg to-transparent" />
    </div>
  );
}

/**
 * The hero. The backdrop runs across the top of the page, darkened at the
 * left and fading into the page, with the title, actions and overview at
 * its foot. On TV focus in the hero keeps the whole hero in view. A TV's
 * hero is shorter, so a series' first episode shows on the first screen
 * below it, and is as tall without artwork as with it.
 */
export function DetailHeader({
  work,
  backdrop,
  eyebrow,
  heading,
  meta,
  primary,
  playing,
  onPlayFromStart,
  children,
  cascadeCount = 0,
  holdOverview = false,
}: Props) {
  const image = backdrop ?? work.images.backdrop;

  return (
    <section
      data-part="hero"
      className={`relative isolate flex flex-col justify-end overflow-hidden tv:min-h-(--layout-tv-hero-min-height) tv:overflow-visible ${image ? "min-h-(--layout-hero-min-height)" : ""}`}
    >
      {(image || isTv) && <HeroBackdrop image={image} />}
      {/* Without a backdrop a TV's hero shows the poster, or its titled tile, on the right, so the screen isn't left empty. */}
      {!image && isTv && (
        <div aria-hidden data-part="hero-poster" className="absolute top-0 right-0 w-52">
          <Artwork image={work.images.poster} title={labelledTitle(work)} shape="poster" width={364} />
        </div>
      )}
      <TvFocusGroup
        focusKey="hero"
        onFocusWithin={(within) => {
          if (within) window.scrollTo({ top: 0, behavior: tvMotion() });
        }}
      >
        <div data-part="detail-header" className="flex max-w-prose flex-col gap-4 px-4 pt-16 pb-8 sm:px-8">
          {eyebrow}
          <h1 className="line-clamp-2 text-3xl font-bold text-text">{heading ?? labelledTitle(work)}</h1>
          {meta ?? <MetaLine work={work} />}
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2">
              {primary}
              {onPlayFromStart && <PlayFromStartButton onPress={onPlayFromStart} />}
              <WatchToggles work={work} cascadeCount={cascadeCount} />
              <DetailMenu work={work} />
            </div>
            {playing && (
              <p data-part="playing" className="truncate text-sm text-text-muted">
                {playing}
              </p>
            )}
          </div>
          {children}
          {(work.overview || holdOverview) && <Overview text={work.overview} lines={3} hold={holdOverview} />}
        </div>
      </TvFocusGroup>
    </section>
  );
}
