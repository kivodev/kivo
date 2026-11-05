import type { PreviewManifest } from "@/api/schemas";

/** Where a position's preview frame is: its sheet, and the frame's offset in it. */
export type SpriteFrame = { sheet: string; x: number; y: number; width: number; height: number };

/**
 * The scrub-preview frame for a position. Each sheet holds `columns` by
 * `rows` frames, with one frame every `intervalMs`. Sheet URLs are resolved
 * against the manifest's own URL.
 */
export function spriteFrame(
  manifest: PreviewManifest,
  manifestUrl: string,
  positionMs: number,
): SpriteFrame | undefined {
  const perSheet = manifest.columns * manifest.rows;
  if (perSheet <= 0 || manifest.intervalMs <= 0) return undefined;
  const frame = Math.max(0, Math.floor(positionMs / manifest.intervalMs));
  const sheet = manifest.sheets[Math.floor(frame / perSheet)] ?? manifest.sheets.at(-1);
  if (!sheet) return undefined;
  const within = Math.min(frame, manifest.sheets.length * perSheet - 1) % perSheet;
  return {
    sheet: new URL(sheet, manifestUrl).href,
    x: (within % manifest.columns) * manifest.width,
    y: Math.floor(within / manifest.columns) * manifest.height,
    width: manifest.width,
    height: manifest.height,
  };
}
