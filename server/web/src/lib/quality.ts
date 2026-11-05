import type { QualityRequest } from "@/api/schemas";

/** A quality as a menu option's ID, which is `auto`, `original` or a rung's bitrate. */
export const qualityKey = (q: QualityRequest | undefined): string =>
  q?.mode === "limit" ? String(q.maxBitrateBps) : (q?.mode ?? "auto");

/** The quality a menu option's ID stands for. It is undefined for Auto and anything unknown. */
export function qualityOf(key: string | null | undefined): QualityRequest | undefined {
  if (key === "original") return { mode: "original" };
  const bitrate = Number(key);
  return Number.isSafeInteger(bitrate) && bitrate > 0 ? { mode: "limit", maxBitrateBps: bitrate } : undefined;
}
