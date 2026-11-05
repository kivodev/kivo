import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { expect, test } from "@playwright/test";
// biome-ignore lint/style/noRestrictedImports: the TV project's logo checks, which e2e has no alias for
import { expectSteadyLogos, recordLogos } from "../tv/logo";

// The built packaged app as the TV opens it: file:///.../index.html, its
// module scripts and stylesheet beside it, its policy in a meta element,
// and its requests to the typed server across origins. On a Tizen 9 TV
// these hold as well (checked by hand): file: module scripts load, a
// server's answer without CORS headers is readable, no Origin is sent, and
// hash routes work. Chromium enforces CORS for a file: page where the TV
// doesn't, so the mock server's answer here allows any origin.

const dist = fileURLToPath(new URL("../../dist-packaged/", import.meta.url));
const page = pathToFileURL(`${dist}index.html`).href;

// The address typed, which nothing but the route below answers.
const typed = "http://kivo-file-test.invalid:8484";

test.beforeAll(() => {
  if (!existsSync(`${dist}index.html`)) throw new Error("No dist-packaged/index.html: run npm run build:packaged");
});

test("opens on the address screen from file:, with its scripts and stylesheet, and probes a typed address", async ({
  page: tab,
}) => {
  const violations: string[] = [];
  await tab.exposeFunction("cspViolation", (v: string) => violations.push(v));
  await tab.addInitScript(() => {
    document.addEventListener("securitypolicyviolation", (e) => {
      (window as unknown as { cspViolation: (v: string) => void }).cspViolation(
        `${e.violatedDirective} ${e.blockedURI}`,
      );
    });
  });
  const failed: string[] = [];
  tab.on("requestfailed", (r) => {
    // Samsung's webapis script exists only on a TV, which resolves $WEBAPIS.
    if (!r.url().includes("$WEBAPIS")) failed.push(r.url());
  });
  const loaded: string[] = [];
  tab.on("requestfinished", (r) => loaded.push(r.url()));
  const probes: string[] = [];
  // The app's identity headers make the probe a preflighted request from
  // the file: page, which Chromium enforces and a TV doesn't.
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*" };
  await tab.route(`${typed}/**`, (route) => {
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    probes.push(route.request().url());
    return route.fulfill({ json: { id: "file-test", name: "Kivo", version: "1.0.0" }, headers: cors });
  });

  await tab.goto(page);
  await expect(tab).toHaveURL(/index\.html#\/server$/);
  await expect(tab.getByRole("heading", { name: "Your Kivo server's address" })).toBeVisible();
  expect(loaded.some((url) => url.startsWith("file:") && url.endsWith(".js"))).toBe(true);
  expect(loaded.some((url) => url.startsWith("file:") && url.endsWith(".css"))).toBe(true);
  // The stylesheet applies: the page has the dark theme's background, not the browser's white.
  expect(await tab.evaluate(() => getComputedStyle(document.body).backgroundColor)).not.toBe("rgb(255, 255, 255)");

  await tab.getByLabel("Address").fill(typed);
  await tab.keyboard.press("Enter");
  await expect.poll(() => probes).toContain(`${typed}/api/v1/server`);
  expect(failed).toEqual([]);
  // Two refusals the app expects and goes on without, each once: zod tries
  // Function("") to see whether it may compile its parsers, and react-aria
  // adds a <style> of touch-action for pressable elements, which a remote
  // doesn't need. Anything else the policy refuses is a failure.
  const expected = ["script-src eval", "style-src-elem inline"];
  expect(violations.filter((v) => !expected.includes(v))).toEqual([]);
  for (const v of expected) expect(violations.filter((w) => w === v).length, v).toBeLessThanOrEqual(1);
});

test("ships only what the page uses: no web app manifest or its icons", () => {
  for (const file of ["manifest.json", "apple-touch-icon.png", "icon-192.png", "icon-512.png"]) {
    expect(existsSync(`${dist}${file}`), file).toBe(false);
  }
  const html = readFileSync(`${dist}index.html`, "utf8");
  expect(html).not.toMatch(/manifest\.json|apple-touch-icon/);
});

test("draws its text in the Inter it carries, loaded from the package beside the page", async ({ page: tab }) => {
  const fonts: string[] = [];
  tab.on("requestfinished", (r) => {
    if (r.url().endsWith(".woff2")) fonts.push(r.url());
  });
  await tab.goto(page);
  await expect(tab.getByRole("heading", { name: "Your Kivo server's address" })).toBeVisible();
  const inter = await tab.evaluate(async () => {
    await document.fonts.ready;
    return [...document.fonts].filter((f) => f.family.replace(/"/g, "") === "Inter Variable" && f.status === "loaded")
      .length;
  });
  expect(inter, "Inter faces loaded").toBeGreaterThan(0);
  expect(fonts.some((url) => url.startsWith("file:") && /inter-latin-wght/.test(url))).toBe(true);
  expect(
    fonts.filter((url) => !url.startsWith("file:")),
    "fonts from anywhere else",
  ).toEqual([]);
});

test("the splash's icon, then the address screen's logo, each keep one size and place from the first frame", async ({
  page: tab,
}) => {
  await recordLogos(tab);
  await tab.goto(page);
  await expect(tab.getByRole("heading", { name: "Your Kivo server's address" })).toBeVisible();
  await tab.evaluate(() => document.fonts.ready);
  await expectSteadyLogos(tab);
});
