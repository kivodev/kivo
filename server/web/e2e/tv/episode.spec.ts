import { expect, type Page, test } from "@playwright/test";
import { back, expectFocus, focusOf, overscan, press, pressUntil } from "./remote";

// An episode's page and a season's cards on TV, by remote: one screen that
// doesn't scroll to its episodes, each card's two stops, and Up returning to
// where it came from.

const expanse = "0190a000-0000-7000-8000-0000000000c8";
const mockId = (n: number) => `0190a000-0000-7000-8000-${n.toString(16).padStart(12, "0")}`;
const reload = mockId(1304);
const long = mockId(1305);

const shots = process.env.KIVO_TV_SHOTS;
const shot = async (page: Page, name: string) => {
  if (shots) await page.screenshot({ path: `${shots}/${name}.png` });
};
const onCard = (f: { key: string | null }) => f.key?.startsWith("card:episodes:") ?? false;
const onTextStop = (page: Page) => page.evaluate(() => document.querySelector("[data-tv-card-text]") !== null);

test.describe("an episode's page", () => {
  test("shows the season's row on one screen, and reaching it doesn't scroll", async ({ page }) => {
    await page.goto(`/works/${reload}`);
    await expectFocus(page, { key: "play" });
    await shot(page, "episode-short");
    const { y } = await overscan(page);
    const row = page.locator('[data-row="episodes"]');
    await expect(row.locator('[data-part="episode-card"]').first()).toBeVisible();
    await press(page, "ArrowDown");
    await expectFocus(page, { key: "card:episodes:", name: "4. Reload" });
    await page.waitForTimeout(400);
    expect(await page.evaluate(() => window.scrollY)).toBe(0);
    const box = await row.locator('[data-part="row-cards"]').boundingBox();
    expect((box?.y ?? 0) + (box?.height ?? 0)).toBeLessThanOrEqual(1080 - y);
    await shot(page, "episode-still-stop");
  });

  test("has no previous and next, and Up from any button goes to the series", async ({ page }) => {
    await page.goto(`/works/${reload}`);
    await expectFocus(page, { key: "play" });
    expect(await page.locator('[data-tv-focus-key="next"], [data-tv-focus-key="previous"]').count()).toBe(0);
    await press(page, "ArrowRight", "ArrowRight");
    expect((await focusOf(page)).key).not.toBe("play");
    await press(page, "ArrowUp");
    await expectFocus(page, { key: "series" });
    await press(page, "ArrowDown");
    await expectFocus(page, { key: "play" });
  });

  test("Up from the row returns to the button you came from", async ({ page }) => {
    await page.goto(`/works/${reload}`);
    await expectFocus(page, { key: "play" });
    await press(page, "ArrowRight");
    const button = (await focusOf(page)).key;
    expect(button).not.toBe("play");
    await press(page, "ArrowDown");
    await expectFocus(page, { key: "card:episodes:" });
    await press(page, "ArrowRight", "ArrowUp");
    await expectFocus(page, { key: button ?? "" });
  });

  test("a description that fits takes no focus", async ({ page }) => {
    await page.goto(`/works/${reload}`);
    await expectFocus(page, { key: "play" });
    expect(await page.locator('[data-part="overview-open"]').count()).toBe(0);
  });

  test("a long description takes focus whole and opens in a panel that scrolls, and Back returns to it", async ({
    page,
  }) => {
    await page.goto(`/works/${long}`);
    await expectFocus(page, { key: "play" });
    await shot(page, "episode-long");
    await press(page, "ArrowDown");
    await expectFocus(page, { part: "overview-open" });
    await shot(page, "episode-long-focus");
    await press(page, "Enter");
    await expectFocus(page, { key: "overview-text" });
    const panel = page.locator('[data-part="overview-full"]');
    await expect(panel).toContainText("Fred Johnson has to decide");
    await shot(page, "episode-long-panel");
    // Down scrolls it, staying on the text, until its end, then goes on to Close.
    const scrolled = () => panel.locator(".overflow-hidden").evaluate((el) => el.scrollTop);
    await press(page, "ArrowDown");
    await expect.poll(scrolled).toBeGreaterThan(0);
    await expectFocus(page, { key: "overview-text" });
    await pressUntil(page, "ArrowDown", (f) => f.key === "overview-close", 8);
    await press(page, "ArrowUp");
    await expectFocus(page, { key: "overview-text" });
    await back(page);
    await expectFocus(page, { key: "overview" });
    await expect(panel).toHaveCount(0);
    await press(page, "ArrowDown");
    await expectFocus(page, { key: "card:episodes:", name: "5." });
    expect(await page.evaluate(() => window.scrollY)).toBe(0);
    await press(page, "ArrowUp");
    await expectFocus(page, { key: "overview" });
  });
});

test.describe("a season's cards", () => {
  const toCard = async (page: Page) => {
    await page.goto(`/works/${expanse}`);
    await expectFocus(page, { key: "play" });
    await pressUntil(page, "ArrowDown", onCard, 4);
  };

  test("OK on the still plays the episode", async ({ page }) => {
    await toCard(page);
    const id = (await focusOf(page)).key?.split(":").pop();
    await press(page, "Enter");
    await expect(page).toHaveURL(new RegExp(`/watch/${id}`));
  });

  test("Down moves to the text, where OK opens the episode's page, and Back returns to the still", async ({ page }) => {
    await toCard(page);
    await shot(page, "series-still-stop");
    const key = (await focusOf(page)).key;
    await press(page, "ArrowDown");
    await expect.poll(() => onTextStop(page)).toBe(true);
    expect((await focusOf(page)).key).toBe(key);
    await page.waitForTimeout(300);
    await shot(page, "series-text-stop");
    await press(page, "Enter");
    await expect(page).toHaveURL(new RegExp(`/works/${key?.split(":").pop()}`));
    await expectFocus(page, { key: "play" });
    await back(page);
    await expectFocus(page, { key: key ?? "" });
    expect(await onTextStop(page)).toBe(false);
  });

  test("Left and Right keep the stop, and Up returns to the still before leaving the row", async ({ page }) => {
    await toCard(page);
    const key = (await focusOf(page)).key;
    await press(page, "ArrowDown", "ArrowRight");
    await expect.poll(async () => (await focusOf(page)).key).not.toBe(key);
    expect(await onTextStop(page)).toBe(true);
    const moved = (await focusOf(page)).key;
    await press(page, "ArrowUp");
    await expect.poll(() => onTextStop(page)).toBe(false);
    expect((await focusOf(page)).key).toBe(moved);
  });

  test("leaving the row for the rail forgets the text stop", async ({ page }) => {
    await toCard(page);
    await press(page, "ArrowDown");
    await expect.poll(() => onTextStop(page)).toBe(true);
    await pressUntil(page, "ArrowLeft", (f) => !onCard(f), 12);
    await expect.poll(() => onTextStop(page)).toBe(false);
  });

  test("Down from the season pills passes over the season's check, and Up from the cards lands on it", async ({
    page,
  }) => {
    await page.goto(`/works/${expanse}`);
    await expectFocus(page, { key: "play" });
    await pressUntil(page, "ArrowDown", (f) => f.key?.startsWith("season:") ?? false, 3);
    await press(page, "ArrowDown");
    await expectFocus(page, { key: "card:episodes:" });
    await press(page, "ArrowUp");
    await expectFocus(page, { key: "season-played" });
  });
});
