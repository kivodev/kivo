import { defineConfig } from "@playwright/test";

// The built packaged TV app (dist-packaged) opened as the TV opens it, from
// a file: URL, with no server of its own. npm run e2e:packaged-file builds
// it first. Chromium reads other file: URLs from a file: page only with
// --allow-file-access-from-files, which a Tizen widget page may do already.
export default defineConfig({
  testDir: "./e2e/packaged-file",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  workers: 1,
  reporter: [["list"]],
  use: {
    channel: "chromium",
    viewport: { width: 1920, height: 1080 },
    deviceScaleFactor: 1,
    launchOptions: { args: ["--allow-file-access-from-files"] },
    trace: "retain-on-failure",
  },
  projects: [{ name: "packaged-file" }],
});
