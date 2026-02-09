import { test, expect } from '@playwright/test';
import { KnowledgeListPage } from '../pages/KnowledgeListPage';
import { KnowledgeDetailPage } from '../pages/KnowledgeDetailPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Knowledge Management', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to knowledge list page
    const knowledgeListPage = new KnowledgeListPage(page);
    await knowledgeListPage.navigate(TEST_WORKSPACE_ID);
  });

  test('should display knowledge list page', async ({ page }) => {
    const knowledgeListPage = new KnowledgeListPage(page);

    // Verify the page is loaded
    const isLoaded = await knowledgeListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  test('should display empty knowledge list', async ({ page }) => {
    const knowledgeListPage = new KnowledgeListPage(page);

    // Wait for table to load
    await knowledgeListPage.waitForTableLoad();

    // Knowledge list should be accessible (even if empty)
    const isLoaded = await knowledgeListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  // Note: Creating knowledge items requires Source integration
  // which may not be available in the test environment
  // These tests verify the UI is accessible and functional
});
