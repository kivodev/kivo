import { defineConfig } from "@playwright/test";

// The TV build with only a remote's keys, against the mock API: the dev
// server runs with the mocks and the TV flag, at the TV's 1920x1080, and the
// tests press arrows, OK and Back and nothing else.
const port = Number(process.env.KIVO_TV_E2E_PORT ?? 5350);

export default defineConfig({
  testDir: "./e2e/tv",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  workers: 1,
  reporter: [["list"]],
  webServer: {
    command: `npm run dev:mock -- --host 127.0.0.1 --port ${port} --strictPort`,
    env: { VITE_KIVO_TV: "1" },
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: false,
  },
  use: {
    baseURL: `http://127.0.0.1:${port}/`,
    channel: "chromium",
    viewport: { width: 1920, height: 1080 },
    deviceScaleFactor: 1,
    trace: "retain-on-failure",
  },
  projects: [{ name: "tv" }],
});
