// Canned server answers for tests and `npm run dev:mock`. Every value is
// typed by the generated schema. Nothing here decides anything the server
// decides. A test that needs another answer sets it on the MockApi it was
// given.
import type {
  ActivityEvent,
  Artwork,
  CollectionSummary,
  CurrentUser,
  Library,
  LibraryScanStatus,
  LogLine,
  MediaFile,
  PlayStarted,
  ServerProblem,
  SubtitleTrackInfo,
  TaskRun,
  Track,
  Version,
  WorkDetail,
} from "../schemas";
import { emptyWork, filePlayTracks, type MockApi, summaryOf } from "./api";

import { addBooks, bookIds } from "./books";
import { mockId } from "./ids";

export { mockId };

/** IDs of the sample works and libraries, for tests and links. */
export const mockIds = {
  movies: mockId(9001),
  shows: mockId(9002),
  videos: mockId(9003),
  dune: mockId(100),
  dunePartTwo: mockId(101),
  bladeRunner: mockId(102),
  bladeRunnerFinalCut: mockId(103),
  duneCollection: mockId(104),
  myList: mockId(9501),
  dateNight: mockId(9502),
  missingReel: mockId(105),
  holiday: mockId(106),
  nightShift: mockId(107),
  expanse: mockId(200),
  expanseSeason3: mockId(213),
  expanseReload: mockId(1304),
  severance: mockId(300),
  severanceSeason1: mockId(301),
  severancePilot: mockId(3101),
  vacation: mockId(400),
  villeneuve: mockId(500),
} as const;

const hour = 60 * 60 * 1000;
const day = 24 * hour;
const minute = 60_000;
const blurhash = "LEHV6nWB2yk8pyo0adR*.7kCMdnj";

const subtitle = (source: SubtitleTrackInfo["source"]): SubtitleTrackInfo => ({ source });

const tracks4k: Track[] = [
  {
    id: "4k-video",
    name: "HEVC 4K HDR10",
    fullName: "HEVC Main 10 4K HDR10",
    type: "video",
    codec: "hevc",
    default: false,
    video: { width: 3840, height: 2160, resolution: "4k", profile: "Main 10", hdr: "hdr10" },
  },
  {
    id: "4k-audio-en",
    name: "English · 7.1 · TrueHD",
    fullName: "English · TrueHD 7.1 · Default",
    type: "audio",
    codec: "truehd",
    language: "en",
    default: true,
    audio: { channels: 8, layout: "7.1" },
  },
  {
    id: "4k-audio-fr",
    name: "French · 5.1",
    fullName: "French · AC-3 5.1",
    type: "audio",
    codec: "ac3",
    language: "fr",
    default: false,
    audio: { channels: 6, layout: "5.1" },
  },
  {
    id: "4k-sub-en",
    name: "English",
    fullName: "English · SRT · External",
    type: "subtitle",
    codec: "subrip",
    language: "en",
    default: false,
    subtitle: subtitle("external"),
  },
  {
    id: "4k-sub-en-sdh",
    name: "English · SDH",
    fullName: "English · SDH · PGS · Embedded",
    type: "subtitle",
    codec: "hdmv_pgs_subtitle",
    language: "en",
    default: false,
    hearingImpaired: true,
    subtitle: subtitle("embedded"),
  },
  {
    id: "4k-sub-fr-forced",
    name: "French · Forced",
    fullName: "French · Forced · ASS · Embedded",
    type: "subtitle",
    codec: "ass",
    language: "fr",
    default: false,
    forced: true,
    subtitle: subtitle("embedded"),
  },
];

const tracks1080: Track[] = [
  {
    id: "1080-video",
    name: "H.264 1080p",
    fullName: "H.264 High 1080p",
    type: "video",
    codec: "h264",
    default: false,
    video: { width: 1920, height: 1080, resolution: "1080p", profile: "High" },
  },
  {
    id: "1080-audio-en",
    name: "English · 2.0",
    fullName: "English · AAC 2.0 · Default",
    type: "audio",
    codec: "aac",
    language: "en",
    default: true,
    audio: { channels: 2, layout: "2.0" },
  },
];

function version(
  id: string,
  workId: string,
  libraryId: string,
  /** The file's name without its extension, which is also the version's. */
  name: string,
  durationMs: number,
  tracks: Track[],
  sizeBytes: number,
): Version {
  // A play starts with the default audio and no subtitles, as for a user with no preferences.
  const audio = tracks.find((t) => t.type === "audio" && t.default);
  const video = tracks.find((t) => t.type === "video")?.video;
  return {
    id,
    name,
    resolution: video?.resolution,
    hdr: video?.hdr,
    available: true,
    libraryId,
    container: "mkv",
    durationMs,
    sizeBytes,
    parts: [{ fileId: `${id.slice(0, -5)}f${id.slice(-4)}`, name: `${name}.mkv`, durationMs, sizeBytes }],
    tracks: structuredClone(tracks),
    audioTrackId: audio?.id,
    workIds: [workId],
  };
}

/** A version's files, as Show files lists them. */
function filesOf(work: WorkDetail): MediaFile[] {
  return work.versions.flatMap((v) =>
    v.parts.map((p, i) => ({
      id: p.fileId,
      libraryId: v.libraryId ?? "",
      path: `/mnt/user/Movies/${work.title} (${work.year})/${work.title} - ${p.name}`,
      sizeBytes: p.sizeBytes,
      modifiedAt: "2024-03-01T12:00:00Z",
      status: "present",
      versionId: v.id,
      part: i,
      container: "mkv",
      durationMs: p.durationMs,
      bitrateBps: Math.round((p.sizeBytes * 8) / (p.durationMs / 1000)),
      tracks: v.tracks,
    })),
  );
}

/** A play of a version's file directly, from startMs. */
function directPlay(work: WorkDetail, v: Version, startMs: number): PlayStarted {
  const video = v.tracks.find((t) => t.type === "video");
  const audio = v.tracks.find((t) => t.type === "audio");
  const base = "plays/play";
  const third = Math.round(v.durationMs / 3);
  return {
    id: "play",
    workId: work.id,
    versionId: v.id,
    startMs,
    durationMs: v.durationMs,
    local: true,
    delivery: "file",
    url: `${base}/files/0`,
    tracks: filePlayTracks(v),
    parts: [{ durationMs: v.durationMs, url: `${base}/files/0` }],
    // For a video whose audio isn't AAC, the boost costs converting the
    // audio to AAC.
    dialogueBoost:
      video && audio
        ? { enabled: false, cost: audio.codec !== "aac" ? ("audioAsAac" as const) : undefined }
        : undefined,
    previews: { ready: false },
    chapters:
      work.kind === "episode"
        ? [
            { startMs: 0, endMs: 60_000, title: "Cold open" },
            { startMs: 60_000, endMs: 150_000, title: "Opening credits" },
            { startMs: 150_000, endMs: v.durationMs - 90_000, title: "Episode" },
            { startMs: v.durationMs - 90_000, endMs: v.durationMs, title: "End credits" },
          ]
        : [1, 2, 3].map((n) => ({ startMs: (n - 1) * third, endMs: n * third, title: `Chapter ${n}` })),
    markers:
      work.kind === "episode"
        ? [
            { kind: "intro", startMs: 60_000, endMs: 150_000, source: "chapter" },
            { kind: "credits", startMs: v.durationMs - 90_000, endMs: v.durationMs, source: "chapter" },
          ]
        : [],
  };
}

const genres = ["Drama", "Comedy", "Science Fiction", "Thriller", "Documentary", "Animation", "Adventure"];
const ratings = ["G", "PG", "PG-13", "R"];
const adjectives =
  "Silent Crimson Hidden Last Broken Golden Northern Electric Distant Quiet Wild Paper Iron Glass Velvet Hollow".split(
    " ",
  );
const nouns =
  "Harbor Garden Signal Empire River Machine Orchard Frontier Lantern Kingdom Tide Archive Summer Mirror Atlas Station".split(
    " ",
  );

/**
 * The Activity page's events at `now`, newest first. Today has plays,
 * root works added, sign-ins and the night's tasks. Yesterday has a user
 * joining, a problem and a play. There are also events from two days
 * before.
 */
function sampleActivity(now: number): ActivityEvent[] {
  const midnight = new Date(now).setHours(0, 0, 0, 0);
  // Today, `m` minutes ago, but never before midnight.
  const today = (m: number) => new Date(Math.max(midnight + (600 - m) * 1000, now - m * minute)).toISOString();
  const before = (days: number, h: number) => new Date(midnight - days * day + h * hour).toISOString();
  const ben = { userId: mockId(7101), username: "ben" };
  const anna = { userId: mockId(7001), username: "anna" };
  const events: Omit<ActivityEvent, "id">[] = [
    {
      occurredAt: today(2),
      kind: "playStarted",
      play: {
        ...ben,
        work: { workId: mockIds.dunePartTwo, kind: "movie", title: "Dune: Part Two" },
        clientName: "Firefox on Windows",
        local: false,
      },
    },
    {
      occurredAt: today(4),
      kind: "playStarted",
      play: {
        ...anna,
        work: {
          workId: mockIds.expanseReload,
          kind: "episode",
          title: "The Expanse",
          seasonNumber: 3,
          episodeNumber: 4,
        },
        clientName: "Living Room TV",
        local: true,
      },
    },
    {
      occurredAt: today(40),
      kind: "added",
      added: {
        libraryName: "TV Shows",
        work: {
          workId: mockIds.expanseReload,
          kind: "episode",
          title: "The Expanse",
          seasonNumber: 3,
          episodeNumber: 4,
        },
        count: 3,
      },
    },
    {
      occurredAt: today(70),
      kind: "playFinished",
      play: {
        userId: mockId(7102),
        username: "carla",
        work: { workId: bookIds.leviathan, kind: "book", title: "Leviathan Wakes" },
        clientName: "Chrome on Android",
        local: true,
      },
    },
    {
      occurredAt: today(165),
      kind: "signedIn",
      signIn: { ...ben, clientName: "ben's iPhone", address: "203.0.113.24", local: false },
    },
    {
      occurredAt: today(172),
      kind: "signInFailed",
      signIn: { ...ben, address: "203.0.113.24", local: false, count: 3 },
    },
    {
      occurredAt: today(200),
      kind: "added",
      added: {
        libraryName: "Movies",
        work: { workId: mockIds.bladeRunner, kind: "movie", title: "Blade Runner" },
        count: 1,
      },
    },
    {
      occurredAt: today(500),
      kind: "taskEnded",
      task: { task: "make-previews", summary: "214 done", failedCount: 2 },
    },
    {
      occurredAt: today(540),
      kind: "taskEnded",
      task: { task: "make-backup", summary: "Backed up to kivo-backup.db", failedCount: 0 },
    },
    { occurredAt: before(1, 21), kind: "userJoined", joined: { userId: mockId(7103), username: "dana" } },
    {
      occurredAt: before(1, 16),
      kind: "problemRaised",
      problem: {
        id: "playback.gpuFailed",
        code: "gpuFailed",
        message: "Quick Sync stopped working",
        severity: "error",
      },
    },
    {
      occurredAt: before(1, 11),
      kind: "playFinished",
      play: {
        ...anna,
        work: {
          workId: mockIds.expanseReload,
          kind: "episode",
          title: "The Expanse",
          seasonNumber: 3,
          episodeNumber: 3,
        },
        clientName: "Living Room TV",
        local: true,
      },
    },
    {
      occurredAt: before(2, 20),
      kind: "userJoined",
      joined: { userId: mockId(7102), username: "carla", addedBy: "anna" },
    },
    {
      occurredAt: before(2, 3),
      kind: "taskEnded",
      task: { task: "make-backup", error: "The backup folder is full.", failedCount: 0 },
    },
  ];
  return events.map((e, i) => ({ id: mockId(8000 + i), ...e }));
}

/** A fresh copy of the sample server: Movies, TV Shows and Home Videos, with an admin signed in. */
export function createMockApi(): MockApi {
  const now = Date.now();
  const ago = (days: number) => new Date(now - days * day).toISOString();
  let n = 100_000;
  const next = () => mockId(n++);
  const image = (withBlur = true) => ({
    id: next(),
    blurhash: withBlur ? blurhash : undefined,
    width: 780,
    height: 1170,
  });
  const works: Record<string, WorkDetail> = {};
  const add = (w: WorkDetail) => {
    works[w.id] = w;
    return w;
  };
  const summaries = (ids: string[]) => ids.map((id) => summaryOf(works[id] as WorkDetail));
  const idle = (libraryId: string): LibraryScanStatus => ({ libraryId, state: "idle" });
  const folder = (path: string, scanned?: string) => ({
    id: next(),
    path,
    reachable: true,
    lastScanAt: scanned,
  });

  // Movies.
  const movie = (title: string, year: number, extra: Partial<WorkDetail> = {}): WorkDetail => {
    const id = extra.id ?? next();
    const durationMs = (90 + (year % 60)) * minute;
    return add({
      ...emptyWork(id, "movie", title),
      year,
      durationMs,
      contentRating: ratings[year % ratings.length] ?? "PG",
      communityRating: 5 + (year % 50) / 10,
      communityVotes: 100 + (year % 40) * 250,
      addedAt: ago(year % 40),
      images: { poster: image(), backdrop: image() },
      genres: [genres[year % genres.length] ?? "Drama"],
      overview: `${title} is a film from ${year}.`,
      movie: {},
      userState: { played: false, playCount: 0 },
      versions: [version(next(), id, mockIds.movies, "1080p", durationMs, tracks1080, 12e9)],
      libraryId: mockIds.movies,
      ...extra,
    });
  };
  const collection: CollectionSummary = {
    id: mockIds.duneCollection,
    name: "Dune Collection",
    workCount: 2,
    source: "provider",
    images: { poster: image() },
  };
  const dune = movie("Dune", 2021, {
    id: mockIds.dune,
    addedAt: ago(30),
    userState: { played: true, playCount: 1 },
  });
  const dune2 = movie("Dune: Part Two", 2024, {
    id: mockIds.dunePartTwo,
    addedAt: ago(2),
    overview:
      "Paul Atreides unites with Chani and the Fremen while on a path of revenge against the conspirators who destroyed his family.",
    genres: ["Science Fiction", "Adventure"],
    communityRating: 8.2,
    communityVotes: 6400,
    contentRating: "PG-13",
    durationMs: 166 * minute,
    userState: { played: false, playCount: 0, positionMs: 72 * minute + 3000 },
    credits: [
      { personId: next(), name: "Timothée Chalamet", role: "actor", character: "Paul Atreides", photo: image(false) },
      { personId: next(), name: "Zendaya", role: "actor", character: "Chani" },
      { personId: mockIds.villeneuve, name: "Denis Villeneuve", role: "director" },
    ],
    collections: [collection],
    versions: [
      version(mockId(110), mockIds.dunePartTwo, mockIds.movies, "4K HDR", 166 * minute, tracks4k, 58e9),
      version(mockId(111), mockIds.dunePartTwo, mockIds.movies, "1080p", 166 * minute, tracks1080, 14e9),
    ],
    extras: [
      {
        ...summaryOf(emptyWork(next(), "extra", "Official Trailer")),
        durationMs: 2 * minute,
        extra: { type: "trailer", parentId: mockIds.dunePartTwo },
        images: { still: image() },
      },
      {
        ...summaryOf(emptyWork(next(), "extra", "Making the Desert")),
        durationMs: 12 * minute,
        extra: { type: "featurette", parentId: mockIds.dunePartTwo },
      },
    ],
  });
  // Blade Runner's folder holds the theatrical cut and the Final Cut in
  // two qualities. Its folder labelled Final Cut is a root work of its own.
  const theatrical = version(next(), mockIds.bladeRunner, mockIds.movies, "1080p", 117 * minute, tracks1080, 12e9);
  movie("Blade Runner", 1982, {
    id: mockIds.bladeRunner,
    versions: [
      theatrical,
      {
        ...version(next(), mockIds.bladeRunner, mockIds.movies, "4K HDR", 117 * minute, tracks4k, 40e9),
        name: "Final Cut · 4K HDR",
        edition: "Final Cut",
      },
      {
        ...version(next(), mockIds.bladeRunner, mockIds.movies, "1080p", 117 * minute, tracks1080, 14e9),
        name: "Final Cut · 1080p",
        edition: "Final Cut",
      },
    ],
    userState: { played: true, playCount: 2, positionMs: 40 * minute, versionId: theatrical.id },
  });
  add({
    ...emptyWork(mockIds.bladeRunnerFinalCut, "movie", "Blade Runner"),
    year: 1982,
    durationMs: 117 * minute,
    images: { poster: image(), backdrop: image() },
    movie: { edition: "Final Cut" },
    versions: [version(next(), mockIds.bladeRunnerFinalCut, mockIds.movies, "4K HDR", 117 * minute, tracks4k, 40e9)],
    userState: { played: false, playCount: 0 },
  });
  movie("Arrival", 2016, { userState: { played: true, playCount: 1 } });
  movie("Holiday 2019 Final", 2019, { id: mockIds.holiday, matchState: "unmatched", images: {}, addedAt: ago(1) });
  const lighthouse = movie("The Lighthouse Keeper", 2025, { matchState: "identifying", images: {}, addedAt: ago(0) });
  const missing = movie("Missing Reel", 1999, { id: mockIds.missingReel, available: false });
  for (const v of missing.versions) v.available = false;
  for (let i = 0; i < 230; i++) {
    const a = adjectives[i % adjectives.length];
    const b = nouns[Math.floor(i / adjectives.length) % nouns.length];
    movie(`${i % 5 === 0 ? "The " : ""}${a} ${b}${i >= 200 ? ` ${i - 199}` : ""}`, 1950 + ((i * 7) % 75));
  }
  const movieIds = Object.values(works)
    .filter((w) => w.kind === "movie")
    .sort((a, b) => a.title.localeCompare(b.title))
    .map((w) => w.id);

  // Shows: The Expanse, its specials last, and one season of Severance.
  const children: Record<string, string[]> = {};
  /** A season of a series. Its episodes are numbered from 1, with IDs from firstId. */
  const season = (series: WorkDetail, seasonId: string, number: number, titles: string[], firstId: number) => {
    const expanse = series.id === mockIds.expanse;
    const eps = titles.map((title, i) => {
      const e = i + 1;
      const id = mockId(firstId + i);
      const played = expanse && number > 0 && (number < 3 || e < 3);
      const durationMs = (expanse ? 44 : 55) * minute;
      return add({
        ...emptyWork(id, "episode", title),
        durationMs,
        contentRating: series.contentRating,
        addedAt: number === 3 && e >= 8 ? ago(1) : ago(400),
        // Severance's first episode has no still. As on the server, the
        // episode then shows its series' poster.
        images: {
          ...(expanse || e > 1 ? { still: image(), backdrop: image() } : {}),
          poster: series.images.poster,
        },
        episode: {
          seasonNumber: number,
          number: e,
          seriesId: series.id,
          seriesTitle: series.title,
          seasonId,
          airDate:
            expanse && number > 0 ? new Date(Date.UTC(2014 + number, 0, 7 * e)).toISOString().slice(0, 10) : undefined,
        },
        userState: {
          played,
          playCount: played ? 1 : 0,
          positionMs: id === mockId(1303) ? 30 * minute : undefined,
        },
        versions: [version(next(), id, mockIds.shows, "1080p", durationMs, tracks1080, 2e9)],
        remembersTracks: true,
        libraryId: series.libraryId,
      });
    });
    const unplayed = eps.filter((ep) => !ep.userState?.played).length;
    add({
      ...emptyWork(seasonId, "season", number === 0 ? "Specials" : `Season ${number}`),
      images: { poster: expanse ? image() : undefined },
      season: {
        number,
        seriesId: series.id,
        seriesTitle: series.title,
        episodeCount: eps.length,
        unplayedCount: unplayed,
      },
      userState: { played: unplayed === 0, playCount: 0 },
      libraryId: series.libraryId,
    });
    children[series.id] = [...(children[series.id] ?? []), seasonId];
    children[seasonId] = eps.map((ep) => ep.id);
    return eps;
  };
  /** Each episode steps to its neighbours in the series' list order. */
  const link = (eps: WorkDetail[]) =>
    eps.forEach((ep, i) => {
      const prev = eps[i - 1];
      const nxt = eps[i + 1];
      if (ep.episode) ep.episode = { ...ep.episode, previousId: prev?.id, nextId: nxt?.id };
    });
  const expanse = add({
    ...emptyWork(mockIds.expanse, "series", "The Expanse"),
    year: 2015,
    addedAt: ago(1),
    overview: "A police detective and a ship's officer uncover a conspiracy that threatens the solar system.",
    genres: ["Science Fiction", "Drama"],
    communityRating: 8.5,
    communityVotes: 3100,
    contentRating: "TV-14",
    images: { poster: image(), backdrop: image() },
    series: { seasonCount: 4, unplayedCount: 10, lastAirYear: 2022, recentlyAddedCount: 3, ordering: "aired" },
    userState: { played: false, playCount: 0 },
    libraryId: mockIds.shows,
  });
  const numbered = (count: number) => Array.from({ length: count }, (_, i) => `Episode ${i + 1}`);
  link([
    ...season(expanse, mockId(211), 1, numbered(6), 1101),
    ...season(expanse, mockId(212), 2, numbered(6), 1201),
    ...season(
      expanse,
      mockIds.expanseSeason3,
      3,
      numbered(10).map((t, i) => (i === 3 ? "Reload" : t)),
      1301,
    ),
    ...season(expanse, mockId(210), 0, numbered(2), 1001),
  ]);
  // An episode whose overview is longer than its page has room for.
  Object.assign(works[mockId(1305)] as WorkDetail, {
    overview: [
      "Holden and the crew of the Rocinante pick up a distress call from a freighter that shouldn't be anywhere near the Belt, and what they find aboard it changes what they thought they knew about the attack on Eros. Naomi works through the night to keep the reactor stable while Amos goes looking for the people who sold them out, and Alex learns that the only way home runs straight through a Martian blockade.",
      "On Earth, Avasarala spends every favour she has left to keep the fleet from firing first, and discovers that someone inside her own office has been reading her messages for months. On Mars, Bobbie's search for the truth about her squad puts her at odds with her own command, and the one person who believes her is the last person she should trust.",
      "With hours left and every ship on Earth's side out of range, the crew must choose between saving the people closest to them and saving everyone else. Meanwhile, Prax follows a lead on his daughter that takes him far from anyone who could help him, and Fred Johnson has to decide how much he is willing to reveal to the people who have never trusted him.",
      "As the fleet's signals go dark one by one, the Rocinante's crew learn that the freighter's distress call was never meant for them, and that whoever sent it knew exactly who would answer. Naomi finds the message buried in the ship's logs, and has to decide whether to tell the others what it says before they reach the blockade, or carry it alone until it's too late for anyone to turn back.",
    ].join("\n\n"),
  });
  Object.assign(works[mockIds.expanseReload] as WorkDetail, {
    overview: "Holden races to stop an asteroid before it reaches Earth.",
    communityRating: 8.4,
    communityVotes: 42,
  });
  if (expanse.series) expanse.series.nextUp = summaryOf(works[mockId(1303)] as WorkDetail);
  const severance = add({
    ...emptyWork(mockIds.severance, "series", "Severance"),
    year: 2022,
    addedAt: ago(90),
    images: { poster: image(), backdrop: image() },
    series: { seasonCount: 1, unplayedCount: 3 },
    userState: { played: false, playCount: 0 },
  });
  link(season(severance, mockIds.severanceSeason1, 1, ["Good News About Hell", "Half Loop", "In Perpetuity"], 3101));

  // Other videos: a video in a folder, then a loose video.
  const vacation: CollectionSummary = {
    id: mockIds.vacation,
    name: "Vacation 2023",
    workCount: 2,
    source: "folder",
    images: {},
  };
  const video = (title: string, year: number, durationMs: number, added: number) =>
    add({
      ...emptyWork(next(), "video", title),
      year,
      durationMs,
      addedAt: ago(added),
      images: { still: image() },
      userState: { played: false, playCount: 0 },
    }).id;
  const vacationVideos = [video("Beach day", 2023, 5 * minute, 10), video("Mountain hike", 2023, 5 * minute, 10)];
  const birthday = video("Birthday party", 2024, 9 * minute, 3);

  const browse = {
    [mockIds.movies]: summaries(movieIds),
    [mockIds.shows]: summaries([mockIds.expanse, mockIds.severance]),
    [mockIds.videos]: summaries([...vacationVideos, birthday]),
  };
  const libraries: Library[] = [
    {
      id: mockIds.movies,
      name: "Movies",
      kind: "movies",
      pinned: true,
      workCount: movieIds.length,
      folders: [folder("/mnt/user/Movies", ago(2 / 24))],
      useLocalMetadata: true,
      previews: "nightly",
      markers: "never",
      subtitles: "nightlyAndAdded",
      refreshDays: 30,
      unmatchedCount: 1,
      scan: idle(mockIds.movies),
    },
    {
      id: mockIds.shows,
      name: "TV Shows",
      kind: "shows",
      pinned: true,
      workCount: 2,
      folders: [folder("/mnt/user/TV")],
      useLocalMetadata: true,
      previews: "nightly",
      markers: "nightlyAndAdded",
      subtitles: "nightlyAndAdded",
      refreshDays: 30,
      unmatchedCount: 0,
      scan: idle(mockIds.shows),
    },
    {
      id: mockIds.videos,
      name: "Home Videos",
      kind: "videos",
      pinned: true,
      workCount: 2,
      folders: [folder("/mnt/user/Home Videos", ago(3))],
      useLocalMetadata: true,
      previews: "nightly",
      markers: "never",
      subtitles: "nightlyAndAdded",
      refreshDays: 0,
      unmatchedCount: 0,
      scan: idle(mockIds.videos),
    },
  ];

  /** When the maintenance window next opens, at 2:00 AM. */
  const opens = new Date(now);
  opens.setHours(2, 0, 0, 0);
  if (opens.getTime() <= now) opens.setDate(opens.getDate() + 1);
  const nightly = opens.toISOString();
  /** How a task's last run ended. */
  const taskRun = (summary: string | undefined, extra: Partial<TaskRun> = {}): TaskRun => ({
    finishedAt: ago(8 / 24),
    summary,
    ...extra,
  });
  const logLine = (id: number, level: LogLine["level"], message: string, attrs: Record<string, string>): LogLine => ({
    id: String(id),
    loggedAt: ago(0),
    level,
    message,
    attributes: Object.entries(attrs).map(([key, value]) => ({ key, value })),
  });
  const problems: ServerProblem[] = [
    {
      id: "scan:folderUnreachable:tv",
      component: "scan",
      code: "folderUnreachable",
      message: "/mnt/user/TV can't be reached (no such file or directory). Nothing in it has been removed.",
      severity: "warning",
      raisedAt: ago(0),
    },
    {
      id: "metadata:tmdbUnavailable",
      component: "metadata",
      code: "tmdbUnavailable",
      message: "TMDb is not responding; metadata updates wait until it is back",
      severity: "warning",
      raisedAt: ago(1),
    },
  ];
  const artwork: Artwork[] = (["poster", "backdrop", "logo"] as const).flatMap((type, t) => {
    const current = dune2.images[type];
    const offered = [1, 2, 3].map((i): Artwork => {
      const id = `0190a000-0000-7000-9${t}${i}0-${dune2.id.slice(-12)}`;
      return {
        id,
        type,
        source: "provider",
        provider: "tmdb",
        language: "en",
        width: 2000,
        height: 3000,
        selected: false,
        image: { id, width: 2000, height: 3000 },
      };
    });
    return [
      ...(current
        ? [
            {
              id: current.id,
              type,
              source: "provider" as const,
              provider: "tmdb" as const,
              selected: true,
              image: current,
            },
          ]
        : []),
      ...offered,
    ];
  });
  const hdFile = filesOf(dune)[0] as MediaFile;
  const currentUser: CurrentUser = {
    id: mockId(7001),
    username: "anna",
    admin: true,
    limits: {},
    playback: { subtitleMode: "forced", rememberTracks: true, autoplayNext: true, bookSpeed: 1 },
  };

  const api: MockApi = {
    setup: { required: false, serverName: "Living room" },
    signedIn: true,
    session: {
      id: mockId(7000),
      kind: "browser",
      name: "Firefox on Linux x86_64",
      createdAt: new Date(now).toISOString(),
      lastUsedAt: new Date(now).toISOString(),
      current: true,
    },
    currentUser,
    users: [
      {
        id: currentUser.id,
        username: "anna",
        admin: true,
        disabled: false,
        onlyAdmin: true,
        libraries: { all: true, libraryIds: [] },
        limits: {},
        parental: { blockUnrated: false },
        createdAt: ago(400),
      },
      {
        id: mockId(7101),
        username: "ben",
        admin: false,
        disabled: false,
        onlyAdmin: false,
        libraries: { all: false, libraryIds: [mockIds.movies] },
        limits: { maxStreams: 2 },
        parental: { maxAge: 13, blockUnrated: true },
        createdAt: ago(30),
      },
    ],
    invites: [
      {
        id: mockId(7201),
        libraries: { all: true, libraryIds: [] },
        limits: {},
        createdBy: "anna",
        createdAt: ago(1),
        expiresAt: new Date(now + 6 * day).toISOString(),
      },
    ],
    createdInvite: {
      id: mockId(7202),
      libraries: { all: true, libraryIds: [] },
      limits: {},
      createdBy: "anna",
      createdAt: new Date(now).toISOString(),
      expiresAt: new Date(now + 7 * day).toISOString(),
      token: "invite-token",
    },
    inviteLink: { invitedBy: "anna", serverName: "Living room", expiresAt: new Date(now + 6 * day).toISOString() },
    sessions: [
      {
        id: mockId(7000),
        kind: "browser",
        name: "Firefox on Linux x86_64",
        createdAt: ago(30),
        lastUsedAt: ago(0),
        current: true,
      },
      {
        id: mockId(7002),
        kind: "app",
        name: "Living Room",
        device: { platform: "androidTv" },
        createdAt: ago(60),
        lastUsedAt: ago(1),
        current: false,
      },
    ],
    apiKeys: [
      { id: mockId(7301), name: "Home Assistant", createdAt: ago(20), lastUsedAt: ago(0) },
      { id: mockId(7302), name: "Overseerr", createdAt: ago(40) },
    ],
    createdApiKey: {
      id: mockId(7303),
      name: "Tautulli",
      createdAt: new Date(now).toISOString(),
      token: "api-key-token",
    },
    waitingDevice: { name: "Living Room", platform: "androidTv", nearby: true, startedAt: ago(0) },
    deviceCodePoll: "waiting",
    server: {
      id: mockId(7200),
      name: "Living room",
      version: "1.0.0",
      localUrls: ["http://192.168.1.10:8484"],
      qualityLadder: [
        { bitrateBps: 20_000_000, height: 1080 },
        { bitrateBps: 12_000_000, height: 1080 },
        { bitrateBps: 8_000_000, height: 1080 },
        { bitrateBps: 4_000_000, height: 720 },
        { bitrateBps: 2_000_000, height: 480 },
        { bitrateBps: 1_000_000, height: 360 },
        { bitrateBps: 720_000, height: 360 },
      ],
    },
    settings: {
      serverName: "Living room",
      metadataLanguage: "en-US",
      contentRatingCountry: "US",
      playedThreshold: 0.9,
      openSubtitlesKey: "",
      openSubtitlesUsername: "",
      openSubtitlesPasswordSet: false,
      fileWatching: true,
      localNetworks: [],
      accelerator: "auto",
      videoTranscodeLimit: 0,
      maintenanceStartHour: 2,
      maintenanceEndHour: 6,
      previewsAtOnce: 2,
      markersAtOnce: 1,
      publicUrl: "",
      remoteQualityCapBps: 0,
    },
    contentRatings: [
      { name: "G", age: 0 },
      { name: "PG", age: 10 },
      { name: "PG-13", age: 13 },
      { name: "R", age: 17 },
      { name: "NC-17", age: 18 },
      { name: "TV-14", age: 14 },
      { name: "TV-MA", age: 17 },
    ],
    directories: {
      "": [
        { name: "mnt", path: "/mnt" },
        { name: "media", path: "/media" },
      ],
      "/mnt": [{ name: "user", path: "/mnt/user" }],
      "/mnt/user": [
        { name: "Movies", path: "/mnt/user/Movies" },
        { name: "TV", path: "/mnt/user/TV" },
        { name: "Home Videos", path: "/mnt/user/Home Videos" },
      ],
      "/media": [],
    },
    libraries,
    facets: {
      [mockIds.movies]: {
        genres: [...genres].sort(),
        contentRatings: ratings,
        years: [...new Set((browse[mockIds.movies] ?? []).map((w) => w.year ?? 0))].sort((a, b) => a - b),
        narrators: [],
        resolutions: ["4k", "1080p"],
        dynamicRanges: ["hdr"],
        collections: true,
      },
      [mockIds.shows]: {
        genres: ["Drama", "Science Fiction"],
        contentRatings: ["TV-14"],
        years: [2015, 2022],
        narrators: [],
        resolutions: ["1080p"],
        dynamicRanges: [],
        collections: false,
      },
      [mockIds.videos]: {
        genres: [],
        contentRatings: [],
        years: [2023, 2024],
        narrators: [],
        resolutions: ["1080p"],
        dynamicRanges: [],
        collections: true,
      },
    },
    browse,
    collections: { [mockIds.movies]: [collection], [mockIds.videos]: [vacation] },
    collectionPages: {
      [mockIds.duneCollection]: {
        ...collection,
        images: { ...collection.images, backdrop: image() },
        items: summaries([mockIds.dune, mockIds.dunePartTwo]),
      },
      [mockIds.vacation]: { ...vacation, items: summaries(vacationVideos) },
    },
    works,
    // A season's episodes carry their synopses, as the server lists them.
    children: Object.fromEntries(
      Object.entries(children).map(([id, ids]) => [
        id,
        summaries(ids).map((s) => {
          const overview = works[s.id]?.overview;
          return s.episode && overview ? { ...s, episode: { ...s.episode, overview } } : s;
        }),
      ]),
    ),
    lists: [
      {
        id: mockIds.myList,
        name: "My List",
        kind: "manual",
        builtIn: true,
        workIds: [mockIds.dune, mockIds.expanse],
      },
      {
        id: mockIds.dateNight,
        name: "Date Night",
        kind: "manual",
        builtIn: false,
        workIds: [mockIds.dune],
      },
    ],
    home: {
      rows: [
        {
          id: "continueWatching",
          kind: "continueWatching",
          items: summaries([mockIds.dunePartTwo, mockId(1303), mockIds.severancePilot]),
        },
        {
          id: `list:${mockIds.myList}`,
          kind: "list",
          list: { id: mockIds.myList, name: "My List" },
          items: summaries([mockIds.dune, mockIds.expanse]),
        },
        {
          id: `recentlyAdded:${mockIds.movies}`,
          kind: "recentlyAdded",
          library: { id: mockIds.movies, name: "Movies", kind: "movies" },
          items: summaries([lighthouse.id, mockIds.holiday, ...movieIds.slice(0, 10)]),
        },
        {
          id: `recentlyAdded:${mockIds.shows}`,
          kind: "recentlyAdded",
          library: { id: mockIds.shows, name: "TV Shows", kind: "shows" },
          items: summaries([mockIds.expanse, severance.id]),
        },
        {
          id: `recentlyAdded:${mockIds.videos}`,
          kind: "recentlyAdded",
          library: { id: mockIds.videos, name: "Home Videos", kind: "videos" },
          items: summaries([birthday, ...vacationVideos]),
        },
      ],
    },
    search: summaries([mockIds.dune, mockIds.dunePartTwo]),
    searchPeople: [{ id: mockIds.villeneuve, name: "Denis Villeneuve", role: "director", workCount: 2 }],
    searchCollections: [collection],
    files: { [mockIds.dunePartTwo]: filesOf(dune2) },
    unmatched: [
      {
        ...summaryOf(works[mockIds.holiday] as WorkDetail),
        libraryId: mockIds.movies,
        path: "/mnt/user/Movies/Holiday 2019 Final.mkv",
      },
    ],
    missingFiles: [
      {
        ...hdFile,
        id: next(),
        path: "/mnt/user/Movies/Missing Reel (1999)/Missing Reel.mkv",
        status: "gone",
        goneAt: ago(2),
        title: "Missing Reel",
      },
    ],
    matches: [
      {
        id: "tmdb-1",
        provider: "tmdb",
        kind: "movie",
        title: "Holiday",
        year: 2019,
        overview: "Holiday, from 2019.",
        score: 92,
      },
      { id: "tmdb-2", provider: "tmdb", kind: "movie", title: "Holiday", year: 1989, score: 61 },
    ],
    artwork,
    taskFailures: {
      "make-previews": [
        {
          itemId: next(),
          title: "Heat (1995)/Heat.mkv",
          reason: "ffmpeg couldn't decode the video: the file is truncated.",
          failedAt: ago(8 / 24),
        },
        {
          itemId: next(),
          title: "Ronin (1998)/Ronin.mkv",
          reason: "ffmpeg couldn't decode the video: the file is truncated.",
          failedAt: ago(8 / 24),
        },
      ],
      "scan-libraries": [
        {
          itemId: next(),
          title: "Broken Reel (2001)/Broken Reel.mkv",
          reason: "The file is truncated: ffprobe found no video stream.",
          failedAt: ago(0),
        },
      ],
    },
    tasks: [
      {
        id: "scan-libraries",
        section: "library",
        nightly: false,
        lastRun: taskRun("Scanned 3 folders", { finishedAt: ago(3 / 24) }),
        nextRunAt: new Date(now + 9 * hour).toISOString(),
        progress: { step: "probe", done: 37, total: 120 },
        pendingCount: 0,
        failedCount: 1,
        running: true,
        stoppable: true,
      },
      {
        id: "refresh-metadata",
        section: "library",
        nightly: true,
        lastRun: taskRun("Refreshed 212 items"),
        nextRunAt: nightly,
        pendingCount: 0,
        failedCount: 12,
        running: false,
        stoppable: false,
      },
      {
        id: "make-previews",
        section: "library",
        nightly: true,
        lastRun: taskRun("Made preview thumbnails for 14,486 files"),
        nextRunAt: nightly,
        pendingCount: 18489,
        failedCount: 2,
        running: false,
        stoppable: false,
      },
      {
        id: "find-markers",
        section: "library",
        nightly: true,
        lastRun: taskRun("Analyzed 312 episodes"),
        nextRunAt: nightly,
        progress: { done: 312, total: 1516 },
        pendingCount: 0,
        failedCount: 0,
        running: true,
        stoppable: true,
      },
      {
        id: "prune-gone-files",
        section: "library",
        nightly: true,
        lastRun: taskRun("Removed 3 missing files"),
        nextRunAt: nightly,
        pendingCount: 0,
        running: false,
        stoppable: false,
      },
      ...(
        [
          "prune-previews",
          "prune-markers",
          "prune-subtitles",
          "prune-downloads",
          "prune-provider-cache",
          "prune-activity",
        ] as const
      ).map((id) => ({
        id,
        section: "maintenance" as const,
        nightly: true,
        lastRun: taskRun(undefined),
        nextRunAt: nightly,
        pendingCount: 0,
        running: false,
        stoppable: false,
      })),
      {
        id: "make-backup",
        section: "maintenance",
        nightly: true,
        lastRun: taskRun(undefined, { error: "The backup folder is full." }),
        nextRunAt: nightly,
        pendingCount: 0,
        running: false,
        stoppable: false,
      },
      {
        id: "optimize-database",
        section: "maintenance",
        nightly: true,
        lastRun: taskRun(undefined),
        nextRunAt: nightly,
        pendingCount: 0,
        running: false,
        stoppable: false,
      },
      {
        id: "prepare-downloads",
        section: "activity",
        nightly: false,
        progress: { done: 0, total: 0 },
        pendingCount: 0,
        running: true,
        stoppable: false,
      },
      {
        id: "extract-subtitles",
        section: "library",
        nightly: true,
        pendingCount: 0,
        failedCount: 0,
        running: false,
        stoppable: false,
      },
      {
        id: "library-changes",
        section: "activity",
        nightly: false,
        pendingCount: 0,
        running: false,
        stoppable: false,
      },
    ],
    problems,
    logs: [
      logLine(1, "info", "kivo: starting", { version: "1.0.0" }),
      logLine(2, "info", "scan: finished", { root: "/mnt/user/Movies", seen: "236", took: "4.2s" }),
      logLine(3, "warning", "scan: folder unreachable; nothing in it changes", {
        root: "/mnt/user/TV",
        err: "no such file or directory",
      }),
      logLine(4, "error", "metadata: provider unavailable", {
        provider: "tmdb",
        err: "egress: get api.themoviedb.org: timeout",
      }),
    ],
    accelerators: {
      tested: true,
      items: [
        {
          id: "qsv:/dev/dri/renderD128",
          backend: "qsv",
          name: "Intel Quick Sync (QSV)",
          encoders: [
            { codec: "h264", gop: "grid" },
            { codec: "hevc", gop: "fixed" },
          ],
          decoders: [
            { codec: "h264", bitDepth: 8 },
            { codec: "hevc", bitDepth: 10 },
          ],
          toneMap: "accelerator",
          burnIn: "accelerator",
        },
        {
          id: "vaapi:/dev/dri/renderD128",
          backend: "vaapi",
          name: "Intel VA-API",
          encoders: [{ codec: "h264", gop: "fixed" }],
          decoders: [{ codec: "h264", bitDepth: 8 }],
          toneMap: "cpu",
          burnIn: "accelerator",
        },
        {
          id: "nvenc:0",
          backend: "nvenc",
          name: "NVIDIA GeForce GTX 1060",
          encoders: [],
          decoders: [],
          problem: { reason: "noDriver", detail: "libcuda.so.1: cannot open shared object file" },
        },
      ],
    },
    starts: {
      [mockIds.dunePartTwo]: directPlay(dune2, dune2.versions[1] as Version, 72 * minute + 3000),
      [mockIds.expanseReload]: directPlay(
        works[mockIds.expanseReload] as WorkDetail,
        (works[mockIds.expanseReload] as WorkDetail).versions[0] as Version,
        0,
      ),
    },
    fallback: {
      ...directPlay(dune2, dune2.versions[1] as Version, 72 * minute + 3000),
      delivery: "stream",
      url: "plays/play/hls/index.m3u8",
      tracks: [
        {
          trackId: "1080-video",
          type: "video",
          selected: true,
          delivery: "stream",
          codec: "h264",
          reason: "fallback",
          transcode: { width: 1920, height: 1080, encoder: "CPU" },
        },
        {
          trackId: "1080-audio-en",
          type: "audio",
          selected: true,
          delivery: "stream",
          codec: "aac",
          reason: "fallback",
          transcode: { channels: 2, bitrateBps: 128_000 },
        },
        { trackId: "off", type: "subtitle", selected: true, delivery: "none" },
      ],
      parts: [{ durationMs: 166 * minute }],
    },
    plays: [],
    history: [],
    activity: sampleActivity(now),
    subtitleResults: [
      {
        id: "os-1",
        language: "en",
        release: "Sample.2024.1080p.WEB",
        downloads: 12_400,
        hashMatch: true,
        forced: false,
        hearingImpaired: false,
      },
      { id: "os-2", language: "en", downloads: 3100, hashMatch: false, forced: false, hearingImpaired: true },
    ],
    addedSubtitle: {
      id: "downloaded-en",
      name: "English · OpenSubtitles",
      fullName: "English · SRT · Downloaded",
      type: "subtitle",
      codec: "subrip",
      language: "en",
      default: false,
      subtitle: subtitle("downloaded"),
    },
    snapshot: {
      type: "snapshot",
      serverVersion: "1.0.0",
      problemCount: problems.length,
    },
    authors: [],
    bookSeries: [],
    bookMatches: [],
    people: {},
    seriesPages: {},
  };
  addBooks(api, image, ago);
  api.people[mockIds.villeneuve] = {
    id: mockIds.villeneuve,
    name: "Denis Villeneuve",
    works: summaries([mockIds.dunePartTwo, mockIds.dune]),
    bookSeries: [],
    standalone: [],
    narrated: [],
  };
  return api;
}

/** Changes to the sample server for other situations. */
export const scenarios = {
  /** Nobody is signed in. */
  signedOut(api: MockApi) {
    api.signedIn = false;
  },
  /** Movies holds 3,000 works from A to Z, each with its own poster, for scrolling a long grid. */
  long(api: MockApi) {
    const sample = api.browse[mockIds.movies]?.[0];
    if (!sample) return;
    const count = 3000;
    api.browse[mockIds.movies] = Array.from({ length: count }, (_, i) => ({
      ...sample,
      id: mockId(500_000 + i),
      title: `${String.fromCharCode(65 + Math.floor((i * 26) / count))} Movie ${i + 1}`,
      images: { poster: { ...sample.images.poster, id: mockId(600_000 + i), width: 780, height: 1170 } },
    }));
  },
  /** No libraries at all. */
  empty(api: MockApi) {
    api.libraries = [];
    api.home = { rows: [] };
  },
  /** A fresh server with no user yet and nothing in it. */
  setup(api: MockApi) {
    scenarios.empty(api);
    api.setup.required = true;
    api.signedIn = false;
  },
  /** Signed in as a user that isn't an admin. */
  viewer(api: MockApi) {
    api.currentUser.admin = false;
    api.users = [];
    api.snapshot = { type: "snapshot", serverVersion: api.snapshot.serverVersion, problemCount: 0 };
  },
  /** The Movies library's first scan is running. Its files have been read, and it's matching them. */
  scanning(api: MockApi) {
    const movies = api.libraries.find((l) => l.id === mockIds.movies);
    if (!movies) return;
    movies.scan = {
      libraryId: mockIds.movies,
      state: "scanning",
      progress: { step: "identify", done: 340, total: 812, remainingMs: 4 * minute },
    };
  },
  /** Home Videos isn't pinned, so it's under More libraries and has no Home row. */
  unpinned(api: MockApi) {
    const videos = api.libraries.find((l) => l.id === mockIds.videos);
    if (!videos) return;
    videos.pinned = false;
    api.home.rows = api.home.rows.filter((r) => r.library?.id !== mockIds.videos);
  },
  /**
   * Three people watching.
   *  - anna plays an episode directly on her iPhone at home.
   *  - ben watches the 4K Dune: Part Two away from home. It is transcoded
   *    on Quick Sync, with its TrueHD 7.1 mixed down to AAC 5.1 and French
   *    subtitles burned in.
   *  - carla listens to Leviathan Wakes, paused.
   */
  nowPlaying(api: MockApi) {
    const episode = api.works[mockIds.expanseReload];
    const film = api.works[mockIds.dunePartTwo];
    const book = api.works[bookIds.leviathan];
    if (!episode || !film || !book) return;
    const [episodeVersion] = episode.versions;
    const [filmVersion] = film.versions;
    const [bookVersion] = book.versions;
    if (!episodeVersion || !filmVersion || !bookVersion) return;
    const tracks = (v: Version, ids: string[]) => v.tracks.filter((t) => ids.includes(t.id));
    const started = (minutes: number) => new Date(Date.now() - minutes * minute).toISOString();
    api.plays = [
      {
        id: "now-1",
        workId: episode.id,
        title: episode.title,
        work: summaryOf(episode),
        versionId: episodeVersion.id,
        userId: api.currentUser.id,
        username: "anna",
        clientName: "anna's iPhone",
        address: "192.168.1.42",
        startedAt: started(21),
        state: "playing",
        local: true,
        positionMs: 20 * minute + 14_000,
        durationMs: episodeVersion.durationMs,
        bitrateBps: 8_200_000,
        device: {
          app: "kivo-apple",
          appVersion: "1.4.0",
          platform: "iPhone",
          osVersion: "18.1",
          deviceModel: "iPhone16,2",
        },
        delivery: "file",
        tracks: filePlayTracks(episodeVersion),
        sourceTracks: tracks(episodeVersion, ["1080-video", "1080-audio-en"]),
      },
      {
        id: "now-2",
        workId: film.id,
        title: film.title,
        work: summaryOf(film),
        versionId: filmVersion.id,
        userId: mockId(7101),
        username: "ben",
        clientName: "Firefox on Windows",
        address: "203.0.113.24",
        startedAt: started(74),
        state: "playing",
        local: false,
        positionMs: 72 * minute + 3000,
        durationMs: filmVersion.durationMs,
        bitrateBps: 8_384_000,
        bandwidthBps: 42_000_000,
        device: { app: "kivo-web", appVersion: "index-3f9c2a", platform: "browser", deviceModel: "Firefox 132" },
        delivery: "stream",
        videoProcessing: "accelerator",
        tracks: [
          {
            trackId: "4k-video",
            type: "video",
            selected: true,
            delivery: "stream",
            codec: "h264",
            reason: "burnSubtitlesEnabled",
            transcode: { width: 1920, height: 1080, bitrateBps: 8_000_000, encoder: "Intel Quick Sync (QSV)" },
          },
          {
            trackId: "4k-audio-en",
            type: "audio",
            selected: true,
            delivery: "stream",
            codec: "aac",
            reason: "audioCodecUnsupported",
            mismatch: { field: "codec", declared: "aac,ac3,eac3,opus,flac,mp3", actual: "truehd" },
            transcode: { channels: 6, bitrateBps: 384_000 },
          },
          {
            trackId: "4k-sub-fr-forced",
            type: "subtitle",
            selected: true,
            delivery: "burn",
            reason: "burnSubtitlesEnabled",
          },
          { trackId: "off", type: "subtitle" },
        ],
        sourceTracks: tracks(filmVersion, ["4k-video", "4k-audio-en", "4k-sub-fr-forced"]),
        transcode: {
          state: "running",
          encodedMs: 75 * minute + 27_000,
          speed: 1.8,
          command:
            "ffmpeg -hwaccel qsv -ss 4320 -i file:/media/Dune.Part.Two.2024.2160p.mkv -c:v h264_qsv -b:v 8M -c:a aac -ac 6 -f hls pipe:1",
        },
      },
      {
        id: "now-3",
        workId: book.id,
        title: book.title,
        work: summaryOf(book),
        versionId: bookVersion.id,
        userId: mockId(7102),
        username: "carla",
        clientName: "Chrome on Android",
        address: "192.168.1.57",
        startedAt: started(48),
        state: "paused",
        local: true,
        positionMs: 52 * minute + 40_000,
        durationMs: bookVersion.durationMs,
        bitrateBps: 64_000,
        device: {
          app: "kivo-web",
          appVersion: "index-3f9c2a",
          platform: "browser",
          osVersion: "14",
          deviceModel: "Chrome 141",
        },
        delivery: "file",
        tracks: filePlayTracks(bookVersion),
        sourceTracks: bookVersion.tracks,
      },
    ];
  },
  /**
   * A busy evening. Three people are watching, Movies' first scan is
   * matching, and there's a run of failed sign-ins and an unmatched series
   * in TV Shows. These come on top of the sample's problems, unmatched
   * root work, missing file and failed tasks.
   */
  busy(api: MockApi) {
    scenarios.nowPlaying(api);
    scenarios.scanning(api);
    const nightShift: WorkDetail = {
      ...emptyWork(mockIds.nightShift, "series", "the.night.shift"),
      year: 2024,
      matchState: "unmatched",
      libraryId: mockIds.shows,
    };
    api.works[nightShift.id] = nightShift;
    api.unmatched = [...api.unmatched, { ...summaryOf(nightShift), path: "/mnt/user/TV/the.night.shift.2024" }];
    const shows = api.libraries.find((l) => l.id === mockIds.shows);
    if (shows) shows.unmatchedCount = 1;
    const signIns: ServerProblem = {
      id: "activity.signInsFailed",
      component: "activity",
      code: "signInsFailed",
      message: "6 failed sign-ins in the last day",
      severity: "warning",
      raisedAt: new Date().toISOString(),
    };
    api.problems = [signIns, ...api.problems];
    api.snapshot.problemCount = api.problems.length;
  },
  /** A quiet day, with one play and nothing else to report. */
  quiet(api: MockApi) {
    scenarios.nowPlaying(api);
    api.plays = api.plays.slice(0, 1);
    api.problems = [];
    api.snapshot.problemCount = 0;
    api.missingFiles = [];
    api.activity = [];
    api.unmatched = [];
    for (const library of api.libraries) library.unmatchedCount = 0;
    for (const task of api.tasks) {
      task.running = false;
      delete task.progress;
      if (task.failedCount !== undefined) task.failedCount = 0;
      if (task.lastRun?.error) delete task.lastRun;
    }
  },
  /** Search finds every movie and show, so its results are rows of several cards. */
  manyResults(api: MockApi) {
    api.search = [...(api.browse[mockIds.movies] ?? []), ...(api.browse[mockIds.shows] ?? [])];
  },
  /** Search finds nothing. */
  noResults(api: MockApi) {
    api.search = [];
    api.searchPeople = [];
    api.searchCollections = [];
  },
  /**
   * Dune: Part Two and The Expanse's "Reload" really play, in the browser:
   * a ten-minute file of a still picture with two silent audio tracks
   * (mock-public/mock-media/film.mp4), made with
   *
   *   ffmpeg -f lavfi -i "color=c=0x22324f:s=640x360:r=1:d=600,drawbox=y=240:w=640:h=120:color=0x3a2a4a:t=fill"
   *     -f lavfi -i anullsrc=r=8000:cl=mono -f lavfi -i anullsrc=r=8000:cl=mono -map 0:v -map 1:a -map 2:a
   *     -t 600 -c:v libx264 -profile:v baseline -pix_fmt yuv420p -g 10 -tune stillimage -crf 35
   *     -c:a aac -b:a 8k -ac 1 -movflags +faststart film.mp4
   *
   * Dune plays its 4K version, with English and French audio, English and
   * French subtitles as files that show at once, and English SDH, which
   * takes a new play that burns it in. Plays start where they're asked to
   * and with the tracks asked for. The episode starts in its intro.
   */
  playable(api: MockApi) {
    api.playsAsAsked = true;
    const length = 10 * minute;
    const file = "/mock-media/film.mp4";
    const playable = (work: WorkDetail | undefined, v: Version | undefined, startMs: number) => {
      if (!work || !v) return undefined;
      const play = directPlay(work, { ...v, durationMs: length }, startMs);
      return { ...play, url: file, parts: [{ durationMs: length, url: file }] };
    };
    const dune = playable(api.works[mockIds.dunePartTwo], api.works[mockIds.dunePartTwo]?.versions[0], 5000);
    if (dune) {
      // English and the French forced come as WebVTT files; English SDH,
      // an image subtitle, has no delivery, so choosing it takes a new play.
      const external = ["4k-sub-en", "4k-sub-fr-forced"];
      api.starts[mockIds.dunePartTwo] = {
        ...dune,
        tracks: dune.tracks.map((t) =>
          external.includes(t.trackId)
            ? { ...t, delivery: "external", codec: "webvtt", url: `plays/play/subtitles/${t.trackId}.vtt` }
            : t,
        ),
      };
    }
    const episode = api.works[mockIds.expanseReload];
    const reload = playable(episode, episode?.versions[0], 61_000);
    if (reload) api.starts[mockIds.expanseReload] = reload;
  },
  /** The TV Shows folder can't be reached. */
  unreachable(api: MockApi) {
    const shows = api.libraries.find((l) => l.id === mockIds.shows);
    if (!shows) return;
    shows.folders = (shows.folders ?? []).map((f) => ({ ...f, reachable: false }));
    shows.scan = { libraryId: mockIds.shows, state: "unreachable" };
  },
};
