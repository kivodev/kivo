import { apiBaseUrl } from "./client";
import { apiPaths } from "./paths";
import type { ImageRef } from "./schemas";

/**
 * The URL of a pre-resized image for an <img>. `cssWidth` is the displayed
 * width. The server picks the smallest stored size at least that wide at
 * the device's pixel ratio. If `maxDensity` is lower than that ratio, it
 * is used instead.
 */
export function imageUrl(image: ImageRef, cssWidth: number, maxDensity = Number.POSITIVE_INFINITY): string {
  const width = Math.ceil(cssWidth * Math.min(globalThis.devicePixelRatio || 1, maxDensity));
  const path = apiPaths.image.replace("{id}", encodeURIComponent(image.id));
  return `${apiBaseUrl()}${path}?width=${width}`;
}
