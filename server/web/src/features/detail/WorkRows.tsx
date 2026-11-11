import { useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import type { CollectionSummary, Credit, Trailer, WorkDetail, WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { Artwork, cardDensity } from "@/components/Artwork";
import { PersonCard, useRoleLabel } from "@/components/PersonCard";
import { Row } from "@/components/Row";
import { markClass, WorkCard } from "@/features/browse/WorkCard";
import { formatClock } from "@/i18n/format";
import { isTv } from "@/tv/flag";

/** Trailers, then cast and crew, then the other extras by type, then each collection's other members. */
export function WorkRows({ work }: { work: WorkDetail }) {
  return (
    <div className="flex flex-col gap-10 px-4 pb-12 sm:px-8 tv:relative">
      <TrailersRow extras={work.extras} trailers={work.movie?.trailers} />
      <CastRow credits={work.credits} />
      <ExtrasRows extras={work.extras} />
      {work.collections.map((c) => (
        <CollectionRow key={c.id} collection={c} currentId={work.id} />
      ))}
    </div>
  );
}

function CastRow({ credits }: { credits: readonly Credit[] }) {
  const { t } = useLingui();
  const roleLabel = useRoleLabel();
  if (credits.length === 0) return null;
  return (
    <Row
      id="cast"
      title={t`Cast & Crew`}
      shape="poster"
      items={credits}
      itemKey={(c) => `${c.personId}:${c.role}:${c.character ?? ""}`}
      renderItem={(c) => (
        <PersonCard personId={c.personId} name={c.name} image={c.photo} detail={c.character ?? roleLabel(c.role)} />
      )}
    />
  );
}

/**
 * A movie's trailers: the trailer files in its folder, which play here,
 * then its trailers on YouTube, which open there. The packaged TV app never
 * leaves its page, so it shows the files alone.
 */
function TrailersRow({ extras, trailers = [] }: { extras: readonly WorkSummary[]; trailers?: readonly Trailer[] }) {
  const { t } = useLingui();
  const items: (WorkSummary | Trailer)[] = [
    ...extras.filter((e) => e.extra?.type === "trailer"),
    ...(isTv ? [] : trailers),
  ];
  if (items.length === 0) return null;
  return (
    <Row
      id="trailers"
      title={t`Trailers`}
      shape="landscape"
      items={items}
      itemKey={(item) => item.id}
      renderItem={(item) =>
        "url" in item ? (
          <TrailerCard trailer={item} />
        ) : (
          <WorkCard
            work={item}
            shape="landscape"
            badge={item.durationMs ? formatClock(item.durationMs) : undefined}
            caption={t`In your library`}
          />
        )
      }
    />
  );
}

/** A trailer on YouTube, which opens there in a new tab. Its mark says so before the click. */
function TrailerCard({ trailer }: { trailer: Trailer }) {
  const { t } = useLingui();
  return (
    <a
      href={trailer.url}
      target="_blank"
      rel="noreferrer"
      data-part="card"
      data-kind="trailer"
      className="flex min-w-0 flex-col gap-2 rounded-md"
    >
      <div className="relative">
        <Artwork
          image={trailer.image}
          title={trailer.title}
          shape="landscape"
          width={288}
          density={cardDensity}
          part="still"
          className="relative shadow-card"
        />
        <span data-part="site" className={markClass}>
          YouTube ↗
        </span>
      </div>
      <div className="min-w-0 px-0.5">
        <div className="truncate text-sm font-semibold text-text">{trailer.title}</div>
        <div className="truncate text-sm text-text-muted">{t`Opens in YouTube`}</div>
      </div>
    </a>
  );
}

function ExtrasRows({ extras }: { extras: readonly WorkSummary[] }) {
  const { t } = useLingui();
  const titles: Record<string, string> = {
    featurette: t`Featurettes`,
    behindTheScenes: t`Behind the Scenes`,
    deletedScene: t`Deleted Scenes`,
    interview: t`Interviews`,
    scene: t`Scenes`,
    short: t`Shorts`,
    other: t`Other extras`,
  };
  const groups = new Map<string, WorkSummary[]>();
  for (const extra of extras) {
    if (extra.extra?.type === "trailer") continue; // in the Trailers row
    const type = extra.extra?.type && extra.extra.type in titles ? extra.extra.type : "other";
    groups.set(type, [...(groups.get(type) ?? []), extra]);
  }
  return (
    <>
      {[...groups].map(([type, items]) => (
        <Row
          key={type}
          id={`extras:${type}`}
          title={titles[type] ?? t`Extras`}
          shape="landscape"
          items={items}
          itemKey={(w) => w.id}
          renderItem={(w) => <WorkCard work={w} shape="landscape" />}
        />
      ))}
    </>
  );
}

function CollectionRow({ collection, currentId }: { collection: CollectionSummary; currentId: string }) {
  const { t } = useLingui();
  const members = useQuery(workQueries.collection(collection.id));
  const items = (members.data?.items ?? []).filter((w) => w.id !== currentId);
  if (items.length === 0) return null;
  return (
    <Row
      id="collection"
      focusKey={`collection:${collection.id}`}
      title={t`Part of ${collection.name}`}
      shape="poster"
      items={items}
      itemKey={(w) => w.id}
      renderItem={(w) => <WorkCard work={w} shape="poster" />}
    />
  );
}
