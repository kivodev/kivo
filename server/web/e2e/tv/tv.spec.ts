import { expect, type Page, test } from "@playwright/test";
import { expectSteadyLogos, recordLogos } from "./logo";
import { back, expectFocus, focusOf, overscan, press, pressUntil } from "./remote";

// The TV build driven the way a TV's remote drives it: the four arrows,
// OK (Enter) and Back (Escape), and nothing else. No test clicks or
// focuses anything itself, so a screen a remote can't reach fails here. Focus is read from what the page focuses.

const card = (row: string) => new RegExp(`^card:${row}:`);
const inRail = (f: { part: string | null }) => f.part === "tv-rail-entry";

/** Opens a screen and waits for focus to settle on its first item. */
async function open(page: Page, path: string, first: Parameters<typeof expectFocus>[1]) {
  await page.goto(path);
  await expectFocus(page, first);
}

/** The key of what has focus, to compare with after Back. */
async function keyOf(page: Page) {
  const { key } = await focusOf(page);
  expect(key).not.toBeNull();
  return key as string;
}

/** Opens the rail with Left from the screen's left edge and moves along it to the entry, which Home sits between Search and the rest. */
async function toRailEntry(page: Page, name: string, direction: "ArrowUp" | "ArrowDown" = "ArrowDown") {
  await pressUntil(page, "ArrowLeft", inRail);
  await pressUntil(page, direction, (f) => f.name === name);
}

/** Opens Movies from the rail, which leaves focus on the grid's first card. */
async function openMovies(page: Page) {
  await open(page, "/", { key: card("continueWatching") });
  await toRailEntry(page, "Movies");
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/libraries\//);
  await expectFocus(page, { key: card("grid") });
}

test("Home starts on the first card of the first row", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching"), name: "Dune: Part Two" });
  await expect(page.getByRole("heading", { level: 2 }).first()).toBeVisible();
});

test("the arrows move along a row and between rows, and Up returns to the same card", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching") });
  await press(page, "ArrowRight");
  await expectFocus(page, { key: card("continueWatching"), name: "The Expanse" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: card("favorites") });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: card("recentlyAdded") });
  // Down kept to the second column.
  const kept = await keyOf(page);
  const column = await page.evaluate(() => {
    const card = document.activeElement?.closest("li");
    return card ? [...(card.parentElement?.children ?? [])].indexOf(card) : -1;
  });
  expect(column).toBe(1);
  await press(page, "ArrowDown");
  await expect.poll(() => keyOf(page)).not.toBe(kept);
  await press(page, "ArrowUp");
  // Up lands on the card Down left, not the row's first.
  await expectFocus(page, { key: kept });
});

test("OK opens a work on Play, and Back returns focus to the card it came from", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching") });
  await press(page, "ArrowRight", "ArrowDown", "ArrowDown");
  await expectFocus(page, { key: card("recentlyAdded") });
  const from = await keyOf(page);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/works\//);
  await expectFocus(page, { key: "play" });
  await back(page);
  await expect(page).toHaveURL(/\/$/);
  await expectFocus(page, { key: from });
});

test("keys pressed while a work's page loads go nowhere, and Play takes focus once it's there", async ({ page }) => {
  // Every answer comes late, as from a TV's slow network, so the page shows its skeleton, with nothing to focus, a while.
  await open(page, "/?delay=1500", { key: card("continueWatching") });
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/works\//);
  await expect.poll(() => focusOf(page)).toMatchObject({ key: null });
  // An arrow doesn't open the rail and keep the page's focus from it, and OK presses nothing.
  await press(page, "ArrowDown", "ArrowLeft", "Enter");
  await expectFocus(page, { key: "play" });
  await expect(page).toHaveURL(/\/works\//);
});

test("Left opens the rail, which dims the screen, and Right closes it onto the card", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching") });
  const from = await keyOf(page);
  await pressUntil(page, "ArrowLeft", inRail);
  await expectFocus(page, { part: "tv-rail-entry", name: "Home" });
  // Open, the rail dims the screen behind it and shows the names beside the icons.
  const scrim = page.locator('[data-part="tv-rail-scrim"]');
  await expect(scrim).toBeAttached();
  const rail = page.getByRole("navigation", { name: "Main" });
  expect((await rail.boundingBox())?.width).toBeGreaterThan(300);
  await press(page, "ArrowRight");
  await expectFocus(page, { key: from });
  await expect(scrim).toHaveCount(0);
  expect((await rail.boundingBox())?.width).toBeLessThan(200);
});

test("the rail opens a library, the grid goes deep, and Back from a work restores the card", async ({ page }) => {
  await openMovies(page);
  await expect(page.getByRole("heading", { name: "Movies", level: 1 })).toBeVisible();
  for (let i = 0; i < 8; i++) await press(page, "ArrowDown");
  await press(page, "ArrowRight", "ArrowRight");
  // Wait for the virtualised grid to settle on a card far from the top.
  await expect.poll(async () => (await page.locator('[data-tv-focus-key^="card:grid:"]').count()) > 6).toBe(true);
  // The page glides to the card, so its place is read once it's there.
  await expect
    .poll(() => page.evaluate(() => document.activeElement?.getBoundingClientRect().top ?? -1))
    .toBeLessThan(1080);
  const deep = await keyOf(page);
  const top = await page.evaluate(() => document.activeElement?.getBoundingClientRect().top ?? -1);
  expect(top).toBeGreaterThanOrEqual(0);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/works\//);
  await expectFocus(page, { key: "play" });
  await back(page);
  await expect(page).toHaveURL(/\/libraries\//);
  await expectFocus(page, { key: deep });
});

test("Back from deep in the grid opens the rail on the library, and Back again goes Home", async ({ page }) => {
  await openMovies(page);
  for (let i = 0; i < 4; i++) await press(page, "ArrowDown");
  await expectFocus(page, { key: card("grid") });
  await back(page);
  await expectFocus(page, { part: "tv-rail-entry", name: "Movies" });
  await back(page);
  await expect(page).toHaveURL(/\/$/);
  await expectFocus(page, { key: card("continueWatching") });
});

test("Sort is reachable from the grid with the arrows, and Back stays on the library", async ({ page }) => {
  await openMovies(page);
  // Up from the first row reaches the sort directly above it, then the tabs.
  await press(page, "ArrowUp");
  await expectFocus(page, { key: "sort", name: "Sort" });
  await press(page, "ArrowUp");
  await expectFocus(page, { key: "section:library" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "sort", name: "Sort" });

  await press(page, "Enter");
  await expectFocus(page, { name: "Title" });
  await press(page, "ArrowDown", "ArrowDown");
  await expectFocus(page, { name: "Release date" });
  await press(page, "Enter");
  await expect(page).toHaveURL(/sort=release/);
  await expectFocus(page, { key: "sort", name: "Newest" });
});

test("Filter is reachable from the grid with the arrows, applies at once and Back closes it", async ({ page }) => {
  await openMovies(page);
  await press(page, "ArrowUp", "ArrowUp", "ArrowUp");
  await expectFocus(page, { key: "filter", name: "Filter" });
  await press(page, "Enter");
  await expectFocus(page, { key: "filter:Watch status" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "filter:Decade:any" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "filter:Decade:1950", name: "1950s" });
  await press(page, "Enter");
  await expect(page).toHaveURL(/yearFrom=1950&yearTo=1959/);
  // Back closes the panel and returns to the Filter button, with the filter kept.
  await back(page);
  await expectFocus(page, { key: "filter", name: "Filter" });
  await expect(page.locator('[data-part="library-header"]').getByText("1950s", { exact: true })).toBeVisible();
  await expect(page).toHaveURL(/\/libraries\//);
});

// The grid keyboard's keys are six across, a to z in order.
const columns = 6;
const at = (letter: string) => ({
  row: Math.floor((letter.charCodeAt(0) - 97) / columns),
  col: (letter.charCodeAt(0) - 97) % columns,
});

/** Types lowercase letters on the on-screen keyboard, starting from the key that has focus. */
async function typeOnKeyboard(page: Page, text: string) {
  const here = at((await focusOf(page)).name);
  for (const letter of text) {
    const to = at(letter);
    for (; here.row < to.row; here.row++) await press(page, "ArrowDown");
    for (; here.row > to.row; here.row--) await press(page, "ArrowUp");
    for (; here.col < to.col; here.col++) await press(page, "ArrowRight");
    for (; here.col > to.col; here.col--) await press(page, "ArrowLeft");
    await expectFocus(page, { part: "tv-key", name: letter });
    await press(page, "Enter");
  }
}

/** Types "dune" in Search, moves to the film among the results and returns its focus key. */
async function searchForDune(page: Page) {
  await open(page, "/", { key: card("continueWatching") });
  await toRailEntry(page, "Search", "ArrowUp");
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/search/);
  await expectFocus(page, { part: "tv-key", name: "a" });
  await typeOnKeyboard(page, "dune");
  await expect(page).toHaveURL(/q=dune/);
  await expect(page.locator('[data-part="search-group"]').first()).toBeVisible();
  // Right from the keyboard's last column reaches the results.
  await pressUntil(page, "ArrowRight", (f) => f.key?.startsWith("card:") === true);
  // The first card is the director; Down is the films' row.
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "card:", name: "Dune" });
  return keyOf(page);
}

test("Search types on the on-screen keyboard and reaches a result", async ({ page }) => {
  await searchForDune(page);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/works\//);
  await expectFocus(page, { key: "play" });
  await back(page);
  await expect(page).toHaveURL(/\/search/);
});

test("Search keeps its field and keyboard aligned and stationary as result rows scroll", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching") });
  const home = await page.locator('[data-part="row"]').first().boundingBox();
  await searchForDune(page);
  const keyboard = page.locator('[data-part="search-keyboard"]');
  const field = page.getByRole("searchbox", { name: "Search" });
  const before = await keyboard.boundingBox();
  expect(before?.x).toBeCloseTo(home?.x ?? 0, 0);
  await press(page, "ArrowDown", "ArrowDown");
  await expectFocus(page, { key: /^card:/ });
  const after = await keyboard.boundingBox();
  expect(after?.y).toBeCloseTo(before?.y ?? 0, 0);
  await expect(field).toBeInViewport({ ratio: 1 });
  await expect
    .poll(() => page.locator('[data-part="search-results"]').evaluate((el) => el.scrollTop))
    .toBeGreaterThan(0);
  await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-key");
  await press(page, "Enter");
  await expect(field).not.toHaveValue("dune");
});

test("Up and Down in Search's results go to the card above or below in its column, never to the keyboard beside them", async ({
  page,
}) => {
  await page.goto("/search?q=dune&mock=manyResults");
  await expectFocus(page, { part: "tv-key", name: "a" });
  await pressUntil(page, "ArrowRight", (f) => f.key?.startsWith("card:search:person:") === true);
  // Nothing is above the first row, and the keyboard's lower keys are
  // nearer than the films' row below.
  await press(page, "ArrowUp");
  await expectFocus(page, { key: "card:search:person:" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "card:search:movie:" });
  // The shows' row remembers its first card, which Down from the second
  // film passes over for the second show, below it.
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "card:search:series:", name: "The Expanse" });
  await press(page, "ArrowUp", "ArrowRight");
  await expectFocus(page, { key: "card:search:movie:" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "card:search:series:", name: "Severance" });
});

test("Back from a search result returns focus to the same card", async ({ page }) => {
  const result = await searchForDune(page);
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/works\//);
  await expectFocus(page, { key: "play" });
  await back(page);
  await expect(page).toHaveURL(/\/search/);
  await expectFocus(page, { key: result });
});

test("Settings > About shows the server's address and its QR code", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching") });
  await toRailEntry(page, "Settings");
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/settings/);
  // The sections are a list beside the one showing; Down to About shows it.
  await expectFocus(page, { key: "settings:section:account", name: "Account" });
  for (let i = 0; i < 12 && (await keyOf(page)) !== "settings:section:about"; i++) await press(page, "ArrowDown");
  await expectFocus(page, { key: "settings:section:about", name: "About" });
  const about = page.getByRole("region", { name: "About" });
  const qr = about.getByRole("img", { name: "QR code of the server address" });
  await expect(qr).toBeInViewport();
  await expect(about.getByText("http://192.168.1.10:8484")).toBeVisible();
});

test("Sign out asks first, with Cancel focused, and Back closes it onto the row", async ({ page }) => {
  await open(page, "/settings", { key: "settings:section:account", name: "Account" });
  // Right goes into the account section, onto Sign out.
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Sign out" });
  const row = await keyOf(page);
  await press(page, "Enter");
  const dialog = page.getByRole("alertdialog", { name: "Sign out this TV?" });
  await expect(dialog).toBeVisible();
  await expectFocus(page, { key: "tv:confirm:cancel", name: "Cancel" });
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "tv:confirm:yes", name: "Sign out" });
  // The dialog holds focus at its edges.
  await press(page, "ArrowRight", "ArrowUp", "ArrowDown");
  await expectFocus(page, { key: "tv:confirm:yes" });
  await back(page);
  await expect(dialog).toBeHidden();
  await expectFocus(page, { key: row, name: "Sign out" });
  await expect(page).toHaveURL(/\/settings/);
});

test("the splash's icon, then sign-in's logo, each keep one size and place from the first frame", async ({ page }) => {
  await recordLogos(page);
  await page.goto("/?mock=signedOut");
  await expect(page.locator('[data-part="sign-in-code"]').getByText("K7Q-2M9")).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
  await expectSteadyLogos(page);
});

test("signed out, the TV shows a code and QR code to approve from another device", async ({ page }) => {
  await page.goto("/?mock=signedOut");
  await expect(page).toHaveURL(/\/sign-in/);
  const panel = page.locator('[data-part="sign-in-code"]');
  await expect(panel.getByText("K7Q-2M9")).toBeVisible();
  await expect(panel.getByRole("img", { name: "QR code to link this browser" })).toBeVisible();
  await expect(panel.getByText(/expires/i)).toHaveCount(0);
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });

  // The password form is a keyboard away, and Back is not needed to reach it.
  await press(page, "Enter");
  await expectFocus(page, { key: "signin:username" });
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "signin:password" });
});

/** Opens sign-in's password form from its code, with focus on the username. */
async function openPasswordForm(page: Page) {
  await page.goto("/?mock=signedOut");
  await expect(page).toHaveURL(/\/sign-in/);
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });
  await press(page, "Enter");
  await expectFocus(page, { key: "signin:username" });
}

test("Back on sign-in's password form returns to the code, rather than asking to exit", async ({ page }) => {
  await openPasswordForm(page);
  await back(page);
  await expect(page.locator('[data-part="sign-in-code"]')).toBeVisible();
  await expect(page.getByRole("alertdialog", { name: "Exit Kivo?" })).toHaveCount(0);
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });
});

test("sign-in's password form has no keyboard of its own, and Down and Up go through it in order", async ({ page }) => {
  await openPasswordForm(page);
  await expect(page.locator('[data-part="tv-keyboard"]')).toHaveCount(0);
  const order = ["signin:username", "signin:password", "signin:submit", "signin:code"];
  for (const key of order.slice(1)) {
    await press(page, "ArrowDown");
    await expectFocus(page, { key });
  }
  // Right has nothing beside the form to go to.
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "signin:code" });
  for (const key of order.slice(0, -1).reverse()) {
    await press(page, "ArrowUp");
    await expectFocus(page, { key });
  }
});

test("OK on a field opens the TV's keyboard, whose Enter goes on from Username to Password and then signs in", async ({
  page,
}) => {
  await openPasswordForm(page);
  const username = page.getByLabel("Username");
  const password = page.getByLabel("Password");
  await press(page, "Enter");
  await expect(username).toBeFocused();
  // What the TV's keyboard types reaches the input as keys.
  await page.keyboard.type("anna");
  await press(page, "Enter");
  await expect(password).toBeFocused();
  // It went on to the password, rather than sending the form without one.
  await expect(password).not.toHaveAttribute("aria-invalid", "true");
  await page.keyboard.type("secret");
  const signIn = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/v1/sessions"));
  await press(page, "Enter");
  expect((await signIn).postDataJSON()).toMatchObject({ username: "anna", password: "secret" });
});

test("Down from a field being typed in goes on, and Back from it returns to the code", async ({ page }) => {
  await openPasswordForm(page);
  await press(page, "Enter");
  await expect(page.getByLabel("Username")).toBeFocused();
  await press(page, "ArrowDown");
  await expectFocus(page, { key: "signin:password" });
  await press(page, "Enter");
  await expect(page.getByLabel("Password")).toBeFocused();
  await back(page);
  await expect(page.locator('[data-part="sign-in-code"]')).toBeVisible();
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });
});

test("sign-in's code and password form stay inside the TV's safe area", async ({ page }) => {
  await page.goto("/?mock=signedOut");
  await expect(page.locator('[data-part="sign-in-code"]').getByText("K7Q-2M9")).toBeVisible();
  const { x, y } = await overscan(page);
  const inside = async (what: string) => {
    const box = await page.locator('[data-part="auth-panel"]').boundingBox();
    expect(box, what).not.toBeNull();
    expect(box?.x ?? 0, what).toBeGreaterThanOrEqual(x);
    expect(box?.y ?? 0, what).toBeGreaterThanOrEqual(y);
    expect((box?.x ?? 0) + (box?.width ?? 0), what).toBeLessThanOrEqual(1920 - x);
    expect((box?.y ?? 0) + (box?.height ?? 0), what).toBeLessThanOrEqual(1080 - y);
  };
  await inside("the code");
  await press(page, "Enter");
  await expectFocus(page, { key: "signin:username" });
  await inside("the password form");
});

test("Back on sign-in, the first screen, asks Exit Kivo? with No focused, and Back closes it", async ({ page }) => {
  await page.goto("/?mock=signedOut");
  await expect(page).toHaveURL(/\/sign-in/);
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });
  await back(page);
  const dialog = page.getByRole("alertdialog", { name: "Exit Kivo?" });
  await expect(dialog).toBeVisible();
  await expectFocus(page, { key: "tv:exit:no", name: "No" });
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "tv:exit:yes", name: "Exit" });
  await back(page);
  await expect(dialog).toBeHidden();
  await expectFocus(page, { key: "signin:password" });
});

test("Back at Home moves to the first card, then asks Exit Kivo? with No focused", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching") });
  const first = await keyOf(page);
  await press(page, "ArrowRight");
  await back(page);
  await expectFocus(page, { key: first });
  await expect(page.getByRole("alertdialog")).toHaveCount(0);

  await back(page);
  const dialog = page.getByRole("alertdialog", { name: "Exit Kivo?" });
  await expect(dialog).toBeVisible();
  await expectFocus(page, { key: "tv:exit:no", name: "No" });
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "tv:exit:yes", name: "Exit" });
  await back(page);
  await expect(dialog).toBeHidden();
  await expectFocus(page, { key: first });
});

test("the player route keeps the keys: the app's focus and Back stand down", async ({ page }) => {
  await open(page, "/", { key: card("continueWatching") });
  await press(page, "Enter");
  await expectFocus(page, { key: "play" });
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/watch\//);
  const player = page.locator('[data-part="player"]');
  await expect(player).toBeVisible();
  // The rail is gone, and an arrow doesn't move app focus out to a card or the rail.
  await expect(page.getByRole("navigation", { name: "Main" })).toHaveCount(0);
  await press(page, "ArrowLeft", "ArrowDown", "ArrowRight");
  const focus = await page.evaluate(() => ({
    inPlayer: document.activeElement?.closest('[data-part="player"]') !== null,
    key: document.activeElement?.getAttribute("data-tv-focus-key") ?? null,
  }));
  expect(focus.inPlayer).toBe(true);
  expect(focus.key === null || focus.key.startsWith("player-error:")).toBe(true);
  // Back leaves the player for the page it came from; the e2e/remote suite covers its keys.
  await back(page);
  await expect(page).toHaveURL(/\/works\//);
  await expectFocus(page, { key: "play" });
});
