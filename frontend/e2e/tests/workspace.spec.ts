import { test, expect } from '@playwright/test';
import { DashboardPage } from '../pages/DashboardPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

// The second workspace loaded by config.review.test.toml (see e2e.sh).
// Used here to prove per-workspace data isolation.
const REVIEW_WORKSPACE_ID = 'review';

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

  test('keeps case data isolated between workspaces', async ({ page }) => {
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    // Create a uniquely-titled case in the `test` workspace.
    const title = `Isolation Case ${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title,
      description: 'Belongs to the test workspace only',
      customFields: { category: 'task' },
    });

    // It is visible in the `test` workspace it was created in.
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    expect(await caseListPage.caseExists(title)).toBeTruthy();

    // It must NOT leak into the `review` workspace — data is scoped per
    // workspace on the backend, and this is the real guarantee worth testing
    // (the old spec only re-checked the URL).
    await caseListPage.navigate(REVIEW_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.fillSearchFilter(title);
    expect(await caseListPage.caseExists(title)).toBeFalsy();
  });
});
