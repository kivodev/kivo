import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { filmPath, mediaDir } from "./media";

/**
 * Makes the film: 44 minutes and 10 seconds of a small test pattern in
 * VP8, as long as the mock's episode of The Expanse, with a keyframe every
 * two seconds so a seek lands where it's asked. KIVO_FFMPEG chooses the
 * tool (default: ffmpeg on the PATH).
 */
export default function globalSetup() {
  mkdirSync(mediaDir, { recursive: true });
  // biome-ignore format: one line per input and output
  execFileSync(process.env.KIVO_FFMPEG ?? "ffmpeg", [
    "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
    "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=4",
    "-t", "2650", "-c:v", "libvpx", "-b:v", "30k", "-g", "8", "-an",
    filmPath,
  ]);
}
