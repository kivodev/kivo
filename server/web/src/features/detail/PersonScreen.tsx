import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useState } from "react";
import { bookQueries, useRenamePerson } from "@/api/books";
import type { Person, WorkSummary } from "@/api/schemas";
import { useIsAdmin } from "@/api/users";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { LoadError } from "@/components/LoadError";
import { Row } from "@/components/Row";
import { RowsSkeleton } from "@/components/Skeleton";
import { WorkCard } from "@/features/browse/WorkCard";
import { isTv } from "@/tv/flag";
import { useTvEmptyScreen } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { BookShelf } from "./BookShelf";
import { Overview } from "./Overview";
import { RenameDialog } from "./RenameDialog";

const personRoute = getRouteApi("/authed/people/$personId");

/**
 * A person's page. It shows their photo and biography when the provider
 * gives them, and their movies, shows and other videos, newest first. Then
 * come the books they wrote, by series in series order and then
 * standalone, and the books they narrated. An author or narrator can be
 * renamed.
 */
export function PersonScreen() {
  const { personId } = personRoute.useParams();
  const read = useQuery(bookQueries.person(personId));
  if (read.data) return <PersonPage person={read.data} />;
  return read.isError ? (
    <LoadError error={read.error} onRetry={() => void read.refetch()} />
  ) : (
    <div className="flex flex-col gap-8 px-4 py-6 sm:px-8">
      <div className="flex flex-col gap-6 sm:flex-row sm:items-center">
        <div className="size-32 shrink-0 animate-pulse rounded-full bg-placeholder" />
        <div className="h-9 w-64 rounded-md bg-placeholder" />
      </div>
      <RowsSkeleton />
    </div>
  );
}

function PersonPage({ person }: { person: Person }) {
  const { t } = useLingui();
  const { personId } = personRoute.useParams();
  const isAdmin = useIsAdmin();
  const navigate = personRoute.useNavigate();
  const rename = useRenamePerson();
  const [renaming, setRenaming] = useState(false);
  const name = person.name;
  useDocumentTitle(name);
  const books = person.bookSeries.length + person.standalone.length + person.narrated.length > 0;
  // The books they wrote, as the iPhone app counts them.
  const written = person.bookSeries.reduce((n, s) => n + s.items.length, 0) + person.standalone.length;
  const worksOf = (kind: string): WorkSummary[] => person.works.filter((w) => w.kind === kind);
  const groups = [
    { kind: "movie", title: t`Movies` },
    { kind: "series", title: t`Shows` },
    { kind: "video", title: t`Other videos` },
  ];
  // The first shelf with anything on it has focus first on TV, and a page
  // with nothing hands focus to the rail.
  const firstGroup = groups.find((g) => worksOf(g.kind).length > 0)?.kind;
  const firstShelf = firstGroup === undefined ? (person.bookSeries[0]?.id ?? "standalone") : undefined;
  useTvEmptyScreen(person.works.length === 0 && !books);

  return (
    <div data-part="person-page" className="flex flex-col gap-8 px-4 py-6 sm:px-8">
      <header className="flex flex-col gap-6 sm:flex-row sm:items-center">
        <div className="w-32 shrink-0">
          <Artwork image={person.photo} title={name} shape="square" width={128} corners="full" part="person-photo" />
        </div>
        <div className="flex max-w-prose flex-col gap-2">
          <h1 className="text-3xl font-bold text-text">{name}</h1>
          <div className="flex flex-wrap items-center gap-x-3 text-sm text-text-muted">
            {written > 0 ? (
              <Plural value={written} one="# book" other="# books" />
            ) : person.works.length > 0 ? (
              <Plural value={person.works.length} one="# title" other="# titles" />
            ) : null}
            {isAdmin && books && !isTv && (
              <Button size="sm" variant="ghost" className="-ml-2" onPress={() => setRenaming(true)}>
                <Trans>Rename</Trans>
              </Button>
            )}
          </div>
          {person.overview && <Overview text={person.overview} />}
        </div>
      </header>
      {groups.map((group) => {
        const found = worksOf(group.kind);
        if (found.length === 0) return null;
        return (
          <Row
            key={group.kind}
            id={group.kind}
            autoFocus={group.kind === firstGroup}
            title={group.title}
            shape="poster"
            items={found}
            itemKey={(w) => w.id}
            renderItem={(w) => <WorkCard work={w} shape="poster" />}
          />
        );
      })}
      {person.bookSeries.map((s) => (
        <section key={s.id} className="flex flex-col gap-3">
          <h2 className="text-lg font-semibold">
            <TvControl focusKey={`book-series:${s.id}`} className="inline-block">
              <Link to="/book-series/$seriesId" params={{ seriesId: s.id }} className="hover:text-accent">
                {s.name}
              </Link>
            </TvControl>
          </h2>
          <BookShelf books={s.items} seriesId={s.id} autoFocus={s.id === firstShelf} />
        </section>
      ))}
      {person.standalone.length > 0 && (
        <section className="flex flex-col gap-3">
          <h2 className="text-lg font-semibold">
            {person.bookSeries.length > 0 ? <Trans>Standalone books</Trans> : <Trans>Books</Trans>}
          </h2>
          <BookShelf books={person.standalone} focusKey="standalone" autoFocus={firstShelf === "standalone"} />
        </section>
      )}
      {person.narrated.length > 0 && (
        <section className="flex flex-col gap-3">
          <h2 className="text-lg font-semibold">
            <Trans>Narrated</Trans>
          </h2>
          <BookShelf books={person.narrated} focusKey="narrated" />
        </section>
      )}
      <RenameDialog
        isOpen={renaming}
        name={name}
        isPending={rename.isPending}
        error={rename.error}
        onClose={() => setRenaming(false)}
        onRename={(next) =>
          rename.mutate(
            { params: { path: { id: personId } }, body: { name: next } },
            {
              onSuccess: (p) => {
                setRenaming(false);
                // Merged into someone else, so show that person's page in
                // this one's place.
                if (p.id !== personId) {
                  void navigate({ to: "/people/$personId", params: { personId: p.id }, replace: true });
                }
              },
            },
          )
        }
      />
    </div>
  );
}
