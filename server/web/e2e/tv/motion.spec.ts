import { expect, type Page, test } from "@playwright/test";
import { expectFocus, focusOf, overscan, press, pressUntil } from "./remote";

// How the TV build moves what's on screen: a row's cards glide to the
// focused card over a quarter of a second, on a transform the TV composites
// rather than a scroll it paints again, and the page scrolls smoothly to its
// row, except while a key is held or motion is reduced, when they jump.

const firstCard = /^card:continueWatching:/;

/** The focused card's row: its index on Home, and how many cards it holds. */
const focusedRow = (page: Page) =>
  page.evaluate(() => {
    const row = document.activeElement?.closest('[data-part="row"]');
    const rows = [...document.querySelectorAll('[data-part="row"]')];
    return {
      index: row ? rows.indexOf(row) : -1,
      cards: row?.querySelectorAll("li:not([data-tv-glow-host])").length ?? 0,
    };
  });

/** Moves focus down Home's rows to the first one longer than the screen. */
async function toLongRow(page: Page) {
  await page.goto("/");
  await expectFocus(page, { key: firstCard });
  for (let i = 0; i < 6 && (await focusedRow(page)).cards < 8; i++) {
    await press(page, "ArrowDown");
    await page.waitForTimeout(400);
  }
  expect((await focusedRow(page)).cards).toBeGreaterThanOrEqual(8);
}

/**
 * Presses `key` (or runs `act`) and reads, each frame for `ms`, the focused
 * row's cards' sideways offset, the row's own scroll and the page's.
 */
async function sample(page: Page, act: () => Promise<void>, ms = 450) {
  await page.evaluate((ms) => {
    const w = window as unknown as { __samples: { x: number; scroll: number; y: number }[] };
    w.__samples = [];
    const start = performance.now();
    const read = () => {
      const row = document.activeElement?.closest('[data-part="row"]');
      const cards = row?.querySelector<HTMLElement>("ul");
      const x = cards ? new DOMMatrixReadOnly(getComputedStyle(cards).transform).m41 : 0;
      let scroll = 0;
      for (let el = cards ?? null; el && el !== row; el = el.parentElement) scroll += el.scrollLeft;
      // Where the page shows: it scrolls at once, and its content eases there on a transform.
      const moved = document.querySelector<HTMLElement>('#main [style*="translateY"]');
      const offset = moved ? new DOMMatrixReadOnly(getComputedStyle(moved).transform).m42 : 0;
      w.__samples.push({ x, scroll, y: window.scrollY - offset });
      if (performance.now() - start < ms) requestAnimationFrame(read);
    };
    requestAnimationFrame(read);
  }, ms);
  await act();
  await page.waitForTimeout(ms + 100);
  return page.evaluate(
    () => (window as unknown as { __samples: { x: number; scroll: number; y: number }[] }).__samples,
  );
}

/** Whether some sample lies strictly between the first and the last, as a glide's frames do. */
const glided = (values: number[]) => {
  const from = values[0] ?? 0;
  const to = values.at(-1) ?? 0;
  return Math.abs(to - from) > 1 && values.some((v) => Math.abs(v - from) > 1 && Math.abs(v - to) > 1);
};

test("a row glides sideways to a card past its edge, moving its cards rather than scrolling", async ({ page }) => {
  await toLongRow(page);
  // Five Rights reach the last poster showing; the sixth moves the row.
  for (let i = 0; i < 5; i++) await press(page, "ArrowRight");
  await page.waitForTimeout(400);
  const samples = await sample(page, () => press(page, "ArrowRight"));
  const xs = samples.map((s) => s.x);
  expect(xs.at(-1), "the cards moved left by a card").toBeLessThan(-100);
  expect(glided(xs), `frames of the move: ${xs.map(Math.round).join(", ")}`).toBe(true);
  expect(Math.max(...samples.map((s) => s.scroll)), "the row never scrolls").toBe(0);
  // The focused card is wholly on screen once the row stops.
  const box = await page.evaluate(() => {
    const r = document.activeElement?.getBoundingClientRect();
    return r && { left: r.left, right: r.right };
  });
  expect(box?.right).toBeLessThanOrEqual(1920 - (await overscan(page)).x);
});

test("no row's track is kept on a layer of its own, moved or not", async ({ page }) => {
  await toLongRow(page);
  for (let i = 0; i < 7; i++) await press(page, "ArrowRight");
  await page.waitForTimeout(400);
  const hints = await page.evaluate(() =>
    [...document.querySelectorAll('[data-part="row-cards"]')].map((track) => getComputedStyle(track).willChange),
  );
  expect(hints.length).toBeGreaterThan(1);
  expect(new Set(hints)).toEqual(new Set(["auto"]));
});

test("a held key moves the row at once, a frame per repeat", async ({ page }) => {
  await toLongRow(page);
  for (let i = 0; i < 5; i++) await press(page, "ArrowRight");
  await page.waitForTimeout(400);
  // The first keydown of a held key, then repeats past the row's edge.
  await page.keyboard.down("ArrowRight");
  await page.waitForTimeout(400);
  const samples = await sample(page, async () => {
    await page.keyboard.down("ArrowRight");
  });
  await page.keyboard.up("ArrowRight");
  const xs = samples.map((s) => s.x);
  expect(glided(xs), `frames of the move: ${xs.map(Math.round).join(", ")}`).toBe(false);
  expect(xs.at(-1)).toBeLessThan((xs[0] ?? 0) - 100);
});

test("with reduced motion a row moves at once", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await toLongRow(page);
  for (let i = 0; i < 5; i++) await press(page, "ArrowRight");
  await page.waitForTimeout(400);
  const xs = (await sample(page, () => press(page, "ArrowRight"))).map((s) => s.x);
  expect(xs.at(-1)).toBeLessThan(-100);
  expect(glided(xs), `frames of the move: ${xs.map(Math.round).join(", ")}`).toBe(false);
});

test("the page glides down to the next row", async ({ page }) => {
  await page.goto("/");
  await expectFocus(page, { key: firstCard });
  // Down to the first row that isn't on the first screen.
  let ys: number[] = [];
  for (let i = 0; i < 6; i++) {
    ys = (await sample(page, () => press(page, "ArrowDown"))).map((s) => s.y);
    if ((ys.at(-1) ?? 0) > 0) break;
  }
  expect(ys.at(-1)).toBeGreaterThan(0);
  expect(glided(ys), `frames of the scroll: ${ys.map(Math.round).join(", ")}`).toBe(true);
});

test("taps in a library keep the page moving down, never easing to a stop between them", async ({ page }) => {
  await page.goto("/");
  await expectFocus(page, { key: firstCard });
  await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
  await pressUntil(page, "ArrowDown", (f) => f.name === "Movies");
  await press(page, "Enter");
  await expectFocus(page, { key: /^card:grid:/ });
  // Down to a row past the first screen, so each tap scrolls by a row.
  await press(page, "ArrowDown");
  await page.waitForTimeout(600);
  // Taps faster than a glide ends, each a fresh press rather than a repeat.
  const samples = await sample(
    page,
    async () => {
      for (let i = 0; i < 4; i++) {
        await page.keyboard.press("ArrowDown");
        await page.waitForTimeout(120);
      }
    },
    1100,
  );
  const ys = samples.map((s) => s.y);
  const steps = ys.slice(1).map((y, i) => y - (ys[i] ?? 0));
  const first = steps.findIndex((d) => d > 1);
  expect(first, `frames of the scroll: ${ys.map(Math.round).join(", ")}`).toBeGreaterThanOrEqual(0);
  // Once moving, the page never slows to under a fifth of its fastest and
  // then speeds up again, as the browser's smooth scroll did, starting each
  // tap's scroll over from rest. Slowing as it arrives is fine.
  const fastest = Math.max(...steps);
  const stalls = steps
    .map((d, i) => ({ d, i }))
    .filter(({ d, i }) => i > first && d < fastest / 5 && steps.slice(i + 1).some((n) => n > fastest * 0.4));
  expect(stalls, `frames of the scroll: ${ys.map(Math.round).join(", ")}`).toEqual([]);
  expect(
    steps.every((d) => d >= -1),
    "the page never moves back up",
  ).toBe(true);
});

test("the rail glides open", async ({ page }) => {
  await page.goto("/");
  await expectFocus(page, { key: firstCard });
  // The rail's animations are read every frame from before the press, so a
  // slow run can't miss them by reading after they've ended.
  await page.evaluate(() => {
    const w = window as unknown as { __rail: number[]; __railOn: boolean };
    w.__rail = [];
    w.__railOn = true;
    const read = () => {
      for (const a of document.getAnimations()) {
        const target = (a.effect as KeyframeEffect | null)?.target;
        if (target?.closest('[data-part="tv-rail"], [data-part="tv-rail-scrim"]')) {
          w.__rail.push(Number(a.effect?.getComputedTiming().duration));
        }
      }
      if (w.__railOn) requestAnimationFrame(read);
    };
    requestAnimationFrame(read);
  });
  await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
  const running = await page.evaluate(() => {
    const w = window as unknown as { __rail: number[]; __railOn: boolean };
    w.__railOn = false;
    return w.__rail;
  });
  expect(running.length, "the rail and the scrim animate in").toBeGreaterThan(0);
  expect(Math.max(...running)).toBeGreaterThanOrEqual(200);
  expect((await focusOf(page)).part).toBe("tv-rail-entry");
});
