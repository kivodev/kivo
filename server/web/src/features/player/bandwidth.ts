import { bandwidthTest } from "@/api/playback";
import type { QualityRequest } from "@/api/schemas";
import { readStoredJson, writeStoredJson } from "@/lib/storage";

// The download rate that Auto quality goes by. It is the last rate measured
// away from the server's local networks within the hour, kept in the
// browser. Without one, a timed download of the server's bandwidth test runs
// before the play. A local client skips the test.

const key = "kivo.bandwidth";
/** The test's size. It lasts a few seconds at the rates where remote quality matters. */
const testBytes = 5_000_000;
/** The test stops after this long and counts what arrived. */
const testMs = 3000;
/** How long a measured rate stays valid for the connection. */
const validMs = 60 * 60 * 1000;

/** The download rate this browser measured within the hour, in bits per second. */
export function lastBandwidth(): number | undefined {
  const kept = readStoredJson(key) as { bps?: unknown; at?: unknown } | null | undefined;
  if (typeof kept?.bps !== "number" || typeof kept.at !== "number" || kept.bps <= 0) return undefined;
  return Date.now() - kept.at < validMs ? kept.bps : undefined;
}

/** Keeps a rate measured away from the server's local networks for the next hour's plays. */
export function rememberBandwidth(bps: number): void {
  writeStoredJson(key, { bps: Math.round(bps), at: Date.now() });
}

/**
 * Times one download of the bandwidth test, for at most `testMs`, and keeps
 * the rate. It returns undefined for a local client, or when nothing arrived.
 */
export async function measureBandwidth(): Promise<number | undefined> {
  const abort = new AbortController();
  const timer = setTimeout(() => abort.abort(), testMs);
  const started = performance.now();
  let bytes = 0;
  try {
    const stream = await bandwidthTest(testBytes, abort.signal);
    if (!stream) return undefined;
    const reader = stream.getReader();
    for (let chunk = await reader.read(); !chunk.done; chunk = await reader.read()) bytes += chunk.value.byteLength;
  } catch {
    // On the deadline or a failure, what arrived still counts.
  } finally {
    clearTimeout(timer);
  }
  if (bytes === 0) return undefined;
  const bps = Math.round((bytes * 8 * 1000) / Math.max(1, performance.now() - started));
  rememberBandwidth(bps);
  return bps;
}

/** A quality as a play sends it. Auto carries the download rate when there is one. */
export function withBandwidth(
  quality: QualityRequest | undefined,
  bps: number | undefined,
): QualityRequest | undefined {
  return quality?.mode === "auto" && bps !== undefined ? { ...quality, bandwidthBps: bps } : quality;
}
