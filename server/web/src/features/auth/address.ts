// The packaged TV app's rules for a server's address: one typed with
// http:// or https:// opens at that URL alone, one typed without a scheme
// is tried over https first, then plain HTTP, and any other scheme is
// refused.

/** A typed address's server: its base URL, without a trailing slash, and whether it's encrypted. */
export type ServerAddress = { base: string; secure: boolean };

/** The port a server listens on when an address names none. */
const defaultPort = "8484";

/**
 * The servers a typed address may name, best first, or undefined when it
 * isn't an address the app opens. Without a scheme it's https on its port,
 * or https's own, then plain HTTP on that port, or on 8484 and on HTTP's
 * own. Its path is kept, so a server behind a reverse proxy at a path
 * (https://nas/kivo) opens too.
 */
export function addressCandidates(text: string): ServerAddress[] | undefined {
  const typed = text.trim();
  if (typed === "") return undefined;
  const named = /^[a-z][a-z0-9+.-]*:\/\//i.test(typed);
  let url: URL;
  try {
    url = new URL(named ? typed : `https://${typed}`);
  } catch {
    return undefined;
  }
  if ((url.protocol !== "http:" && url.protocol !== "https:") || !url.hostname) return undefined;
  const path = url.pathname.replace(/\/+/g, "/").replace(/\/$/, "");
  const at = (scheme: "http" | "https", port: string) => ({
    base: `${scheme}://${url.hostname}${port ? `:${port}` : ""}${path}`,
    secure: scheme === "https",
  });
  if (named) return [{ base: url.origin + path, secure: url.protocol === "https:" }];
  if (url.port) return [at("https", url.port), at("http", url.port)];
  return [at("https", ""), at("http", defaultPort), at("http", "")];
}
