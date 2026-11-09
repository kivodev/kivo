import { expect, test } from "@playwright/test";
import { mockIds } from "@/api/mocks/fixtures";

// The My List and watched toggles on a work's page: a tap saves the toggle,
// and the button then looks on, drawn in the accent colour.

for (const [name, label, saved] of [
  ["My List", /My List/, /\/lists\//],
  ["watched", /Mark as/, /user-state/],
] as const) {
  test(`${name} toggle on the detail page`, async ({ page }, info) => {
    const saves: string[] = [];
    page.on("request", (r) => {
      if (r.method() !== "GET" && saved.test(r.url())) saves.push(r.url());
    });
    await page.goto(`works/${mockIds.dunePartTwo}`);
    const button = page.getByRole("button", { name: label });
    await expect(button).toHaveAttribute("aria-pressed", "false");
    const color = () => button.evaluate((el) => getComputedStyle(el).color);
    const before = await color();
    if (info.project.name.startsWith("phone")) await button.tap();
    else await button.click();
    await expect.poll(() => saves.length).toBeGreaterThan(0);
    await expect(button).toHaveAttribute("aria-pressed", "true");
    const accent = await page.evaluate(() => {
      const probe = document.createElement("span");
      probe.className = "text-accent";
      document.body.append(probe);
      const value = getComputedStyle(probe).color;
      probe.remove();
      return value;
    });
    expect(accent).not.toBe(before);
    // The colour eases in over the button's transition.
    await expect.poll(color).toBe(accent);
  });
}

test("Add to List from a card's menu", async ({ page }, info) => {
  await page.goto(`libraries/${mockIds.movies}`);
  const card = page.locator('[data-part="card"]').first();
  const more = card.getByRole("button", { name: /^More actions for/ });
  if (info.project.name.startsWith("phone")) await more.tap();
  else {
    await card.hover();
    await more.click();
  }
  await page.getByRole("menuitem", { name: "Add to List" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("checkbox", { name: /My List/ })).toBeVisible();
  await dialog.getByRole("button", { name: "Done" }).click();
  await expect(dialog).toBeHidden();
});
