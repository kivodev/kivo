import { tmpdir } from "node:os";
import { join } from "node:path";

// Where the end-to-end run keeps its server, known before the global setup
// starts it: KIVO_E2E_PORT, or 48484.
const port = Number(process.env.KIVO_E2E_PORT ?? 48484);
const state = join(tmpdir(), `kivo-e2e-${port}`);

export const e2e = {
  port,
  baseURL: `http://127.0.0.1:${port}`,
  /** The run's own directory: the binary, media, data and cache. */
  dir: state,
  /** The signed-in admin's cookie, for every test. */
  storageState: join(state, "storage-state.json"),
  /** What the setup made, for the tests: see Fixture. */
  fixture: join(state, "fixture.json"),
  /** The server's process ID, for the teardown. */
  pid: join(state, "server.pid"),
};

/** What the global setup made. */
export type Fixture = {
  /** A two-minute HEVC film with intro and credits chapters, English and French SubRip, and English ASS with a font. */
  workId: string;
  /** Its subtitle tracks' IDs. */
  english: string;
  french: string;
  styled: string;
  /** An audiobooks library of three books: Project Hail Mary (one m4b), and The Churn (Book 1.5) and Leviathan Wakes (three MP3 files, Book 1) of The Expanse. */
  audiobooksId: string;
  hailMaryId: string;
  churnId: string;
  leviathanId: string;
};
