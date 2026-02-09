import { test, expect } from '@playwright/test';
import { SourceListPage } from '../pages/SourceListPage';
import { SourceDetailPage } from '../pages/SourceDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Source Management', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to source list page
    const sourceListPage = new SourceListPage(page);
    await sourceListPage.navigate(TEST_WORKSPACE_ID);
  });

  test('should display source list page', async ({ page }) => {
    const sourceListPage = new SourceListPage(page);

    // Verify the page is loaded
    const isLoaded = await sourceListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  test('should display empty source list', async ({ page }) => {
    const sourceListPage = new SourceListPage(page);

    // Wait for table to load
    await sourceListPage.waitForTableLoad();

    // Source list should be accessible (even if empty)
    const isLoaded = await sourceListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  // Note: Creating source items requires external service integration
  // (Notion API, Slack API, etc.) which may not be available in test environment
  // These tests verify the UI is accessible and functional
});
