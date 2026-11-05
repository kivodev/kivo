import { Plural } from "@lingui/react/macro";
import { Link, type LinkProps } from "@tanstack/react-router";
import type { ReactNode } from "react";
import type { BookSeriesSummary, CollectionSummary, ImageRef, PersonSummary } from "@/api/schemas";
import { Artwork, type ArtworkShape, cardDensity } from "@/components/Artwork";

type Props = {
  /** The card's data-kind, as a work card's data-kind names its kind. */
  kind: "collection" | "author" | "bookSeries";
  link: LinkProps;
  image: ImageRef | undefined;
  title: string;
  subtitle: ReactNode;
  shape: ArtworkShape;
  width: number | undefined;
  /** A person's round photo or initials. */
  round?: boolean;
  /** A book series' first cover on a stack. */
  stacked?: boolean;
};

/**
 * The card for a group of works, such as a collection, an author or a
 * book series. It shows the artwork, name and count, and opens the group's
 * page. It has no watch state and no menu.
 */
function GroupCard({ kind, link, image, title, subtitle, shape, width, round = false, stacked = false }: Props) {
  return (
    <div data-part="card" data-kind={kind} className="group relative flex min-w-0 flex-col gap-2">
      <Link {...link} className="flex flex-col gap-2 rounded-md">
        <div data-tv-ring className={`relative ${stacked ? "aspect-square px-1.5 pt-3" : ""}`}>
          {stacked && (
            <>
              <span
                aria-hidden="true"
                className="absolute inset-x-4 top-0 h-4 rounded-t-md bg-text-subtle opacity-40"
              />
              <span
                aria-hidden="true"
                className="absolute inset-x-2.5 top-1.5 h-4 rounded-t-md bg-text-subtle opacity-70"
              />
            </>
          )}
          <Artwork
            image={image}
            title={title}
            shape={shape}
            width={width ?? (shape === "landscape" ? 288 : 160)}
            density={cardDensity}
            part={{ poster: "poster", landscape: "still", square: "cover" }[shape]}
            corners={round ? "full" : "md"}
            className="relative shadow-card"
          />
          <span
            aria-hidden="true"
            className={`absolute inset-0 bg-badge opacity-0 transition-opacity group-hover:opacity-50 pointer-coarse:hidden ${round ? "rounded-full" : "rounded-md"}`}
          />
        </div>
        <div className={`min-w-0 px-0.5 tv:px-0 ${round ? "text-center" : ""}`}>
          <div className="truncate text-sm font-semibold text-text">{title}</div>
          <div className="truncate text-sm text-text-muted">{subtitle}</div>
        </div>
      </Link>
    </div>
  );
}

/**
 * A movie collection, or a videos library's folder, with its poster and a
 * count of what it holds.
 */
export function CollectionCard({
  collection,
  shape,
  width,
}: {
  collection: CollectionSummary;
  shape: ArtworkShape;
  width?: number;
}) {
  const n = collection.workCount;
  return (
    <GroupCard
      kind="collection"
      link={{ to: "/collections/$collectionId", params: { collectionId: collection.id } }}
      image={collection.images.poster}
      title={collection.name}
      subtitle={
        collection.source === "provider" ? (
          <Plural value={n} one="# movie" other="# movies" />
        ) : (
          <Plural value={n} one="# item" other="# items" />
        )
      }
      shape={shape}
      width={width}
    />
  );
}

/** An audiobooks library's author, with their round photo and how many of the library's books they wrote. */
export function AuthorCard({ author, width }: { author: PersonSummary; width?: number }) {
  return (
    <GroupCard
      kind="author"
      link={{ to: "/people/$personId", params: { personId: author.id } }}
      image={author.photo}
      title={author.name}
      subtitle={<Plural value={author.workCount} one="# book" other="# books" />}
      shape="square"
      width={width}
      round
    />
  );
}

/** An audiobooks library's book series, shown as its first book's cover on a stack. */
export function BookSeriesCard({ series, width }: { series: BookSeriesSummary; width?: number }) {
  return (
    <GroupCard
      kind="bookSeries"
      link={{ to: "/book-series/$seriesId", params: { seriesId: series.id } }}
      image={series.cover}
      title={series.name}
      subtitle={<Plural value={series.bookCount} one="# book" other="# books" />}
      shape="square"
      width={width}
      stacked
    />
  );
}
