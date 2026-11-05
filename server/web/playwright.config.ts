import { defineConfig } from "@playwright/test";
import { e2e } from "./e2e/env";

// Playwright against the real server: the
// global setup builds Kivo from this tree with the web client embedded,
// makes test media with ffmpeg, and runs the server on a free port with a
// library of it. Playwright's Chromium is a Chrome for Testing build, which
// plays H.264 and AAC; its new headless mode (the `chromium` channel) is the
// full browser rather than the shell.
export default defineConfig({
  testDir: "./e2e",
  // The TV build's remote, the TV build and the mock's runs have their own
  // configs: playwright.remote.config.ts, playwright.tv.config.ts and
  // playwright.mock.config.ts.
  testIgnore: ["remote/**", "tv/**", "mock/**"],
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  timeout: 90_000,
  expect: { timeout: 20_000 },
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL: e2e.baseURL,
    storageState: e2e.storageState,
    channel: "chromium",
    trace: "retain-on-failure",
  },
  projects: [{ name: "web" }],
});
