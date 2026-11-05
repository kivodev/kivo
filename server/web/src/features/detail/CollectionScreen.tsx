import { Plural } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { workQueries } from "@/api/works";
import { Artwork } from "@/components/Artwork";
import { CardGrid } from "@/components/CardGrid";
import { useDocumentTitle } from "@/components/documentTitle";
import { LoadError } from "@/components/LoadError";
import { GridSkeleton } from "@/components/Skeleton";
import { WorkCard } from "@/features/browse/WorkCard";
import { useTvEmptyScreen } from "@/tv/focus";
import { HeroBackdrop } from "./DetailHeader";

const collectionRoute = getRouteApi("/authed/collections/$collectionId");

/**
 * A movie collection, or a folder of an other-videos library. It shows the
 * backdrop, poster, name and count, and the root works in release order.
 */
export function CollectionScreen() {
  const { collectionId } = collectionRoute.useParams();
  const read = useQuery(workQueries.collection(collectionId));
  const collection = read.data;
  useDocumentTitle(collection?.name);
  useTvEmptyScreen(collection !== undefined && collection.items.length === 0);
  if (!collection) {
    return read.isError ? (
      <LoadError error={read.error} onRetry={() => void read.refetch()} />
    ) : (
      <div className="px-4 pt-16 pb-12 sm:px-8">
        <GridSkeleton rail={false} />
      </div>
    );
  }
  const shape = collection.source === "folder" ? "landscape" : "poster";
  const { poster, backdrop } = collection.images;
  return (
    <div data-part="detail" data-kind="collection">
      <section
        data-part="hero"
        className={`relative isolate flex flex-col justify-end overflow-hidden tv:overflow-visible ${backdrop ? "min-h-(--layout-hero-min-height)" : ""}`}
      >
        {backdrop && <HeroBackdrop image={backdrop} />}
        <div data-part="detail-header" className="flex items-end gap-6 px-4 pt-16 pb-8 sm:px-8">
          {poster && (
            <div className="hidden w-32 shrink-0 sm:block">
              <Artwork image={poster} title={collection.name} shape="poster" width={128} className="shadow-card" />
            </div>
          )}
          <div className="flex max-w-prose flex-col gap-4">
            <h1 className="line-clamp-2 text-3xl font-bold text-text">{collection.name}</h1>
            <p className="text-sm text-text-muted">
              <Plural value={collection.items.length} one="# item" other="# items" />
            </p>
          </div>
        </div>
      </section>
      <section className="px-4 pb-12 sm:px-8 tv:relative">
        <CardGrid
          shape={shape}
          items={collection.items}
          itemKey={(w) => w.id}
          renderItem={(w) => <WorkCard work={w} shape={shape} />}
          focusKey="members"
          autoFocus
        />
      </section>
    </div>
  );
}
