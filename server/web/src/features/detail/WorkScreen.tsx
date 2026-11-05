import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Navigate } from "@tanstack/react-router";
import type { WorkDetail } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { useDocumentTitle } from "@/components/documentTitle";
import { LoadError } from "@/components/LoadError";
import { labelledTitle } from "@/features/browse/WorkCard";
import { BookDetail } from "./BookDetail";
import { DetailSkeleton, useSummary } from "./DetailSkeleton";
import { EpisodeDetail } from "./EpisodeDetail";
import { MovieDetail } from "./MovieDetail";
import { SeriesDetail } from "./SeriesDetail";

const workRoute = getRouteApi("/authed/works/$workId");

/**
 * The detail page for any work, by kind. A season opens as its series'
 * page with that season shown. Kinds this client doesn't know get the
 * generic page. Until the work arrives, the hero shows what the card it
 * was opened from showed, and keeps that if the work fails to load.
 */
export function WorkScreen() {
  const { workId } = workRoute.useParams();
  const summary = useSummary();
  const read = useQuery(workQueries.detail(workId));
  const work = read.data;
  const known = work ?? summary;
  useDocumentTitle(known && (known.episode ? `${known.episode.seriesTitle}: ${known.title}` : labelledTitle(known)));
  if (!work) {
    if (!read.isError) return <DetailSkeleton work={summary} />;
    const failure = <LoadError error={read.error} onRetry={() => void read.refetch()} />;
    return summary ? <DetailSkeleton work={summary} failure={failure} /> : failure;
  }
  if (work.kind === "season" && work.season) {
    return (
      <Navigate to="/works/$workId" params={{ workId: work.season.seriesId }} search={{ season: work.id }} replace />
    );
  }
  return detailOf(work);
}

function detailOf(work: WorkDetail) {
  switch (work.kind) {
    case "series":
      return <SeriesDetail work={work} />;
    case "episode":
      return <EpisodeDetail work={work} />;
    case "book":
      return <BookDetail work={work} />;
    default:
      return <MovieDetail work={work} />;
  }
}
