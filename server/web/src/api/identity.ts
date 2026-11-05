import { deviceModel, osVersion } from "@/lib/browser";
import { isPackaged } from "@/tv/flag";
import { appVersion } from "@/tv/versions";

// The identity headers, which say which app and device sends a request.
// The server keeps it with the session and the plays it starts, for Now
// playing and Where you're signed in. It names the browser coarsely, by
// family and major version, and never the screen or the device's name.

/**
 * The client's build: the time it was built, which the build sets as
 * VITE_KIVO_BUILD, as the client has no version of its own apart from the
 * server it's embedded in.
 */
export const buildVersion: string = import.meta.env.DEV ? "dev" : import.meta.env.VITE_KIVO_BUILD;

/**
 * The identity headers, a header per field, leaving out what this browser
 * can't say. The packaged TV app is an app of its own, with its release's
 * version.
 */
export function identityHeaders(userAgent = navigator.userAgent): Record<string, string> {
  const fields: [string, string | undefined][] = [
    ["Kivo-App", isPackaged ? "kivo-tizen" : "kivo-web"],
    ["Kivo-App-Version", isPackaged ? appVersion : buildVersion],
    ["Kivo-Platform", isPackaged ? "tizen" : "browser"],
    ["Kivo-OS-Version", osVersion(userAgent)],
    ["Kivo-Device-Model", deviceModel(userAgent)],
  ];
  return Object.fromEntries(fields.filter((f): f is [string, string] => Boolean(f[1])));
}
