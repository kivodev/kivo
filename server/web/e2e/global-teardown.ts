import { readFileSync, rmSync } from "node:fs";
import { e2e } from "./env";

/** Stops the server the setup started and removes everything it made. */
export default async function globalTeardown() {
  try {
    const pid = Number(readFileSync(e2e.pid, "utf8"));
    // The server's own process group: it ends its ffmpeg jobs with it.
    process.kill(-pid, "SIGTERM");
    for (let i = 0; i < 100; i++) {
      process.kill(pid, 0);
      await new Promise((r) => setTimeout(r, 100));
    }
  } catch {
    // Gone already, or never started.
  }
  if (!process.env.KIVO_E2E_KEEP) rmSync(e2e.dir, { recursive: true, force: true });
}
