import { useLingui } from "@lingui/react/macro";
import type { ActivityGroup } from "@/api/schemas";

/**
 * What fixes a problem. It has a sentence, and the admin page to fix it
 * on, if any, with the page's filter and the word for a link to it.
 */
export type ProblemAction = {
  text: string;
  page?:
    | "/admin/libraries"
    | "/admin/logs"
    | "/admin/settings/general"
    | "/admin/settings/transcoding"
    | "/admin/activity";
  search?: { show: ActivityGroup };
  link?: string;
};

/**
 * The fix for each problem code the server raises. The problem's own
 * message says what's wrong, and this says what to do. A code this client
 * doesn't know has no action.
 */
export function useProblemActions(): (code: string) => ProblemAction | undefined {
  const { t } = useLingui();
  const actions: Record<string, ProblemAction> = {
    folderUnreachable: {
      text: t`Check that the disk or share is mounted, then scan the library again.`,
      page: "/admin/libraries",
    },
    watchLimit: { text: t`Raise fs.inotify.max_user_watches on the host to watch every folder.` },
    tmdbKeyRejected: { text: t`Set a working tmdb_key in server.toml or KIVO_TMDB_KEY, then restart the server.` },
    openSubtitlesNoKey: {
      text: t`Enter an OpenSubtitles API key in General settings, or turn off subtitle downloads for the library.`,
      page: "/admin/settings/general",
    },
    openSubtitlesAccountRejected: {
      text: t`Re-enter the OpenSubtitles username and password in General settings.`,
      page: "/admin/settings/general",
    },
    symlinkSkipped: { text: t`Move the symlink targets into the library folder, or remove the symlinks.` },
    databaseNotLocal: { text: t`Move the data folder to a local disk.` },
    gpuUnusable: {
      text: t`See the fix in Transcoding settings.`,
      page: "/admin/settings/transcoding",
    },
    noAccelerator: {
      text: t`Turn hardware acceleration back on in Transcoding settings.`,
      page: "/admin/settings/transcoding",
    },
    acceleratorMissing: {
      text: t`In Transcoding settings, choose Automatic or an accelerator that passed testing.`,
      page: "/admin/settings/transcoding",
    },
    gpuFailed: {
      text: t`If this repeats, update the driver or choose another accelerator.`,
      page: "/admin/settings/transcoding",
    },
    cpuFailed: { text: t`See the test encode error in Logs.`, page: "/admin/logs" },
    cacheFull: { text: t`Free up space on the cache disk.` },
    signInsFailed: {
      text: t`See who tried in Activity. If it wasn't them, change the user's password.`,
      page: "/admin/activity",
      search: { show: "signIns" },
      link: t`Review`,
    },
  };
  return (code) => {
    if (code in actions) return actions[code];
    // Provider problems are named after the provider, such as
    // openSubtitlesKeyRejected. An unavailable provider, such as
    // tvdbUnavailable, clears itself and has no action.
    if (code.endsWith("KeyRejected"))
      return { text: t`Re-enter the provider's API key in General settings.`, page: "/admin/settings/general" };
    return undefined;
  };
}
