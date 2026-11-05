import { expect, test } from "@playwright/test";
import { mockIds } from "@/api/mocks/fixtures";

// The heart and the check on a work's page: a tap saves the toggle, and the
// button then looks on, its icon filled and drawn in the accent colour.

for (const [name, label] of [
  ["favorite", /favorites/],
  ["watched", /Mark as/],
] as const) {
  test(`${name} toggle on the detail page`, async ({ page }, info) => {
    const patches: string[] = [];
    page.on("request", (r) => {
      if (r.method() === "PATCH") patches.push(`${r.url()} ${r.postData()}`);
    });
    await page.goto(`works/${mockIds.dunePartTwo}`);
    const button = page.getByRole("button", { name: label });
    await expect(button).toHaveAttribute("aria-pressed", "false");
    const look = () =>
      button.evaluate((el) => ({
        color: getComputedStyle(el).color,
        fill: el.querySelector("svg")?.getAttribute("fill"),
      }));
    const before = await look();
    expect(before.fill).toBe("none");
    if (info.project.name.startsWith("phone")) await button.tap();
    else await button.click();
    await expect.poll(() => patches.length).toBeGreaterThan(0);
    await expect(button).toHaveAttribute("aria-pressed", "true");
    const accent = await page.evaluate(() => {
      const probe = document.createElement("span");
      probe.className = "text-accent";
      document.body.append(probe);
      const color = getComputedStyle(probe).color;
      probe.remove();
      return color;
    });
    expect(accent).not.toBe(before.color);
    // The colour eases in over the button's transition.
    await expect.poll(async () => (await look()).color).toBe(accent);
    expect((await look()).fill).toBe("currentColor");
  });
}

test("favorite from a card's menu", async ({ page }, info) => {
  const patches: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "PATCH") patches.push(`${r.url()} ${r.postData()}`);
  });
  await page.goto(`libraries/${mockIds.movies}`);
  const more = page.getByRole("button", { name: "More actions for Arrival" });
  const phone = info.project.name.startsWith("phone");
  await (phone ? more.tap() : more.click());
  const add = page.getByRole("menuitem", { name: "Add to favorites" });
  await (phone ? add.tap() : add.click());
  await expect.poll(() => patches.length).toBeGreaterThan(0);
  await (phone ? more.tap() : more.click());
  await expect(page.getByRole("menuitem", { name: "Remove from favorites" })).toBeVisible();
});
