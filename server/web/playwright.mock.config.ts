import { defineConfig, devices } from "@playwright/test";

// The web client against the mock API, in WebKit and Chromium, on a phone at
// 3x and a TV-sized 1920x1080 window at 1x: the dev server runs with the
// mocks, and each test picks its scenario in the URL.
const port = Number(process.env.KIVO_MOCK_E2E_PORT ?? 5198);
const phone = devices["iPhone 15"];
const tv = { viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 };

export default defineConfig({
  testDir: "./e2e/mock",
  timeout: 240_000,
  expect: { timeout: 10_000 },
  workers: 1,
  reporter: [["list"]],
  webServer: {
    command: `npm run dev:mock -- --port ${port} --strictPort`,
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: false,
  },
  use: {
    baseURL: `http://127.0.0.1:${port}/`,
    trace: "retain-on-failure",
  },
  projects: [
    { name: "phone-webkit", use: { ...phone, browserName: "webkit" } },
    { name: "phone-chromium", use: { ...phone, browserName: "chromium" } },
    { name: "tv-webkit", use: { ...tv, browserName: "webkit" } },
    { name: "tv-chromium", use: { ...tv, browserName: "chromium" } },
  ],
});
