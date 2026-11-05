import { expect, type Locator, type Page, test } from "@playwright/test";
import { back, expectFocus, press } from "./remote";

// The TV player with only a remote, playing real media: the mock's playable
// scenario plays a ten-minute file, with two audio tracks and subtitles
// that show at once or take a new play.

// A TV app plays as it opens, with no click first.
test.use({ launchOptions: { args: ["--autoplay-policy=no-user-gesture-required"] } });

const white = "rgb(255, 255, 255)";
const player = (page: Page) => page.locator('[data-part="player"]');
const video = (page: Page) => page.locator('[data-part="player"] video');
const controls = (page: Page) => player(page).getAttribute("data-controls");

/** Dune: Part Two from Home's first card, playing. */
async function playDune(page: Page) {
  await page.goto("/?mock=playable");
  await expectFocus(page, { name: "Dune: Part Two" });
  await press(page, "Enter");
  await expectFocus(page, { key: "play" });
  await press(page, "Enter");
  await expect.poll(() => video(page).evaluate((v: HTMLVideoElement) => v.currentTime)).toBeGreaterThan(5);
  await expect.poll(() => video(page).evaluate((v: HTMLVideoElement) => v.paused)).toBe(false);
}

/** Waits for the controls to hide on their own, as they do while playing. */
async function waitHidden(page: Page) {
  await expect.poll(() => controls(page), { timeout: 15_000 }).toBe("hidden");
}

/** Waits until the scrub bar has focus. */
const expectOnBar = (page: Page) =>
  expect
    .poll(() => page.evaluate(() => document.activeElement?.closest('[data-part="scrub-bar"]') != null), {
      message: "focus on the scrub bar",
    })
    .toBe(true);

/** A key a remote sends by key code alone, as Tizen's media keys and Info are. */
const remoteKey = (page: Page, keyCode: number) =>
  page.evaluate((code) => {
    for (const type of ["keydown", "keyup"]) {
      document.activeElement?.dispatchEvent(
        new KeyboardEvent(type, { key: "Unidentified", keyCode: code, bubbles: true, cancelable: true }),
      );
    }
  }, keyCode);

/** The computed background, outline and contrast of the focused element's face. */
const focusLook = (el: Locator) =>
  el.evaluate((e) => {
    const rgb = (value: string) => (value.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number);
    const luminance = (value: string) => {
      const [r = 0, g = 0, b = 0] = rgb(value).map((c) => {
        const s = c / 255;
        return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    };
    const style = getComputedStyle(e);
    const [hi, lo] = [luminance(style.color), luminance(style.backgroundColor)].sort((a, b) => b - a);
    return {
      background: style.backgroundColor,
      outline: style.outlineStyle,
      contrast: ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05),
    };
  });

const focused = (page: Page) => page.locator(":focus");

/** The checked rows of the open Audio & Subtitles menu, by their text. */
const checked = (page: Page) =>
  page
    .getByRole("menu", { name: "Audio & Subtitles" })
    .locator('[role="menuitemradio"][aria-checked="true"]')
    .allInnerTexts();

test("the controls appear with Play/Pause focused, from the start and from OK, Up or Down", async ({ page }) => {
  await playDune(page);
  await expectFocus(page, { name: "Pause" });
  for (const key of ["Enter", "ArrowUp", "ArrowDown"] as const) {
    await waitHidden(page);
    await press(page, key);
    await expect.poll(() => controls(page)).toBe("shown");
    await expectFocus(page, { name: "Pause" });
  }
  // Left and Right scrub instead, on the scrub bar.
  await waitHidden(page);
  await press(page, "ArrowRight");
  await expectOnBar(page);
});

test("Left and Right go along the buttons in order and never into the scrub bar; Up reaches it", async ({ page }) => {
  await playDune(page);
  await expectFocus(page, { name: "Pause" });
  const order = ["Back 10 seconds", "Pause", "Forward 30 seconds", "Audio & Subtitles", "Playback Settings"];
  for (const name of order.slice(2)) {
    await press(page, "ArrowRight");
    await expectFocus(page, { name });
  }
  // The last button keeps focus.
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Playback Settings" });
  for (const name of order.slice(0, -1).reverse()) {
    await press(page, "ArrowLeft");
    await expectFocus(page, { name });
  }
  await press(page, "ArrowLeft");
  await expectFocus(page, { name: "Back 10 seconds" });
  await press(page, "ArrowUp");
  await expectOnBar(page);
  // Left and Right scrub there: the time shown moves back without leaving the bar.
  const before = await video(page).evaluate((v: HTMLVideoElement) => v.currentTime);
  await press(page, "ArrowLeft");
  await expectOnBar(page);
  await expect(page.locator('[data-part="scrub-preview"]')).toBeVisible();
  await press(page, "Enter");
  await expect
    .poll(() => video(page).evaluate((v: HTMLVideoElement) => v.currentTime))
    .toBeLessThan(Math.max(1, before - 5));
  await press(page, "ArrowDown");
  await expectFocus(page, { name: /^(Play|Pause)$/ });
});

test("the controls stay up long enough to read and move along them", async ({ page }) => {
  await playDune(page);
  await press(page, "ArrowRight", "ArrowRight");
  await expectFocus(page, { name: "Audio & Subtitles" });
  await page.waitForTimeout(3500);
  expect(await controls(page)).toBe("shown");
  await press(page, "ArrowLeft");
  await expectFocus(page, { name: "Forward 30 seconds" });
});

test("Audio & Subtitles opens on the audio playing, and the arrows move within and between its columns", async ({
  page,
}) => {
  await playDune(page);
  await press(page, "ArrowRight", "ArrowRight", "Enter");
  await expectFocus(page, { name: /^English · 7\.1/ });
  // Up within Audio reaches the boost and stops there.
  await press(page, "ArrowUp");
  await expectFocus(page, { name: "Dialogue boost" });
  await press(page, "ArrowUp");
  await expectFocus(page, { name: "Dialogue boost" });
  await press(page, "ArrowDown", "ArrowDown", "ArrowDown");
  await expectFocus(page, { name: /^French/ });
  // The last audio track keeps focus, rather than going on to Subtitles.
  await press(page, "ArrowDown");
  await expectFocus(page, { name: /^French/ });
  // Right lands on Subtitles' ticked row, and Left on Audio's.
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Off" });
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Off" });
  await press(page, "ArrowUp");
  await expectFocus(page, { name: "Off" });
  await press(page, "ArrowDown");
  await expectFocus(page, { name: /^English/ });
  await press(page, "ArrowLeft");
  await expectFocus(page, { name: /^English · 7\.1/ });
  // Back closes it, with focus back on its button.
  await back(page);
  await expect(page.getByRole("menu")).toHaveCount(0);
  await expectFocus(page, { name: "Audio & Subtitles" });
});

test("choosing an audio track with OK plays it, ticks it and keeps the menu open", async ({ page }) => {
  await playDune(page);
  await press(page, "ArrowRight", "ArrowRight", "Enter");
  await expectFocus(page, { name: /^English · 7\.1/ });
  await press(page, "ArrowDown");
  await expectFocus(page, { name: /^French/ });
  const started = page.waitForRequest((r) => r.url().endsWith("/api/v1/plays") && r.method() === "POST");
  await press(page, "Enter");
  expect((await started).postDataJSON()).toMatchObject({ audioTrackId: "4k-audio-fr" });
  await expect.poll(() => checked(page)).toEqual([expect.stringMatching(/^French/), "Off"]);
  await expectFocus(page, { name: /^French/ });
  // The new play plays on from where it was.
  await expect.poll(() => video(page).evaluate((v: HTMLVideoElement) => v.paused)).toBe(false);
  await expect.poll(() => video(page).evaluate((v: HTMLVideoElement) => v.currentTime)).toBeGreaterThan(5);
});

test("choosing a subtitle shows it on the video, and Off takes it away", async ({ page }) => {
  await playDune(page);
  await press(page, "ArrowRight", "ArrowRight", "Enter");
  await expectFocus(page, { name: /^English · 7\.1/ });
  await press(page, "ArrowRight", "ArrowDown");
  await expectFocus(page, { name: /^English/ });
  await press(page, "Enter");
  const showing = () =>
    video(page).evaluate((v: HTMLVideoElement) =>
      [...v.textTracks].filter((t) => t.mode === "showing").map((t) => t.label),
    );
  await expect.poll(showing).toHaveLength(1);
  await expect.poll(() => checked(page)).toEqual([expect.stringMatching(/^English · 7\.1/), "English"]);
  await press(page, "ArrowUp", "Enter");
  await expect.poll(showing).toHaveLength(0);
  await expect.poll(() => checked(page)).toEqual([expect.stringMatching(/^English · 7\.1/), "Off"]);
});

/**
 * Where the browser draws the subtitle, measured on the screen: the test
 * colours cues magenta, a colour nothing else in the player has, and finds
 * the rows they cover. The cue boxes are in the video's own shadow tree, so
 * the page can't measure them.
 */
async function cueRows(page: Page) {
  const shot = await page.screenshot({ type: "png" });
  return page.evaluate(async (data) => {
    const image = new Image();
    image.src = `data:image/png;base64,${data}`;
    await image.decode();
    const canvas = new OffscreenCanvas(image.width, image.height);
    const context = canvas.getContext("2d") as OffscreenCanvasRenderingContext2D;
    context.drawImage(image, 0, 0);
    const { data: px, width, height } = context.getImageData(0, 0, image.width, image.height);
    let top = -1;
    let bottom = -1;
    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x += 2) {
        const i = (y * width + x) * 4;
        const [r = 0, g = 0, b = 0] = px.subarray(i, i + 3);
        if (r > 200 && g < 80 && b > 200) {
          if (top < 0) top = y;
          bottom = y;
          break;
        }
      }
    }
    return { top, bottom };
  }, shot.toString("base64"));
}

test("a subtitle sits above the controls while they show, and inside the safe area when they hide", async ({
  page,
}) => {
  await playDune(page);
  await page.addStyleTag({ content: "video::cue { color: rgb(255, 0, 255); background: transparent; }" });
  await press(page, "ArrowRight", "ArrowRight", "Enter");
  await press(page, "ArrowRight", "ArrowDown");
  await expectFocus(page, { name: /^English/ });
  await press(page, "Enter");
  await back(page);
  await expect(page.getByRole("menu")).toHaveCount(0);
  expect(await controls(page)).toBe("shown");
  const bar = await page.locator('[data-part="scrub-bar"]').boundingBox();
  await expect.poll(async () => (await cueRows(page)).bottom).toBeGreaterThan(0);
  await expect.poll(async () => (await cueRows(page)).bottom).toBeLessThanOrEqual(bar?.y ?? 0);
  await waitHidden(page);
  // The bottom of the safe area: the screen less the overscan.
  const safe = await page.evaluate(
    () =>
      1080 - Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--layout-tv-overscan-y")),
  );
  await expect.poll(async () => (await cueRows(page)).bottom).toBeLessThanOrEqual(safe);
  // Settled back down, it's no higher than it was above the controls.
  expect((await cueRows(page)).bottom).toBeGreaterThan(bar?.y ?? 0);
});

test("a focused control or menu row is lit white with dark text, with no outline", async ({ page }) => {
  await playDune(page);
  await expectFocus(page, { name: "Pause" });
  for (const name of ["Pause", "Forward 30 seconds", "Audio & Subtitles"]) {
    await expectFocus(page, { name });
    const look = await focusLook(focused(page));
    expect(look.background, name).toBe(white);
    expect(look.outline, name).toBe("none");
    expect(look.contrast, name).toBeGreaterThanOrEqual(4.5);
    await press(page, "ArrowRight");
  }
  await press(page, "ArrowLeft", "Enter");
  await expectFocus(page, { name: /^English · 7\.1/ });
  const row = await focusLook(focused(page));
  expect(row.background).toBe(white);
  expect(row.outline).toBe("none");
  expect(row.contrast).toBeGreaterThanOrEqual(4.5);
  // The ticked row that hasn't focus isn't lit; its check mark says it's chosen.
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Off" });
  const audio = page.getByRole("menuitemradio", { name: /^English · 7\.1/ });
  expect((await focusLook(audio)).background).not.toBe(white);
  await expect(audio).toHaveAttribute("aria-checked", "true");
});

test("the scrub bar shows focus on its thumb, without an outline", async ({ page }) => {
  await playDune(page);
  await press(page, "ArrowUp");
  await expectOnBar(page);
  const thumb = page
    .locator('[data-part="scrub-bar"] input')
    .locator("xpath=ancestor::div[contains(@class, 'rounded-full')][1]");
  await page.waitForTimeout(400);
  const look = await thumb.evaluate((e) => {
    const middle = (r: DOMRect) => (r.top + r.bottom) / 2;
    const bar = (e.parentElement as Element).querySelector(":scope > div")?.getBoundingClientRect() ?? new DOMRect();
    return {
      outline: getComputedStyle(e).outlineStyle,
      scale: getComputedStyle(e).scale,
      offBar: Math.abs(middle(e.getBoundingClientRect()) - middle(bar)),
    };
  });
  expect(look.outline).toBe("none");
  expect(look.scale).not.toBe("none");
  // Grown, it stays centred on the bar.
  expect(look.offBar).toBeLessThan(1);
});

test("Playback Settings: Down moves along its rows, Right opens a list on its choice, Back goes back a list at a time", async ({
  page,
}) => {
  await playDune(page);
  await press(page, "ArrowRight", "ArrowRight", "ArrowRight", "Enter");
  await expectFocus(page, { name: "Audio & Subtitles" });
  await press(page, "ArrowDown");
  await expectFocus(page, { name: /^Quality/ });
  await press(page, "ArrowRight");
  await expectFocus(page, { name: /^Auto/ });
  await back(page);
  await expectFocus(page, { name: /^Quality/ });
  await back(page);
  await expect(page.getByRole("menu")).toHaveCount(0);
  await expectFocus(page, { name: "Playback Settings" });
});

test("Back closes the menu, then hides the controls, then leaves for the detail page on Play", async ({ page }) => {
  await playDune(page);
  await press(page, "ArrowRight", "ArrowRight", "Enter");
  await expect(page.getByRole("menu")).toBeVisible();
  await back(page);
  await expect(page.getByRole("menu")).toHaveCount(0);
  expect(await controls(page)).toBe("shown");
  await back(page);
  await expect.poll(() => controls(page)).toBe("hidden");
  await back(page);
  await expect(page).toHaveURL(/\/works\//);
  await expectFocus(page, { key: "play" });
});

test("the media keys and Info work whatever has focus", async ({ page }) => {
  await playDune(page);
  const paused = () => video(page).evaluate((v: HTMLVideoElement) => v.paused);
  // Play/Pause (10252), Stop (413) and Play (415).
  await remoteKey(page, 10252);
  await expect.poll(paused).toBe(true);
  await remoteKey(page, 10252);
  await expect.poll(paused).toBe(false);
  await remoteKey(page, 413);
  await expect.poll(paused).toBe(true);
  await remoteKey(page, 415);
  await expect.poll(paused).toBe(false);
  // Fast forward (417) moves on 30 seconds.
  const at = await video(page).evaluate((v: HTMLVideoElement) => v.currentTime);
  await remoteKey(page, 417);
  await expect.poll(() => video(page).evaluate((v: HTMLVideoElement) => v.currentTime)).toBeGreaterThan(at + 20);
  // Info (457) opens the stream info and closes it again.
  await remoteKey(page, 457);
  await expect(page.getByRole("dialog")).toBeVisible();
  await remoteKey(page, 457);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("Skip Intro takes focus over hidden controls, lit, and OK skips past the intro", async ({ page }) => {
  await page.goto("/watch/0190a000-0000-7000-8000-000000000518?mock=playable");
  const skip = page.locator('[data-part="skip-marker"]');
  await expect(skip).toBeVisible();
  await waitHidden(page);
  await expectFocus(page, { name: "Skip Intro" });
  expect((await focusLook(skip)).background).toBe(white);
  await press(page, "Enter");
  await expect.poll(() => video(page).evaluate((v: HTMLVideoElement) => v.currentTime)).toBeGreaterThanOrEqual(150);
  expect(await controls(page)).toBe("hidden");
});
