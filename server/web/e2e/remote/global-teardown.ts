import { rmSync } from "node:fs";
import { mediaDir } from "./media";

/** Removes the film. */
export default function globalTeardown() {
  rmSync(mediaDir, { recursive: true, force: true });
}
