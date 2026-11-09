import { expect, type Page, test } from "@playwright/test";

// Pinned libraries in a real browser against the real server: Settings ›
// Account saves the order and pins for the user, the sidebar and Home
// follow, and a library that isn't pinned is under More libraries. The
// test puts the default back, since the other tests share the user.

/** Drags a library to the top of the order with the keyboard: Enter on its handle, Up to the top, Enter. */
async function moveToTop(page: Page, name: string) {
  const order = page.getByRole("grid", { name: "Libraries" });
  await order.getByRole("button", { name: `Drag ${name}` }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: `Insert after ${name}` })).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  await expect(page.getByRole("button", { name: /^Insert before / })).toBeFocused();
  await page.keyboard.press("Enter");
}

test("pinned libraries order the sidebar and Home, and the rest are under More libraries", async ({ page }) => {
  await page.goto("settings");
  const order = page.getByRole("grid", { name: "Libraries" });
  await expect(order.getByRole("row")).toHaveText(["Movies", "Audiobooks"]);
  const sidebar = page.getByRole("navigation", { name: "Main" });
  const sidebarLibraries = sidebar.locator('[data-part="sidebar-library"]');

  await moveToTop(page, "Audiobooks");
  await order.getByText("Movies").click(); // the checkbox's label
  await expect(sidebarLibraries).toHaveText(["Audiobooks"]);
  await expect(sidebar.getByRole("link", { name: "Lists" })).toBeVisible();

  // Saved on the server: a reload keeps it.
  await page.reload();
  await expect(order.getByRole("row")).toHaveText(["Audiobooks", "Movies"]);
  await expect(order.getByRole("checkbox", { name: "Pinned: Movies" })).not.toBeChecked();

  await page.goto("");
  await expect(page.getByRole("heading", { name: "Recently Added in Audiobooks" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Recently Added in Movies" })).toHaveCount(0);
  await sidebar.getByRole("button", { name: "More libraries" }).click();
  await page.getByRole("menuitem", { name: "Movies" }).click();
  await expect(page.getByRole("heading", { name: "Movies", level: 1 })).toBeVisible();

  // Search still finds what isn't pinned.
  await page.goto("search?q=wanderers");
  await expect(page.locator('[data-part="card"]', { hasText: "Wanderers" })).toBeVisible();

  await page.goto("settings");
  await order.getByText("Movies").click(); // the checkbox's label
  await moveToTop(page, "Movies");
  await expect(order.getByRole("row")).toHaveText(["Movies", "Audiobooks"]);
  await expect(sidebarLibraries).toHaveText(["Movies", "Audiobooks"]);
});

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test("the bottom bar is Home, Libraries and Search, and Libraries lists the libraries", async ({ page }) => {
    await page.goto("");
    const bar = page.getByRole("navigation", { name: "Main" });
    const tab = bar.getByRole("link", { name: "Libraries" });
    await expect(bar.locator('[data-part="sidebar-library"]').first()).toBeHidden();
    await tab.click();
    await expect(page.getByRole("heading", { name: "Libraries", level: 1 })).toBeVisible();
    await expect(tab).toHaveAttribute("aria-current", "page");

    // Lists, then the pinned libraries, each library with how many items it has.
    const rows = page.locator('[data-part="library-row"]');
    await expect(rows.first()).toContainText("Lists");
    await expect(rows.filter({ hasText: "Movies" })).toHaveText(/Movies\s*\d+/);

    // A row opens its library under its title alone, with Back to the list.
    await rows.filter({ hasText: "Audiobooks" }).click();
    await expect(page.getByRole("heading", { name: "Audiobooks", level: 1 })).toBeVisible();
    await expect(page.getByRole("button", { name: "Audiobooks" })).toHaveCount(0);
    await page.locator("#main").getByRole("link", { name: "Libraries" }).click();
    await expect(page.getByRole("heading", { name: "Libraries", level: 1 })).toBeVisible();

    // The tab opens the library this browser last opened, and chosen there, goes back to the list.
    await rows.filter({ hasText: "Movies" }).click();
    await expect(page.getByRole("heading", { name: "Movies", level: 1 })).toBeVisible();
    await bar.getByRole("link", { name: "Home" }).click();
    await tab.click();
    await expect(page.getByRole("heading", { name: "Movies", level: 1 })).toBeVisible();
    await tab.click();
    await expect(page.getByRole("heading", { name: "Libraries", level: 1 })).toBeVisible();
  });
});
