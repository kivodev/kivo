// The mock server's state, and the work shapes every fixture builds on.
import type {
  AcceleratorList,
  ActivityEvent,
  ApiKey,
  Artwork,
  BookSeries,
  BookSeriesSummary,
  Collection,
  CollectionSummary,
  CurrentUser,
  Directory,
  Home,
  HomeRowInfo,
  Invite,
  InviteLink,
  Library,
  LogLine,
  Match,
  MediaFile,
  PendingDevice,
  Person,
  PersonSummary,
  Play,
  PlayEvent,
  PlayStarted,
  PlayTrack,
  ServerInfo,
  ServerProblem,
  Session,
  Settings,
  SetupState,
  SnapshotMessage,
  SubtitleResult,
  Task,
  TaskFailure,
  Track,
  User,
  Version,
  WorkDetail,
  WorkField,
  WorkFieldValue,
  WorkKind,
  WorkList,
  WorkSummary,
} from "../schemas";

/** A list as the mock keeps it. Its count is worked out when it's read. */
export type MockList = Omit<WorkList, "workCount"> & { workIds: string[] };

/** What the mock server answers. Handlers read it at each request, and tests change it. */
export type MockApi = {
  setup: SetupState;
  /** Whether the request for the current user answers, or gets a 401. */
  signedIn: boolean;
  /** The session that signing in makes. Its user is currentUser. */
  session: Session;
  currentUser: CurrentUser;
  /**
   * Admin › Users, by username, with the signed-in user first. Adding a
   * user answers with the last one.
   */
  users: User[];
  /** Pending invite links. */
  invites: Invite[];
  /** The invite that Create invite link makes, with its token. */
  createdInvite: Invite;
  /** What any invite link's page shows. It is absent for a dead link. */
  inviteLink?: InviteLink;
  /** Where you're signed in, in Settings, with this browser first. */
  sessions: Session[];
  /** Admin › API keys, newest first. */
  apiKeys: ApiKey[];
  /** The key that Create key makes, with its token. */
  createdApiKey: ApiKey;
  /** The device waiting on any link code. It is absent for a wrong or expired code. */
  waitingDevice?: PendingDevice;
  /** How this browser's own link code is answered when it polls. Approved signs it in. */
  deviceCodePoll: "waiting" | "approved" | "denied";
  settings: Settings;
  server: ServerInfo;
  contentRatings: { name: string; age: number }[];
  directories: Record<string, Directory[]>;
  libraries: Library[];
  /** The values a library's works have for the fields its grid filters by, by library and field. */
  libraryFieldValues: Record<string, Record<string, WorkFieldValue[]>>;
  /** A library's grid, whatever the sort and filters asked. */
  browse: Record<string, WorkSummary[]>;
  /** A library's collections, by library. */
  collections: Record<string, CollectionSummary[]>;
  /** Collections' pages, by collection. */
  collectionPages: Record<string, Collection>;
  works: Record<string, WorkDetail>;
  /** A series' seasons and a season's episodes. */
  children: Record<string, WorkSummary[]>;
  home: Home;
  /** Home's layout once it's been changed. */
  homeLayout?: HomeRowInfo[];
  /** The user's lists, each with its works' IDs, most recently added first. */
  lists: MockList[];
  /** What rules and sorts can name. */
  workFields: WorkField[];
  /** The values of each field that chooses from a list, by field. */
  workFieldValues: Record<string, WorkFieldValue[]>;
  /** Search results, whatever was searched. */
  search: WorkSummary[];
  /** The people search finds, whatever was searched. */
  searchPeople: PersonSummary[];
  /** The collections search finds, whatever was searched. */
  searchCollections: CollectionSummary[];
  /** Show files, by work. */
  files: Record<string, MediaFile[]>;
  /** The Unmatched list, each card with its library and path. */
  unmatched: WorkSummary[];
  missingFiles: MediaFile[];
  matches: Match[];
  /** A book's matches, from Audible. */
  bookMatches: Match[];
  /** The choices in Choose artwork, listed by their type. */
  artwork: Artwork[];
  /** The Tasks page's rows. */
  tasks: Task[];
  /** The failed items of each task that keeps failures. */
  taskFailures: Record<string, TaskFailure[]>;
  problems: ServerProblem[];
  logs: LogLine[];
  accelerators: AcceleratorList;
  /** What starting a play of each work answers. */
  starts: Record<string, PlayStarted>;
  /**
   * Whether a play starts where the request asks and with the tracks it
   * asks for, as the server's does, rather than as `starts` has it. A
   * subtitle that takes a new play comes burned in.
   */
  playsAsAsked?: boolean;
  /** What a fallback answers. */
  fallback: PlayStarted;
  /** The plays the admin's Now playing lists. */
  plays: Play[];
  history: PlayEvent[];
  /** The Activity page's events, newest first. */
  activity: ActivityEvent[];
  subtitleResults: SubtitleResult[];
  /** The track Download adds. */
  addedSubtitle: Track;
  /** The live-events socket's first message. */
  snapshot: SnapshotMessage;
  /** An audiobooks library's Authors tab. */
  authors: PersonSummary[];
  /** An audiobooks library's Series tab. */
  bookSeries: BookSeriesSummary[];
  /** People's pages, by person. */
  people: Record<string, Person>;
  /** Book series pages, by series. */
  seriesPages: Record<string, BookSeries>;
};

export function emptyWork(id: string, kind: WorkKind, title: string): WorkDetail {
  return {
    id,
    kind,
    title,
    matchState: "matched",
    available: true,
    images: {},
    genres: [],
    studios: [],
    credits: [],
    externalIds: [],
    versions: [],
    extras: [],
    collections: [],
  };
}

/** The card fields of a work, which are what lists hold. */
export function summaryOf(work: WorkDetail): WorkSummary {
  const {
    originalTitle: _o,
    sortTitle: _s,
    overview: _ov,
    genres: _g,
    studios: _st,
    credits: _c,
    externalIds: _e,
    versions: _v,
    extras: _x,
    listIds: _li,
    collections: _co,
    lockedFields: _l,
    ...summary
  } = work;
  const { nextUp: _n, ordering: _or, ...series } = summary.series ?? { seasonCount: 0, unplayedCount: 0 };
  const {
    previousId: _p,
    nextId: _nx,
    ...episode
  } = summary.episode ?? {
    seasonNumber: 0,
    number: 0,
    seriesId: "",
    seriesTitle: "",
  };
  return {
    ...summary,
    series: summary.series && series,
    episode: summary.episode && episode,
  };
}

/**
 * A direct play's tracks of a version: its video and first audio track
 * from the file, as their stream index says, and subtitles off. Another
 * audio or subtitle track takes a new play, as for a browser that switches
 * neither inside a file.
 */
export function filePlayTracks(version: Version): PlayTrack[] {
  const video = version.tracks.find((t) => t.type === "video");
  const audio = version.tracks.find((t) => t.type === "audio");
  return [
    ...version.tracks.map((t, index): PlayTrack => {
      const playing = t === video || t === audio;
      return playing
        ? { trackId: t.id, type: t.type, selected: true, delivery: "file", codec: t.codec, index }
        : { trackId: t.id, type: t.type };
    }),
    { trackId: "off", type: "subtitle", selected: true, delivery: "none" },
  ];
}
