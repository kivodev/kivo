import { type APIRequestContext, expect, test } from "@playwright/test";
import { e2e } from "./env";

// Link a device against the real server. A device starts a code, a
// signed-in browser answers it at /link, and the device's poll gets its
// session or hears it was denied. A browser can be the device too: its
// sign-in page shows the code, and approving it signs that browser in.

type DeviceCode = { code: string; secret: string; linkUrl: string; interval: number };

/** The admin's cookie answers a code only from the server's own origin. */
const sameOrigin = { headers: { Origin: e2e.baseURL } };

/** A device without the admin's cookie, as a TV app is. */
async function startLink(tv: APIRequestContext, name: string): Promise<DeviceCode> {
  const started = await tv.post("/api/v1/device-codes", { data: { name, platform: "androidTv" } });
  expect(started.status()).toBe(201);
  return (await started.json()) as DeviceCode;
}

test("a TV signs in by its QR code, approved at /link, and is signed out from Settings", async ({
  page,
  playwright,
}) => {
  const tv = await playwright.request.newContext({ baseURL: e2e.baseURL });
  const link = await startLink(tv, "TV");
  expect(link.code).toMatch(/^[A-Z2-9]{6}$/);
  expect(link.interval).toBe(2);
  const server = (await (await tv.get("/api/v1/server")).json()) as { id: string };
  expect(link.linkUrl).toBe(`${e2e.baseURL}/link?code=${link.code}&server=${server.id}`);
  const poll = () => tv.post("/api/v1/device-codes/poll", { data: { secret: link.secret } });
  expect((await poll()).status()).toBe(202);

  await page.goto(link.linkUrl);
  await expect(page.getByRole("heading", { name: "Allow TV to sign in as admin?" })).toBeVisible();
  await expect(page.getByText("Android TV · On your home network · Just now")).toBeVisible();
  await page.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(page.getByRole("heading", { name: "TV is signed in" })).toBeVisible();

  const approved = await poll();
  expect(approved.status()).toBe(201);
  const session = (await approved.json()) as { token: string; kind: string; name: string; platform: string };
  expect(session).toMatchObject({ kind: "app", name: "TV", platform: "androidTv" });
  const auth = { Authorization: `Bearer ${session.token}` };
  const me = (await (await tv.get("/api/v1/users/me", { headers: auth })).json()) as { username: string };
  expect(me.username).toBe("admin");
  // The code worked once.
  expect((await poll()).status()).toBe(410);

  await page.goto("settings");
  const sessions = page.getByRole("list", { name: "Signed-in devices" });
  await expect(sessions.getByRole("listitem").filter({ hasText: /^TV/ })).toContainText("Android TV");
  await sessions.getByRole("button", { name: "Sign out TV", exact: true }).click();
  await page
    .getByRole("alertdialog", { name: "Sign out TV?" })
    .getByRole("button", { name: "Sign out", exact: true })
    .click();
  await expect(sessions.getByRole("listitem").filter({ hasText: /^TV/ })).toHaveCount(0);
  expect((await tv.get("/api/v1/home", { headers: auth })).status()).toBe(401);
  await tv.dispose();
});

test("a code typed at Link a device from the account menu signs the device in", async ({ page, playwright }) => {
  const tv = await playwright.request.newContext({ baseURL: e2e.baseURL });
  const link = await startLink(tv, "Den");

  await page.goto("");
  await page.getByRole("button", { name: /^Account menu for/ }).click();
  await page.getByRole("menuitem", { name: "Link a device" }).click();
  await expect(page.getByRole("heading", { name: "Link a device" })).toBeVisible();
  const typed = `${link.code.slice(0, 3)} ${link.code.slice(3)}`.toLowerCase();
  await page.getByRole("textbox", { name: "Code" }).fill(typed);
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Den is signed in" })).toBeVisible();

  const approved = await tv.post("/api/v1/device-codes/poll", { data: { secret: link.secret } });
  expect(approved.status()).toBe(201);
  const { token } = (await approved.json()) as { token: string };
  expect(
    (await tv.delete("/api/v1/sessions/current", { headers: { Authorization: `Bearer ${token}` } })).status(),
  ).toBe(204);
  await tv.dispose();
});

test("Don't allow denies the device: its poll says so, and the code can't be approved after", async ({
  page,
  playwright,
}) => {
  const tv = await playwright.request.newContext({ baseURL: e2e.baseURL });
  const link = await startLink(tv, "Stranger");

  await page.goto(link.linkUrl);
  await page.getByRole("button", { name: "Don't allow" }).click();
  await expect(page.getByRole("heading", { name: "Stranger wasn't signed in" })).toBeVisible();

  const denied = await tv.post("/api/v1/device-codes/poll", { data: { secret: link.secret } });
  expect(denied.status()).toBe(403);
  expect(((await denied.json()) as { code: string }).code).toBe("denied");
  expect((await page.request.post(`api/v1/device-codes/${link.code}/approve`, sameOrigin)).status()).toBe(404);
  await tv.dispose();
});

test("a TV's browser opens on its code and is signed in once another device approves it", async ({ browser, page }) => {
  const tvBrowser = await browser.newContext({
    baseURL: e2e.baseURL,
    storageState: { cookies: [], origins: [] },
    userAgent:
      "Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0 TV Safari/537.36",
  });
  const tv = await tvBrowser.newPage();
  await tv.goto(`${e2e.baseURL}/`);
  const code = (await tv.locator("[data-part=code]").textContent({ timeout: 20_000 }))?.replace("-", "") ?? "";
  expect(code).toMatch(/^[A-Z2-9]{6}$/);

  expect((await page.request.post(`api/v1/device-codes/${code}/approve`, sameOrigin)).status()).toBe(204);
  await expect(tv.getByRole("button", { name: /^Account menu for admin/ })).toBeVisible();

  await tv.goto(`${e2e.baseURL}/settings`);
  const sessions = tv.getByRole("list", { name: "Signed-in devices" });
  await expect(sessions.getByRole("listitem").filter({ hasText: "This browser" })).toContainText("Browser");
  await tvBrowser.close();
});

test("a desktop browser signs in with a code approved from another browser at /link", async ({ browser, page }) => {
  const other = await browser.newContext({ baseURL: e2e.baseURL, storageState: { cookies: [], origins: [] } });
  const desktop = await other.newPage();
  await desktop.goto(`${e2e.baseURL}/`);
  await expect(desktop.getByLabel("Username")).toBeVisible();
  await desktop.getByRole("button", { name: "Sign in with another device" }).click();
  const qr = desktop.getByRole("img", { name: "QR code to link this browser" });
  await expect(qr).toBeVisible();
  const code = (await desktop.locator("[data-part=code]").textContent())?.replace("-", "") ?? "";

  await page.goto(`link?code=${code}`);
  await expect(page.getByText("Browser · On your home network · Just now")).toBeVisible();
  await page.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(page.getByRole("heading", { name: /is signed in$/ })).toBeVisible();

  await expect(desktop.getByRole("button", { name: /^Account menu for admin/ })).toBeVisible();
  await other.close();
});
