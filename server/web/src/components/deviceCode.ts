import { useLingui } from "@lingui/react/macro";

/** A link code as shown, in two groups of three, such as "K7Q-2M9". */
export function groupedCode(code: string): string {
  return `${code.slice(0, 3)}-${code.slice(3)}`;
}

/** Names for what a client runs on, such as "Android TV". A platform this client doesn't know has none. */
export function usePlatformName(): (platform: string | undefined) => string | undefined {
  const { t } = useLingui();
  const names: Record<string, string> = {
    androidTv: t`Android TV`,
    appleTv: t`Apple TV`,
    iPhone: t`iPhone`,
    iPad: t`iPad`,
    androidPhone: t`Android phone`,
    browser: t`Browser`,
  };
  return (platform) => (platform === undefined ? undefined : names[platform]);
}
