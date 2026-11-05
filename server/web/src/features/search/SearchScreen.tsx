import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Input, SearchField } from "react-aria-components";
import { libraryQueries } from "@/api/libraries";
import { errorCode } from "@/api/problem";
import type { CollectionSummary, PersonSummary, WorkSummary } from "@/api/schemas";
import { searchQueries } from "@/api/search";
import { onSignedOut } from "@/api/session";
import { useIsAdmin } from "@/api/users";
import type { ArtworkShape } from "@/components/Artwork";
import { Button } from "@/components/Button";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { Icon } from "@/components/Icon";
import { PersonCard, useRoleLabel } from "@/components/PersonCard";
import { Row } from "@/components/Row";
import { CollectionCard } from "@/features/browse/GroupCard";
import { WorkCard } from "@/features/browse/WorkCard";
import { describeProblem } from "@/i18n/problems";
import { isTv } from "@/tv/flag";
import { TvFocusGroup } from "@/tv/focus";
import { TvControl } from "@/tv/forms/TvControl";
import { GridKeyboard } from "@/tv/keyboard/GridKeyboard";
import { focusTv } from "@/tv/spatial";

// On TV the results sit beside the keyboard, in two thirds of the screen, so
// a row fits four posters, each the size it is on Home, rather than six.
const searchAcross = 4;

const debounceMs = 350;

type Group = { kind: string; title: string; shape: ArtworkShape };

const searchRoute = getRouteApi("/authed/search");

// The last search typed on TV, which Search opens on while the app runs.
// It lives in the page, so a restart starts clean, and signing
// out forgets it so the next person on a shared TV doesn't see it.
let lastSearch = "";

/** Forgets the last search. */
export function forgetLastSearch() {
  lastSearch = "";
}
onSignedOut(forgetLastSearch);

/**
 * The query in the URL, and a change to it, which replaces it in history.
 * On TV, Search opened without one takes the last search typed.
 */
function useSearchQuery() {
  const inUrl = searchRoute.useSearch().q;
  const q = inUrl ?? (isTv ? lastSearch : "");
  const navigate = searchRoute.useNavigate();
  const setQuery = useCallback(
    (next: string) => {
      if (isTv) lastSearch = next;
      void navigate({ search: { q: next || undefined }, replace: true });
    },
    [navigate],
  );
  useEffect(() => {
    if (isTv && inUrl === undefined && lastSearch !== "") void navigate({ search: { q: lastSearch }, replace: true });
  }, [inUrl, navigate]);
  return [q, setQuery] as const;
}

/**
 * The search page. It has one box, then the people who match in a row,
 * then results in a row per kind, then the collections. While the next
 * query loads, the last results stay, dimmed, and the new ones replace
 * them at once.
 */
export function SearchScreen() {
  const { t } = useLingui();
  const [q, setQuery] = useSearchQuery();
  const isAdmin = useIsAdmin();
  useDocumentTitle(q ? t`Search: ${q}` : t`Search`);
  const [text, setText] = useState(q);
  const query = q.trim();
  const results = useQuery({ ...searchQueries.results(query), enabled: query !== "", placeholderData: (p) => p });
  const libraries = useQuery({ ...libraryQueries.list(), enabled: isAdmin });

  useEffect(() => {
    if (text === q) return;
    const timer = setTimeout(() => setQuery(text), debounceMs);
    return () => clearTimeout(timer);
  }, [text, q, setQuery]);

  const groups: Group[] = [
    { kind: "movie", title: t`Movies`, shape: "poster" },
    { kind: "series", title: t`Shows`, shape: "poster" },
    { kind: "episode", title: t`Episodes`, shape: "landscape" },
    { kind: "video", title: t`Other videos`, shape: "landscape" },
    { kind: "book", title: t`Books`, shape: "square" },
  ];
  const works = query === "" ? [] : (results.data?.works ?? []);
  const people = query === "" ? [] : (results.data?.people ?? []);
  const collections = query === "" ? [] : (results.data?.collections ?? []);
  const loading = query !== "" && results.isPlaceholderData;
  const byKind = (kind: string): WorkSummary[] => works.filter((w) => w.kind === kind);
  const unmatchedLibrary = libraries.data?.items.find((l) => (l.unmatchedCount ?? 0) > 0);

  const field = (
    <SearchField
      value={text}
      onChange={setText}
      onClear={() => setQuery("")}
      aria-label={t`Search`}
      // The TV's keyboard opens when the field takes focus, so it waits
      // for OK on it.
      autoFocus={!isTv}
      className="flex max-w-prose items-center gap-2 rounded-md border border-border bg-surface px-3 tv:max-w-none"
    >
      <Icon name="search" className="size-5 text-text-muted" />
      <Input
        placeholder={t`Search titles and people`}
        className="h-12 min-w-0 flex-1 bg-transparent text-lg text-text outline-none placeholder:text-text-subtle [&::-webkit-search-cancel-button]:hidden"
      />
    </SearchField>
  );
  const hasResults = works.length + people.length + collections.length > 0;
  const available = useRef(hasResults);
  useLayoutEffect(() => {
    available.current = hasResults;
  }, [hasResults]);
  const toResults = useCallback(() => {
    if (!available.current) return true;
    focusTv("search:results");
    return false;
  }, []);
  const type = useCallback((c: string) => setText((current) => current + c), []);
  const backspace = useCallback(() => setText((current) => current.slice(0, -1)), []);
  const clear = useCallback(() => {
    setText("");
    setQuery("");
  }, [setQuery]);

  return (
    <div
      data-part="search"
      className="flex flex-col gap-8 px-4 py-6 sm:px-8 tv:fixed tv:inset-y-0 tv:left-(--layout-tv-content-left) tv:right-(--layout-tv-overscan-x) tv:flex-row tv:items-stretch tv:gap-12 tv:py-(--layout-tv-overscan-y)"
    >
      {isTv ? (
        <div data-part="search-keyboard" className="flex w-1/3 shrink-0 flex-col gap-6">
          <TvControl focusKey="search:field" kind="field">
            {field}
          </TvControl>
          <GridKeyboard
            name="search"
            autoFocus
            onType={type}
            onBackspace={backspace}
            onClear={clear}
            onRight={toResults}
          />
        </div>
      ) : (
        field
      )}
      {/* Up and Down stay among the results. The keyboard beside them is
          nearer than the next row down can be, and would otherwise take
          focus back; Left is the way to it. */}
      <TvFocusGroup
        focusKey="search:results"
        part="search-results"
        column
        scroller
        className="min-w-0 flex-1 tv:-mx-4 tv:overflow-y-auto tv:px-4"
      >
        <div
          aria-busy={loading || undefined}
          className={`flex flex-col gap-8 transition-opacity ${loading ? "opacity-60" : ""}`}
        >
          {/* With nothing typed the keyboard has the screen to itself, so the results' two thirds say what to do. The app keeps no list of recent searches, only the last. */}
          {isTv && query === "" && (
            <div data-part="search-prompt" className="flex flex-col items-center gap-4 pt-24 text-center">
              <Icon name="search" className="size-16 text-text-subtle" />
              <p className="text-xl font-semibold text-text-muted">
                <Trans>Search your libraries by title, person or collection</Trans>
              </p>
            </div>
          )}
          {query !== "" && results.isError && (
            <EmptyState tone="error" title={t`Search failed.`}>
              {describeProblem(errorCode(results.error))}
            </EmptyState>
          )}
          {query !== "" &&
            results.isSuccess &&
            works.length === 0 &&
            people.length === 0 &&
            collections.length === 0 && (
              <EmptyState
                title={t`No results for “${query}”.`}
                action={
                  <div className="flex flex-col items-center gap-3">
                    <TvControl focusKey="search:clear">
                      <Button
                        variant="primary"
                        onPress={() => {
                          setText("");
                          setQuery("");
                        }}
                      >
                        <Trans>Clear search</Trans>
                      </Button>
                    </TvControl>
                    {/* The Match filter is an admin's, and admin is web-only. */}
                    {isAdmin && !isTv && unmatchedLibrary && (
                      <Link
                        to="/libraries/$libraryId"
                        params={{ libraryId: unmatchedLibrary.id }}
                        search={{ matchState: "unmatched" }}
                        className="text-sm text-text-muted hover:text-text"
                      >
                        <Trans>View unmatched items</Trans>
                      </Link>
                    )}
                  </div>
                }
              />
            )}
          {people.length > 0 && <PeopleRow people={people} />}
          {groups.map((group) => {
            const found = byKind(group.kind);
            if (found.length === 0) return null;
            return (
              <div key={group.kind} data-part="search-group" data-kind={group.kind}>
                <Row
                  title={group.title}
                  focusKey={`search:${group.kind}`}
                  tvAcross={searchAcross}
                  shape={group.shape}
                  items={found}
                  itemKey={(w) => w.id}
                  renderItem={(w) => <WorkCard work={w} shape={group.shape} />}
                />
              </div>
            );
          })}
          {collections.length > 0 && <CollectionsRow collections={collections} />}
        </div>
      </TvFocusGroup>
    </div>
  );
}

/** The collections that match, each with its poster and its count of root works. */
function CollectionsRow({ collections }: { collections: readonly CollectionSummary[] }) {
  const { t } = useLingui();
  return (
    <div data-part="search-group" data-kind="collection">
      <Row
        title={t`Collections`}
        focusKey="search:collection"
        tvAcross={searchAcross}
        shape="poster"
        items={collections}
        itemKey={(c) => c.id}
        renderItem={(c) => <CollectionCard collection={c} shape="poster" />}
      />
    </div>
  );
}

/** The people who match, each with their most frequent role and their count of root works. */
function PeopleRow({ people }: { people: readonly PersonSummary[] }) {
  const { t } = useLingui();
  const roleLabel = useRoleLabel();
  return (
    <div data-part="search-group" data-kind="person">
      <Row
        title={t`People`}
        focusKey="search:person"
        tvAcross={searchAcross}
        shape="poster"
        items={people}
        itemKey={(p) => p.id}
        renderItem={(p) => (
          <PersonCard
            personId={p.id}
            name={p.name}
            image={p.photo}
            detail={
              <>
                {roleLabel(p.role)} · <Plural value={p.workCount} one="# title" other="# titles" />
              </>
            }
          />
        )}
      />
    </div>
  );
}
