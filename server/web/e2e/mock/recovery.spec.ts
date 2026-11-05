import { expect, type Page, test } from "@playwright/test";
import { mockIds } from "@/api/mocks/fixtures";

// A grid page whose request fails while its rows are in view loads once the
// connection is back: within its retries while the grid sits still, and
// after them as the grid next settles, the window takes focus or the
// connection comes back. The page's requests (those past the first) fail as
// the connection is "down": with no answer, as a dropped connection's, or
// with a 429. The clock is the page's, so the retries' minute or so passes
// at once.

type Mode = "up" | "down" | "429";

async function open(page: Page) {
  await page.clock.install();
  await page.addInitScript(() => {
    const w = window as unknown as { mode: Mode; asked: number };
    w.mode = "up";
    w.asked = 0;
    const orig = window.fetch.bind(window);
    window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(input, init);
      const u = new URL(req.url);
      if (/\/api\/v1\/libraries\/[^/]+\/works$/.test(u.pathname) && u.searchParams.has("offset")) {
        w.asked++;
        if (w.mode === "down") throw new TypeError("Load failed");
        if (w.mode === "429")
          return new Response(
            JSON.stringify({
              type: "about:blank",
              title: "rateLimited",
              status: 429,
              code: "rateLimited",
              requestId: "x",
            }),
            { status: 429, headers: { "Content-Type": "application/problem+json" } },
          );
      }
      return orig(req);
    };
  });
  await page.goto(`libraries/${mockIds.movies}?mock=long`);
  await expect(page.locator('[data-part="card"]').first()).toBeVisible();
}

const setMode = (page: Page, mode: Mode) =>
  page.evaluate((mode) => {
    (window as unknown as { mode: Mode }).mode = mode;
  }, mode);

const asked = (page: Page) => page.evaluate(() => (window as unknown as { asked: number }).asked);

/** The indexes of the cells in view still showing a skeleton: no card, as their page hasn't loaded. */
const skeletons = (page: Page) =>
  page.evaluate(() =>
    [...document.querySelectorAll<HTMLElement>("[data-index]")]
      .filter((c) => {
        const r = c.getBoundingClientRect();
        return r.bottom > 0 && r.top < innerHeight && !c.querySelector('[data-part="card"]');
      })
      .map((c) => Number(c.dataset.index)),
  );

/** Scrolls halfway down, where the page in view fails. */
async function failMidway(page: Page, mode: Mode) {
  await setMode(page, mode);
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight / 2));
  await expect.poll(() => asked(page)).toBeGreaterThan(0);
  await expect.poll(async () => (await skeletons(page)).length).toBeGreaterThan(0);
}

/** Runs the page's clock on, a second at a time, by seconds. */
async function pass(page: Page, seconds: number) {
  for (let i = 0; i < seconds; i++) {
    await page.clock.runFor(1000);
    await page.waitForTimeout(20);
  }
}

const loaded = (page: Page) => expect.poll(() => skeletons(page), { timeout: 10_000 }).toEqual([]);

for (const [label, mode] of [
  ["a dropped connection", "down"],
  ["a 429", "429"],
] as const) {
  test(`a page that failed with ${label} for 5 seconds loads on its own, with the grid left where it is`, async ({
    page,
  }) => {
    await open(page);
    await failMidway(page, mode);
    await pass(page, 5);
    expect((await skeletons(page)).length).toBeGreaterThan(0);
    await setMode(page, "up");
    await pass(page, 10);
    await loaded(page);
  });
}

/** Fails the page in view until its retries are spent, and brings the connection back. */
async function exhaust(page: Page) {
  await failMidway(page, "down");
  await pass(page, 40);
  const spent = await asked(page);
  await pass(page, 40);
  expect(await asked(page)).toBe(spent);
  await setMode(page, "up");
  await pass(page, 5);
  expect((await skeletons(page)).length).toBeGreaterThan(0);
}

test("a page whose retries ran out loads as the grid settles after the smallest scroll", async ({ page }) => {
  await open(page);
  await exhaust(page);
  await page.evaluate(() => window.scrollBy(0, 2));
  await pass(page, 1);
  await loaded(page);
});

test("a page whose retries ran out loads as the window takes focus", async ({ page }) => {
  await open(page);
  await exhaust(page);
  await page.evaluate(() => window.dispatchEvent(new Event("visibilitychange")));
  await loaded(page);
});

test("a page whose retries ran out loads as the connection comes back", async ({ page }) => {
  await open(page);
  await exhaust(page);
  await page.evaluate(() => {
    window.dispatchEvent(new Event("offline"));
    window.dispatchEvent(new Event("online"));
  });
  await loaded(page);
});
