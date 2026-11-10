// Whether the packaged TV app and its server work together. The app has a
// release of its own, apart from its server's, so each says the oldest of
// the other it works with: the server in its info, the app here.
import { appVersion } from "./versions";

/**
 * The oldest server release the packaged app works with. A server older
 * than it is shown as needing an update, rather than called. Raise it in
 * the change that first needs a newer server, to the release that server
 * ships in.
 */
export const minServerVersion = "0.3.0";

/** How the app and a server stand: they work together, or which of them is too old, with the versions to say. */
export type Compatibility =
  | { state: "compatible" }
  | { state: "serverTooOld"; serverVersion: string; minServerVersion: string; appVersion: string }
  | { state: "appTooOld"; appVersion: string; minAppVersion: string };

/** A version's major, minor and patch, as in v0.4.0 or 0.4.0-dev+abc1234, or undefined without all three. */
function parse(version: string): [number, number, number] | undefined {
  const parts = version
    .trim()
    .replace(/^v/, "")
    .replace(/[-+].*$/, "")
    .split(".");
  if (parts.length !== 3 || !parts.every((part) => /^\d+$/.test(part))) return undefined;
  const [major, minor, patch] = parts.map(Number);
  return [major as number, minor as number, patch as number];
}

/**
 * Compares two versions by major, minor and patch: negative when `a` is
 * older than `b`, zero when they're the same release, positive when newer.
 * A leading v and anything from the first hyphen or plus on are ignored,
 * so a dev build or a prerelease is never older than the release it
 * names. Undefined when either lacks the three numbers, as "dev", a commit
 * hash or an empty version do.
 */
export function compareVersions(a: string, b: string): number | undefined {
  const x = parse(a);
  const y = parse(b);
  if (x === undefined || y === undefined) return undefined;
  for (let i = 0; i < 3; i++) {
    const difference = (x[i] as number) - (y[i] as number);
    if (difference !== 0) return difference;
  }
  return 0;
}

/** A version as a person reads it: its major.minor.patch, or as it is when it has none. */
function release(version: string): string {
  return parse(version)?.join(".") ?? version;
}

/**
 * How the app and a server at `serverVersion` that supports apps from
 * `minAppVersion` stand. A version that isn't a release's, or a server
 * that names no oldest app, never makes them incompatible.
 */
export function compatibility(
  serverVersion: string,
  minAppVersion: string | undefined,
  app: string = appVersion,
  oldestServer: string = minServerVersion,
): Compatibility {
  if ((compareVersions(serverVersion, oldestServer) ?? 0) < 0) {
    return {
      state: "serverTooOld",
      serverVersion: release(serverVersion),
      minServerVersion: release(oldestServer),
      appVersion: release(app),
    };
  }
  if (minAppVersion !== undefined && (compareVersions(app, minAppVersion) ?? 0) < 0) {
    return { state: "appTooOld", appVersion: release(app), minAppVersion: release(minAppVersion) };
  }
  return { state: "compatible" };
}
