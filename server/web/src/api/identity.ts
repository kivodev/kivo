import { browserOf, deviceModel, osVersion } from "@/lib/browser";
import { isPackaged } from "@/tv/flag";
import { appVersion } from "@/tv/versions";

// The identity headers, which say which app and device sends a request.
// The server keeps it with the session and the plays it starts, for Now
// playing and Where you're signed in. It names the browser coarsely, by
// family and major version, and never the screen or the device's name.

/**
 * The client's version, which its requests and error reports name. The
 * packaged TV app has a release of its own; the web client is its server's,
 * which ships it and writes its version into the page's kivo-version tag.
 */
export const clientVersion: string = isPackaged
  ? appVersion
  : document.querySelector<HTMLMetaElement>('meta[name="kivo-version"]')?.content || "dev";

/**
 * The identity headers, a header per field, leaving out what this browser
 * can't say. The packaged TV app is one build for both TVs, so it is the LG
 * TV app where the user agent names webOS, and the Samsung TV app elsewhere.
 */
export function identityHeaders(userAgent = navigator.userAgent): Record<string, string> {
  const platform = !isPackaged ? "browser" : browserOf(userAgent, navigator.vendor).webos ? "webos" : "tizen";
  const fields: [string, string | undefined][] = [
    ["Kivo-App", isPackaged ? `kivo-${platform}` : "kivo-web"],
    ["Kivo-App-Version", clientVersion],
    ["Kivo-Platform", platform],
    ["Kivo-OS-Version", osVersion(userAgent)],
    ["Kivo-Device-Model", deviceModel(userAgent)],
  ];
  return Object.fromEntries(fields.filter((f): f is [string, string] => Boolean(f[1])));
}
