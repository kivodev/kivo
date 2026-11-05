import { defineConfig } from "@playwright/test";

// The TV build's remote in the player, against the mock API: the dev
// server runs with the mocks and the TV flag, and the global setup makes a
// 44-minute film the tests play in place of the mock's empty media, which
// the dev server serves itself.
const port = Number(process.env.KIVO_REMOTE_E2E_PORT ?? 5199);

export default defineConfig({
  testDir: "./e2e/remote",
  globalSetup: "./e2e/remote/global-setup.ts",
  globalTeardown: "./e2e/remote/global-teardown.ts",
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
    launchOptions: { args: ["--autoplay-policy=no-user-gesture-required"] },
    trace: "retain-on-failure",
  },
  projects: [{ name: "remote" }],
});
