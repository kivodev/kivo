// The build's open source licenses: licenses.js beside index.html, which
// vite.config.ts writes from the bundle (openSourceLicenses).

/** A package in the bundle, a font, or terms the bundle can't report. */
export type Credit = {
  name: string;
  version?: string;
  /** An SPDX expression, or the terms' name. */
  license: string;
  /** Where the terms are, for those with no text of their own. */
  url?: string;
  text?: string;
};

/** The build's licenses, read when the list opens. The dev server has none, and the read fails. */
export async function loadLicenses(): Promise<Credit[]> {
  const url = new URL("licenses.js", document.baseURI).href;
  const module: { default: Credit[] } = await import(/* @vite-ignore */ url);
  return module.default;
}
