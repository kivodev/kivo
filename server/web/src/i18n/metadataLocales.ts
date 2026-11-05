import { i18n } from "@lingui/core";

// The metadata languages and content rating countries that the setup
// wizard, Library settings and Edit library offer, named in the viewer's
// locale.

const languages = [
  "en-US",
  "en-GB",
  "de-DE",
  "fr-FR",
  "es-ES",
  "es-MX",
  "it-IT",
  "nl-NL",
  "pt-BR",
  "pt-PT",
  "sv-SE",
  "nb-NO",
  "da-DK",
  "fi-FI",
  "pl-PL",
  "cs-CZ",
  "ru-RU",
  "ja-JP",
  "ko-KR",
  "zh-CN",
  "zh-TW",
];

const countries = [
  "US",
  "GB",
  "CA",
  "AU",
  "NZ",
  "IE",
  "DE",
  "AT",
  "CH",
  "FR",
  "ES",
  "MX",
  "IT",
  "NL",
  "BE",
  "SE",
  "NO",
  "DK",
  "FI",
  "PL",
  "CZ",
  "BR",
  "PT",
  "JP",
  "KR",
  "IN",
];

/** The metadata defaults from the browser's locale, such as `de-AT` → de-AT and AT. */
export function preferredMetadataDefaults(browserLocale: string): { language: string; country: string } {
  try {
    const locale = new Intl.Locale(browserLocale).maximize();
    const region = locale.region ?? "US";
    return { language: `${locale.language}-${region}`, country: region };
  } catch {
    return { language: "en-US", country: "US" };
  }
}

function withSelected(list: string[], selected: string): string[] {
  return selected === "" || list.includes(selected) ? list : [selected, ...list];
}

export function metadataLanguages(selected: string) {
  const names = new Intl.DisplayNames([i18n.locale], { type: "language" });
  return withSelected(languages, selected).map((id) => ({ id, label: names.of(id) ?? id }));
}

export function contentRatingCountries(selected: string) {
  const names = new Intl.DisplayNames([i18n.locale], { type: "region" });
  return withSelected(countries, selected).map((id) => ({ id, label: names.of(id) ?? id }));
}
