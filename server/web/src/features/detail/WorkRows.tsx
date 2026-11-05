import { useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import type { CollectionSummary, Credit, WorkDetail, WorkSummary } from "@/api/schemas";
import { workQueries } from "@/api/works";
import { PersonCard, useRoleLabel } from "@/components/PersonCard";
import { Row } from "@/components/Row";
import { WorkCard } from "@/features/browse/WorkCard";

/** Cast and crew, then extras by type, then each collection's other members. */
export function WorkRows({ work }: { work: WorkDetail }) {
  return (
    <div className="flex flex-col gap-10 px-4 pb-12 sm:px-8 tv:relative">
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

function ExtrasRows({ extras }: { extras: readonly WorkSummary[] }) {
  const { t } = useLingui();
  const titles: Record<string, string> = {
    trailer: t`Trailers`,
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
