import display from "@shared/display/en.json";
import media from "@shared/display/media.json";

// The display tables that the server and the native apps share: methods,
// subtitle kinds, reason texts, dialogue boost costs, track words and
// formats (display), and codec, container and channel names (media).
export { display, media };

/** A display table's entry for key, or undefined when it has none. */
export const lookup = <T>(table: Record<string, T>, key: string): T | undefined =>
  Object.hasOwn(table, key) ? table[key] : undefined;
