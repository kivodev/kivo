import { Trans } from "@lingui/react/macro";
import type { ReactNode } from "react";

const block = "animate-pulse rounded-md bg-placeholder";
const line = "rounded-sm bg-placeholder";

/** A placeholder that says what it is. There is no spinner without a label. */
function Busy({ className, children }: { className: string; children: ReactNode }) {
  return (
    <div aria-busy="true" className={className}>
      <span className="sr-only">
        <Trans>Loading…</Trans>
      </span>
      {children}
    </div>
  );
}

/** A page placeholder, padded as pages are. */
function Loading({ gap, children }: { gap: string; children: ReactNode }) {
  return <Busy className={`flex flex-col px-4 py-6 sm:px-8 ${gap}`}>{children}</Busy>;
}

/** A row's title and its first cards, laid out as Row lays them out. */
function RowSkeleton({ shape }: { shape: "poster" | "landscape" }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex h-10 items-center">
        <div className={`h-6 w-48 ${line}`} />
      </div>
      <div className="flex gap-4 overflow-hidden pb-2">
        {[0, 1, 2, 3, 4, 5, 6, 7].map((i) => (
          <div
            key={i}
            className={`shrink-0 ${shape === "landscape" ? "w-(--layout-landscape-min-width)" : "w-(--layout-poster-min-width)"}`}
          >
            <CardSkeleton shape={shape} />
          </div>
        ))}
      </div>
    </div>
  );
}

/** A page's title while it loads. It shows the title itself when it's known. */
function TitleSkeleton({ title, width }: { title: ReactNode; width: string }) {
  return (
    <div className="flex min-h-10 items-center">
      {title ?? <div className={`h-8 ${width} rounded-md bg-placeholder`} />}
    </div>
  );
}

/** A content page while its screen's code loads, shown as a title and a row of posters. */
export function PageSkeleton() {
  return (
    <Loading gap="gap-6">
      <div className={`h-8 w-64 ${block}`} />
      <RowSkeleton shape="poster" />
    </Loading>
  );
}

/**
 * Home while it loads. It shows Continue Watching's landscape cards, then
 * a row of posters, as Home lays them out.
 */
export function HomeSkeleton() {
  return (
    <Loading gap="gap-10">
      <RowSkeleton shape="landscape" />
      <RowSkeleton shape="poster" />
    </Loading>
  );
}

/** Rows of posters while they load, as a page of rows lays them out below its title. */
export function RowsSkeleton() {
  return (
    <Busy className="flex flex-col gap-10">
      <RowSkeleton shape="poster" />
      <RowSkeleton shape="poster" />
    </Busy>
  );
}

/**
 * A library while its screen's code loads, laid out as the library is. It
 * shows the title, whether known or not, the count line and the grid.
 */
export function LibrarySkeleton({ title }: { title?: ReactNode }) {
  return (
    <div className="flex flex-col gap-4 px-4 py-6 sm:px-8">
      <div className="flex flex-col gap-3">
        <TitleSkeleton title={title} width="w-48" />
        <CountSkeleton />
      </div>
      <GridSkeleton rail />
    </div>
  );
}

/** A count line while it loads. */
export function CountSkeleton() {
  return (
    <div aria-hidden="true" className="flex h-5 items-center">
      <div className={`h-4 w-32 ${line}`} />
    </div>
  );
}

/** A count in a line of text while it loads, as before a library's sort. */
export function NumberSkeleton() {
  return <span aria-hidden="true" className={`inline-block h-4 w-20 ${line}`} />;
}

/**
 * A grid of cards while it loads, posters unless the library's are
 * another shape, three across on a phone. On TV it's the library grid's
 * six posters or three landscape cards across. With `rail`, it leaves room
 * for the letter rail's column.
 */
export function GridSkeleton({ rail, shape = "poster" }: { rail: boolean; shape?: "poster" | "landscape" | "square" }) {
  const landscape = shape === "landscape";
  return (
    <Busy className="flex gap-2">
      <div
        className={`grid flex-1 gap-(--layout-grid-gap) tv:gap-(--layout-tv-row-gap) tv:tv-grid-cols ${landscape ? "grid-cols-(--layout-landscape-grid) tv:tv-across-landscape" : "grid-cols-(--layout-poster-grid) max-sm:grid-cols-3 tv:tv-across-poster"}`}
      >
        {Array.from({ length: 18 }, (_, i) => i).map((i) => (
          <CardSkeleton key={i} shape={shape} />
        ))}
      </div>
      {rail && <div className="w-(--layout-rail-width) shrink-0 pointer-coarse:-mr-3 pointer-coarse:w-11 tv:w-16" />}
    </Busy>
  );
}

/** A card-shaped placeholder. Its caption is the height of a card's title and subtitle lines. */
export function CardSkeleton({ shape }: { shape: "poster" | "landscape" | "square" }) {
  return (
    <div className="flex flex-col gap-2" aria-hidden="true">
      <div
        className={`${{ poster: "aspect-poster", landscape: "aspect-landscape", square: "aspect-square" }[shape]} w-full ${block}`}
      />
      <div className="flex h-10 flex-col justify-center gap-2">
        <div className={`h-3 w-3/4 ${line}`} />
        <div className={`h-3 w-1/2 ${line}`} />
      </div>
    </div>
  );
}

/** A page that isn't content, while it loads. It shows nothing until the page is there. */
export function BlankPending() {
  return null;
}
