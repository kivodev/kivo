import { expect, type Page, test } from "@playwright/test";
import { back, expectFocus, overscan, press } from "./remote";

async function openFailedPlay(page: Page) {
  await page.goto("/");
  await expectFocus(page, { name: "Dune: Part Two" });
  await press(page, "Enter");
  await expectFocus(page, { key: "play" });
  await press(page, "Enter");
  await expect(page.locator('[data-part="player-error"]')).toBeVisible();
}

test("playback failure starts on Try again and the arrows reach every recovery action", async ({ page }) => {
  await openFailedPlay(page);
  await expectFocus(page, { name: "Try again" });
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Play another version" });
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Back" });
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "View diagnostics" });
  await press(page, "Enter");
  await expect(page.getByText("Code:", { exact: true })).toBeVisible();
  await press(page, "ArrowLeft", "ArrowLeft", "ArrowLeft");
  await expectFocus(page, { name: "Try again" });
  const title = await page.getByRole("heading", { name: "Dune: Part Two", exact: true }).boundingBox();
  const inset = await overscan(page);
  expect(title?.x).toBeGreaterThanOrEqual(inset.x);
  expect(title?.y).toBeGreaterThanOrEqual(inset.y);
});

/** The contrast ratio of the focused control's text on its fill, from their computed colours. */
const focusedContrast = (page: Page) =>
  page.locator(":focus > :first-child").evaluate((face) => {
    const rgb = (value: string) => (value.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number);
    const luminance = (value: string) => {
      const [r = 0, g = 0, b = 0] = rgb(value).map((c) => {
        const s = c / 255;
        return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    };
    const style = getComputedStyle(face);
    const [hi, lo] = [luminance(style.color), luminance(style.backgroundColor)].sort((a, b) => b - a);
    return ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05);
  });

test("each focused recovery action's label is legible on its lit fill", async ({ page }) => {
  await openFailedPlay(page);
  for (const name of ["Try again", "Play another version", "Back", "View diagnostics"]) {
    await expectFocus(page, { name });
    await expect.poll(() => focusedContrast(page), { message: name }).toBeGreaterThanOrEqual(4.5);
    await press(page, "ArrowRight");
  }
});

test("remote Back leaves a playback failure and restores working navigation", async ({ page }) => {
  await openFailedPlay(page);
  await back(page);
  await expect(page).toHaveURL(/\/works\//);
  await expectFocus(page, { key: "play" });
  // The very first key, sent the instant the page shows, moves focus.
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "restart" });
});
