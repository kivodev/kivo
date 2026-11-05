import { Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useId, useState } from "react";
// biome-ignore lint/style/noRestrictedImports: a search box, not a form that saves.
import { Form } from "react-aria-components";
import type { Match, WorkSummary } from "@/api/schemas";
import { useMatchWork, workQueries } from "@/api/works";
import { Button } from "@/components/Button";
import { FormError } from "@/components/FormError";
import { ModalDialog } from "@/components/ModalDialog";
import { NumberField } from "@/components/NumberField";
import { TextField } from "@/components/TextField";
import { formatRuntime } from "@/i18n/format";

/** `stay` keeps the page where it is after a match, as a list needs. */
type Props = { work: WorkSummary; isOpen: boolean; onClose: () => void; stay?: boolean };

/**
 * Fix match. It is a provider search, prefilled with the parsed title and
 * year, with the matches side by side. Choosing one moves the work's
 * versions to it. If that is another work, the page follows it, unless
 * `stay` is set. For a book this is Match, an Audible search by title and
 * author. Choosing a result makes its ASIN the book's.
 */
export function FixMatchDialog({ work, isOpen, onClose, stay = false }: Props) {
  const title = work.title;
  return (
    <ModalDialog
      isOpen={isOpen}
      onClose={onClose}
      width="page"
      title={work.kind === "book" ? <Trans>Match: {title}</Trans> : <Trans>Fix match: {title}</Trans>}
    >
      <FixMatchSearch work={work} stay={stay} onDone={onClose} />
    </ModalDialog>
  );
}

type Search = { query?: string; year?: number; author?: string };

/** A search by title and year, or for a book by title and author. */
function searchOf(isBook: boolean, query: string, year: number | undefined, author: string): Search {
  const q = query.trim();
  const a = author.trim();
  return {
    query: q || undefined,
    year: isBook || Number.isNaN(year) ? undefined : year,
    author: isBook && a ? a : undefined,
  };
}

function FixMatchSearch({ work, stay, onDone }: { work: WorkSummary; stay: boolean; onDone: () => void }) {
  const { t } = useLingui();
  const navigate = useNavigate();
  const isBook = work.kind === "book";
  // A provisional work's title and year are what its file name parsed to.
  // A book is searched by its title and first author.
  const [query, setQuery] = useState(work.title);
  const [year, setYear] = useState(work.year);
  const [author, setAuthor] = useState(work.book?.authors[0]?.name ?? "");
  const [search, setSearch] = useState(() => searchOf(isBook, query, year, author));
  const matches = useQuery(workQueries.matches(work.id, search));
  const fix = useMatchWork();
  const hint = useId();
  const runtime = work.durationMs ? formatRuntime(work.durationMs) : undefined;

  const choose = (match: Match) =>
    fix.mutate(
      { params: { path: { id: work.id } }, body: { matchId: match.id } },
      {
        onSuccess: (matched) => {
          onDone();
          if (!stay && matched.id !== work.id) void navigate({ to: "/works/$workId", params: { workId: matched.id } });
        },
      },
    );

  return (
    <div className="flex flex-col gap-5">
      <Form
        className="flex flex-col gap-1.5"
        onSubmit={(e) => {
          e.preventDefault();
          setSearch(searchOf(isBook, query, year, author));
        }}
      >
        {/* The hint sits under the row, so Search lines up with the boxes. */}
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-0 flex-1 basis-full sm:basis-0">
            <TextField variant="form" label={t`Title`} value={query} onChange={setQuery} aria-describedby={hint} />
          </div>
          {isBook ? (
            <div className="min-w-0 flex-1">
              <TextField variant="form" label={t`Author`} value={author} onChange={setAuthor} />
            </div>
          ) : (
            <div className="w-28">
              <NumberField
                variant="form"
                label={t`Year`}
                value={year ?? Number.NaN}
                onChange={(y) => setYear(Number.isNaN(y) ? undefined : y)}
                formatOptions={{ useGrouping: false }}
              />
            </div>
          )}
          <Button type="submit" variant="primary" isPending={matches.isFetching}>
            <Trans>Search</Trans>
          </Button>
        </div>
        <p id={hint} className="text-xs text-text-muted">
          {isBook ? (
            <Trans>Or an ASIN, such as B08G9PRS1K.</Trans>
          ) : (
            <Trans>Or a provider ID, such as tmdb-438631.</Trans>
          )}
        </p>
      </Form>
      {isBook && runtime && (
        <p className="text-sm text-text-muted">
          <Trans>Runtime: {runtime}</Trans>
        </p>
      )}
      <FormError error={matches.error} />
      <FormError error={fix.error} />
      {matches.data?.items.length === 0 && (
        <p className="text-sm text-text-muted">
          {isBook ? (
            <Trans>No matches. Try a different title or author.</Trans>
          ) : (
            <Trans>No matches. Try a different title or remove the year.</Trans>
          )}
        </p>
      )}
      {matches.data && matches.data.items.length > 0 && (
        <ul aria-label={t`Matches`} className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {matches.data.items.map((c) => (
            <MatchItem key={c.id} match={c} isPending={fix.isPending} onChoose={() => choose(c)} />
          ))}
        </ul>
      )}
    </div>
  );
}

function MatchItem({ match, isPending, onChoose }: { match: Match; isPending: boolean; onChoose: () => void }) {
  const { t } = useLingui();
  const name = match.year ? `${match.title} (${match.year})` : match.title;
  const score = match.score;
  const authors = match.authors?.join(", ");
  const narrators = match.narrators?.join(", ");
  return (
    <li className="flex flex-col gap-2 rounded-md border border-border bg-surface p-4">
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-semibold">{name}</span>
        {score !== undefined && (
          <span className="shrink-0 text-xs text-text-muted">
            <Trans>Score {score}</Trans>
          </span>
        )}
      </div>
      {match.originalTitle && match.originalTitle !== match.title && (
        <span className="text-sm text-text-muted">{match.originalTitle}</span>
      )}
      {match.subtitle && <span className="text-sm text-text-muted">{match.subtitle}</span>}
      {authors && (
        <span className="text-sm text-text-muted">
          <Trans>By {authors}</Trans>
        </span>
      )}
      {narrators && (
        <span className="text-sm text-text-muted">
          <Trans>Narrated by {narrators}</Trans>
        </span>
      )}
      {match.durationMs !== undefined && (
        <span className="text-sm text-text-muted">{formatRuntime(match.durationMs)}</span>
      )}
      <span className="text-xs tracking-wide text-text-subtle uppercase">{match.provider}</span>
      {match.overview && <p className="line-clamp-4 text-sm text-text-muted">{match.overview}</p>}
      <Button
        size="sm"
        variant="primary"
        className="mt-auto self-start"
        aria-label={t`Choose ${name}`}
        isDisabled={isPending}
        onPress={onChoose}
      >
        <Trans>Choose</Trans>
      </Button>
    </li>
  );
}
