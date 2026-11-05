import { copyFileSync, readFileSync, utimesSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { e2e, type Fixture } from "./env";

// Live updates, in a real browser against the real server, pushed by the
// server with no reload and no polling: the admin's Overview follows a
// play from another tab, and Home shows a film dropped into the library's
// folder.

const fixture = JSON.parse(readFileSync(e2e.fixture, "utf8")) as Fixture;

test("the Overview shows a play start and end as they happen", async ({ page, context }) => {
  await page.goto("admin");
  const nowPlaying = page.locator("section").filter({ has: page.getByRole("heading", { name: "Now playing" }) });
  await expect(nowPlaying.getByText("No active streams")).toBeVisible();

  const player = await context.newPage();
  await player.goto(`watch/${fixture.workId}?from=start`);
  await expect(nowPlaying.getByText(/^Wanderers · /)).toBeVisible();

  await player.goto(""); // leaving the player stops its play
  await expect(nowPlaying.getByText("No active streams")).toBeVisible();
  await player.close();
});

test("Home shows a film dropped into the library's folder without a reload", async ({ page }) => {
  await page.goto("");
  const recent = page
    .locator("[data-row]")
    .filter({ has: page.getByRole("heading", { name: "Recently Added in Movies" }) });
  await expect(recent.locator('[data-part="card"]', { hasText: "Wanderers" })).toBeVisible();
  await page.evaluate(() => {
    (window as { notReloaded?: boolean }).notReloaded = true;
  });

  // A loose file in the library's folder, settled as a finished download
  // is, under a title no provider knows, so it keeps its name. The file
  // watcher finds it: the library was added while the server ran, and its
  // folder was watched from then on.
  const film = join(e2e.dir, "media", "Kivo Drop Test (2019).mkv");
  copyFileSync(join(e2e.dir, "media", "Wanderers (2020)", "Wanderers (2020).mkv"), film);
  const old = new Date(Date.now() - 3_600_000);
  utimesSync(film, old, old);

  await expect(recent.locator('[data-part="card"]', { hasText: "Kivo Drop Test" })).toBeVisible({ timeout: 30_000 });
  expect(await page.evaluate(() => (window as { notReloaded?: boolean }).notReloaded)).toBe(true);
});
