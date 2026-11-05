import { devices, expect, type Page, test } from "@playwright/test";
import { mockIds } from "@/api/mocks/fixtures";

// A phone turned sideways, scrolled through a library. Its posters are about
// as wide as upright, with more across. As the page scrolls, Safari
// collapses its toolbar and the window grows 50 to 70 pixels taller; the
// cards are sized by the grid's width alone, so they, their columns and the
// scroll position stay as they were. An emulated browser has no toolbar, so
// the window's height is changed instead.

const portrait = devices["iPhone 15"].viewport;
const landscape = devices["iPhone 15 landscape"].viewport;
test.use({ viewport: landscape });

type Layout = { width: number; height: number; columns: number; top: number; scrollY: number };

const layout = (page: Page) =>
  page.evaluate((): Layout => {
    const cards = [...document.querySelectorAll<HTMLElement>('[data-part="grid"] [data-index]')];
    const box = cards[0]?.getBoundingClientRect();
    const row = document.querySelector('[data-part="grid"] > div') as Element;
    const top = cards.find((c) => c.getBoundingClientRect().bottom > 0);
    return {
      width: Math.round(box?.width ?? 0),
      height: Math.round(box?.height ?? 0),
      columns: getComputedStyle(row).gridTemplateColumns.split(" ").length,
      top: Number(top?.getAttribute("data-index")),
      scrollY: Math.round(window.scrollY),
    };
  });

test("a phone's posters keep their size turned sideways and as its toolbar collapses", async ({ page, isMobile }) => {
  test.skip(!isMobile, "a phone");
  await page.goto(`libraries/${mockIds.movies}?mock=long`);
  await expect(page.locator('[data-part="card"]').first()).toBeVisible();
  const before = await layout(page);

  // Upright, three across, about as wide as sideways.
  await page.setViewportSize(portrait);
  await expect.poll(async () => (await layout(page)).columns).toBe(3);
  const upright = await layout(page);
  expect(before.columns).toBeGreaterThan(upright.columns);
  expect(Math.abs(before.width - upright.width)).toBeLessThanOrEqual(4);
  await page.setViewportSize(landscape);
  await expect.poll(async () => (await layout(page)).columns).toBe(before.columns);

  // Scrolled partway, the toolbar collapses; further on, it comes back.
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight / 3));
  await page.waitForTimeout(300);
  const scrolled = await layout(page);
  expect(scrolled).toMatchObject({ width: before.width, height: before.height, columns: before.columns });
  for (const by of [64, 0, 56]) {
    await page.setViewportSize({ width: landscape.width, height: landscape.height + by });
    await page.waitForTimeout(300);
    expect(await layout(page)).toEqual(scrolled);
  }
});
