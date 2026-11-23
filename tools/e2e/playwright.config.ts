import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  timeout: 300_000,
  expect: { timeout: 20_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "line",
  use: {
    channel: "chrome",
    viewport: { width: 360, height: 800 },
    trace: "off",
    video: "off",
    screenshot: "off",
  },
});
