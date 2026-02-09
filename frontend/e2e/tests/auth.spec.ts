import { test, expect } from '@playwright/test';
import { DashboardPage } from '../pages/DashboardPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Authentication', () => {
  test('should access dashboard in no-auth mode', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);

    // Navigate to dashboard (in no-auth mode, should work without login)
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Verify dashboard is accessible
    const isVisible = await dashboardPage.isDashboardVisible();
    expect(isVisible).toBeTruthy();
  });

  test('should navigate between pages', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);

    // Navigate to dashboard
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Verify we can navigate to different sections
    await dashboardPage.goToCases();
    expect(page.url()).toContain('/cases');

    // Go back to dashboard
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    await dashboardPage.goToActions();
    expect(page.url()).toContain('/actions');
  });

  // Note: Actual Slack OAuth authentication testing requires
  // external service integration and is out of scope for E2E tests
  // These tests verify the application is accessible in no-auth mode
});
