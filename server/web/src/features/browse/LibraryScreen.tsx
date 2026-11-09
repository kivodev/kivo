import { Plural, Trans, useLingui } from "@lingui/react/macro";
import { useQueries, useQuery } from "@tanstack/react-query";
import { getRouteApi, Link } from "@tanstack/react-router";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { Button as AriaButton, Checkbox } from "react-aria-components";
import { libraryQueries, useLibrary } from "@/api/libraries";
import type { LetterStart, Library, LibraryFacets, WorkSummary } from "@/api/schemas";
import { useIsAdmin } from "@/api/users";
import { type GridPageQuery, gridPageSize, gridPages, workQueries } from "@/api/works";
import { Artwork, type ArtworkShape, cardDensity } from "@/components/Artwork";
import { Banner } from "@/components/Banner";
import { Button } from "@/components/Button";
import { Chip } from "@/components/Chip";
import { useDocumentTitle } from "@/components/documentTitle";
import { EmptyState } from "@/components/EmptyState";
import { Icon } from "@/components/Icon";
import { LetterRail, letterAt, letterOffset, type RailLetter } from "@/components/LetterRail";
import { LoadError } from "@/components/LoadError";
import { CountSkeleton, GridSkeleton, LibrarySkeleton, NumberSkeleton } from "@/components/Skeleton";
import { SmartListDialog } from "@/features/lists/SmartListDialog";
import { queryFromGrid } from "@/features/lists/smartQuery";
import { formatCommunityRating, formatRuntime } from "@/i18n/format";
import { isTv } from "@/tv/flag";
import { TvFocusable, TvFocusGroup } from "@/tv/focus";
import { BatchEditDialog } from "./BatchEditDialog";
import {
  type BrowseSearch,
  bookSorts,
  clearFilters,
  hasFilters,
  showSorts,
  sorts,
  storeBrowse,
  toFilters,
  useBrowseSearch,
  videoSorts,
} from "./browseSearch";
import { CountNoun, SortMenu, useEmptyTitle } from "./CountLine";
import { FilterPanel } from "./FilterPanel";
import { filterChips } from "./filterChips";
import { AuthorCard, BookSeriesCard, CollectionCard } from "./GroupCard";
import { LibrarySwitcher } from "./LibrarySwitcher";
import { useBrowseLabels } from "./useBrowseLabels";
import { type GridHandle, type GridRange, useRefetchFailed, VirtualGrid } from "./VirtualGrid";
import { WorkCard } from "./WorkCard";

const libraryRoute = getRouteApi("/authed/libraries/$libraryId");

function unreachable(library: Library): boolean {
  return library.scan?.state === "unreachable" || (library.folders ?? []).some((f) => !f.reachable);
}

const noFacets: LibraryFacets = {
  narrators: [],
  genres: [],
  contentRatings: [],
  years: [],
  resolutions: [],
  dynamicRanges: [],
  collections: false,
};

/**
 * The library view, a virtualized grid with sort and filters in the URL.
 * Its title and controls show at once, from the list of libraries, and so
 * does a change of sort or filter. The count and grid show placeholders
 * until the first page arrives.
 *
 * On TV the grid has focus first, six posters across, and Up from its
 * first row reaches the sections, filters and sort. The rail lists the
 * libraries, so the title is not a menu, and there's no list view or
 * multi-select.
 */
export function LibraryScreen() {
  const { t } = useLingui();
  const { libraryId } = libraryRoute.useParams();
  const isAdmin = useIsAdmin();
  const [search, setSearch] = useBrowseSearch();
  // Next time, the library opens as it was left.
  useEffect(() => storeBrowse(libraryId, search), [libraryId, search]);
  const labels = useBrowseLabels();
  const read = useLibrary(libraryId);
  const library = read.data;
  useDocumentTitle(library?.name);
  // The facets say what the filters offer and whether the Collections tab
  // shows. A failed read offers neither.
  const facetsRead = useQuery(libraryQueries.facets(libraryId));
  const facets = facetsRead.data ?? noFacets;
  const facetsSettled = facetsRead.data !== undefined || facetsRead.isError;
  const filters = toFilters(search, isAdmin);
  const tab = search.tab ?? "library";
  // The first page of the section the tab shows.
  const worksRead = useQuery({ ...workQueries.browsePage(libraryId, filters, 0), enabled: tab === "library" });
  const collectionsRead = useQuery({ ...workQueries.collectionsPage(libraryId, 0), enabled: tab === "collections" });
  const authorsRead = useQuery({ ...workQueries.authorsPage(libraryId, 0), enabled: tab === "authors" });
  const seriesRead = useQuery({ ...workQueries.bookSeriesPage(libraryId, 0), enabled: tab === "series" });
  const firstRead = { library: worksRead, collections: collectionsRead, authors: authorsRead, series: seriesRead }[tab];
  const first = firstRead.data;
  // An admin's multi-select of books, for a batch edit. It is undefined
  // when not selecting.
  const [selected, setSelected] = useState<Set<string> | undefined>(undefined);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const chips = filterChips(search, labels, isAdmin, facets.narrators);
  // The header's controls take focus once the grid has first shown, so the
  // screen's initial focus is the grid's first card, not a control above it.
  // A read that failed counts as shown, so the header is still reachable,
  // and each library waits for its own.
  const [shownFor, setShownFor] = useState<string>();
  const settled = (first !== undefined || firstRead.isError) && facetsSettled;
  if (settled && shownFor !== libraryId) setShownFor(libraryId);
  const shown = shownFor === libraryId;

  if (!library) {
    return read.isError ? <LoadError error={read.error} onRetry={() => void read.refetch()} /> : <LibrarySkeleton />;
  }

  const books = library.kind === "audiobooks";
  // The Collections tab and an audiobooks library's Authors and Series tabs
  // list collections, people and series by name, with no sort, filters or
  // list view.
  const listing = tab !== "library";
  const view = listing || isTv ? "grid" : (search.view ?? "grid");
  const sort = search.sort ?? "title";
  const sortOptions = books ? bookSorts : library.kind === "shows" ? showSorts : videoSorts;
  const shape = books ? "square" : library.kind === "videos" ? "landscape" : "poster";
  const set = (patch: Partial<BrowseSearch>) => setSearch({ ...search, ...patch });
  // The count and the grid wait for the first page and the facets, which
  // say whether the sections above show.
  const ready = first !== undefined && facetsSettled;
  // The library's sections, as chips. An audiobooks library has Books,
  // Authors and Series. A movies or shows library that has collections has
  // Collections beside the works. A library with only one section shows
  // no chips.
  const hasCollections =
    tab === "collections" || ((library.kind === "movies" || library.kind === "shows") && facets.collections);
  const sections = books
    ? ([
        ["library", t`Books`],
        ["authors", t`Authors`],
        ["series", t`Series`],
      ] as const)
    : hasCollections
      ? ([
          ["library", library.kind === "shows" ? t`Shows` : t`Movies`],
          ["collections", t`Collections`],
        ] as const)
      : [];
  const selecting = selected !== undefined && books && !listing && !isTv;
  const toggle = (id: string, on: boolean) => {
    const next = new Set(selected);
    if (on) next.add(id);
    else next.delete(id);
    setSelected(next);
  };

  return (
    // On TV the page runs right to the overscan, where the Filter button and
    // the letter rail sit, close to the screen's edge.
    <div data-part="library" className="flex flex-col gap-4 px-4 py-6 sm:px-8 tv:pr-0">
      <TvFocusGroup focusKey="library-header">
        <header data-part="library-header" className="flex flex-col gap-3">
          <div className="flex min-h-10 items-center gap-4">
            <LibrarySwitcher title={library.name} current={library.id} />
            <div className="ml-auto flex shrink-0 items-center gap-2">
              {books && isAdmin && !listing && !isTv && (
                <SelectionBar
                  selected={selected}
                  onStart={() => setSelected(new Set())}
                  onEdit={() => setEditing(true)}
                  onCancel={() => setSelected(undefined)}
                />
              )}
              {!listing && (
                <HeaderFocusable focusKey="filter" enabled={isTv && shown}>
                  <FilterPanel facets={facets} labels={labels} books={books} />
                </HeaderFocusable>
              )}
              {!isTv && (
                <fieldset className={listing ? "hidden" : "flex max-sm:hidden"}>
                  <legend className="sr-only">
                    <Trans>View</Trans>
                  </legend>
                  <Button
                    size="icon"
                    variant={view === "grid" ? "secondary" : "ghost"}
                    aria-label={t`Grid`}
                    aria-pressed={view === "grid"}
                    onPress={() => set({ view: undefined })}
                  >
                    <Icon name="grid" />
                  </Button>
                  <Button
                    size="icon"
                    variant={view === "list" ? "secondary" : "ghost"}
                    aria-label={t`List`}
                    aria-pressed={view === "list"}
                    onPress={() => set({ view: "list" })}
                  >
                    <Icon name="list" />
                  </Button>
                </fieldset>
              )}
            </div>
          </div>
          {sections.length > 0 && (
            <nav data-part="library-sections" className="flex flex-wrap gap-2" aria-label={t`Library sections`}>
              {sections.map(([section, label]) => {
                const active = tab === section;
                return (
                  <HeaderFocusable key={section} focusKey={`section:${section}`} enabled={isTv && shown}>
                    <AriaButton
                      aria-current={active ? "page" : false}
                      data-current={active || undefined}
                      onPress={() => {
                        setSelected(undefined);
                        set({ tab: section === "library" ? undefined : section });
                      }}
                      className="inline-flex h-8 cursor-pointer items-center rounded-full bg-surface-raised px-4 touch-target text-sm font-semibold text-text-muted transition-colors hover:bg-surface-hover hover:text-text data-current:bg-accent data-current:text-on-accent"
                    >
                      {label}
                    </AriaButton>
                  </HeaderFocusable>
                );
              })}
            </nav>
          )}
          {unreachable(library) && (
            <Banner tone="warning">
              <Trans>{library.name} can't be reached. Nothing has been removed.</Trans>
            </Banner>
          )}
          {chips.length > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              {chips.map((chip) => (
                <HeaderFocusable key={chip.key} focusKey={`chip:${chip.key}`} enabled={isTv && shown}>
                  <Chip label={chip.label} onRemove={() => setSearch(chip.without)} />
                </HeaderFocusable>
              ))}
              <HeaderFocusable focusKey="clear-all" enabled={isTv && shown}>
                <Button size="sm" variant="ghost" onPress={() => setSearch(clearFilters(search))}>
                  <Trans>Clear all</Trans>
                </Button>
              </HeaderFocusable>
              {!isTv && tab === "library" && chips.some((chip) => chip.key !== "matchState") && (
                <Button size="sm" variant="ghost" onPress={() => setSaving(true)}>
                  <Trans>Save as Smart List</Trans>
                </Button>
              )}
            </div>
          )}
          {ready || !listing ? (
            // On TV the count and the sort are one focusable. Up from the grid
            // goes to the item nearest its column, and the sort alone, set to
            // the right of the count, would lose to the tabs above it.
            <HeaderFocusable focusKey="sort" enabled={isTv && shown && !listing} face="inside" className="w-fit">
              <div data-part="library-count" className="flex items-center gap-1.5 text-sm text-text-muted">
                {ready ? (
                  <span>
                    <CountNoun count={first.total} kind={library.kind} tab={search.tab} />
                  </span>
                ) : (
                  <NumberSkeleton />
                )}
                {!listing && (
                  <>
                    <span aria-hidden="true">·</span>
                    <SortMenu
                      options={sortOptions}
                      selected={sort}
                      labels={labels}
                      onChange={(s) => set({ sort: sorts.find((x) => x === s) ?? "title" })}
                    />
                  </>
                )}
              </div>
            </HeaderFocusable>
          ) : (
            <CountSkeleton />
          )}
        </header>
      </TvFocusGroup>
      {firstRead.isError ? (
        <LoadError error={firstRead.error} onRetry={() => void firstRead.refetch()} />
      ) : ready && tab === "library" && worksRead.data ? (
        <LibraryGrid
          library={library}
          first={worksRead.data}
          page={(i) => workQueries.browsePage(library.id, filters, i)}
          view={view}
          shape={shape}
          letterSort={sort === "title"}
          oneFilter={chips.length === 1}
          renderCard={(work, width) =>
            selecting ? (
              <SelectableCard
                work={work}
                width={width}
                isSelected={selected?.has(work.id) ?? false}
                onChange={(on) => toggle(work.id, on)}
              />
            ) : (
              <WorkCard work={work} shape={shape} width={width} inLibrary />
            )
          }
          renderListRow={(work) => <ListRow work={work} />}
        />
      ) : ready && tab === "collections" && collectionsRead.data ? (
        <LibraryGrid
          library={library}
          first={collectionsRead.data}
          page={(i) => workQueries.collectionsPage(library.id, i)}
          view="grid"
          shape={shape}
          letterSort
          oneFilter={false}
          renderCard={(c, width) => <CollectionCard collection={c} shape={shape} width={width} />}
        />
      ) : ready && tab === "authors" && authorsRead.data ? (
        <LibraryGrid
          library={library}
          first={authorsRead.data}
          page={(i) => workQueries.authorsPage(library.id, i)}
          view="grid"
          shape={shape}
          letterSort
          oneFilter={false}
          renderCard={(a, width) => <AuthorCard author={a} width={width} />}
        />
      ) : ready && tab === "series" && seriesRead.data ? (
        <LibraryGrid
          library={library}
          first={seriesRead.data}
          page={(i) => workQueries.bookSeriesPage(library.id, i)}
          view="grid"
          shape={shape}
          letterSort
          oneFilter={false}
          renderCard={(s, width) => <BookSeriesCard series={s} width={width} />}
        />
      ) : (
        <GridSkeleton rail shape={shape} />
      )}
      <BatchEditDialog
        workIds={[...(selected ?? [])]}
        isOpen={editing}
        onClose={() => setEditing(false)}
        onDone={() => {
          setEditing(false);
          setSelected(undefined);
        }}
      />
      {saving && (
        <SmartListDialog name={library.name} query={queryFromGrid(library, search)} onClose={() => setSaving(false)} />
      )}
    </div>
  );
}

/** A page of one of a library's grids, whatever it lists. */
type GridPage = { items: { id: string }[]; total: number; letters?: LetterStart[] };

type GridProps<P extends GridPage> = {
  library: Library;
  /** The result's first page, which holds its total and letters. */
  first: P;
  /** Page `index` of the result. */
  page: (index: number) => GridPageQuery<P>;
  view: "grid" | "list";
  shape: ArtworkShape;
  /** Whether the result is in letter order, which the letter rail jumps through. */
  letterSort: boolean;
  /** Whether a single filter narrows the result, for the empty state's title. */
  oneFilter: boolean;
  renderCard: (item: P["items"][number], width: number) => ReactNode;
  /** The list view's row. A grid without one has no list view. */
  renderListRow?: (item: P["items"][number]) => ReactNode;
};

/**
 * The library's grid and letter rail, from the result's first page. The
 * pages around where the grid settles load as it needs them.
 */
function LibraryGrid<P extends GridPage>({
  library,
  first,
  page,
  view,
  shape,
  letterSort,
  oneFilter,
  renderCard,
  renderListRow = () => null,
}: GridProps<P>) {
  const { t } = useLingui();
  const [search, setSearch] = useBrowseSearch();
  const [range, setRange] = useState<GridRange>({ start: 0, end: 0, top: 0, topEnd: 0, settles: 0 });
  const total = first.total;
  // The pages of the items around where the grid settles.
  const indexes = gridPages(range.start, range.end, total);
  const rest = useQueries({ queries: indexes.map(page) });
  useRefetchFailed(rest, range.settles);
  const pages = new Map([[0, first], ...indexes.map((i, k) => [i, rest[k]?.data] as const)]);
  // Every item of the result. An item is undefined until its page loads.
  const items: (P["items"][number] | undefined)[] = Array.from(
    { length: total },
    (_, i) => pages.get(Math.floor(i / gridPageSize))?.items[i % gridPageSize],
  );
  const grid = useRef<GridHandle>(null);
  const emptyTitle = useEmptyTitle(library.kind, search.tab, search, oneFilter);

  // The rail scrolls the grid to where a letter starts, and # is the top.
  // It marks the letter of the top row's first item, or the letter it
  // scrolled to while that letter's start is in the top row. The title
  // sort has the rail, and so do the Authors and Series tabs, which list by
  // name.
  const letters = letterSort ? first.letters : undefined;
  const [jumpedTo, setJumpedTo] = useState(-1);
  // The grid holds off loading while the rail is held. It stops holding
  // off once the rail is gone, as on a sort change mid-scrub.
  const [scrubbing, setScrubbing] = useState(false);
  const jump = (letter: RailLetter) => {
    const offset = letter === "#" ? -1 : letterOffset(letters ?? [], letter, total);
    setJumpedTo(offset);
    if (offset < 0) window.scrollTo({ top: 0, behavior: "instant" });
    else grid.current?.scrollToIndex(offset);
  };
  const shown = jumpedTo >= range.top && jumpedTo < range.topEnd ? jumpedTo : range.top;

  if (total === 0) {
    return hasFilters(search) ? (
      <EmptyState
        title={emptyTitle}
        action={
          <TvFocusable focusKey="clear-filters" autoFocus>
            <Button variant="primary" onPress={() => setSearch(clearFilters(search))}>
              <Trans>Clear filters</Trans>
            </Button>
          </TvFocusable>
        }
      />
    ) : (
      <EmptyState title={t`${library.name} is empty.`}>
        <Trans>Items appear here as the library's folders are scanned.</Trans>
      </EmptyState>
    );
  }
  return (
    // On TV the grid keeps its width without the letter rail.
    <div className={letters ? "flex gap-2" : "flex gap-2 tv:pr-14"}>
      <div className="min-w-0 flex-1">
        <VirtualGrid
          ref={grid}
          items={items}
          shape={shape}
          view={view}
          onRangeChange={setRange}
          scrubbing={scrubbing && letters !== undefined}
          renderCard={renderCard}
          renderListRow={renderListRow}
        />
      </div>
      {letters && (
        <LetterRail letters={letters} onJump={jump} onScrub={setScrubbing} current={letterAt(letters, shown)} />
      )}
    </div>
  );
}

/** A header control that takes focus on TV once `enabled`, and is the control alone elsewhere. */
function HeaderFocusable({
  focusKey,
  enabled,
  face,
  className,
  children,
}: {
  focusKey: string;
  enabled: boolean;
  /** Which element inside lights up, when it isn't the first (TvFocusable). */
  face?: "inside";
  className?: string;
  children: ReactNode;
}) {
  if (!enabled) return children;
  return (
    <TvFocusable focusKey={focusKey} face={face} className={className}>
      {children}
    </TvFocusable>
  );
}

/** Select, then Edit for the books chosen, or Cancel. */
function SelectionBar({
  selected,
  onStart,
  onEdit,
  onCancel,
}: {
  selected: Set<string> | undefined;
  onStart: () => void;
  onEdit: () => void;
  onCancel: () => void;
}) {
  if (selected === undefined) {
    return (
      <Button size="sm" onPress={onStart}>
        <Trans>Select</Trans>
      </Button>
    );
  }
  const count = selected.size;
  return (
    <>
      <Button size="sm" variant="primary" isDisabled={count === 0} onPress={onEdit}>
        <Plural value={count} one="Edit # book" other="Edit # books" />
      </Button>
      <Button size="sm" variant="ghost" onPress={onCancel}>
        <Trans>Cancel</Trans>
      </Button>
    </>
  );
}

/** A book's card while selecting. Its cover and title toggle the selection. */
function SelectableCard({
  work,
  width,
  isSelected,
  onChange,
}: {
  work: WorkSummary;
  width: number;
  isSelected: boolean;
  onChange: (on: boolean) => void;
}) {
  return (
    <Checkbox
      isSelected={isSelected}
      onChange={onChange}
      data-part="card"
      data-kind={work.kind}
      className="group relative flex min-w-0 cursor-pointer flex-col gap-2 rounded-md"
    >
      <div className="relative">
        <Artwork
          image={work.images.poster}
          title={work.title}
          shape="square"
          width={width}
          density={cardDensity}
          part="cover"
          className="shadow-card group-data-selected:outline-4 group-data-selected:outline-accent"
        />
        <span className="absolute top-2 left-2 flex size-6 items-center justify-center rounded-full border-2 border-on-badge bg-scrim text-on-accent group-data-selected:border-accent group-data-selected:bg-accent">
          <Icon name="check" className="size-4 opacity-0 group-data-selected:opacity-100" />
        </span>
      </div>
      <div className="min-w-0 px-0.5">
        <div className="truncate text-sm font-semibold text-text">{work.title}</div>
        <div className="truncate text-sm text-text-muted">{work.book?.authors[0]?.name}</div>
      </div>
    </Checkbox>
  );
}

/**
 * The list view's row. Its columns are year, runtime, content rating,
 * rating, added date and played state. On a narrow screen it shows only
 * the title, year and played state.
 */
function ListRow({ work }: { work: WorkSummary }) {
  const { i18n } = useLingui();
  return (
    <Link
      to="/works/$workId"
      params={{ workId: work.id }}
      data-part="list-row"
      className="grid h-(--layout-list-row-height) grid-cols-5 items-center gap-4 border-b border-border px-2 text-sm hover:bg-surface-hover md:grid-cols-8"
    >
      <span className="col-span-3 truncate font-semibold md:col-span-2">{work.title}</span>
      <span className="text-text-muted">{work.year}</span>
      <span className="text-text-muted max-md:hidden">{work.durationMs ? formatRuntime(work.durationMs) : ""}</span>
      <span className="text-text-muted max-md:hidden">{work.contentRating}</span>
      <span className="text-text-muted max-md:hidden">
        {work.communityRating !== undefined ? `★ ${formatCommunityRating(work.communityRating)}` : ""}
      </span>
      <span className="truncate text-text-muted max-md:hidden">
        {work.addedAt ? i18n.date(work.addedAt, { dateStyle: "medium" }) : ""}
      </span>
      <span className="text-right text-text-muted">
        {work.userState?.played ? <Icon name="check" className="ml-auto size-4 text-accent" /> : null}
      </span>
    </Link>
  );
}
