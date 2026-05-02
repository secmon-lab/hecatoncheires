/// <reference types="node" />
import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for Hecatoncheires E2E tests
 * See https://playwright.dev/docs/test-configuration
 */
export default defineConfig({
  // Test directory
  testDir: './e2e/tests',

  // Run tests in files in parallel
  fullyParallel: true,

  // Fail the build on CI if you accidentally left test.only in the source code
  forbidOnly: !!process.env.CI,

  // Retry on CI only. Keep at 1 — failing tests should be fixed, not papered over with extra retries.
  retries: process.env.CI ? 1 : 0,

  // E2E tests share a single backend with an in-memory repository, so we keep
  // the workers low to avoid cross-test data races. 2 is a safe modest bump
  // from full serialization.
  workers: process.env.CI ? 2 : undefined,

  // Reporter to use
  reporter: [
    ['html'],
    ['list'],
    ...(process.env.CI ? [['github'] as [string]] : []),
  ],

  // Shared settings for all the projects below
  use: {
    // Base URL to use in actions like `await page.goto('/')`
    baseURL: process.env.BASE_URL || 'http://localhost:8080',

    // Collect trace when retrying the failed test
    trace: 'on-first-retry',

    // Screenshot on failure
    screenshot: 'only-on-failure',

    // Video on failure
    video: 'retain-on-failure',

    // Maximum time each action such as `click()` can take. 5s is plenty for
    // a local app; the previous 10s mostly inflated failure latency.
    actionTimeout: 5000,
  },

  // Global timeout for each test
  timeout: 20000,

  // Global timeout for the whole test run
  globalTimeout: process.env.CI ? 600000 : 120000, // 10 minutes on CI, 2 minutes locally

  // Expect timeout
  expect: {
    timeout: 5000,
  },

  // Configure projects for major browsers
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },

    // Additional browsers (uncomment if cross-browser testing is needed)
    // {
    //   name: 'firefox',
    //   use: { ...devices['Desktop Firefox'] },
    // },
    //
    // {
    //   name: 'webkit',
    //   use: { ...devices['Desktop Safari'] },
    // },

    // Mobile viewports (optional, can be enabled later)
    // {
    //   name: 'Mobile Chrome',
    //   use: { ...devices['Pixel 5'] },
    // },
    // {
    //   name: 'Mobile Safari',
    //   use: { ...devices['iPhone 12'] },
    // },
  ],

  // Run your local dev server before starting the tests
  // This is commented out as we'll start the server manually for now
  // webServer: {
  //   command: 'cd .. && go run . serve --repository-backend=memory --config=frontend/e2e/fixtures/config.test.toml --no-auth=U000000000',
  //   url: 'http://localhost:8080',
  //   reuseExistingServer: !process.env.CI,
  //   timeout: 120000,
  // },
});
