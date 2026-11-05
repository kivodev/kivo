import { useLingui } from "@lingui/react/macro";

/**
 * The TV build's splash: the Kivo icon on the theme's background, with a
 * spinner, while the app starts. The TV build's index.html has the same
 * markup in #root, which shows before the app's code has loaded (see
 * vite.config.ts), and its styles (splash.css). It takes over from that
 * copy as React replaces it, so the screen doesn't go blank between them.
 */
export function TvSplash() {
  const { t } = useLingui();
  return (
    <div data-part="tv-splash" role="status" aria-label={t`Kivo is starting`}>
      <img src="icon.svg" alt="" />
      <span data-part="tv-splash-spinner" />
    </div>
  );
}
