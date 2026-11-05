// Migration (api-naming): remove after every client has updated past this release.
// What an earlier version of this client kept in the browser's storage under
// names the server has since changed, read once in the names it uses now.
import { readStored, writeStored } from "@/lib/storage";

// Migration (api-naming): remove after every client has updated past this release.
const legacyLocalQualityKey = "kivo.quality.home";

// Migration (api-naming): remove after every client has updated past this release.
/** Moves a local quality kept under its earlier key to `localQualityKey`, once. */
export function migrateLocalQuality(localQualityKey: string): void {
  const legacy = readStored(legacyLocalQualityKey);
  if (legacy === undefined) return;
  if (readStored(localQualityKey) === undefined) writeStored(localQualityKey, legacy);
  writeStored(legacyLocalQualityKey, undefined);
}

// Migration (api-naming): remove after every client has updated past this release.
/** A library's kept view, its filters once kept as `hdr` and `match` and its last-episode sort as `lastEpisodeAdded`. */
export function migrateStoredBrowse(stored: unknown): unknown {
  if (!stored || typeof stored !== "object" || Array.isArray(stored)) return stored;
  const { hdr, match, ...rest } = stored as Record<string, unknown>;
  const out: Record<string, unknown> = { ...rest };
  if (hdr !== undefined && out.dynamicRange === undefined) out.dynamicRange = hdr;
  if (match !== undefined && out.matchState === undefined) out.matchState = match;
  if (out.sort === "lastEpisodeAdded") out.sort = "lastEpisodeAddedAt";
  return out;
}
