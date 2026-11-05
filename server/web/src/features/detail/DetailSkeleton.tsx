import { Trans } from "@lingui/react/macro";
import { getRouteApi, useLocation } from "@tanstack/react-router";
import type { ReactNode } from "react";
import type { WorkSummary } from "@/api/schemas";
import { Artwork } from "@/components/Artwork";
import { episodeCode, labelledTitle } from "@/features/browse/WorkCard";

const workRoute = getRouteApi("/authed/works/$workId");

/** The card a work's page was opened from, if it was. */
export function useSummary(): WorkSummary | undefined {
  const { workId } = workRoute.useParams();
  const work = useLocation({ select: (location) => location.state.work });
  return work?.id === workId ? work : undefined;
}

type Props = {
  /** The card the page was opened from. */
  work?: WorkSummary;
  /** Why the page couldn't load, shown under the hero in place of its actions. */
  failure?: ReactNode;
};

/**
 * A detail page while it loads. It has the hero's shape, so the page fills
 * in where it stands. Opened from a card, it shows the card's title, year
 * and backdrop at once, and keeps them if the page fails.
 */
export function DetailSkeleton({ work, failure }: Props) {
  const image = work?.kind === "episode" ? (work.images.still ?? work.images.backdrop) : work?.images.backdrop;
  const code = work && episodeCode(work);
  return (
    <div aria-busy={failure ? undefined : "true"}>
      {!failure && (
        <span className="sr-only">
          <Trans>Loading…</Trans>
        </span>
      )}
      <section data-part="hero" className="relative isolate min-h-(--layout-hero-min-height) overflow-hidden">
        {image && (
          <div className="absolute inset-0 -z-10">
            <Artwork image={image} title="" shape="landscape" width={1920} corners="none" className="size-full" />
            <div className="absolute inset-0 bg-linear-to-r from-scrim via-scrim to-scrim-soft" />
            <div className="absolute inset-0 bg-linear-to-t from-bg to-transparent" />
          </div>
        )}
        <div className="flex max-w-prose flex-col gap-4 px-4 pt-16 pb-8 sm:px-8">
          {work ? (
            <h1 className="line-clamp-2 text-3xl font-bold text-text">
              {code ? (
                <>
                  <span className="text-text-muted">{code} ·</span> {work.title}
                </>
              ) : (
                labelledTitle(work)
              )}
            </h1>
          ) : (
            <div className="h-9 w-2/3 animate-pulse rounded-md bg-placeholder" />
          )}
          {work?.year ? (
            <p className="text-sm text-text-muted">{work.year}</p>
          ) : (
            !failure && <div className="h-5 w-40 rounded-sm bg-placeholder" />
          )}
          {!failure && (
            <div className="flex gap-2">
              <div className="h-10 w-32 animate-pulse rounded-md bg-placeholder" />
              <div className="size-10 rounded-md bg-placeholder" />
              <div className="size-10 rounded-md bg-placeholder" />
            </div>
          )}
        </div>
      </section>
      {failure}
    </div>
  );
}
