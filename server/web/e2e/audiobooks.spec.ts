import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { e2e, type Fixture } from "./env";

// Audiobooks in a real browser against the real server: the
// library's Books, Authors and Series tabs over books scanned from folders
// and tags, a book's page, its series' page and its author's page.

const fixture = JSON.parse(readFileSync(e2e.fixture, "utf8")) as Fixture;

test("browses an audiobooks library by books, authors and series", async ({ page }) => {
  await page.goto(`/libraries/${fixture.audiobooksId}`);
  await expect(page.getByRole("heading", { name: "Audiobooks", level: 1 })).toBeVisible();
  const sections = page.getByRole("navigation", { name: "Library sections" });
  await expect(sections.getByRole("button")).toHaveText(["Books", "Authors", "Series"]);
  const hailMary = page.locator('[data-part="card"][data-kind="book"]', { hasText: "Project Hail Mary" });
  await expect(hailMary).toContainText("Andy Weir");

  await sections.getByRole("button", { name: "Authors" }).click();
  await expect(page.getByRole("link", { name: /James S. A. Corey/ })).toContainText("2 books");

  await sections.getByRole("button", { name: "Series" }).click();
  await expect(page.getByRole("link", { name: /The Expanse/ })).toContainText("2 books");
});

test("a book's page links its author, narrator and series", async ({ page }) => {
  await page.goto(`/works/${fixture.leviathanId}`);
  await expect(page.getByRole("heading", { name: "Leviathan Wakes", level: 1 })).toBeVisible();
  await expect(page.getByText(/Narrated by/)).toHaveText("Narrated by Jefferson Mays");
  // Three files without chapters are a chapter each.
  await expect(page.getByText(/3 chapters/)).toBeVisible();
  await expect(page.locator('[data-part="book-files"] li')).toHaveCount(3);
  await expect(page.getByText(/Book 1 of/)).toHaveText("Book 1 of The Expanse");

  await page.getByRole("link", { name: "The Expanse" }).click();
  await expect(page.getByRole("heading", { name: "The Expanse", level: 1 })).toBeVisible();
  await expect(page.locator('[data-part="series-position"]')).toHaveText(["Book 1", "Book 1.5"]);

  await page.locator('[data-part="card"]', { hasText: "The Churn" }).getByRole("link").first().click();
  await expect(page.getByRole("heading", { name: "The Churn", level: 1 })).toBeVisible();
  await expect(page.getByText(/2 chapters/)).toBeVisible();
  await page.getByRole("link", { name: "James S. A. Corey" }).click();
  await expect(page.getByRole("heading", { name: "James S. A. Corey", level: 1 })).toBeVisible();
  await expect(page.getByRole("heading", { name: "The Expanse", level: 2 })).toBeVisible();
});
