import { test, expect } from '@playwright/test';
import { DashboardPage } from '../pages/DashboardPage';
import { CaseListPage } from '../pages/CaseListPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Workspace', () => {
  test('should display correct workspace context', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);

    // Navigate to test workspace
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Verify we're in the correct workspace by checking URL
    expect(page.url()).toContain(`/ws/${TEST_WORKSPACE_ID}`);

    // Verify dashboard is visible
    const isVisible = await dashboardPage.isDashboardVisible();
    expect(isVisible).toBeTruthy();
  });

  test('should maintain workspace context when navigating', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);
    const caseListPage = new CaseListPage(page);

    // Start from dashboard
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Navigate to cases
    await dashboardPage.goToCases();

    // Verify workspace is maintained in URL
    expect(page.url()).toContain(`/ws/${TEST_WORKSPACE_ID}/cases`);

    // Verify case list loaded correctly
    const isLoaded = await caseListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  test('should handle workspace-specific data', async ({ page }) => {
    const caseListPage = new CaseListPage(page);

    // Navigate to cases in the test workspace
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();

    // Verify we're looking at the test workspace
    expect(page.url()).toContain(`/ws/${TEST_WORKSPACE_ID}`);

    // The page should load without errors
    const hasError = await caseListPage.hasError();
    expect(hasError).toBeFalsy();
  });

  // Note: Multi-workspace switching tests would require
  // multiple workspace configurations, which is beyond the
  // scope of basic E2E testing
});
