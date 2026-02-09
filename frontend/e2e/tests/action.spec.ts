import { test, expect } from '@playwright/test';
import { ActionListPage } from '../pages/ActionListPage';
import { ActionFormPage } from '../pages/ActionFormPage';
import { ActionDetailPage } from '../pages/ActionDetailPage';
import { CaseListPage } from '../pages/CaseListPage';
import { CaseFormPage } from '../pages/CaseFormPage';
import { TEST_WORKSPACE_ID } from '../fixtures/testData';

test.describe('Action Management', () => {
  test.beforeEach(async ({ page }) => {
    // Create a test case first (Actions require a parent case)
    const caseListPage = new CaseListPage(page);
    const caseFormPage = new CaseFormPage(page);

    await caseListPage.navigate(TEST_WORKSPACE_ID);
    await caseListPage.waitForTableLoad();
    await caseListPage.clickNewCaseButton();
    await caseFormPage.createCase({
      title: 'Parent Case for Actions',
      description: 'Case to hold test actions',
      customFields: {
        category: 'task',
      },
    });

    await page.waitForTimeout(1000);

    // Navigate to action list page
    const actionListPage = new ActionListPage(page);
    await actionListPage.navigate(TEST_WORKSPACE_ID);
  });

  test('should display action list page', async ({ page }) => {
    const actionListPage = new ActionListPage(page);

    // Verify the page is loaded
    const isLoaded = await actionListPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();
  });

  test('should create a new action', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);

    // Click "New Action" button
    await actionListPage.clickNewActionButton();

    // Fill in the form
    await actionFormPage.createAction({
      title: 'E2E Test Action',
      description: 'This is a test action created by Playwright',
      caseTitle: 'Parent Case for Actions',
    });

    // Wait for the form to close
    await page.waitForTimeout(1000);

    // Verify the action appears in the list
    const exists = await actionListPage.actionExists('E2E Test Action');
    expect(exists).toBeTruthy();
  });

  test('should view action details', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);
    const actionDetailPage = new ActionDetailPage(page);

    // Create a test action first
    await actionListPage.clickNewActionButton();
    await actionFormPage.createAction({
      title: 'Action for Detail View',
      description: 'Testing action detail view',
      caseTitle: 'Parent Case for Actions',
    });

    await page.waitForTimeout(1000);

    // Click on the action to view details
    await actionListPage.clickActionByTitle('Action for Detail View');

    // Verify detail page is loaded
    const isLoaded = await actionDetailPage.isPageLoaded();
    expect(isLoaded).toBeTruthy();

    // Verify the title matches
    const title = await actionDetailPage.getTitle();
    expect(title).toContain('Action for Detail View');
  });

  test('should list multiple actions', async ({ page }) => {
    const actionListPage = new ActionListPage(page);
    const actionFormPage = new ActionFormPage(page);

    // Create multiple actions
    const actionCount = 3;
    for (let i = 1; i <= actionCount; i++) {
      await actionListPage.clickNewActionButton();
      await actionFormPage.createAction({
        title: `Action ${i}`,
        description: `Description for action ${i}`,
        caseTitle: 'Parent Case for Actions',
      });
      await page.waitForTimeout(500);
    }

    // Reload to get fresh data
    await page.reload();
    await actionListPage.waitForTableLoad();

    // Verify all actions are listed
    for (let i = 1; i <= actionCount; i++) {
      const exists = await actionListPage.actionExists(`Action ${i}`);
      expect(exists).toBeTruthy();
    }
  });
});
