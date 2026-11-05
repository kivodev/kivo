// Writes languages/<locale>.json, the names the clients give languages in
// track names, and languages/aliases.json, the codes each tag's language is
// canonicalized from, from the CLDR data in Node's ICU, as browsers name
// them with Intl.DisplayNames. Run with Node 24 after adding a display
// locale or upgrading Node: `node shared/display/languages.ts`.
import { mkdirSync, readdirSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const here = new URL(".", import.meta.url);
const out = fileURLToPath(new URL("languages/", here));
const locales = readdirSync(here)
  .filter((f) => /^[a-z]{2,3}(-[A-Za-z0-9]+)*\.json$/.test(f))
  .map((f) => f.replace(/\.json$/, ""));

const letters = "abcdefghijklmnopqrstuvwxyz";
const codes = (length: number): string[] =>
  length === 0 ? [""] : codes(length - 1).flatMap((c) => [...letters].map((l) => c + l));
const upper = (s: string) => s.toUpperCase();
const title = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const canonical = (tag: string) => {
  try {
    return Intl.getCanonicalLocales(tag)[0];
  } catch {
    return undefined;
  }
};

const languageCodes: string[] = [];
const aliases: Record<string, string> = {};
for (const code of [...codes(2), ...codes(3)]) {
  const c = canonical(code);
  if (c === code) languageCodes.push(code);
  else if (c) aliases[code] = c;
}
const regionCodes = [
  ...codes(2).map(upper),
  ...Array.from({ length: 1000 }, (_, i) => String(i).padStart(3, "0")),
];
const scriptCodes = codes(4).map(title);

const sorted = <T>(o: Record<string, T>) => Object.fromEntries(Object.entries(o).sort(([a], [b]) => (a < b ? -1 : 1)));
function names(locale: string, type: "language" | "region" | "script", list: string[]): Record<string, string> {
  const display = new Intl.DisplayNames([locale], { type, fallback: "none" });
  const found: Record<string, string> = {};
  for (const code of list) {
    const name = display.of(code);
    if (name) found[code] = name;
  }
  return found;
}

mkdirSync(out, { recursive: true });
for (const locale of locales) {
  const display = new Intl.DisplayNames([locale], { type: "language", fallback: "none" });
  const languages = names(locale, "language", languageCodes);
  const regions = names(locale, "region", regionCodes);
  const scripts = names(locale, "script", scriptCodes);

  // A language with a script and a region, named piece by piece, shows how
  // the locale puts the pieces together: "Korean (Latin, United States)".
  const template = (display.of("ko-Latn-US") as string)
    .replace(languages.ko as string, "{0}")
    .replace(scripts.Latn as string, "{1}")
    .replace(regions.US as string, "{2}");
  const pieces = /\{1\}.*\{2\}/.exec(template)?.[0] as string;
  const pattern = template.replace(pieces, "{1}");
  const separator = pieces.replace("{1}", "{0}").replace("{2}", "{1}");
  // Parentheses inside a piece become brackets: "Afar (Myanmar [Burma])".
  const compose = (language: string, piece: string) =>
    pattern.replace("{0}", language).replace("{1}", piece.replaceAll("(", "[").replaceAll(")", "]"));

  // A language with only a script or only a region can have a name of its
  // own, a dialect: "American English", "Simplified Chinese".
  for (const language of Object.keys(languages)) {
    for (const [code, piece] of [...Object.entries(regions), ...Object.entries(scripts)]) {
      const tag = `${language}-${code}`;
      const name = display.of(tag);
      if (name && canonical(tag) === tag && name !== compose(languages[language] as string, piece)) {
        languages[tag] = name;
      }
    }
  }
  const table = { pattern, separator, languages: sorted(languages), scripts: sorted(scripts), regions: sorted(regions) };
  writeFileSync(`${out}${locale}.json`, `${JSON.stringify(table, null, 2)}\n`);
}
writeFileSync(`${out}aliases.json`, `${JSON.stringify(sorted(aliases), null, 2)}\n`);
