// Apart from packaged.ts, so its tests can stand in for it: jsdom loads no
// pages.

/**
 * Loads the packaged app's page again, from its first screen, as when the
 * app starts. Its routes are in the URL's hash, so the page without one
 * opens on the first screen, and going to it loads the page anew.
 */
export function restartApp() {
  location.replace(location.href.replace(/#.*$/, ""));
}
