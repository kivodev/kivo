import { execFileSync, spawn } from "node:child_process";
import { mkdirSync, rmSync, utimesSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { request } from "@playwright/test";
import { e2e, type Fixture } from "./env";

// Builds Kivo from this tree with the web client embedded, makes the test
// media with ffmpeg, starts the server, sets it up and adds a library of the
// media, then waits until probing, subtitle preparation and scrub previews
// are done. KIVO_BIN skips the build; KIVO_FFMPEG and KIVO_FFPROBE choose
// the tools (default: ffmpeg and ffprobe on the PATH).

const webDir = resolve(import.meta.dirname, "..");
const serverDir = resolve(webDir, "..");
const ffmpeg = process.env.KIVO_FFMPEG ?? "ffmpeg";
const ffprobe = process.env.KIVO_FFPROBE ?? "ffprobe";
const password = "correct horse battery";

/** SubRip with a two-second cue every two seconds for two minutes, so one always shows. */
function subrip(words: string): string {
  const stamp = (s: number) =>
    `00:${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")},000`;
  const cues = [];
  for (let s = 0, n = 1; s < 120; s += 2, n++) cues.push(`${n}\n${stamp(s)} --> ${stamp(s + 2)}\n${words} ${n}\n`);
  return cues.join("\n");
}

/** A font for the film to carry: the player's own Latin fallback, a real font in the tree. */
const font = join(webDir, "src", "features", "player", "fonts", "kivo-fallback-latin-700.woff2");

/**
 * The film: HEVC, which Chrome needs transcoded; AAC; chapters with an
 * intro and credits; English and French SubRip, the English as ASS, and a
 * font attached for the ASS.
 */
function makeFilm(dir: string): string {
  const film = join(dir, "media", "Wanderers (2020)", "Wanderers (2020).mkv");
  mkdirSync(dirname(film), { recursive: true });
  const en = join(dir, "en.srt");
  const fr = join(dir, "fr.srt");
  const chapters = join(dir, "chapters.txt");
  writeFileSync(en, subrip("English"));
  writeFileSync(fr, subrip("French"));
  writeFileSync(
    chapters,
    ";FFMETADATA1\n" +
      "[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=20000\ntitle=Intro\n" +
      "[CHAPTER]\nTIMEBASE=1/1000\nSTART=20000\nEND=100000\ntitle=Story\n" +
      "[CHAPTER]\nTIMEBASE=1/1000\nSTART=100000\nEND=120000\ntitle=Credits\n",
  );
  // biome-ignore format: one line per input and output
  execFileSync(ffmpeg, [
    "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
    "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24",
    "-f", "lavfi", "-i", "sine=frequency=330:sample_rate=48000",
    "-i", en, "-i", fr, "-i", chapters,
    "-map", "0", "-map", "1", "-map", "2", "-map", "3", "-map", "2", "-map_chapters", "4", "-t", "120",
    // Grain and a 3 Mb/s target give the quality menu rungs below the original.
    "-vf", "noise=alls=40:allf=t", "-c:v", "libx265", "-preset", "ultrafast", "-b:v", "3M", "-pix_fmt", "yuv420p",
    "-x265-params", "log-level=error:keyint=48:min-keyint=48:scenecut=0",
    "-c:a", "aac", "-b:a", "96k", "-c:s:0", "srt", "-c:s:1", "srt", "-c:s:2", "ass",
    "-metadata:s:s:0", "language=eng", "-metadata:s:s:1", "language=fre",
    "-metadata:s:s:2", "language=eng", "-metadata:s:s:2", "title=Styled",
    "-attach", font, "-metadata:s:t:0", "mimetype=font/woff2",
    film,
  ]);
  // Settled, as a scan wants a file that isn't still being written.
  const old = new Date(Date.now() - 3_600_000);
  utimesSync(film, old, old);
  return dirname(dirname(film));
}

/** Chapters a minute each, in ffmpeg's metadata format. */
function chapterFile(dir: string, titles: string[]): string {
  const path = join(dir, `chapters-${titles.length}.txt`);
  writeFileSync(
    path,
    `;FFMETADATA1\n${titles
      .map((title, i) => `[CHAPTER]\nTIMEBASE=1/1000\nSTART=${i * 60_000}\nEND=${(i + 1) * 60_000}\ntitle=${title}\n`)
      .join("")}`,
  );
  return path;
}

/**
 * The audiobooks, in the layouts a real library has: a single chaptered
 * m4b under Author/Title, three minutes long so its last chapter starts past
 * the two minutes a book plays before it resumes, one under
 * Author/Series/Book 1.5 - Title, and a book of three MP3 files with track
 * tags and no chapters.
 */
function makeBooks(dir: string): string {
  const root = join(dir, "audiobooks");
  // biome-ignore format: one line per input and output
  const m4b = (rel: string, tags: Record<string, string>, chapters: string[]) => {
    const out = join(root, rel);
    mkdirSync(dirname(out), { recursive: true });
    execFileSync(ffmpeg, [
      "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
      "-f", "lavfi", "-i", `sine=frequency=440:sample_rate=22050:duration=${chapters.length * 60}`,
      "-i", chapterFile(dir, chapters), "-map", "0:a", "-map_chapters", "1",
      "-c:a", "aac", "-b:a", "24k",
      ...Object.entries(tags).flatMap(([k, v]) => ["-metadata", `${k}=${v}`]),
      out,
    ]);
    return out;
  };
  const files = [
    m4b(
      "Andy Weir/Project Hail Mary/Project Hail Mary.m4b",
      {
        album: "Project Hail Mary",
        artist: "Andy Weir",
        composer: "Ray Porter",
        date: "2021",
        description: "A lone astronaut.",
      },
      ["Opening Credits", "Chapter 1", "Chapter 2"],
    ),
    m4b(
      "James S. A. Corey/The Expanse/Book 1.5 - The Churn/The Churn.m4b",
      { album: "The Churn", artist: "James S. A. Corey", composer: "Ray Porter", date: "2014" },
      ["Chapter 1", "Chapter 2"],
    ),
  ];
  for (const n of [1, 2, 3]) {
    const out = join(root, "James S. A. Corey/The Expanse/Book 1 - Leviathan Wakes", `Leviathan Wakes ${n} of 3.mp3`);
    mkdirSync(dirname(out), { recursive: true });
    // biome-ignore format: one line per input and output
    execFileSync(ffmpeg, [
      "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
      "-f", "lavfi", "-i", "sine=frequency=330:sample_rate=22050:duration=20",
      "-c:a", "libmp3lame", "-b:a", "32k",
      "-metadata", "album=Leviathan Wakes", "-metadata", "artist=James S. A. Corey",
      "-metadata", "composer=Jefferson Mays", "-metadata", `track=${n}/3`,
      out,
    ]);
    files.push(out);
  }
  const old = new Date(Date.now() - 3_600_000);
  for (const f of files) utimesSync(f, old, old);
  return root;
}

function build(dir: string): string {
  if (process.env.KIVO_BIN) return process.env.KIVO_BIN;
  const bin = join(dir, "kivo");
  execFileSync("npm", ["run", "build"], { cwd: webDir, stdio: "inherit" });
  execFileSync("go", ["build", "-tags", "webui", "-o", bin, "./cmd/kivo"], { cwd: serverDir, stdio: "inherit" });
  return bin;
}

async function until<T>(what: string, seconds: number, check: () => Promise<T | undefined>): Promise<T> {
  const deadline = Date.now() + seconds * 1000;
  for (;;) {
    const value = await check().catch(() => undefined);
    if (value !== undefined) return value;
    if (Date.now() > deadline) throw new Error(`e2e setup: ${what} took over ${seconds} s`);
    await new Promise((r) => setTimeout(r, 250));
  }
}

export default async function globalSetup() {
  rmSync(e2e.dir, { recursive: true, force: true });
  mkdirSync(e2e.dir, { recursive: true });
  const bin = build(e2e.dir);
  const media = makeFilm(e2e.dir);
  const books = makeBooks(e2e.dir);

  const server = spawn(bin, [], {
    cwd: e2e.dir,
    detached: true,
    stdio: ["ignore", "inherit", "inherit"],
    env: {
      ...process.env,
      KIVO_LISTEN: `127.0.0.1:${e2e.port}`,
      KIVO_DATA_DIR: join(e2e.dir, "data"),
      KIVO_CACHE_DIR: join(e2e.dir, "cache"),
      KIVO_BACKUP_DIR: join(e2e.dir, "backups"),
      KIVO_FFMPEG: ffmpeg,
      KIVO_FFPROBE: ffprobe,
      KIVO_LOG_LEVEL: "warn",
      // Books are described from their files alone: no test reaches Audible.
      KIVO_AUDIBLE: "false",
    },
  });
  writeFileSync(e2e.pid, String(server.pid));
  server.unref();

  const api = await request.newContext({ baseURL: e2e.baseURL, extraHTTPHeaders: { Origin: e2e.baseURL } });
  await until("the server starting", 30, async () => ((await api.get("/healthz")).ok() ? true : undefined));
  const setup = await api.post("/api/v1/setup", { data: { username: "admin", password } });
  if (!setup.ok()) throw new Error(`e2e setup: setup: ${setup.status()} ${await setup.text()}`);
  const library = await api.post("/api/v1/libraries", {
    data: { name: "Movies", kind: "movies", folders: [media], previews: "nightlyAndAdded" },
  });
  if (!library.ok()) throw new Error(`e2e setup: library: ${library.status()} ${await library.text()}`);
  const { id: libraryId } = (await library.json()) as { id: string };

  const workId = await until("the scan", 60, async () => {
    const read = await api.post("/api/v1/works/query", { data: { query: { kinds: ["movie"] }, library: libraryId } });
    const page = (await read.json()) as { items: { id: string }[] };
    return page.items[0]?.id;
  });

  // The film's subtitle tracks, by their IDs: English and French SubRip,
  // then English ASS, in the file's order.
  const detail = (await (await api.get(`/api/v1/works/${workId}`)).json()) as {
    versions: { tracks: { id: string; type: string }[] }[];
  };
  const [english, french, styled] = (detail.versions[0]?.tracks ?? [])
    .filter((t) => t.type === "subtitle")
    .map((t) => t.id);
  if (!english || !french || !styled) throw new Error("e2e setup: the film's subtitle tracks are missing");

  // Analysis runs in the background as files are added: wait for the
  // subtitle preparation and the scrub previews, asking as a client that
  // plays the file directly and draws ASS, so every subtitle track shows
  // as an external file, the ASS styled with its font.
  const both = ["file", "stream"];
  const direct = {
    file: { containers: ["mkv"] },
    video: [{ codec: "hevc", delivery: both }],
    audio: [{ codec: "aac", delivery: both, maxChannels: 2 }],
    subtitles: [
      { codec: "webvtt", delivery: ["stream", "external"] },
      { codec: "ass", delivery: ["external"] },
    ],
  };
  await until("subtitle preparation and previews", 120, async () => {
    const res = await api.post("/api/v1/plays", {
      data: { workId, capabilities: direct, subtitleTrackId: french },
    });
    const play = (await res.json()) as {
      id: string;
      tracks: { trackId: string; type: string; delivery?: string; fonts?: string[] }[];
      previews?: { ready: boolean };
    };
    await api.post(`/api/v1/plays/${play.id}/stop`, { data: {} });
    const subtitles = play.tracks.filter((t) => t.type === "subtitle" && t.trackId !== "off");
    const prepared =
      subtitles.length === 3 &&
      subtitles.every((t) => t.delivery === "external") &&
      subtitles.find((t) => t.trackId === styled)?.fonts?.length === 1;
    return prepared && play.previews?.ready ? true : undefined;
  });

  // The audiobooks: each folder one book, described from its folders and
  // tags once identified.
  const shelf = await api.post("/api/v1/libraries", {
    data: { name: "Audiobooks", kind: "audiobooks", folders: [books] },
  });
  if (!shelf.ok()) throw new Error(`e2e setup: audiobooks: ${shelf.status()} ${await shelf.text()}`);
  const { id: audiobooksId } = (await shelf.json()) as { id: string };
  type Card = { id: string; title: string; book?: { authors: unknown[]; series: unknown[] } };
  const byTitle = await until("the audiobooks scan", 90, async () => {
    const read = await api.post("/api/v1/works/query", { data: { query: { kinds: ["book"] }, library: audiobooksId } });
    const page = (await read.json()) as { items: Card[] };
    const ready = page.items.length === 3 && page.items.every((c) => (c.book?.authors.length ?? 0) > 0);
    return ready ? new Map(page.items.map((c) => [c.title, c.id])) : undefined;
  });

  await api.storageState({ path: e2e.storageState });
  await api.dispose();
  const fixture: Fixture = {
    workId,
    english,
    french,
    styled,
    audiobooksId,
    hailMaryId: byTitle.get("Project Hail Mary") ?? "",
    churnId: byTitle.get("The Churn") ?? "",
    leviathanId: byTitle.get("Leviathan Wakes") ?? "",
  };
  writeFileSync(e2e.fixture, JSON.stringify(fixture));
}
