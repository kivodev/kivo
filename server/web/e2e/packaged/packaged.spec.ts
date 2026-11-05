import { expect, type Page, type Request, test } from "@playwright/test";
// biome-ignore lint/style/noRestrictedImports: the TV project's remote, which e2e has no alias for
import { back, expectFocus, press, pressUntil } from "../tv/remote";

// The packaged TV app driven with a remote's keys, against the
// mocks. Each test starts as a fresh install, with nothing in the app's
// storage. Fields are filled the way the TV's own keyboard fills them, as
// neither the address screen nor sign-in has a keyboard of its own; the
// rest is arrows, OK and Back.

const card = (row: string) => new RegExp(`^card:${row}:`);

/** The app's storage under its "kivo." keys. */
const stored = (page: Page) =>
  page.evaluate(() => ({
    server: window.localStorage.getItem("kivo.server"),
    token: window.localStorage.getItem("kivo.token"),
  }));

/** Opens the app as a fresh install, on the address screen. */
async function openFresh(page: Page) {
  await page.goto("/");
  await expect(page).toHaveURL(/\/#\/server$/);
  await expect(page.getByRole("heading", { name: "Your Kivo server's address" })).toBeVisible();
  await expectFocus(page, { key: "address:field" });
}

/** Types the dev server's own origin, passes the plain-HTTP warning and opens sign-in. */
async function connect(page: Page, origin: string) {
  await page.getByLabel("Address").fill(origin);
  await press(page, "Enter");
  await expect(page.getByText("Connecting without encryption. Press Open to continue.")).toBeVisible();
  await expectFocus(page, { key: "address:open", name: "Open" });
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/#\/sign-in/);
  await expect(page.getByRole("heading", { name: "Sign in", level: 1 })).toBeVisible();
}

/**
 * Signs in with a password typed on the TV's own keyboard, which OK on a
 * field opens, and whose Enter goes on to the next field and then signs
 * in. It lands on Home's first card.
 */
async function signIn(page: Page) {
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });
  await press(page, "Enter");
  await expectFocus(page, { key: "signin:username" });
  await press(page, "Enter");
  await page.keyboard.type("anna");
  await press(page, "Enter");
  await expect(page.getByLabel("Password")).toBeFocused();
  await page.keyboard.type("secret");
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/#\/$/);
  await expectFocus(page, { key: card("continueWatching") });
}

/** Opens Settings from Home, on its Account section. */
async function openSettings(page: Page) {
  await pressUntil(page, "ArrowLeft", (f) => f.part === "tv-rail-entry");
  await pressUntil(page, "ArrowDown", (f) => f.name === "Settings");
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/#\/settings/);
  await expectFocus(page, { key: "settings:section:account", name: "Account" });
}

const originOf = (baseURL: string | undefined) => new URL(baseURL ?? "").origin;

test("a fresh install opens on the address screen, at a hash URL", async ({ page }) => {
  await openFresh(page);
  await expect(page.getByLabel("Address")).toHaveValue("");
  expect(await stored(page)).toEqual({ server: null, token: null });
});

test("the address, the plain-HTTP warning and Open reach sign-in, and signing in reaches Home", async ({
  page,
  baseURL,
}) => {
  const origin = originOf(baseURL);
  await openFresh(page);
  await connect(page, origin);
  const before = await stored(page);
  expect(JSON.parse(before.server ?? "null")).toMatchObject({ address: origin });
  expect(before.token).toBeNull();
  await signIn(page);
  expect((await stored(page)).token).toBe("mock-app-token");
});

test("signed in, every API request carries the bearer and omits cookies, and no URL holds the token", async ({
  page,
  baseURL,
}) => {
  const origin = originOf(baseURL);
  const requests: Request[] = [];
  // Opening the server loads the page again, which starts the page's own
  // record of its fetches afresh, so this record starts afresh with each page.
  page.on("request", (r) => {
    if (r.resourceType() === "document") requests.length = 0;
    requests.push(r);
  });
  // The mocks answer in the service worker, so the browser never adds a
  // cookie to a request and Playwright can't show whether it would. The page
  // records each fetch's credentials mode instead: "omit" sends no cookie.
  await page.addInitScript(() => {
    const fetched: { url: string; credentials: RequestCredentials }[] = [];
    Object.assign(window, { kivoFetched: fetched });
    window.fetch = new Proxy(window.fetch, {
      apply(target, self, args: Parameters<typeof window.fetch>) {
        const [input, init] = args;
        // A Request is read as it is: wrapping it in another would use up its body.
        const request = input instanceof Request && init === undefined ? input : new Request(input, init);
        fetched.push({ url: request.url, credentials: request.credentials });
        return Reflect.apply(target, self, args);
      },
    });
  });
  await openFresh(page);
  await connect(page, origin);
  // The events socket goes through the mocks in the page, where Playwright
  // doesn't see it, so the page records what each socket is opened with.
  await page.evaluate(() => {
    const opened: { url: string; protocols: unknown }[] = [];
    Object.assign(window, { kivoSockets: opened });
    window.WebSocket = new Proxy(window.WebSocket, {
      construct(target, args: ConstructorParameters<typeof window.WebSocket>, newTarget) {
        opened.push({ url: String(args[0]), protocols: args[1] });
        return Reflect.construct(target, args, newTarget);
      },
    });
  });
  await signIn(page);
  // Home's rows and the shell's queries have all been asked by now.
  await expect(page.getByRole("heading", { level: 2 }).first()).toBeVisible();

  const token = (await stored(page)).token;
  expect(token).toBe("mock-app-token");
  const api = requests.filter((r) => r.url().startsWith(`${origin}/api/`) && r.resourceType() === "fetch");
  const signedInAt = api.findIndex((r) => r.method() === "POST" && new URL(r.url()).pathname === "/api/v1/sessions");
  expect(signedInAt).toBeGreaterThanOrEqual(0);
  const before = api.slice(0, signedInAt + 1);
  const after = api.slice(signedInAt + 1);
  expect(after.length).toBeGreaterThan(3);
  // The probe and sign-in go out with no token; everything after carries it.
  for (const r of before) expect(r.headers().authorization, `${r.method()} ${r.url()}`).toBeUndefined();
  for (const r of after) expect(r.headers().authorization, `${r.method()} ${r.url()}`).toBe(`Bearer ${token}`);
  for (const r of requests) expect(r.url()).not.toContain(token);
  const fetched = await page.evaluate(
    () => (window as unknown as { kivoFetched: { url: string; credentials: string }[] }).kivoFetched,
  );
  const apiFetched = fetched.filter((f) => f.url.startsWith(`${origin}/api/`));
  expect(apiFetched.length).toBe(api.length);
  for (const f of apiFetched) expect(f.credentials, f.url).toBe("omit");

  const sockets = await page.evaluate(
    () => (window as unknown as { kivoSockets: { url: string; protocols: unknown }[] }).kivoSockets,
  );
  expect(sockets.length).toBeGreaterThan(0);
  for (const socket of sockets) {
    expect(socket.url).toBe(`${origin.replace(/^http/, "ws")}/api/v1/events`);
    expect(socket.protocols).toEqual(["kivo", `kivo-bearer.${token}`]);
  }
});

test("a reload comes back to Home signed in, with no screen between", async ({ page, baseURL }) => {
  await openFresh(page);
  await connect(page, originOf(baseURL));
  await signIn(page);
  // From the reload on, the page records every URL it shows and every heading.
  await page.addInitScript(() => {
    const seen: string[] = [location.href];
    Object.assign(window, { kivoSeen: seen });
    for (const name of ["pushState", "replaceState"] as const) {
      const original = history[name].bind(history);
      history[name] = (data, unused, url) => {
        if (url !== undefined && url !== null) seen.push(String(url));
        original(data, unused, url);
      };
    }
    new MutationObserver(() => {
      for (const h of document.querySelectorAll("h1")) seen.push(`h1:${h.textContent}`);
    }).observe(document, { childList: true, subtree: true });
  });
  await page.reload();
  await expectFocus(page, { key: card("continueWatching") });
  await expect(page).toHaveURL(/\/#\/$/);
  const seen = await page.evaluate(() => (window as unknown as { kivoSeen: string[] }).kivoSeen);
  expect(seen.filter((s) => /sign-in|server|h1:(Sign in|Your Kivo server)/.test(s))).toEqual([]);
});

test("Settings › Sign out returns to sign-in with the address kept", async ({ page, baseURL }) => {
  const origin = originOf(baseURL);
  await openFresh(page);
  await connect(page, origin);
  await signIn(page);
  await openSettings(page);
  await press(page, "ArrowRight");
  await expectFocus(page, { name: "Sign out" });
  await press(page, "Enter");
  const dialog = page.getByRole("alertdialog", { name: "Sign out this TV?" });
  await expect(dialog).toBeVisible();
  await expectFocus(page, { key: "tv:confirm:cancel" });
  const revoke = page.waitForRequest(
    (r) => r.method() === "DELETE" && new URL(r.url()).pathname === "/api/v1/sessions/current",
  );
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "tv:confirm:yes", name: "Sign out" });
  await press(page, "Enter");
  expect((await (await revoke).allHeaders()).authorization).toBe("Bearer mock-app-token");
  await expect(page).toHaveURL(/\/#\/sign-in/);
  await expect(page.getByRole("heading", { name: "Sign in", level: 1 })).toBeVisible();
  const after = await stored(page);
  expect(JSON.parse(after.server ?? "null")).toMatchObject({ address: origin });
  expect(after.token).toBeNull();
});

test("Settings › Change server returns to the address screen", async ({ page, baseURL }) => {
  await openFresh(page);
  await connect(page, originOf(baseURL));
  await signIn(page);
  await openSettings(page);
  await pressUntil(page, "ArrowDown", (f) => f.key === "settings:section:about");
  await press(page, "ArrowRight");
  // The whole setting takes focus, its name with its button's.
  await pressUntil(page, "ArrowDown", (f) => f.name.endsWith("Change…"));
  await press(page, "Enter");
  const dialog = page.getByRole("alertdialog", { name: "Change server?" });
  await expect(dialog).toBeVisible();
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "tv:confirm:yes", name: "Change server" });
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/#\/server$/);
  await expect(page.getByRole("heading", { name: "Your Kivo server's address" })).toBeVisible();
  expect(await stored(page)).toEqual({ server: null, token: null });
});

test("the plain-HTTP warning shows without moving the field or Open", async ({ page, baseURL }) => {
  await openFresh(page);
  // The heading is above both, and doesn't scale with focus as they do.
  const heading = page.getByRole("heading", { name: "Your Kivo server's address" });
  const before = await heading.boundingBox();
  await page.getByLabel("Address").fill(originOf(baseURL));
  await press(page, "Enter");
  await expect(page.getByText("Connecting without encryption. Press Open to continue.")).toBeVisible();
  expect(await heading.boundingBox()).toEqual(before);
});

test("Change server on sign-in opens the address screen with the address kept, and Back returns", async ({
  page,
  baseURL,
}) => {
  const origin = originOf(baseURL);
  await openFresh(page);
  await connect(page, origin);
  await expectFocus(page, { key: "signin:password", name: "Sign in with password" });
  await press(page, "ArrowRight");
  await expectFocus(page, { key: "signin:change-server", name: "Change server" });
  await press(page, "Enter");
  await expect(page).toHaveURL(/\/#\/server$/);
  await expect(page.getByLabel("Address")).toHaveValue(origin);
  await back(page);
  await expect(page).toHaveURL(/\/#\/sign-in/);
  await expect(page.getByRole("alertdialog", { name: "Exit Kivo?" })).toHaveCount(0);
  await expectFocus(page, { key: "signin:password" });
});

/**
 * Samsung's player as the TV's page has it, before the app loads: it
 * records what the app asks of it, and while it plays it reports its
 * position every quarter second, as AVPlay does.
 */
function fakeAvplay() {
  type Listener = Record<string, ((...args: unknown[]) => void) | undefined>;
  const log: unknown[][] = [];
  let state = "NONE";
  let listener: Listener = {};
  let at = 0;
  let timer: ReturnType<typeof setInterval> | undefined;
  const w = window as unknown as Record<string, unknown>;
  w.__avplay = { log };
  w.webapis = {
    avplay: {
      open: (url: string) => {
        log.push(["open", url]);
        state = "IDLE";
      },
      close: () => {
        log.push(["close"]);
        state = "NONE";
        clearInterval(timer);
      },
      stop: () => {
        log.push(["stop"]);
        state = "IDLE";
      },
      prepareAsync: (ok?: () => void) => {
        log.push(["prepareAsync"]);
        setTimeout(() => {
          state = "READY";
          ok?.();
        }, 50);
      },
      setDisplayRect: (...args: unknown[]) => log.push(["setDisplayRect", ...args]),
      setDisplayMethod: (mode: string) => log.push(["setDisplayMethod", mode]),
      setBufferingParam: (...args: unknown[]) => log.push(["setBufferingParam", ...args]),
      setTimeoutForBuffering: () => {},
      setSilentSubtitle: () => {},
      setListener: (l: Listener) => {
        listener = l;
      },
      play: () => {
        log.push(["play"]);
        state = "PLAYING";
        clearInterval(timer);
        timer = setInterval(() => {
          if (state !== "PLAYING") return;
          at += 250;
          listener.oncurrentplaytime?.(at);
        }, 250);
      },
      pause: () => {
        log.push(["pause"]);
        state = "PAUSED";
      },
      seekTo: (ms: number, ok?: () => void) => {
        log.push(["seekTo", ms]);
        at = ms;
        setTimeout(() => ok?.(), 30);
      },
      getCurrentTime: () => at,
      getDuration: () => 600_000,
      getState: () => state,
      getTotalTrackInfo: () => [
        { index: 0, type: "VIDEO", extra_info: JSON.stringify({ Width: "3840", Height: "2160" }) },
        { index: 1, type: "AUDIO", extra_info: "{}" },
        { index: 2, type: "AUDIO", extra_info: "{}" },
      ],
      setSelectTrack: (...args: unknown[]) => log.push(["setSelectTrack", ...args]),
      getStreamingProperty: () => "0",
    },
  };
}

const avplayLog = (page: Page) =>
  page.evaluate(() => (window as unknown as { __avplay: { log: unknown[][] } }).__avplay.log);
const avplayCalls = async (page: Page) => (await avplayLog(page)).map(([name]) => name);

test("a file plays through Samsung's own player, which shows through the page, with its subtitle drawn", async ({
  page,
  baseURL,
}) => {
  await page.addInitScript(fakeAvplay);
  await openFresh(page);
  await connect(page, originOf(baseURL));
  await signIn(page);
  // Dune's 4K version, a file, with English subtitles as a WebVTT file.
  await page.goto("/?mock=playable#/watch/0190a000-0000-7000-8000-000000000065");

  await expect.poll(() => avplayCalls(page)).toContain("play");
  const log = await avplayLog(page);
  expect(log.find(([name]) => name === "open")?.[1]).toMatch(/\/mock-media\/film\.mp4$/);
  expect(log).toContainEqual(["setDisplayRect", 0, 0, 1920, 1080]);
  // It starts where the play does, five seconds in.
  expect(log).toContainEqual(["seekTo", 5000]);
  // The video element loads nothing; AVPlay has the file.
  expect(await page.locator('[data-part="player"] video').getAttribute("src")).toBeNull();

  // The page is see-through down to AVPlay's plane, and shows only the player.
  const look = await page.evaluate(() => {
    const bg = (el: Element | null) => (el ? getComputedStyle(el).backgroundColor : "");
    const player = document.querySelector('[data-part="player"]');
    return {
      root: bg(document.documentElement),
      body: bg(document.body),
      player: bg(player),
      appVisibility: getComputedStyle(document.getElementById("root") as Element).visibility,
      playerVisibility: player ? getComputedStyle(player).visibility : "",
      plane: document.querySelector('object[type="application/avplayer"]') !== null,
    };
  });
  expect(look).toEqual({
    root: "rgba(0, 0, 0, 0)",
    body: "rgba(0, 0, 0, 0)",
    player: "rgba(0, 0, 0, 0)",
    appVisibility: "hidden",
    playerVisibility: "visible",
    plane: true,
  });

  // English subtitles, chosen from Audio & Subtitles, are drawn by AVPlay's
  // position, and the browser, whose clock doesn't run, doesn't draw them.
  await press(page, "ArrowRight", "ArrowRight", "Enter");
  await expectFocus(page, { name: /^English · 7\.1/ });
  await press(page, "ArrowRight", "ArrowDown");
  await expectFocus(page, { name: /^English/ });
  await press(page, "Enter");
  await expect(page.locator('[data-part="avplay-cues"]')).toHaveText("A mock subtitle");
  expect(
    await page
      .locator('[data-part="player"] video')
      .evaluate((v: HTMLVideoElement) => [...v.textTracks].map((t) => t.mode)),
  ).toEqual(["hidden"]);
  await back(page);

  // AVPlay plays at normal speed only, so the settings offer no other.
  await press(page, "ArrowRight", "Enter");
  await expect(page.getByRole("menuitem", { name: /^Quality/ })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: /^Speed/ })).toHaveCount(0);
  await back(page);

  // The remote's Play/Pause pauses AVPlay.
  await page.keyboard.press("MediaPlayPause");
  await expect.poll(() => avplayCalls(page)).toContain("pause");

  // Leaving the player closes AVPlay and gives the page back. Back hides
  // the controls first, then leaves.
  await expect
    .poll(async () => {
      await back(page);
      return avplayCalls(page);
    })
    .toContain("close");
  await expect(page.locator('[data-part="player"]')).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => document.documentElement.hasAttribute("data-avplay"))).toBe(false);
});
