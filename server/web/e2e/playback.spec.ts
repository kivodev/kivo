import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { type Cookie, chromium, expect, type Page, test } from "@playwright/test";
import { e2e, type Fixture } from "./env";

// Playback in a real browser against the real server: play a
// file that needs a video transcode, scrub with previews, reach its credits,
// switch subtitle tracks, change quality and boost its dialogue.

const fixture = JSON.parse(readFileSync(e2e.fixture, "utf8")) as Fixture;

const video = (page: Page) => page.locator("video");

/** The video's position in seconds. */
const position = (page: Page) => video(page).evaluate((v: HTMLVideoElement) => v.currentTime);

/** Waits until the video is playing at or past `seconds`. */
async function playingPast(page: Page, seconds: number) {
  await expect
    .poll(() => video(page).evaluate((v: HTMLVideoElement) => !v.paused && v.readyState >= 3 && v.currentTime), {
      timeout: 45_000,
    })
    .toBeGreaterThan(seconds);
}

/** The text of every cue showing now, from every text track that's showing. */
function shownCues(page: Page): Promise<string[]> {
  return video(page).evaluate((v: HTMLVideoElement) =>
    Array.from(v.textTracks)
      .filter((t) => t.mode === "showing")
      .flatMap((t) => Array.from(t.activeCues ?? []).map((c) => (c as VTTCue).text)),
  );
}

async function streamInfo(page: Page) {
  await reveal(page);
  await page.getByRole("button", { name: "Playback Settings" }).click();
  await page.getByRole("menuitem", { name: "Stream info" }).click();
  const info = page.getByRole("dialog", { name: "Stream info" });
  await expect(info).toBeVisible();
  return info;
}

/** Shows the controls, which hide while playing. */
async function reveal(page: Page) {
  await page.mouse.move(10, 10);
  await page.mouse.move(20, 20);
}

test("a file that needs a video transcode plays, scrubs with previews and offers Skip Credits", async ({ page }) => {
  await page.goto(`watch/${fixture.workId}?from=start`);
  await playingPast(page, 1);

  const info = await streamInfo(page);
  await expect(info.getByText("Transcode", { exact: true })).toBeVisible();
  await expect(info.getByText("Why: This device can't play the video codec.").first()).toBeVisible();
  await page.keyboard.press("Escape");

  // A film's Intro chapter is no marker: it has no Skip Intro.
  await reveal(page);
  await expect(page.getByRole("button", { name: "Skip Intro" })).toHaveCount(0);

  // Dragging the scrub bar shows the preview frame for the target; the seek happens on release.
  await reveal(page);
  const bar = page.locator('[data-part="scrub-bar"]');
  const box = await bar.boundingBox();
  if (!box) throw new Error("no scrub bar");
  const y = box.y + box.height / 2;
  await page.mouse.move(box.x + box.width * 0.1, y);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.75, y, { steps: 8 });
  await expect(page.locator('[data-part="scrub-frame"]')).toBeVisible();
  await page.mouse.up();
  await playingPast(page, 85);

  // Its credits run from 1:40 to the end; Skip Credits shows as playback enters them.
  await expect(page.getByRole("button", { name: "Skip Credits" })).toBeVisible({ timeout: 45_000 });
});

test("subtitles: the chosen track in the stream, another from the stream at once, then off", async ({ page }) => {
  const starts: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith("/api/v1/plays")) starts.push(r.url());
  });
  await page.goto(`watch/${fixture.workId}?from=start&subtitle=${fixture.english}`);
  await playingPast(page, 1);
  // English is in the stream, showing from the first segment.
  await expect.poll(() => shownCues(page)).toEqual([expect.stringMatching(/^English \d+$/)]);

  // Every text track is a rendition of the stream: French shows at once, in the same play.
  await reveal(page);
  await page.getByRole("button", { name: "Audio & Subtitles" }).click();
  await page.getByRole("menuitemradio", { name: /^French/ }).click();
  await expect.poll(() => shownCues(page)).toEqual([expect.stringMatching(/^French \d+$/)]);

  await reveal(page);
  await page.getByRole("button", { name: "Audio & Subtitles" }).click();
  await page.getByRole("menuitemradio", { name: "Off" }).click();
  await expect.poll(() => shownCues(page)).toEqual([]);
  expect(starts).toHaveLength(1);
});

test("changing quality starts a new play at the same place, within the cap", async ({ page }) => {
  await page.goto(`watch/${fixture.workId}?from=start`);
  await playingPast(page, 3);

  await reveal(page);
  await page.getByRole("button", { name: "Playback Settings" }).click();
  await page.getByRole("menuitem", { name: /^Quality/ }).click();
  const start = page.waitForRequest(
    (r) =>
      r.method() === "POST" &&
      r.url().endsWith("/api/v1/plays") &&
      r.postDataJSON()?.settings?.quality?.mode === "limit",
  );
  await page.getByRole("menuitemradio", { name: /^480p · 2 Mb\/s/ }).click();
  const body = (await start).postDataJSON() as { settings: { quality: { maxBitrateBps: number } }; positionMs: number };
  expect(body.settings.quality.maxBitrateBps).toBe(2_000_000);
  expect(body.positionMs).toBeGreaterThan(2000);

  await playingPast(page, body.positionMs / 1000);
  const info = await streamInfo(page);
  await expect(info.getByText("Limited to 2 Mb/s")).toBeVisible();
  expect(await position(page)).toBeGreaterThan(body.positionMs / 1000);
});

test("dialogue boost starts a new play at the same place, its audio encoded for it", async ({ page }) => {
  await page.goto(`watch/${fixture.workId}?from=start`);
  await playingPast(page, 3);

  await reveal(page);
  await page.getByRole("button", { name: "Audio & Subtitles" }).click();
  const boost = page.getByRole("menuitemcheckbox", { name: /^Dialogue boost/ });
  await expect(boost).not.toBeChecked();
  const start = page.waitForRequest(
    (r) => r.method() === "POST" && r.url().endsWith("/api/v1/plays") && r.postDataJSON()?.dialogueBoost === true,
  );
  await boost.click();
  const body = (await start).postDataJSON() as { positionMs: number };
  expect(body.positionMs).toBeGreaterThan(2000);
  await expect(boost).toBeChecked();
  await page.keyboard.press("Escape");

  await playingPast(page, body.positionMs / 1000);
  const info = await streamInfo(page);
  await expect(info.getByText("Why: The audio is re-encoded to boost dialogue.")).toBeVisible();
});

/**
 * Plays the styled ASS track from the start and waits for JASSUB's canvas
 * over the video. JASSUB fetches the track once its fonts are loaded.
 */
async function playStyled(page: Page) {
  const ass = page.waitForResponse((r) => r.ok() && r.url().endsWith(`/subtitles/${fixture.styled}.ass`));
  await page.goto(`watch/${fixture.workId}?from=start&subtitle=${fixture.styled}`);
  await ass;
  await playingPast(page, 1);
  const canvas = page.locator('[data-part="subtitle-layer"] canvas');
  await expect(canvas).toHaveCount(1);
  await expect.poll(async () => (await canvas.boundingBox())?.width ?? 0).toBeGreaterThan(100);
}

/** How many times the server has answered the fonts route, from its metrics. */
async function fontsServed(page: Page): Promise<number> {
  const metrics = await (await page.request.get("api/v1/metrics")).text();
  return metrics
    .split("\n")
    .filter((line) => line.startsWith("kivo_http_requests_total{") && line.includes("/api/v1/fonts/"))
    .reduce((n, line) => n + Number(line.slice(line.lastIndexOf(" ") + 1)), 0);
}

test("styled ASS is drawn by JASSUB over the video, with the file's font", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const loaded = { ass: false, font: false, wasm: false };
  page.on("response", (r) => {
    if (r.ok() && r.url().endsWith(`/subtitles/${fixture.styled}.ass`)) loaded.ass = true;
    if (r.ok() && /\/api\/v1\/fonts\/[0-9a-f]{64}$/.test(r.url())) loaded.font = true;
    if (r.ok() && r.url().endsWith(".wasm")) loaded.wasm = true;
  });
  await playStyled(page);
  // Prepared, so the play lists the original and its font rather than streaming it plain.
  await expect.poll(() => ({ ...loaded })).toEqual({ ass: true, font: true, wasm: true });
  expect(await shownCues(page)).toEqual([]);
  const info = await streamInfo(page);
  await expect(info.getByText(/subtitles are being prepared/)).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("a second play draws styled ASS with the fonts the browser cached", async () => {
  // A browser's user data on disk, as a viewer's is: a test context caches in a
  // small store in memory, which the video's segments soon fill.
  const userDataDir = mkdtempSync(join(tmpdir(), "kivo-e2e-browser-"));
  const context = await chromium.launchPersistentContext(userDataDir, { channel: "chromium", baseURL: e2e.baseURL });
  try {
    const state = JSON.parse(readFileSync(e2e.storageState, "utf8")) as { cookies: Cookie[] };
    await context.addCookies(state.cookies);
    const page = context.pages()[0] ?? (await context.newPage());
    await playStyled(page);
    const served = await fontsServed(page);
    expect(served).toBeGreaterThan(0);
    // Another play: its font URLs are the same, cacheable for good, so
    // none reaches the server.
    await page.goto("");
    await playStyled(page);
    expect(await fontsServed(page)).toBe(served);
  } finally {
    await context.close();
    rmSync(userDataDir, { recursive: true, force: true });
  }
});
