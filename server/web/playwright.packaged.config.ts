import { defineConfig } from "@playwright/test";

// The packaged TV app against the mock API: the dev server runs
// with the mocks and both the TV and packaged flags, at the TV's 1920x1080.
// The app keeps an absolute server address, and the tests type the dev
// server's own origin, so the probe, the API and the events socket all reach
// the mocks.
const port = Number(process.env.KIVO_PACKAGED_E2E_PORT ?? 5360);

export default defineConfig({
  testDir: "./e2e/packaged",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  workers: 1,
  reporter: [["list"]],
  webServer: {
    command: `npm run dev:mock -- --host 127.0.0.1 --port ${port} --strictPort`,
    env: { VITE_KIVO_TV: "1", VITE_KIVO_PACKAGED: "1" },
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
  projects: [{ name: "packaged" }],
});
