import { readFileSync } from "node:fs";
import { expect, type Page, test } from "@playwright/test";
import { e2e, type Fixture } from "./env";

// Listening in a real browser against the real server: a
// chaptered m4b plays in the mini player while browsing, moves by chapter
// and speed in Now Playing, resumes where it stopped, and a book of three
// MP3 files plays across them by chapter.

const fixture = JSON.parse(readFileSync(e2e.fixture, "utf8")) as Fixture;

const audio = (page: Page) => page.locator("audio");
const miniPlayer = (page: Page) => page.locator('[data-part="mini-player"]');

/** Waits until the book is playing at or past `seconds` on its current file. */
async function playingPast(page: Page, seconds: number) {
  await expect
    .poll(() => audio(page).evaluate((a: HTMLAudioElement) => !a.paused && a.readyState >= 3 && a.currentTime), {
      timeout: 30_000,
    })
    .toBeGreaterThan(seconds);
}

test.afterEach(async ({ page }) => {
  // The speed is the user's: leave it at normal for the other tests.
  await page.request.patch("/api/v1/users/me", {
    data: { playback: { bookSpeed: 1 } },
    headers: { Origin: e2e.baseURL },
  });
});

test("a chaptered m4b plays while browsing, by chapter and speed, and resumes", async ({ page }) => {
  await page.goto(`works/${fixture.hailMaryId}`);
  await page.getByRole("button", { name: "Play from Opening Credits" }).click();
  await playingPast(page, 0.5);
  await expect(miniPlayer(page)).toContainText("Project Hail Mary");
  await expect(miniPlayer(page).locator('[data-part="mini-player-chapter"]')).toHaveText("Opening Credits");

  // It keeps playing on another page, and the server lists it as playing.
  await page.getByRole("link", { name: "Home" }).click();
  await expect(page.getByRole("heading", { name: "Recently Added in Audiobooks" })).toBeVisible();
  await playingPast(page, 1);
  const plays = (await (await page.request.get("/api/v1/plays")).json()) as {
    items: { workId: string }[];
  };
  expect(plays.items.map((s) => s.workId)).toContain(fixture.hailMaryId);

  await miniPlayer(page).getByRole("button", { name: "Now Playing: Project Hail Mary" }).click();
  const sheet = page.getByRole("dialog", { name: "Now Playing" });
  await expect(sheet.getByText("Narrated by Ray Porter")).toBeVisible();
  await expect(sheet.getByText("Chapter 1 of 3")).toBeVisible();
  await sheet.getByRole("button", { name: "Next chapter" }).click();
  await expect(sheet.getByText("Chapter 2 of 3")).toBeVisible();
  await playingPast(page, 60);
  await expect(sheet.getByRole("slider", { name: "Seek in the chapter" })).toHaveAttribute("min", "60000");
  await sheet.getByRole("button", { name: "Next chapter" }).click();
  await expect(sheet.getByText("Chapter 3 of 3")).toBeVisible();
  await playingPast(page, 120);

  await sheet.getByRole("button", { name: /Speed/ }).click();
  await page.getByRole("option", { name: "1.5×" }).click();
  await expect.poll(() => audio(page).evaluate((a: HTMLAudioElement) => a.playbackRate)).toBe(1.5);
  const user = (await (await page.request.get("/api/v1/users/me")).json()) as {
    playback: { bookSpeed: number };
  };
  expect(user.playback.bookSpeed).toBe(1.5);

  // Stopping past two minutes saves the place; Resume on the page picks it
  // up in chapter 3.
  await sheet.getByRole("button", { name: "Stop listening" }).click();
  await expect(miniPlayer(page)).toHaveCount(0);
  await page.goto(`works/${fixture.hailMaryId}`);
  await page.getByRole("button", { name: "Resume", exact: true }).click();
  await expect(miniPlayer(page).locator('[data-part="mini-player-chapter"]')).toHaveText("Chapter 2");
  await playingPast(page, 120);
  await expect.poll(() => audio(page).evaluate((a: HTMLAudioElement) => a.playbackRate)).toBe(1.5);
});

test("a book of three MP3 files plays across them by chapter", async ({ page }) => {
  await page.goto(`works/${fixture.leviathanId}`);
  await page.getByRole("button", { name: "Play from Leviathan Wakes 3 of 3" }).click();
  await playingPast(page, 0.2);
  await expect(audio(page)).toHaveAttribute("src", /files\/2$/);
  await expect(miniPlayer(page).locator('[data-part="mini-player-chapter"]')).toHaveText("Leviathan Wakes 3 of 3");

  // Within 3 seconds of a chapter's start, previous goes to the chapter before: the second file.
  await miniPlayer(page).getByRole("button", { name: "Now Playing: Leviathan Wakes" }).click();
  const sheet = page.getByRole("dialog", { name: "Now Playing" });
  await sheet.getByRole("button", { name: "Previous chapter" }).click();
  await expect(audio(page)).toHaveAttribute("src", /files\/1$/);
  await expect(sheet.getByText("Chapter 2 of 3")).toBeVisible();
  await playingPast(page, 0.2);
  await sheet.getByRole("button", { name: "Stop listening" }).click();
});
