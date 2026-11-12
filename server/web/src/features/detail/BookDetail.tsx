import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { Link } from "@tanstack/react-router";
import { Fragment, type ReactNode } from "react";
import type { PersonRef, WorkDetail } from "@/api/schemas";
import { Artwork } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { Icon } from "@/components/Icon";
import { useBookPlayer } from "@/features/player/useBookPlayer";
import { formatClock, formatDate, formatRuntime } from "@/i18n/format";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { DetailMenu } from "./DetailMenu";
import { Overview } from "./Overview";
import { PlayFromStartButton } from "./PlaybackControls";
import { defaultVersion } from "./versions";
import { WatchToggles } from "./WatchToggles";
import { SimilarRow } from "./WorkRows";

/** People as links to their pages, joined with commas. `kind` keeps the focus keys apart for a person who is both. */
function People({ people, kind }: { people: readonly PersonRef[]; kind: "author" | "narrator" }) {
  return (
    <>
      {people.map((p, i) => (
        <Fragment key={p.id}>
          {i > 0 && ", "}
          <TvControl focusKey={`${kind}:${p.id}`} className="inline-block">
            <Link
              to="/people/$personId"
              params={{ personId: p.id }}
              className="font-semibold text-text hover:text-accent"
            >
              {p.name}
            </Link>
          </TvControl>
        </Fragment>
      ))}
    </>
  );
}

function Fact({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="flex gap-2">
      <dt className="text-text-muted">{label}</dt>
      <dd className="text-text">{children}</dd>
    </div>
  );
}

// A line of links: a paragraph, or on TV a div, as each link's focus
// wrapper there is a div, which a paragraph can't hold.
const LinkLine = isTv ? "div" : "p";

/**
 * A book's page. It shows the square cover, the authors and narrators as
 * links to their pages, the series with the book's position in it, and
 * the details. Then come Play or Resume with the time left, the books like
 * it, the chapters, each starting playback there, and the files.
 */
export function BookDetail({ work }: { work: WorkDetail }) {
  const { t } = useLingui();
  const { play } = useBookPlayer();
  const book = work.book ?? { authors: [], narrators: [], series: [] };
  const version = defaultVersion(work);
  const position = work.userState?.positionMs;
  const playable = work.available && version?.available === true;
  const chapters = version?.chapters ?? [];
  const meta: ReactNode[] = [];
  if (work.year) meta.push(work.year);
  if (work.durationMs) meta.push(formatRuntime(work.durationMs));
  if (version?.chapterCount) {
    meta.push(<Plural key="chapters" value={version.chapterCount} one="# chapter" other="# chapters" />);
  }
  if (work.genres.length > 0) meta.push(work.genres.join(", "));

  return (
    <div data-part="detail" data-kind="book" className="flex flex-col gap-8 px-4 py-6 sm:px-8">
      <section data-part="detail-header" className="flex flex-col gap-6 sm:flex-row">
        <div className="w-full max-w-64 shrink-0">
          <Artwork image={work.images.poster} title={work.title} shape="square" width={480} part="cover" blur />
        </div>
        <div className="flex max-w-prose flex-col gap-3">
          <h1 className="text-3xl font-bold text-text">{work.title}</h1>
          {book.subtitle && <p className="text-lg text-text-muted">{book.subtitle}</p>}
          {book.authors.length > 0 && (
            <LinkLine className="text-sm text-text-muted">
              <Trans>
                By <People people={book.authors} kind="author" />
              </Trans>
            </LinkLine>
          )}
          {book.narrators.length > 0 && (
            <LinkLine className="text-sm text-text-muted">
              <Trans>
                Narrated by <People people={book.narrators} kind="narrator" />
              </Trans>
            </LinkLine>
          )}
          {book.series.map((s) => {
            const link = (
              <TvControl focusKey={`book-series:${s.id}`} className="inline-block">
                <Link
                  to="/book-series/$seriesId"
                  params={{ seriesId: s.id }}
                  className="font-semibold text-text hover:text-accent"
                >
                  {s.name}
                </Link>
              </TvControl>
            );
            const position = s.position;
            return (
              <LinkLine key={s.id} data-part="book-series" className="text-sm text-text-muted">
                {position ? (
                  <Trans>
                    Book {position} of {link}
                  </Trans>
                ) : (
                  link
                )}
              </LinkLine>
            );
          })}
          <p className="text-sm text-text-muted">
            {meta.map((m, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: the parts are fixed in order.
              <Fragment key={i}>
                {i > 0 && " · "}
                {m}
              </Fragment>
            ))}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            {playable ? (
              <TvFocusable focusKey="play" autoFocus>
                <Button variant="primary" data-part="play" onPress={() => play(work)}>
                  <Icon name="play" className="size-4" />
                  {position ? <Trans>Resume</Trans> : <Trans>Play</Trans>}
                </Button>
              </TvFocusable>
            ) : (
              <p data-part="missing" role="status" className="flex items-center gap-2 font-semibold text-warning">
                <Icon name="alert" />
                <Trans>This file is missing</Trans>
              </p>
            )}
            {playable && position ? <PlayFromStartButton onPress={() => play(work, 0)} /> : null}
            <WatchToggles work={work} />
            <DetailMenu work={work} />
          </div>
          {playable && position && work.durationMs ? (
            <p data-part="playing" className="text-sm text-text-muted">
              <Trans>{formatRuntime(work.durationMs - position)} left</Trans>
            </p>
          ) : null}
          {work.overview && <Overview text={work.overview} />}
          {(book.publisher || book.releaseDate) && (
            <dl className="flex flex-col gap-1 text-sm">
              {book.publisher && <Fact label={<Trans>Publisher</Trans>}>{book.publisher}</Fact>}
              {book.releaseDate && <Fact label={<Trans>Released</Trans>}>{formatDate(book.releaseDate)}</Fact>}
            </dl>
          )}
        </div>
      </section>
      <SimilarRow similar={work.similar} />
      {chapters.length > 0 && (
        <section data-part="book-chapters" className="flex max-w-prose flex-col gap-2">
          <h2 className="text-lg font-semibold">
            <Trans>Chapters</Trans>
          </h2>
          <TvFocusGroup focusKey="chapters" as="div">
            <ol className="flex flex-col text-sm">
              {chapters.map((c, i) => {
                const title = c.title ?? t`Chapter ${i + 1}`;
                return (
                  <li key={c.startMs} className="border-b border-border">
                    <TvFocusable focusKey={`chapter:${c.startMs}`} kind="row">
                      <button
                        type="button"
                        disabled={!playable}
                        aria-label={t`Play from ${title}`}
                        onClick={() => play(work, c.startMs)}
                        className="flex w-full cursor-pointer gap-4 py-2 text-left hover:text-accent disabled:cursor-default disabled:hover:text-text"
                      >
                        <span className="w-8 shrink-0 text-text-muted">{i + 1}</span>
                        <span className="min-w-0 flex-1 truncate">{title}</span>
                        <span className="text-text-muted tabular-nums">{formatClock(c.endMs - c.startMs)}</span>
                      </button>
                    </TvFocusable>
                  </li>
                );
              })}
            </ol>
          </TvFocusGroup>
        </section>
      )}
      {version && version.parts.length > 0 && (
        <section data-part="book-files" className="flex max-w-prose flex-col gap-2">
          <h2 className="text-lg font-semibold">
            <Plural value={version.parts.length} one="# file" other="# files" />
          </h2>
          <ol className="flex flex-col text-sm">
            {version.parts.map((p, i) => (
              <li key={p.fileId} className="flex gap-4 border-b border-border py-2">
                <span className="w-8 shrink-0 text-text-muted">{i + 1}</span>
                <span className="min-w-0 flex-1 truncate">{p.name}</span>
                <span className="text-text-muted">{formatClock(p.durationMs)}</span>
              </li>
            ))}
          </ol>
        </section>
      )}
    </div>
  );
}
