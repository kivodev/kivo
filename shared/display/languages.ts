// Writes languages/en.json, the names track names give languages, and
// languages/aliases.json, the codes each tag's language is canonicalized
// from, from the CLDR data in Node's ICU, as browsers name them with
// Intl.DisplayNames. Run with Node 24 after upgrading Node:
// `node shared/display/languages.ts`.
import { mkdirSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const out = fileURLToPath(new URL("languages/", import.meta.url));

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
function names(type: "language" | "region" | "script", list: string[]): Record<string, string> {
  const display = new Intl.DisplayNames(["en"], { type, fallback: "none" });
  const found: Record<string, string> = {};
  for (const code of list) {
    const name = display.of(code);
    if (name) found[code] = name;
  }
  return found;
}

mkdirSync(out, { recursive: true });
const display = new Intl.DisplayNames(["en"], { type: "language", fallback: "none" });
const languages = names("language", languageCodes);
const regions = names("region", regionCodes);
const scripts = names("script", scriptCodes);

// A language with a script and a region, named piece by piece, shows how
// English puts the pieces together: "Korean (Latin, United States)".
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
writeFileSync(`${out}en.json`, `${JSON.stringify(table, null, 2)}\n`);
writeFileSync(`${out}aliases.json`, `${JSON.stringify(sorted(aliases), null, 2)}\n`);
