import { expect, type Page, test } from "@playwright/test";
import { mockIds } from "@/api/mocks/fixtures";

// A tap opens the page at once, with what's already known, and its content
// fills in as it arrives: the API answers 1.5 s late once the page before
// it has loaded, and the new page's title and placeholder are on screen
// within about 100 ms of the tap, before any of its answers. Another
// library, sort or filter shows the same way, never the one before. The next
// page's code is loaded first: the dev server serves it as many modules,
// whose loading holds up the browser's frames, where the build has one
// small file that a hover or touch has already started loading.

const late = 1500;

/** Opens path, its API answering `late` ms late once watchFor is called, with `screen`'s code loaded. */
async function open(page: Page, path: string, screen: string) {
  await page.addInitScript((late) => {
    const w = window as unknown as { slow: boolean };
    w.slow = false;
    const orig = window.fetch.bind(window);
    window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(input, init);
      if (w.slow && new URL(req.url).pathname.includes("/api/v1/")) {
        await new Promise((resolve) => setTimeout(resolve, late));
      }
      return orig(req);
    };
  }, late);
  await page.goto(path);
  await page.evaluate((screen) => import(screen), `/src/features/${screen}.tsx`);
}

/**
 * Once the page showing has settled, makes the API slow, then records the
 * frame on which an h1 reading `title` and a placeholder are first both on
 * screen, and the sort reading `sort` when it's given, after the next press.
 */
async function watchFor(page: Page, title: string, sort?: string) {
  await page.waitForTimeout(500);
  await page.evaluate(
    ({ title, sort }) => {
      const w = window as unknown as { slow: boolean; clickedAt?: number; shownAt?: number };
      w.slow = true;
      document.addEventListener(
        "pointerdown",
        () => {
          w.clickedAt = performance.now();
        },
        { capture: true, once: true },
      );
      const check = () => {
        const heading = [...document.querySelectorAll("h1")].some((h) => h.textContent?.trim() === title);
        const sorted =
          sort === undefined || document.querySelector('[data-part="sort"] button')?.textContent?.trim() === sort;
        if (w.clickedAt !== undefined && heading && sorted && document.querySelector('[aria-busy="true"]')) {
          w.shownAt = performance.now();
          return;
        }
        requestAnimationFrame(check);
      };
      requestAnimationFrame(check);
    },
    { title, sort },
  );
}

/** Milliseconds from the click to the frame showing the page, once it has. */
const shownAfter = (page: Page) =>
  page.evaluate(() => {
    const w = window as unknown as { clickedAt?: number; shownAt?: number };
    return w.clickedAt !== undefined && w.shownAt !== undefined ? w.shownAt - w.clickedAt : undefined;
  });

/**
 * A tap, or a click after the pointer has come to rest on the target, as
 * a mouse does: WebKit takes a few hundred ms over a big grid's hover
 * styles, which would otherwise land between the click and the next frame.
 */
async function press(page: Page, phone: boolean, target: ReturnType<Page["locator"]>) {
  if (phone) return target.tap();
  // A corner, clear of a card's Play button.
  const position = { x: 8, y: 8 };
  await target.hover({ position });
  await page.waitForTimeout(500);
  await target.click({ position });
}

test("a library opens at once with its name and its grid's placeholder, then its cards", async ({ page, isMobile }) => {
  await open(page, "libraries", "browse/LibraryScreen");
  const row = page.locator('[data-part="library-row"]').filter({ hasText: "Movies" });
  await expect(row).toBeVisible();
  await watchFor(page, "Movies");
  await press(page, isMobile, row);
  await expect.poll(() => shownAfter(page)).toBeLessThan(100);
  await expect(page.getByRole("heading", { name: "Movies", level: 1 })).toBeVisible();
  expect(await page.locator('[data-part="card"]').count()).toBe(0);
  await expect(page.locator('[data-part="card"]').first()).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('[data-part="library-count"]')).toBeVisible();
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
});

test("another library opens at once with its name and placeholders, not the library before", async ({
  page,
  isMobile,
}) => {
  await open(page, `libraries/${mockIds.movies}`, "browse/LibraryScreen");
  await expect(page.locator('[data-part="card"]').first()).toBeVisible();
  // A phone reaches another library through the Libraries tab; a desktop's sidebar lists it.
  let target = page.locator('[data-part="sidebar-library"]').filter({ hasText: "TV Shows" });
  if (isMobile) {
    await page.locator('[data-part="libraries-tab"]').tap();
    target = page.locator('[data-part="library-row"]').filter({ hasText: "TV Shows" });
  }
  await expect(target).toBeVisible();
  await watchFor(page, "TV Shows");
  await press(page, isMobile, target);
  await expect.poll(() => shownAfter(page)).toBeLessThan(100);
  expect(await page.locator('[data-part="card"]').count()).toBe(0);
  await expect(page.locator('[data-part="card"]').first()).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('[data-part="library-count"]')).toContainText("2 shows");
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
});

test("another sort shows at once, with its grid's placeholders, then its cards", async ({ page, isMobile }) => {
  await open(page, `libraries/${mockIds.movies}`, "browse/LibraryScreen");
  await expect(page.locator('[data-part="card"]').first()).toBeVisible();
  await page.locator('[data-part="sort"] button').click();
  const option = page.getByRole("option", { name: "Date added" });
  await expect(option).toBeVisible();
  await watchFor(page, "Movies", "Recently added");
  await press(page, isMobile, option);
  await expect.poll(() => shownAfter(page)).toBeLessThan(100);
  expect(await page.locator('[data-part="card"]').count()).toBe(0);
  await expect(page.locator('[data-part="card"]').first()).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('[data-part="sort"] button')).toHaveText("Recently added");
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
});

test("a work's page opens at once with the card's title, then its actions", async ({ page, isMobile }) => {
  await open(page, `libraries/${mockIds.movies}`, "detail/WorkScreen");
  const card = page.getByRole("link", { name: /^Arrival/ }).first();
  await expect(card).toBeVisible();
  await watchFor(page, "Arrival");
  await press(page, isMobile, card);
  await expect.poll(() => shownAfter(page)).toBeLessThan(100);
  await expect(page.getByRole("heading", { name: "Arrival", level: 1 })).toBeVisible();
  await expect(page.locator('[data-part="hero"]')).toBeVisible();
  await expect(page.getByRole("link", { name: /^(Play|Resume)/ }).first()).toBeVisible({ timeout: 10_000 });
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
});
