// The packaged TV app's version: the app has a release of its own, apart
// from its server's.

/**
 * The app's version, the release it was built from (clients/tizen's
 * config.xml), which the packaged build sets as VITE_KIVO_APP_VERSION.
 */
export const appVersion: string = import.meta.env.VITE_KIVO_APP_VERSION || "dev";
