import JASSUB from "jassub";
import wasmUrl from "jassub/dist/wasm/jassub-worker.wasm?url";
import modernWasmUrl from "jassub/dist/wasm/jassub-worker-modern.wasm?url";
import jassubWorkerUrl from "jassub/dist/worker/worker.js?worker&url";
import { PgsRenderer } from "libpgs";
import pgsWorkerUrl from "libpgs/dist/libpgs.worker.js?url";
import fallbackCjkUrl from "./fonts/kivo-fallback-cjk.woff2?url";
import fallbackLatinUrl from "./fonts/kivo-fallback-latin-400.woff2?url";
import fallbackLatinBoldUrl from "./fonts/kivo-fallback-latin-700.woff2?url";
import { frameClock } from "./frameClock";

// The subtitle renderers for files the browser can't show as text tracks.
// JASSUB (libass in WebAssembly, in a worker) draws styled ASS with the
// file's fonts, and libpgs draws PGS pictures. Each draws on its own canvas
// over the video, kept in step with the video's time. This module is loaded
// only when a play shows one of them.

/** Stops a renderer and removes its canvas. */
export type StopRenderer = () => void;

/**
 * The family name of the bundled fallback fonts (see fonts/OFL.txt). It
 * holds Arimo, which is metric-compatible with Arial, for Latin, Greek and
 * Cyrillic. It also holds a subset of Noto Sans JP for kana and common kanji
 * and hanzi, renamed into the same family. When the file doesn't carry a
 * font, libass draws the text in its default family and picks among that
 * family's fonts by glyph.
 */
const fallbackFamily = "Kivo Fallback";
const fallbackFonts = [fallbackLatinUrl, fallbackLatinBoldUrl, fallbackCjkUrl].map(
  (u) => new URL(u, location.href).href,
);

/**
 * Draws an ASS file with its styling. The file's fonts and the fallback
 * fonts load before the first frame. The browser caches every font
 * permanently.
 */
export function drawAss(
  video: HTMLVideoElement,
  canvas: HTMLCanvasElement,
  url: string,
  fonts: readonly string[],
): StopRenderer {
  const instance = new JASSUB({
    video,
    canvas,
    subUrl: url,
    workerUrl: jassubWorkerUrl,
    wasmUrl,
    modernWasmUrl,
    fonts: [...fonts, ...fallbackFonts],
    defaultFont: fallbackFamily,
    // Nothing loads by name later, because every font is loaded above.
    availableFonts: {},
    // Never ask for the viewer's own fonts, nor look any up online.
    queryFonts: false,
  });
  let ready = false;
  void instance.ready.then(() => {
    ready = true;
  });
  const stopClock = frameClock(video, (frame) => {
    if (ready) void instance.manualRender(frame);
  });
  return () => {
    stopClock();
    void instance.destroy();
  };
}

/** Draws a PGS file's pictures. */
export function drawPgs(video: HTMLVideoElement, canvas: HTMLCanvasElement, url: string): StopRenderer {
  const renderer = new PgsRenderer({ video, canvas, subUrl: url, workerUrl: pgsWorkerUrl, aspectRatio: "contain" });
  return () => renderer.dispose();
}
