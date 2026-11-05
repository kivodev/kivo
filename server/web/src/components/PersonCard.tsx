import { useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import type { ImageRef } from "@/api/schemas";
import { isTv } from "@/tv/flag";
import { Artwork, cardDensity } from "./Artwork";

type Props = {
  personId: string;
  name: string;
  image: ImageRef | undefined;
  /** Below the name, such as their character or "Actor · 23 titles". */
  detail?: ReactNode;
};

/** A person's round photo or initials, and their name. It opens their page. */
export function PersonCard({ personId, name, image, detail }: Props) {
  const photo = <Artwork image={image} title={name} shape="square" width={152} density={cardDensity} corners="full" />;
  return (
    <Link
      to="/people/$personId"
      params={{ personId }}
      data-part="person"
      className="flex flex-col gap-2 rounded-md hover:[&_[data-part=person-name]]:text-accent"
    >
      {isTv ? (
        // The focus ring hugs the photo, with the name below it.
        <div data-tv-ring>{photo}</div>
      ) : (
        photo
      )}
      <div className="text-center">
        <div data-part="person-name" className="truncate text-sm font-semibold">
          {name}
        </div>
        {detail && <div className="truncate text-xs text-text-muted">{detail}</div>}
      </div>
    </Link>
  );
}

/** A credit's role as people read it: Actor, Director, Author. */
export function useRoleLabel(): (role: string) => string {
  const { t } = useLingui();
  const labels: Record<string, string> = {
    actor: t`Actor`,
    director: t`Director`,
    writer: t`Writer`,
    author: t`Author`,
    narrator: t`Narrator`,
  };
  return (role) => labels[role] ?? role.charAt(0).toUpperCase() + role.slice(1);
}
