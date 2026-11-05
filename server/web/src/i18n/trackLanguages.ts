import { i18n } from "@lingui/core";

// The audio and subtitle languages that Settings and Edit library offer.
// They are base languages, because playback compares tracks by base
// language, and they are named in the viewer's locale.

const languages = [
  "ar",
  "cs",
  "da",
  "de",
  "el",
  "en",
  "es",
  "fi",
  "fr",
  "he",
  "hi",
  "hu",
  "id",
  "it",
  "ja",
  "ko",
  "nl",
  "no",
  "pl",
  "pt",
  "ro",
  "ru",
  "sv",
  "th",
  "tr",
  "uk",
  "vi",
  "zh",
];

/** The languages by name, with `selected` among them when it's set. */
export function trackLanguages(selected: string | undefined) {
  const names = new Intl.DisplayNames([i18n.locale], { type: "language" });
  const list = selected && !languages.includes(selected) ? [selected, ...languages] : languages;
  return list
    .map((id) => ({ id, label: names.of(id) ?? id }))
    .sort((a, b) => a.label.localeCompare(b.label, i18n.locale));
}
