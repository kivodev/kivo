import { expect, type Page, test } from "@playwright/test";
import { mockId } from "@/api/mocks/ids";
import { filmUrl } from "./media";

// The TV build's player, key by key, against the mock's The Expanse S3E4
// (44 minutes, an intro from 1:00 to 2:30, Up next from 43:25): each press
// is owned from its first keydown to its keyup, and what it does follows
// the player's remote rules. OK and the arrows are real key presses; Back, the media keys and
// Info are the KeyboardEvents a TV's browser sends for them.

/** The Expanse S3E4 "Reload", and the episode after it. */
const episode = mockId(1304);
const next = mockId(1305);

const video = (page: Page) => page.locator("video");
const seekBar = (page: Page) => page.getByRole("slider", { name: "Seek" });
const button = (page: Page, name: string) => page.getByRole("button", { name, exact: true });

/** The video's position in seconds. */
const position = (page: Page) => video(page).evaluate((v: HTMLVideoElement) => v.currentTime);

/** Whether the controls show. */
const controls = (page: Page) => page.locator('[data-part="player"]').getAttribute("data-controls");

/** The media's play, pause and seeking events since the player settled at its start position. */
const mediaEvents = (page: Page) => page.evaluate(() => (window as unknown as { mediaEvents: string[] }).mediaEvents);

async function playingPast(page: Page, seconds: number) {
  await expect
    .poll(() => video(page).evaluate((v: HTMLVideoElement) => !v.paused && v.readyState >= 3 && v.currentTime))
    .toBeGreaterThanOrEqual(seconds);
}

/**
 * Opens the player playing from `at` seconds, its media the run's film; with
 * `loading`, the media never loads. Records the media's events from there.
 */
async function open(page: Page, { at = 600, loading = false } = {}) {
  await page.addInitScript(
    ({ filmUrl, loading }) => {
      // The mock's service worker passes the film on without ranges, so it plays from memory, where it seeks.
      // biome-ignore lint/style/noRestrictedGlobals: the test's film, not the API
      const film = fetch(filmUrl)
        .then((r) => r.blob())
        .then((b) => URL.createObjectURL(b));
      const src = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, "src");
      Object.defineProperty(HTMLMediaElement.prototype, "src", {
        configurable: true,
        get() {
          return src?.get?.call(this);
        },
        set(url: string) {
          if (!/\/plays\/[^/]+\/files\/\d+$/.test(url)) src?.set?.call(this, url);
          else if (!loading) void film.then((blob) => src?.set?.call(this, blob));
        },
      });
      const events: string[] = [];
      (window as unknown as { mediaEvents: string[] }).mediaEvents = events;
      for (const type of ["play", "pause", "seeking"]) document.addEventListener(type, () => events.push(type), true);
    },
    { filmUrl, loading },
  );
  await page.goto(`watch/${episode}`);
  if (loading) {
    await expect(page.getByRole("status").filter({ hasText: "Loading" })).toBeAttached();
    return;
  }
  await playingPast(page, 0);
  await playTo(page, at);
  await playingPast(page, at);
  await page.evaluate(() => {
    (window as unknown as { mediaEvents: string[] }).mediaEvents.length = 0;
  });
}

/** Sends a key as a TV's browser does, to what has focus: `repeats` repeats, then its release. */
async function send(page: Page, key: string, { keyCode = 0, repeats = 0 } = {}) {
  await page.evaluate(
    ({ key, keyCode, repeats }) => {
      const target = document.activeElement ?? document.body;
      const event = (type: string, repeat: boolean) =>
        target.dispatchEvent(new KeyboardEvent(type, { key, keyCode, repeat, bubbles: true, cancelable: true }));
      event("keydown", false);
      for (let i = 0; i < repeats; i++) event("keydown", true);
      event("keyup", false);
    },
    { key, keyCode, repeats },
  );
}

/** Holds a real key down for `repeats` repeats, then releases it. */
async function hold(page: Page, key: string, repeats: number) {
  for (let i = 0; i <= repeats; i++) await page.keyboard.down(key);
  await page.keyboard.up(key);
}

/** Whether focus is on something in the player, not left on the page. */
const focusInPlayer = (page: Page) =>
  page.evaluate(() => document.activeElement?.closest('[data-part="player"]') !== null);

async function hidden(page: Page) {
  await expect.poll(() => controls(page), { timeout: 5_000 }).toBe("hidden");
}

async function exited(page: Page) {
  await expect(page).not.toHaveURL(/\/watch\//);
}

test("Back while the video loads leaves in one press", async ({ page }) => {
  // R1
  await open(page, { loading: true });
  expect(await controls(page)).toBe("shown");
  await send(page, "GoBack");
  await exited(page);
});

test("OK with the controls hidden shows them on play or pause and keeps playing, held or not", async ({ page }) => {
  // R2, R3
  await open(page);
  await hidden(page);
  await hold(page, "Enter", 20);
  expect(await controls(page)).toBe("shown");
  await expect(button(page, "Pause")).toBeFocused();
  expect(await mediaEvents(page)).toEqual([]);
});

test("OK held on the scrub bar plays or pauses once", async ({ page }) => {
  // R4
  await open(page);
  await send(page, "MediaPause");
  await expect(seekBar(page)).toBeFocused();
  await hold(page, "Enter", 20);
  await expect.poll(() => mediaEvents(page)).toEqual(["pause", "play"]);
});

test("Back with a button focused hides the controls and presses nothing", async ({ page }) => {
  // R5
  await open(page);
  await page.keyboard.press("ArrowDown");
  await expect(button(page, "Pause")).toBeFocused();
  await send(page, "GoBack");
  expect(await controls(page)).toBe("hidden");
  await expect(page).toHaveURL(/\/watch\//);
  expect(await mediaEvents(page)).toEqual([]);
});

test("Back held hides the controls once", async ({ page }) => {
  // R6
  await open(page);
  await send(page, "GoBack", { repeats: 10 });
  expect(await controls(page)).toBe("hidden");
  await expect(page).toHaveURL(/\/watch\//);
});

test("Back with the controls hidden leaves", async ({ page }) => {
  // R7
  await open(page);
  await hidden(page);
  await send(page, "GoBack");
  await exited(page);
});

test("Back while paused hides the controls, then leaves", async ({ page }) => {
  // R8
  await open(page);
  await send(page, "MediaPause");
  await send(page, "GoBack");
  expect(await controls(page)).toBe("hidden");
  await expect(page).toHaveURL(/\/watch\//);
  await send(page, "GoBack");
  await exited(page);
});

test("Right with the controls hidden shows them and seeks 10 seconds on a second later", async ({ page }) => {
  // R9
  await open(page);
  await hidden(page);
  const from = await position(page);
  await page.keyboard.press("ArrowRight");
  expect(await controls(page)).toBe("shown");
  await expect(seekBar(page)).toBeFocused();
  await expect.poll(() => mediaEvents(page)).toEqual(["seeking"]);
  expect(await position(page)).toBeGreaterThanOrEqual(from + 10);
  expect(await position(page)).toBeLessThan(from + 13);
});

test("Right held from hidden scrubs faster and seeks once, on the scrub bar throughout", async ({ page }) => {
  // R10
  await open(page);
  await hidden(page);
  const from = await position(page);
  await hold(page, "ArrowRight", 20);
  await expect(seekBar(page)).toBeFocused();
  await expect.poll(() => mediaEvents(page)).toEqual(["seeking"]);
  // Five steps of 10 seconds, ten of 30 and six of 60.
  expect(await position(page)).toBeGreaterThanOrEqual(from + 710);
  expect(await position(page)).toBeLessThan(from + 713);
});

test("Back during a scrub drops it and hides the controls", async ({ page }) => {
  // R11
  await open(page);
  await hidden(page);
  await page.keyboard.press("ArrowRight");
  await send(page, "GoBack");
  expect(await controls(page)).toBe("hidden");
  await page.waitForTimeout(1_500);
  expect(await mediaEvents(page)).toEqual([]);
});

test("OK during a scrub seeks at once and stays paused", async ({ page }) => {
  // R12
  await open(page);
  await send(page, "MediaPause");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("Enter");
  await expect.poll(() => mediaEvents(page)).toEqual(["pause", "seeking"]);
  expect(await video(page).evaluate((v: HTMLVideoElement) => v.paused)).toBe(true);
});

test("Up and Down on the scrub bar move focus and never seek", async ({ page }) => {
  // R13
  await open(page);
  await expect(seekBar(page)).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await expect(seekBar(page)).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(button(page, "Pause")).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await expect(seekBar(page)).toBeFocused();
  await page.waitForTimeout(1_200);
  expect(await mediaEvents(page)).toEqual([]);
});

test("Right over Skip Intro shows the controls and scrubs", async ({ page }) => {
  // R14
  await open(page, { at: 70 });
  await hidden(page);
  await expect(button(page, "Skip Intro")).toBeFocused();
  const from = await position(page);
  await page.keyboard.press("ArrowRight");
  expect(await controls(page)).toBe("shown");
  await expect.poll(() => mediaEvents(page)).toEqual(["seeking"]);
  expect(await position(page)).toBeGreaterThanOrEqual(from + 10);
});

test("OK over Skip Intro skips it with the controls hidden", async ({ page }) => {
  // R15
  await open(page, { at: 70 });
  await hidden(page);
  await page.keyboard.press("Enter");
  await expect.poll(() => position(page)).toBeGreaterThanOrEqual(150);
  expect(await controls(page)).toBe("hidden");
});

test("Skip Intro takes focus as the controls hide under it, and isn't pressed", async ({ page }) => {
  // R16
  await open(page, { at: 70 });
  await page.keyboard.press("ArrowUp");
  await expect(seekBar(page)).toBeFocused();
  await hidden(page);
  await expect(button(page, "Skip Intro")).toBeFocused();
  expect(await mediaEvents(page)).toEqual([]);
});

test("Skip Intro hides 10 seconds after the intro starts, and comes back with the controls", async ({ page }) => {
  await open(page, { at: 70 });
  await hidden(page);
  await expect(button(page, "Skip Intro")).toBeFocused();
  await expect(button(page, "Skip Intro")).toBeHidden({ timeout: 12_000 });
  await page.keyboard.press("ArrowUp");
  expect(await controls(page)).toBe("shown");
  await expect(button(page, "Skip Intro")).toBeVisible();
  await hidden(page);
  await expect(button(page, "Skip Intro")).toBeHidden();
});

/** Moves the video to `seconds`, as playing on would, without a key. */
async function playTo(page: Page, seconds: number) {
  await video(page).evaluate((v: HTMLVideoElement, s) => {
    v.currentTime = s;
  }, seconds);
}

test("Up next appearing over shown controls leaves focus where it is", async ({ page }) => {
  // R17
  await open(page, { at: 2590 });
  await send(page, "MediaPause");
  await expect(seekBar(page)).toBeFocused();
  await playTo(page, 2606);
  await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
  await expect(seekBar(page)).toBeFocused();
});

test("Left and Right over Up next move between its buttons with the controls hidden", async ({ page }) => {
  // R18
  await open(page, { at: 2606 });
  await hidden(page);
  await expect(button(page, "Play now")).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(button(page, "Hide")).toBeFocused();
  await page.keyboard.press("ArrowLeft");
  await expect(button(page, "Play now")).toBeFocused();
  expect(await controls(page)).toBe("hidden");
});

test("Back over Up next leaves", async ({ page }) => {
  // R19
  await open(page, { at: 2606 });
  await hidden(page);
  await send(page, "GoBack");
  await exited(page);
});

test("Up next's countdown stops while a menu is open, and Back closes the menu onto its button", async ({ page }) => {
  // R20, R30
  await open(page, { at: 2606 });
  await hidden(page);
  await page.keyboard.press("ArrowDown");
  const trigger = page.locator('[data-part="track-menus"] button').first();
  await expect(trigger).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu")).toBeVisible();
  const countdown = page.getByText(/Playing in \d+ s/);
  const frozen = await countdown.textContent();
  await page.waitForTimeout(3_000);
  expect(await countdown.textContent()).toBe(frozen);
  await send(page, "GoBack");
  await expect(page.getByRole("menu")).toBeHidden();
  await expect(trigger).toBeFocused();
  await expect(countdown).not.toHaveText(frozen ?? "");
});

test("OK at Still watching? plays the next episode with the controls shown", async ({ page }) => {
  // R21
  await open(page, { at: 2590 });
  await send(page, "MediaPause");
  // Three episodes played on without input, through the dev server's copy of the player's module; a key would end the run.
  await page.evaluate(async (path) => {
    const { autoplay } = await import(/* @vite-ignore */ path);
    for (let i = 0; i < 3; i++) autoplay.played(true);
  }, "/src/features/player/controller.ts");
  await playTo(page, 2606);
  await expect(page.getByText("Still watching?")).toBeVisible();
  expect(await controls(page)).toBe("shown");
  await expect(button(page, "Play now")).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(new RegExp(`/watch/${next}`));
});

test("media keys in a menu do what they do without one", async ({ page }) => {
  // R22
  await open(page);
  await hidden(page);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu")).toBeVisible();
  await send(page, "MediaPlayPause");
  await expect.poll(() => video(page).evaluate((v: HTMLVideoElement) => v.paused)).toBe(true);
  const from = await position(page);
  await send(page, "MediaFastForward");
  await expect.poll(() => position(page)).toBeGreaterThanOrEqual(from + 30);
  await send(page, "MediaRewind");
  await expect.poll(() => position(page)).toBeLessThan(from + 21);
  await expect(page.getByRole("menu")).toBeVisible();
  await send(page, "MediaTrackNext");
  await expect(page).toHaveURL(new RegExp(`/watch/${next}`));
});

test("Info opens and closes the stream info, whatever has focus", async ({ page }) => {
  // R24
  await open(page);
  await send(page, "Info");
  const info = page.getByRole("dialog", { name: "Stream info" });
  await expect(info).toBeVisible();
  await send(page, "Info");
  await expect(info).toBeHidden();
});

test("Play/Pause held toggles once", async ({ page }) => {
  // R25
  await open(page);
  await hidden(page);
  await send(page, "MediaPlayPause", { repeats: 20 });
  await page.waitForTimeout(300);
  expect(await mediaEvents(page)).toEqual(["pause"]);
});

test("Next held goes to one episode", async ({ page }) => {
  // R26
  await open(page);
  await hidden(page);
  await send(page, "MediaTrackNext", { repeats: 10 });
  await expect(page).toHaveURL(new RegExp(`/watch/${next}`));
  await page.waitForTimeout(500);
  await expect(page).toHaveURL(new RegExp(`/watch/${next}`));
});

test("Fast forward held skips on each repeat", async ({ page }) => {
  // R27
  await open(page);
  await hidden(page);
  const from = await position(page);
  await send(page, "MediaFastForward", { repeats: 10 });
  await expect.poll(() => position(page)).toBeGreaterThanOrEqual(from + 330);
});

test("fast forward and rewind move a pending scrub target", async ({ page }) => {
  await open(page);
  await hidden(page);
  const from = await position(page);
  await page.keyboard.press("ArrowRight");
  await send(page, "MediaFastForward");
  await send(page, "MediaRewind");
  expect(await mediaEvents(page)).toEqual([]);
  await expect.poll(() => mediaEvents(page)).toEqual(["seeking"]);
  expect(await position(page)).toBeGreaterThanOrEqual(from + 30);
  expect(await position(page)).toBeLessThan(from + 33);
});

/** Moves focus right along the controls to Playback Settings, the last button. */
async function focusSettings(page: Page) {
  await page.keyboard.press("ArrowDown");
  const settings = button(page, "Playback Settings");
  while (!(await settings.evaluate((b) => b === document.activeElement))) await page.keyboard.press("ArrowRight");
}

test("OK held on a menu's button opens it and chooses nothing until a fresh press", async ({ page }) => {
  // R28
  await open(page);
  await hidden(page);
  await focusSettings(page);
  await hold(page, "Enter", 20);
  const menu = page.getByRole("menu", { name: "Playback Settings" });
  await expect(menu).toBeVisible();
  await page.waitForTimeout(300);
  await expect(menu).toBeVisible();
  // A fresh press chooses the first row, Audio & Subtitles, which opens its menu in this one's place.
  await page.keyboard.press("Enter");
  await expect(menu).toBeHidden();
  await expect(page.getByRole("menu", { name: "Audio & Subtitles" })).toBeVisible();
});

test("the controls hide 3 seconds after the last key while playing, and not while paused or in a menu", async ({
  page,
}) => {
  // R29
  await open(page);
  await page.keyboard.press("ArrowUp");
  await page.waitForTimeout(2_600);
  expect(await controls(page)).toBe("shown");
  await page.waitForTimeout(700);
  expect(await controls(page)).toBe("hidden");
  await send(page, "MediaPause");
  await page.waitForTimeout(4_000);
  expect(await controls(page)).toBe("shown");
  await send(page, "MediaPlay");
  await send(page, "Info");
  await page.waitForTimeout(4_000);
  expect(await controls(page)).toBe("shown");
});

for (const [name, key, keyCode] of [
  ["Escape", "Escape", 27],
  ["GoBack", "GoBack", 0],
  ["BrowserBack", "BrowserBack", 166],
  ["XF86Back", "XF86Back", 10009],
  ["webOS's Back", "Unidentified", 461],
] as const) {
  test(`${name} is Back: it hides shown controls and leaves over Skip`, async ({ page }) => {
    // R31
    await open(page, { at: 70 });
    await send(page, key, { keyCode });
    expect(await controls(page)).toBe("hidden");
    await expect(button(page, "Skip Intro")).toBeFocused();
    await send(page, key, { keyCode });
    await exited(page);
  });
}

test("focus stays in the player after the stream info closes and after a quality pick", async ({ page }) => {
  // R32
  await open(page);
  await send(page, "Info");
  await expect(page.getByRole("dialog", { name: "Stream info" })).toBeVisible();
  await send(page, "Escape");
  await expect(page.getByRole("dialog", { name: "Stream info" })).toBeHidden();
  await expect.poll(() => focusInPlayer(page)).toBe(true);

  // Quality is the gear's second row; Right opens its submenu, on Auto.
  await focusSettings(page);
  await page.keyboard.press("Enter");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("menu", { name: /^Quality/ })).toBeVisible();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu")).toBeHidden();
  // The new play loads with the controls shown, and focus comes back to them as it goes live.
  expect(await controls(page)).toBe("shown");
  await expect.poll(() => focusInPlayer(page)).toBe(true);
});

test("a release whose press wasn't seen does nothing", async ({ page }) => {
  // R33
  await open(page);
  await hidden(page);
  await page.evaluate(() => {
    document.body.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
    document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", repeat: true, bubbles: true }));
    document.body.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
  });
  await page.waitForTimeout(300);
  expect(await controls(page)).toBe("hidden");
  expect(await mediaEvents(page)).toEqual([]);
});

test("Down with the controls hidden shows them on Audio & Subtitles, over Skip too, and Right reaches the gear", async ({
  page,
}) => {
  // R34
  await open(page, { at: 70 });
  await hidden(page);
  await page.keyboard.press("ArrowDown");
  await expect(button(page, "Audio & Subtitles")).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(button(page, "Playback Settings")).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(button(page, "Playback Settings")).toBeFocused();
  await page.keyboard.press("ArrowLeft");
  await expect(button(page, "Audio & Subtitles")).toBeFocused();
});

test("the dialogue boost switches from the remote, its menu staying open, and Back closes it", async ({ page }) => {
  await open(page);
  await hidden(page);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  const menu = page.getByRole("menu", { name: "Audio & Subtitles" });
  const boost = menu.getByRole("menuitemcheckbox", { name: /^Dialogue boost/ });
  await expect(boost).not.toBeChecked();
  while (!(await boost.evaluate((b) => b === document.activeElement))) await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  // The new play, asked for with the boost, says it's on.
  await expect(boost).toBeChecked();
  await expect(menu).toBeVisible();
  await send(page, "GoBack");
  await expect(menu).toBeHidden();
  await expect(button(page, "Audio & Subtitles")).toBeFocused();
});

test("the gear's rows come in order, and Stream info opens the stream info", async ({ page }) => {
  await open(page);
  await focusSettings(page);
  await page.keyboard.press("Enter");
  const menu = page.getByRole("menu", { name: "Playback Settings" });
  await expect(menu.getByRole("menuitem")).toHaveText(["Audio & Subtitles", /^Quality/, /^Speed/, "Stream info"]);
  const streamInfo = menu.getByRole("menuitem", { name: "Stream info" });
  while (!(await streamInfo.evaluate((b) => b === document.activeElement))) await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "Stream info" })).toBeVisible();
  await expect(menu).toBeHidden();
});
