import { test, expect } from '@playwright/test';
import { DashboardPage } from '../pages/DashboardPage';
import { CaseListPage } from '../pages/CaseListPage';
import { ActionListPage } from '../pages/ActionListPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Authentication', () => {
  test('should access dashboard in no-auth mode', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);
    const caseListPage = new CaseListPage(page);

    // Navigate to dashboard (in no-auth mode, should work without login)
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Verify dashboard is accessible
    const isVisible = await dashboardPage.isDashboardVisible();
    expect(isVisible).toBeTruthy();

    // Beyond the logo: authenticated content actually renders. The default
    // route redirects to the case list, which must load (not an auth wall).
    expect(await caseListPage.isPageLoaded()).toBeTruthy();
  });

  test('should navigate between pages', async ({ page }) => {
    const dashboardPage = new DashboardPage(page);
    const caseListPage = new CaseListPage(page);
    const actionListPage = new ActionListPage(page);

    // Navigate to dashboard
    await dashboardPage.navigate(TEST_WORKSPACE_ID);

    // Cases: URL changes AND the case list page actually renders.
    await dashboardPage.goToCases();
    expect(page.url()).toContain('/cases');
    expect(await caseListPage.isPageLoaded()).toBeTruthy();

    // Actions: URL changes AND the kanban board actually renders (a column
    // is present), not just that the route matched.
    await dashboardPage.navigate(TEST_WORKSPACE_ID);
    await dashboardPage.goToActions();
    expect(page.url()).toContain('/actions');
    await actionListPage.waitForBoardLoad();
    expect(await actionListPage.columnExists('Backlog')).toBeTruthy();
  });

  // Note: Actual Slack OAuth authentication testing requires
  // external service integration and is out of scope for E2E tests
  // These tests verify the application is accessible in no-auth mode
});
