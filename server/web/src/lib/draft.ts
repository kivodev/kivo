import { useState } from "react";

// A form's edits to what's saved, and which of them differ from it. The
// differences are what a save sends.

/**
 * A page's edits to a copy of what's saved. It returns the draft, a way to
 * change part of it, and a discard that resets it to `saved` as it is at
 * that moment.
 */
export function useDraft<T extends object>(saved: T) {
  const [draft, setDraft] = useState(saved);
  return {
    draft,
    edit: (change: Partial<T>) => setDraft((d) => ({ ...d, ...change })),
    discard: () => setDraft(saved),
  };
}

/**
 * The fields of `edited` whose values differ from `saved`. A save sends
 * only these, so an edit made elsewhere in the meantime is kept. It is
 * empty when nothing changed.
 */
export function changes<T extends object>(saved: { [K in keyof T]?: unknown }, edited: T): Partial<T> {
  const out: Partial<T> = {};
  for (const key of Object.keys(edited) as (keyof T)[]) {
    if (!same(edited[key], saved[key])) out[key] = edited[key];
  }
  return out;
}

/** Whether two values hold the same data, whatever their keys' order. */
function same(a: unknown, b: unknown): boolean {
  return JSON.stringify(sorted(a)) === JSON.stringify(sorted(b));
}

function sorted(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(sorted);
  if (v && typeof v === "object") {
    return Object.fromEntries(
      Object.entries(v)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([k, x]) => [k, sorted(x)]),
    );
  }
  return v;
}
