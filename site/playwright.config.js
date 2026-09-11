import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "**/*.spec.js",
  use: {
    baseURL: "http://127.0.0.1:4173",
    browserName: "chromium",
  },
  webServer: {
    command: "wrangler dev --local --ip 127.0.0.1 --port 4173",
    url: "http://127.0.0.1:4173",
  },
});
