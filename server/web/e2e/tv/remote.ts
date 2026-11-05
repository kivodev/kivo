import { expect, type Page } from "@playwright/test";

// What a TV remote can do: the four arrows, OK and Back. The tests drive the
// app with these and nothing else, so a screen a remote can't reach fails.

export type Key = "ArrowUp" | "ArrowDown" | "ArrowLeft" | "ArrowRight" | "Enter" | "Escape";

/** Presses the keys in order, each a full keydown and keyup as a remote sends. */
export async function press(page: Page, ...keys: Key[]) {
  for (const key of keys) await page.keyboard.press(key);
}

/** Back, which Escape is on a remote's browser. */
export const back = (page: Page) => press(page, "Escape");

export type Focus = { part: string | null; name: string; key: string | null };

/** What has focus: its data-part, its accessible name (or its text) and its focus key. */
export const focusOf = (page: Page): Promise<Focus> =>
  page.evaluate(() => {
    const el = document.activeElement;
    if (!el || el === document.body) return { part: null, name: "", key: null };
    const labelled = el.getAttribute("aria-label");
    const labelledBy = el
      .getAttribute("aria-labelledby")
      ?.split(" ")
      .map((id) => document.getElementById(id)?.textContent ?? "")
      .join(" ");
    const name = labelled ?? labelledBy ?? (el as HTMLElement).innerText ?? el.textContent ?? "";
    return {
      part: el.getAttribute("data-part"),
      name: name.replace(/\s+/g, " ").trim(),
      key: el.getAttribute("data-tv-focus-key"),
    };
  });

export type Expected = {
  /** The focused element's data-part, for the parts that name one (keys, rail entries). */
  part?: string;
  /** Its focus key, as a string it starts with or a pattern: cards are `card:<row>:<id>`. */
  key?: string | RegExp;
  /** Its accessible name or text, as a string it contains or a pattern. */
  name?: string | RegExp;
};

const matches = (actual: string | null, want: string | RegExp, prefix: boolean) => {
  if (actual === null) return false;
  if (typeof want !== "string") return want.test(actual);
  return prefix ? actual.startsWith(want) : actual.includes(want);
};

/**
 * Waits until the focused element is the one described. Focus moves a frame
 * after a key, so this polls and reports what had focus instead of reading once.
 */
export async function expectFocus(page: Page, want: Expected) {
  await expect
    .poll(
      async () => {
        const f = await focusOf(page);
        if (want.part !== undefined && f.part !== want.part) return `part ${f.part} (${f.name})`;
        if (want.key !== undefined && !matches(f.key, want.key, true)) return `key ${f.key} (${f.name})`;
        if (want.name !== undefined && !matches(f.name, want.name, false)) return `name ${f.name} (${f.key})`;
        return "ok";
      },
      { message: `focus on ${JSON.stringify(want, (_, v) => (v instanceof RegExp ? v.source : v))}` },
    )
    .toBe("ok");
}

/** Presses `key` until the focus satisfies `until`, at most `limit` times. */
export async function pressUntil(page: Page, key: Key, until: (f: Focus) => boolean, limit = 40) {
  for (let i = 0; i < limit; i++) {
    if (until(await focusOf(page))) return;
    await press(page, key);
    // A press moves focus within a frame or two; give the next read that.
    await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
  }
  expect(until(await focusOf(page)), `focus reached after ${limit} ${key} presses`).toBe(true);
}

/** The screen's overscan insets in pixels, as the app reads them from its tokens. */
export const overscan = (page: Page): Promise<{ x: number; y: number }> =>
  page.evaluate(() => {
    const root = getComputedStyle(document.documentElement);
    const px = (name: string) => Number.parseFloat(root.getPropertyValue(name));
    return { x: px("--layout-tv-overscan-x"), y: px("--layout-tv-overscan-y") };
  });
