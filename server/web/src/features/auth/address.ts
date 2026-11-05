// The packaged TV app's rules for a server's address: an address typed
// without a scheme opens over https only, plain HTTP only when typed with
// http://, and any other scheme is refused.

/** A typed address's server: its base URL, without a trailing slash, and whether it's encrypted. */
export type ServerAddress = { base: string; secure: boolean };

/**
 * The server a typed address names, or undefined when it isn't an address
 * the app opens. Its path is kept, so a server behind a reverse proxy at a
 * path (https://nas/kivo) opens too.
 */
export function parseAddress(text: string): ServerAddress | undefined {
  const typed = text.trim();
  if (typed === "") return undefined;
  let url: URL;
  try {
    url = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(typed) ? typed : `https://${typed}`);
  } catch {
    return undefined;
  }
  if ((url.protocol !== "http:" && url.protocol !== "https:") || !url.hostname) return undefined;
  const path = url.pathname.replace(/\/+/g, "/").replace(/\/$/, "");
  return { base: url.origin + path, secure: url.protocol === "https:" };
}
