import { i18n } from "@lingui/core";
import en from "@shared/display/en.json";
import media from "@shared/display/media.json";

// The display tables that the server and the native apps share. Each
// locale has its own methods, subtitle kinds, reason texts, dialogue boost
// costs and track words. Codec, container and channel names are shared by
// every locale. English is the source locale. More tables are added as
// their catalogs are translated.
type Display = typeof en;
const tables: Record<string, Display> = { en };

/** The active locale's display table. */
export function display(): Display {
  return tables[i18n.locale] ?? en;
}

export { media };

/** A display table's entry for key, or undefined when it has none. */
export const lookup = <T>(table: Record<string, T>, key: string): T | undefined =>
  Object.hasOwn(table, key) ? table[key] : undefined;
