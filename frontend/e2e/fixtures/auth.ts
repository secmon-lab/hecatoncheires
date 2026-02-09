import { test as base, Page } from '@playwright/test';
import { TEST_WORKSPACE_ID } from './testData';

/**
 * Fixture for authenticated page
 * In no-auth mode, we just need to navigate to the workspace
 */
export const authFixture = base.extend<{ authenticatedPage: Page }>({
  authenticatedPage: async ({ page }, use) => {
    // In no-auth mode, the server automatically authenticates
    // Just navigate to the workspace to ensure we're in the right context
    await page.goto(`/ws/${TEST_WORKSPACE_ID}`);

    // Wait for navigation to complete
    await page.waitForLoadState('networkidle');

    // Use the page
    await use(page);

    // Cleanup if needed
  },
});
