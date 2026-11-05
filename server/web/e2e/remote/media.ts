import { resolve } from "node:path";

/** The run's film, in the dev server's tree so it serves it, with ranges, at `filmUrl`. */
export const mediaDir = resolve(import.meta.dirname, "media");
export const filmPath = resolve(mediaDir, "film.webm");
export const filmUrl = "/e2e/remote/media/film.webm";
