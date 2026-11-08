// The sample audiobooks library, as canned answers. It has books with
// authors, narrators and series, and pages for its people and series.
import type {
  BookSeries,
  BookSeriesRef,
  Chapter,
  HomeRow,
  Library,
  Person,
  PersonRef,
  PlayStarted,
  Version,
  WorkDetail,
  WorkSummary,
} from "../schemas";
import { emptyWork, filePlayTracks, type MockApi, summaryOf } from "./api";
import { mockId } from "./ids";

const minute = 60_000;

/** IDs of the sample books, people and series. */
export const bookIds = {
  audiobooks: mockId(9004),
  hailMary: mockId(500),
  leviathan: mockId(501),
  churn: mockId(502),
  calibansWar: mockId(503),
  weir: mockId(600),
  corey: mockId(601),
  porter: mockId(602),
  mays: mockId(603),
  expanse: mockId(700),
} as const;

/**
 * A book's chapters on its timeline. A book of several files gets one
 * chapter per file, as the server gives them. Otherwise the chapters are
 * spread evenly, and the first is the opening credits.
 */
function bookChapters(files: [string, number][], count: number): Chapter[] {
  if (files.length > 1) {
    let at = 0;
    return files.map(([name, d]) => {
      const c = { startMs: at, endMs: at + d, title: name.replace(/\.[^.]+$/, "") };
      at += d;
      return c;
    });
  }
  const durationMs = files[0]?.[1] ?? 0;
  const each = Math.floor(durationMs / count);
  return Array.from({ length: count }, (_, i) => ({
    startMs: i * each,
    endMs: i === count - 1 ? durationMs : (i + 1) * each,
    title: i === 0 ? "Opening Credits" : `Chapter ${i}`,
  }));
}

/** A book's single version, with its files as parts and its chapters. */
function bookVersion(id: string, workId: string, files: [string, number][], chapterCount: number): Version {
  const durationMs = files.reduce((n, [, d]) => n + d, 0);
  return {
    id,
    name: "Audio",
    available: true,
    libraryId: bookIds.audiobooks,
    container: "mov,mp4,m4a,3gp,3g2,mj2",
    durationMs,
    sizeBytes: Math.round(durationMs * 8),
    chapterCount,
    chapters: bookChapters(files, chapterCount),
    parts: files.map(([name, d], i) => ({
      fileId: `${id.slice(0, -5)}b${i}${id.slice(-3)}`,
      name,
      durationMs: d,
      sizeBytes: d * 8,
    })),
    tracks: [
      {
        id: `${id}-audio`,
        name: "Unknown language · 2.0",
        fullName: "Unknown language · AAC 2.0 · Default",
        type: "audio",
        codec: "aac",
        default: true,
        audio: { channels: 2, layout: "2.0" },
      },
    ],
    workIds: [workId],
  };
}

/** A direct play of a book, with each file as a part and the chapters on the timeline. */
function bookPlay(book: WorkDetail, startMs: number): PlayStarted {
  const v = book.versions[0] as Version;
  const base = `plays/book-${book.id}`;
  return {
    id: `book-${book.id}`,
    workId: book.id,
    versionId: v.id,
    startMs,
    durationMs: v.durationMs,
    local: true,
    delivery: "file",
    url: `${base}/files/0`,
    tracks: filePlayTracks(v),
    parts: v.parts.map((p, i) => ({ durationMs: p.durationMs, url: `${base}/files/${i}` })),
    previews: { ready: false },
    chapters: v.chapters ?? [],
  };
}

const numberOf = (position: string | undefined) => {
  const n = Number.parseFloat(position ?? "");
  return Number.isNaN(n) ? Number.POSITIVE_INFINITY : n;
};

/** Books in series order: by position, with books that have none last, then by title. */
function inSeriesOrder(books: WorkDetail[], seriesId: string): WorkSummary[] {
  const at = (w: WorkDetail) => numberOf(w.book?.series.find((s) => s.id === seriesId)?.position);
  return [...books].sort((a, b) => at(a) - at(b) || a.title.localeCompare(b.title)).map(summaryOf);
}

/** Adds the audiobooks library, its books, its tabs and its people's and series' pages. */
export function addBooks(
  api: MockApi,
  image: () => { id: string; width: number; height: number },
  ago: (days: number) => string,
) {
  const library: Library = {
    id: bookIds.audiobooks,
    name: "Audiobooks",
    kind: "audiobooks",
    pinned: true,
    workCount: 4,
    folders: [{ id: mockId(9104), path: "/mnt/user/media/audiobooks", reachable: true }],
    useLocalMetadata: true,
    previews: "never",
    markers: "never",
    subtitles: "never",
    unmatchedCount: 0,
  };
  api.libraries.push(library);
  const weir = { id: bookIds.weir, name: "Andy Weir" };
  const corey = { id: bookIds.corey, name: "James S. A. Corey" };
  const porter = { id: bookIds.porter, name: "Ray Porter" };
  const mays = { id: bookIds.mays, name: "Jefferson Mays" };
  const expanse = (position: string): BookSeriesRef[] => [
    { id: bookIds.expanse, name: "The Expanse", position, primary: true },
  ];
  const book = (
    id: string,
    title: string,
    extra: Partial<WorkDetail>,
    files: [string, number][],
    chapters: number,
  ): WorkDetail => {
    const w: WorkDetail = {
      ...emptyWork(id, "book", title),
      images: { poster: image() },
      userState: { played: false, favorite: false, playCount: 0 },
      libraryId: library.id,
      ...extra,
    };
    w.versions = [bookVersion(mockId(Number.parseInt(id.slice(-4), 16) + 1000), id, files, chapters)];
    w.durationMs = w.versions[0]?.durationMs ?? 0;
    api.works[id] = w;
    return w;
  };
  const hailMary = book(
    bookIds.hailMary,
    "Project Hail Mary",
    {
      year: 2021,
      addedAt: ago(4),
      overview: "A lone astronaut must save the earth from disaster.",
      genres: ["Science Fiction"],
      externalIds: [{ provider: "audible", kind: "book", id: "B08G9PRS1K" }],
      book: {
        authors: [weir],
        narrators: [porter],
        series: [],
        subtitle: "A Novel",
        publisher: "Audible Studios",
        releaseDate: "2021-05-04",
      },
    },
    [["Project Hail Mary.m4b", 970 * minute]],
    30,
  );
  const leviathan = book(
    bookIds.leviathan,
    "Leviathan Wakes",
    {
      year: 2011,
      addedAt: ago(9),
      book: { authors: [corey], narrators: [mays], series: expanse("1"), releaseDate: "2011" },
    },
    [
      ["01 - Opening Credits.mp3", 2 * minute],
      ["02 - Chapter 1.mp3", 40 * minute],
      ["03 - Chapter 2.mp3", 38 * minute],
    ],
    3,
  );
  const churn = book(
    bookIds.churn,
    "The Churn",
    {
      year: 2014,
      addedAt: ago(9),
      book: { authors: [corey], narrators: [porter], series: expanse("1.5"), releaseDate: "2014" },
    },
    [["The Churn.m4b", 90 * minute]],
    6,
  );
  const caliban = book(
    bookIds.calibansWar,
    "Caliban's War",
    {
      year: 2012,
      addedAt: ago(9),
      book: { authors: [corey], narrators: [mays], series: expanse("2"), releaseDate: "2012" },
    },
    [["Caliban's War.m4b", 1200 * minute]],
    52,
  );
  // Project Hail Mary is half listened to. Leviathan Wakes is finished, so
  // The Churn is next in The Expanse.
  const hailMaryAt = 485 * minute;
  hailMary.userState = { played: false, favorite: false, playCount: 0, positionMs: hailMaryAt };
  leviathan.userState = { played: true, favorite: false, playCount: 1 };
  const books = [caliban, leviathan, hailMary, churn];
  for (const b of books) api.starts[b.id] = bookPlay(b, b === hailMary ? hailMaryAt : 0);
  api.browse[library.id] = books.map(summaryOf);
  const recent: HomeRow = {
    id: `recentlyAdded:${library.id}`,
    kind: "recentlyAdded",
    library: { id: library.id, name: library.name, kind: library.kind },
    items: [hailMary, churn].map(summaryOf),
  };
  // The book rows come after every video row, as the server orders them.
  api.home.rows.push({ id: "continueListening", kind: "continueListening", items: [summaryOf(hailMary)] }, recent);
  api.facets[library.id] = {
    genres: ["Science Fiction"],
    contentRatings: [],
    years: [2011, 2012, 2014, 2021],
    narrators: [mays, porter],
    resolutions: [],
    dynamicRanges: [],
    collections: false,
  };

  const expanseBooks = inSeriesOrder([leviathan, churn, caliban], bookIds.expanse);
  api.authors = [
    { ...weir, role: "author", workCount: 1 },
    { ...corey, role: "author", workCount: 3 },
  ];
  api.bookSeries = [
    { id: bookIds.expanse, name: "The Expanse", bookCount: 3, cover: leviathan.images.poster ?? image() },
  ];
  const expansePage: BookSeries = { id: bookIds.expanse, name: "The Expanse", items: expanseBooks };
  api.seriesPages = { [bookIds.expanse]: expansePage };
  api.bookMatches = [
    {
      id: "audible:book:B08G9PRS1K",
      provider: "audible",
      kind: "book",
      title: "Project Hail Mary",
      subtitle: "A Novel",
      authors: ["Andy Weir"],
      narrators: ["Ray Porter"],
      year: 2021,
      durationMs: 970 * minute,
    },
    {
      id: "audible:book:B0BXP6Z3TH",
      provider: "audible",
      kind: "book",
      title: "Project Hail Mary",
      authors: ["Andy Weir"],
      narrators: ["Full Cast"],
      year: 2023,
      durationMs: 1012 * minute,
    },
  ];
  const person = (ref: PersonRef, page: Partial<Person>): Person => ({
    ...ref,
    works: [],
    bookSeries: [],
    standalone: [],
    narrated: [],
    ...page,
  });
  api.people = {
    [weir.id]: person(weir, { standalone: [summaryOf(hailMary)] }),
    [corey.id]: person(corey, {
      overview: "James S. A. Corey is the pen name of Daniel Abraham and Ty Franck.",
      photo: image(),
      bookSeries: [expansePage],
    }),
    [porter.id]: person(porter, { narrated: [summaryOf(hailMary), summaryOf(churn)] }),
    [mays.id]: person(mays, { narrated: [summaryOf(caliban), summaryOf(leviathan)] }),
  };
}
