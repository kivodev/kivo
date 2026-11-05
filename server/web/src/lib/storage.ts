// What this browser keeps between visits, in its local storage under
// "kivo." keys. Storage can be unavailable, as in a private window, or
// full. Reads then find nothing and writes are dropped, so every caller
// has a default to fall back on.

/** The value kept under key, or undefined. */
export function readStored(key: string): string | undefined {
  try {
    return localStorage.getItem(key) ?? undefined;
  } catch {
    return undefined;
  }
}

/** Keeps value under key. An undefined value removes it. */
export function writeStored(key: string, value: string | undefined): void {
  try {
    if (value === undefined) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // The write is dropped, and the caller's default applies next time.
  }
}

/** The JSON kept under key, parsed, or undefined when there's none or it doesn't parse. */
export function readStoredJson(key: string): unknown {
  const text = readStored(key);
  if (text === undefined) return undefined;
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

/** Keeps value under key as JSON. */
export function writeStoredJson(key: string, value: unknown): void {
  writeStored(key, JSON.stringify(value));
}
